// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package golib defines utilities for using the Go toolchain to build
// Vanadium binaries.
package golib

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"sort"
	"strings"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/buildinfo"
	"v.io/x/lib/lookpath"
	"v.io/x/lib/metadata"
	"v.io/x/lib/set"
)

// ExtraLDFlagsFlagDescription describes the --extra-ldflags flag, to be added
// to any tool that allows adding extra ldflags to those automatically
// generated.
const ExtraLDFlagsFlagDescription = `This tool sets some ldflags automatically, e.g. to set binary metadata.  The extra-ldflags are appended to the end of those automatically generated ldflags.  Note that if your go command line specifies -ldflags explicitly, it will override both the automatically generated ldflags as well as the extra-ldflags.`

// PrepareGo runs recommended checks on the environment and related commands
// before execution of the Go toolchain. The Go toolchain should use the
// returned args.
//
// For example, it ensures that all Go files generated by the VDL compiler are
// up-to-date. It also generates flags so that build information can be embedded
// in resulting binaries.
func PrepareGo(jirix *jiri.X, env map[string]string, args []string, extraLDFlags, installSuffix string) ([]string, error) {
	switch args[0] {
	case "build", "install":
		// Provide default ldflags to populate build info metadata in the
		// binary. Any manual specification of ldflags already in the args
		// will override this.
		var err error
		if args, err = setBuildInfoFlags(jirix, args, env, extraLDFlags, installSuffix); err != nil {
			return nil, err
		}
		fallthrough
	case "generate", "run", "test":
		// Check that all non-master branches have been merged with the
		// master branch to make sure the vdl tool is not run against
		// out-of-date code base.
		if err := reportOutdatedBranches(jirix); err != nil {
			return nil, err
		}

		// Generate vdl files, if necessary.
		if err := generateVDL(jirix, env, args[0], args[1:]); err != nil {
			return nil, err
		}
	}
	return args, nil
}

// getPlatform identifies the target platform by querying the go tool
// for the values of the GOARCH and GOOS environment variables.
func getPlatform(jirix *jiri.X, env map[string]string) (string, error) {
	goBin, err := lookpath.Look(env, "go")
	if err != nil {
		return "", err
	}
	s := jirix.NewSeq()
	var out bytes.Buffer
	if err = s.Env(env).Capture(&out, nil).Last(goBin, "env", "GOARCH"); err != nil {
		return "", err
	}
	arch := strings.TrimSpace(out.String())
	out.Reset()
	if err = s.Env(env).Capture(&out, nil).Last(goBin, "env", "GOOS"); err != nil {
		return "", err
	}
	os := strings.TrimSpace(out.String())
	return fmt.Sprintf("%s-%s", arch, os), nil
}

// setBuildInfoFlags augments the list of arguments with flags for the
// go compiler that encoded the build information expected by the
// v.io/x/lib/metadata package.
func setBuildInfoFlags(jirix *jiri.X, args []string, env map[string]string, extraLDFlags, installSuffix string) ([]string, error) {
	info := buildinfo.T{Time: time.Now()}
	// Compute the "platform" value.
	platform, err := getPlatform(jirix, env)
	if err != nil {
		return nil, err
	}
	info.Platform = platform
	// Compute the "manifest" value.
	latestManifest := jirix.UpdateHistoryLatestLink()
	manifest, err := project.ManifestFromFile(jirix, latestManifest)
	if err != nil {
		if !runutil.IsNotExist(err) {
			return nil, err
		}
		fmt.Fprintf(jirix.Stderr(), `WARNING: Could not find %s.
The contents of this file are stored as metadata in binaries the jiri
tool builds. To fix this problem, please run "jiri update".
`, latestManifest)
		manifest = &project.Manifest{}
	}

	info.Manifest = *manifest
	// Compute the "pristine" value.
	states, err := project.GetProjectStates(jirix, true)
	if err != nil {
		return nil, err
	}
	info.Pristine = true
	for _, state := range states {
		if state.CurrentBranch != "master" || state.HasUncommitted || state.HasUntracked {
			info.Pristine = false
			break
		}
	}
	// Compute the "user" value.
	if currUser, err := user.Current(); err == nil {
		info.User = currUser.Name
	}
	// Encode buildinfo as metadata and extract the appropriate ldflags.
	md, err := info.ToMetaData()
	if err != nil {
		return nil, err
	}
	ldflags := "-ldflags=" + metadata.LDFlag(md)
	if extraLDFlags != "" {
		ldflags += " " + extraLDFlags
	}
	args = append([]string{args[0], ldflags}, args[1:]...)
	if installSuffix != "" {
		args = append([]string{args[0], "-installsuffix=" + installSuffix}, args[1:]...)
	}
	return args, nil
}

// generateVDL generates VDL for the transitive Go package dependencies.
//
// Note that the vdl tool takes VDL packages as input, but we're supplying Go
// packages.  We're assuming the package paths for the VDL packages we want to
// generate have the same path names as the Go package paths.  Some of the Go
// package paths may not correspond to a valid VDL package, so we silently
// ignore these paths.
//
// It's fine if the VDL packages have dependencies not reflected in the Go
// packages; the vdl tool will compute the transitive closure of VDL package
// dependencies, as usual.
//
// TODO(toddw): Change the vdl tool to return vdl packages given the full Go
// dependencies, after vdl config files are implemented.
func generateVDL(jirix *jiri.X, env map[string]string, cmd string, args []string) error {
	// Compute which VDL-based Go packages might need to be regenerated.
	goPkgs, goFiles, goTags := processGoCmdAndArgs(cmd, args)
	goDeps, err := computeGoDeps(jirix, env, append(goPkgs, goFiles...), goTags, cmd == "test")
	if err != nil {
		return err
	}

	// Regenerate the VDL-based Go packages.
	// -ignore_unknown:  Silently ignore unknown package paths.
	vdlArgs := []string{"-ignore_unknown", "generate", "-lang=go"}
	vdlArgs = append(vdlArgs, goDeps...)
	vdlBin, err := lookpath.Look(env, "vdl")
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := jirix.NewSeq().Env(env).Capture(&out, &out).Last(vdlBin, vdlArgs...); err != nil {
		return fmt.Errorf("failed to generate vdl: %v\n%s", err, out.String())
	}
	return nil
}

// reportOutdatedProjects checks if the currently checked out branches
// are up-to-date with respect to the local master branch. For each
// branch that is not, a notification is printed.
func reportOutdatedBranches(jirix *jiri.X) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)
	projects, err := project.LocalProjects(jirix, false)
	if err != nil {
		return err
	}
	s := jirix.NewSeq()
	for _, project := range projects {
		if err := s.Chdir(project.Path).Done(); err != nil {
			return err
		}
		switch project.Protocol {
		case "git":
			branches, _, err := gitutil.New(jirix.NewSeq()).GetBranches("--merged")
			if err != nil {
				return err
			}
			found := false
			for _, branch := range branches {
				if branch == "master" {
					found = true
					break
				}
			}
			merging, err := gitutil.New(jirix.NewSeq()).MergeInProgress()
			if err != nil {
				return err
			}
			if !found && !merging {
				fmt.Fprintf(jirix.Stderr(), "NOTE: project=%q path=%q\n", project.Name, project.Path)
				fmt.Fprintf(jirix.Stderr(), "This project is on a non-master branch that is out of date.\n")
				fmt.Fprintf(jirix.Stderr(), "Please update this branch using %q.\n", "git merge master")
				fmt.Fprintf(jirix.Stderr(), "Until then the %q tool might not function properly.\n", "jiri")
			}
		}
	}
	return nil
}

// processGoCmdAndArgs is given the cmd and args for the go tool, filters out
// flags, and returns the PACKAGES or GOFILES that were specified in args, as
// well as "foo" if -tags=foo was specified in the args.  Note that all commands
// that accept PACKAGES also accept GOFILES.
//
//   go build    [build flags]              [-o out]      [PACKAGES]
//   go generate                            [-run regexp] [PACKAGES]
//   go install  [build flags]                            [PACKAGES]
//   go run      [build flags]              [-exec prog]  [GOFILES]  [run args]
//   go test     [build flags] [test flags] [-exec prog]  [PACKAGES] [testbin flags]
//
// Sadly there's no way to do this syntactically.  It's easy for single token
// -flag and -flag=x, but non-boolean flags may be two tokens "-flag x".
//
// We keep track of all non-boolean flags F, and skip every token that starts
// with - or --, and also skip the next token if the flag is in F and isn't of
// the form -flag=x.  If we forget to update F, we'll still handle the -flag and
// -flag=x cases correctly, but we'll get "-flag x" wrong.
func processGoCmdAndArgs(cmd string, args []string) ([]string, []string, string) {
	var goTags string
	var nonBool map[string]bool
	switch cmd {
	case "build":
		nonBool = nonBoolGoBuild
	case "generate":
		nonBool = nonBoolGoGenerate
	case "install":
		nonBool = nonBoolGoInstall
	case "run":
		nonBool = nonBoolGoRun
	case "test":
		nonBool = nonBoolGoTest
	}

	// Move start to the start of PACKAGES or GOFILES, by skipping flags.
	start := 0
	for start < len(args) {
		// Handle special-case terminator --
		if args[start] == "--" {
			start++
			break
		}
		match := goFlagRE.FindStringSubmatch(args[start])
		if match == nil {
			break
		}
		// Skip this flag, and maybe skip the next token for the "-flag x" case.
		//   match[1] is the flag name
		//   match[2] is the optional "=" for the -flag=x case
		start++
		if nonBool[match[1]] && match[2] == "" {
			start++
		}
		// Grab the value of -tags, if it is specified.
		if match[1] == "tags" {
			if match[2] == "=" {
				goTags = match[3]
			} else {
				goTags = args[start-1]
			}
		}
	}

	// Move end to the end of PACKAGES or GOFILES.
	var end int
	switch cmd {
	case "test":
		// Any arg starting with - is a testbin flag.
		// https://golang.org/cmd/go/#hdr-Test_packages
		for end = start; end < len(args); end++ {
			if strings.HasPrefix(args[end], "-") {
				break
			}
		}
	case "run":
		// Go run takes gofiles, which are defined as a file ending in ".go".
		// https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
		for end = start; end < len(args); end++ {
			if !strings.HasSuffix(args[end], ".go") {
				break
			}
		}
	default:
		end = len(args)
	}

	// Decide whether these are packages or files.
	switch {
	case start == end:
		return nil, nil, goTags
	case (start < len(args) && strings.HasSuffix(args[start], ".go")):
		return nil, args[start:end], goTags
	default:
		return args[start:end], nil, goTags
	}
}

var (
	goFlagRE     = regexp.MustCompile(`^--?([^=]+)(=?)(.*)`)
	nonBoolBuild = []string{
		"p", "asmflags", "buildmode", "ccflags", "compiler", "gccgoflags", "gcflags", "installsuffix", "ldflags", "pkgdir", "tags", "toolexec",
	}
	nonBoolTest = []string{
		"bench", "benchtime", "blockprofile", "blockprofilerate", "count", "covermode", "coverpkg", "coverprofile", "cpu", "cpuprofile", "memprofile", "memprofilerate", "outputdir", "parallel", "run", "timeout", "trace",
	}
	nonBoolGoBuild    = set.StringBool.FromSlice(append(nonBoolBuild, "o"))
	nonBoolGoGenerate = set.StringBool.FromSlice([]string{"run"})
	nonBoolGoInstall  = set.StringBool.FromSlice(nonBoolBuild)
	nonBoolGoRun      = set.StringBool.FromSlice(append(nonBoolBuild, "exec"))
	nonBoolGoTest     = set.StringBool.FromSlice(append(append(nonBoolBuild, nonBoolTest...), "exec", "o"))
)

// computeGoDeps computes the transitive Go package dependencies for the given
// set of pkgs.  The strategy is to run "go list <pkgs>" with a special format
// string that dumps the specified pkgs and all deps as space / newline
// separated tokens.  The pkgs may be in any format recognized by "go list"; dir
// paths, import paths, or go files.
func computeGoDeps(jirix *jiri.X, env map[string]string, pkgs []string, tags string, test bool) ([]string, error) {
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	goBin, err := lookpath.Look(env, "go")
	if err != nil {
		return nil, err
	}
	if test {
		// In order to compute the test dependencies, we need to first grab the
		// direct test imports, and use the resulting set of packages to capture the
		// transitive dependencies.  We can't do this with a single run of "go
		// list", since unlike Dep, TestImports and XTestImports don't include
		// transitive dependencies.
		testDeps, err := runGoList(jirix, goBin, env, pkgs, tags, `{{join .TestImports " "}} {{join .XTestImports " "}}`)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, testDeps...)
	}
	return runGoList(jirix, goBin, env, pkgs, tags, `{{.ImportPath}} {{join .Deps " "}}`)
}

func runGoList(jirix *jiri.X, goBin string, env map[string]string, pkgs []string, tags, format string) ([]string, error) {
	goListArgs := []string{`list`, `-f`, format}
	if tags != "" {
		goListArgs = append(goListArgs, "-tags="+tags)
	}
	goListArgs = append(goListArgs, pkgs...)
	var stdout, stderr bytes.Buffer

	// TODO(jsimsa): Avoid buffering all of the output in memory
	// either by extending the runutil API to support piping of
	// output, or by writing the output to a temporary file
	// instead of an in-memory buffer.
	// TODO(cnicolaou): the sequence code in runutil streams using a pipe
	// internally so that could probably be taken advantage of here by having
	// stdout be a pipe that the scanner reads below.
	if err := jirix.NewSeq().Env(env).Capture(&stdout, &stderr).Last(goBin, goListArgs...); err != nil {
		return nil, fmt.Errorf("failed to compute go deps: %v\n%s\n%v", err, stderr.String(), pkgs)
	}
	scanner := bufio.NewScanner(&stdout)
	scanner.Split(bufio.ScanWords)
	depsMap := make(map[string]bool)
	for scanner.Scan() {
		// Ignore bad packages:
		//   command-line-arguments is the dummy import path for "go run".
		if dep := scanner.Text(); dep != "command-line-arguments" {
			depsMap[dep] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	deps := set.StringBool.ToSlice(depsMap)
	sort.Strings(deps)
	return deps, nil
}
