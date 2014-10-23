// Package util provides utility functions for veyron tools.
//
// TODO(jsimsa): Create a repoutil package that hides different
// version control systems behind a single interface.
package util

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gitutil"
	"tools/lib/runutil"
)

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a list of veyron projects.
type Manifest struct {
	Projects []Project `xml:"projects>project"`
	Tools    []Tool    `xml:"tools>tool"`
}

// Projects maps veyron project names to their detailed description.
type Projects map[string]Project

// Project represents a veyron project.
type Project struct {
	// Name is the URL at which the project is hosted.
	Name string `xml:"name,attr"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the VEYRON_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// VEYRON_ROOT environment variable as a prefix.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr"`
}

// Tools maps veyron tool names, to their detailed description.
type Tools map[string]Tool

// Tool represents a veyron tool.
type Tool struct {
	// Name is the name of the tool binary.
	Name string `xml:"name,attr"`
	// Package is the package path of the tool.
	Package string `xml:"package,attr"`
	// Project identifies the project that contains the tool. If
	// not set, "https://veyron.googlesource.com/tools" is used as
	// the default.
	Project string `xml:"project,attr"`
}

type UnsupportedProtocolErr string

func (e UnsupportedProtocolErr) Error() string {
	return fmt.Sprintf("unsupported protocol %v", e)
}

// Update represents an update of veyron projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

// ListProjects lists the existing local projects to stdout.
func ListProjects(ctx *Context, listBranches bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)
	projects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		project := projects[name]
		fmt.Fprintf(ctx.Stdout(), "%q in %q\n", path.Base(name), project.Path)
		if listBranches {
			if err := ctx.Run().Function(runutil.Chdir(project.Path)); err != nil {
				return err
			}
			branches, current := []string{}, ""
			switch project.Protocol {
			case "git":
				branches, current, err = ctx.Git().GetBranches()
				if err != nil {
					return err
				}
			case "hg":
				branches, current, err = ctx.Hg().GetBranches()
				if err != nil {
					return err
				}
			default:
				return UnsupportedProtocolErr(project.Protocol)
			}
			for _, branch := range branches {
				if branch == current {
					fmt.Fprintf(ctx.Stdout(), "  * %v\n", branch)
				} else {
					fmt.Fprintf(ctx.Stdout(), "  %v\n", branch)
				}
			}
		}
	}
	return nil
}

// LocalProjects scans the local filesystem to identify existing
// projects.
func LocalProjects(ctx *Context) (Projects, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	projects := Projects{}
	if err := findLocalProjects(ctx, root, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// PollProjects returns the set of changelists that exist remotely
// but not locally. Changes are grouped by veyron repositories and
// contain author identification and a description of their content.
func PollProjects(ctx *Context, manifest string, projectSet map[string]struct{}) (Update, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer os.Chdir(cwd)
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	remoteProjects, _, err := ReadLatestManifest(ctx, manifest)
	if err != nil {
		return nil, err
	}
	ops, err := computeOperations(localProjects, remoteProjects, false)
	if err != nil {
		return nil, err
	}
	update := Update{}
	for _, op := range ops {
		if len(projectSet) > 0 {
			if _, ok := projectSet[op.project.Name]; !ok {
				continue
			}
		}
		cls := []CL{}
		if op.ty == updateOperation {
			switch op.project.Protocol {
			case "git":
				if err := ctx.Run().Function(runutil.Chdir(op.destination)); err != nil {
					return nil, err
				}
				if err := ctx.Git().Fetch("origin", "master"); err != nil {
					return nil, err
				}
				commitsText, err := ctx.Git().Log("FETCH_HEAD", "master", "%an%n%ae%n%B")
				if err != nil {
					return nil, err
				}
				for _, commitText := range commitsText {
					if got, want := len(commitText), 3; got < want {
						return nil, fmt.Errorf("Unexpected length of %v: got %v, want at least %v", commitText, got, want)
					}
					cls = append(cls, CL{
						Author:      commitText[0],
						Email:       commitText[1],
						Description: strings.Join(commitText[2:], "\n"),
					})
				}
			default:
				return nil, UnsupportedProtocolErr(op.project.Protocol)
			}
		}
		update[op.project.Name] = cls
	}
	return update, nil
}

// ReadLatestManifest retrieves and parses the latest version of the
// project manifest to identify the latest version of the veyron
// projects and tools.
func ReadLatestManifest(ctx *Context, manifest string) (Projects, Tools, error) {
	m, err := readLatestManifest(ctx, manifest)
	if err != nil {
		return nil, nil, err
	}
	projects := Projects{}
	root, err := VeyronRoot()
	if err != nil {
		return nil, nil, err
	}
	for _, project := range m.Projects {
		// Replace the relative path with an absolute one.
		project.Path = filepath.Join(root, project.Path)
		// Use git as the default protocol.
		if project.Protocol == "" {
			project.Protocol = "git"
		}
		projects[project.Name] = project
	}
	tools := Tools{}
	for _, tool := range m.Tools {
		// Use the "tools" project as the default project.
		if tool.Project == "" {
			tool.Project = "https://veyron.googlesource.com/tools"
		}
		tools[tool.Name] = tool
	}
	return projects, tools, nil
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(ctx *Context, manifest string, gc bool) error {
	remoteProjects, remoteTools, err := ReadLatestManifest(ctx, manifest)
	if err != nil {
		return err
	}
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(ctx, remoteProjects, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory under $VEYRON_ROOT.
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	prefix := "tmp-veyron-tools-build"
	tmpDir, err := ioutil.TempDir(root, prefix)
	if err != nil {
		return fmt.Errorf("TempDir(%v, %v) failed: %v", root, prefix, err)
	}
	defer os.Remove(tmpDir)
	if err := buildTools(ctx, remoteTools, tmpDir); err != nil {
		return err
	}
	// 3. Install the tools into $VEYRON_ROOT/bin.
	return installTools(ctx, tmpDir)
}

// applyToLocalMaster applies an operation expressed as the given
// function to the local master branch of the given project.
func applyToLocalMaster(ctx *Context, project Project, fn func() error) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)
	if err := ctx.Run().Function(runutil.Chdir(project.Path)); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		branch, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		stashed, err := ctx.Git().Stash()
		if err != nil {
			return err
		}
		if stashed {
			defer ctx.Git().StashPop()
		}
		if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
			return err
		}
		defer ctx.Git().CheckoutBranch(branch, !gitutil.Force)
	case "hg":
		branch, err := ctx.Hg().CurrentBranchName()
		if err != nil {
			return err
		}
		if err := ctx.Hg().CheckoutBranch("default"); err != nil {
			return err
		}
		defer ctx.Hg().CheckoutBranch(branch)
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
	return fn()
}

// buildTool builds the given tool, placing the resulting binary into
// the given directory.
func buildTool(ctx *Context, outputDir string, tool Tool, project Project) error {
	buildFn := func() error {
		env, err := VeyronEnvironment(HostPlatform())
		if err != nil {
			return err
		}
		output := filepath.Join(outputDir, tool.Name)
		var count int
		switch project.Protocol {
		case "git":
			gitCount, err := ctx.Git().CountCommits("HEAD", "")
			if err != nil {
				return err
			}
			count = gitCount
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
		ldflags := fmt.Sprintf("-X tools/lib/version.Version %d", count)
		args := []string{"build", "-ldflags", ldflags, "-o", output, tool.Package}
		var stderr bytes.Buffer
		if err := ctx.Run().Command(ioutil.Discard, &stderr, env.Map(), "go", args...); err != nil {
			return fmt.Errorf("%v tool build failed\n%v", tool.Name, stderr.String())
		}
		return nil
	}
	return applyToLocalMaster(ctx, project, buildFn)
}

// buildTools builds and installs all veyron tools using the version
// available in the local master branch of the tools
// repository. Notably, this function does not perform any version
// control operation on the master branch.
func buildTools(ctx *Context, remoteTools Tools, outputDir string) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	failed := false
	names := []string{}
	for name, _ := range remoteTools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool := remoteTools[name]
		updateFn := func() error {
			project, ok := localProjects[tool.Project]
			if !ok {
				return fmt.Errorf("unknown project %v", tool.Project)
			}
			return buildTool(ctx, outputDir, tool, project)
		}
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		if err := ctx.Run().FunctionWithVerbosity(true, updateFn, "build tool %q", tool.Name); err != nil {
			// TODO(jsimsa): Switch this to Run().Output()?
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// findLocalProjects implements LocalProjects.
func findLocalProjects(ctx *Context, path string, projects Projects) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)
	if err := ctx.Run().Function(runutil.Chdir(path)); err != nil {
		return err
	}
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		name, err := ctx.Git().RepoName()
		if err != nil {
			return err
		}
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "git",
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", gitDir, err)
	}
	hgDir := filepath.Join(path, ".hg")
	if _, err := os.Stat(hgDir); err == nil {
		name, err := ctx.Hg().RepoName()
		if err != nil {
			return err
		}
		if project, ok := projects[name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", project.Path, path, name)
		}
		projects[name] = Project{
			Name:     name,
			Path:     path,
			Protocol: "hg",
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", hgDir, err)
	}
	ignoreSet, ignorePath := make(map[string]struct{}, 0), filepath.Join(path, ".veyronignore")
	file, err := os.Open(ignorePath)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			ignoreSet[scanner.Text()] = struct{}{}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scan() failed: %v", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Open(%v) failed: %v", ignorePath, err)
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fi := range fis {
		if _, ignore := ignoreSet[fi.Name()]; fi.IsDir() && !strings.HasPrefix(fi.Name(), ".") && !ignore {
			if err := findLocalProjects(ctx, filepath.Join(path, fi.Name()), projects); err != nil {
				return err
			}
		}
	}
	return nil
}

// installTools installs the tools from the given directory into
// $VEYRON_ROOT/bin.
func installTools(ctx *Context, dir string) error {
	root, err := VeyronRoot()
	if err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", dir, err)
	}
	failed := false
	for _, fi := range fis {
		installFn := func() error {
			src := filepath.Join(dir, fi.Name())
			dst := filepath.Join(root, "bin", fi.Name())
			if err := ctx.Run().Function(runutil.Rename(src, dst)); err != nil {
				return err
			}
			return nil
		}
		if err := ctx.Run().FunctionWithVerbosity(true, installFn, "install tool %q", fi.Name()); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// reportNonMaster checks if the given project is on master branch and
// if not, reports this fact along with information on how to update it.
func reportNonMaster(ctx *Context, project Project) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(project.Path); err != nil {
		return err
	}
	current, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	if current != "master" {
		line1 := fmt.Sprintf(`NOTE: "veyron update" only updates the "master" branch and the current branch is %q`, current)
		line2 := fmt.Sprintf(`to update the %q branch once the master branch is updated, run "git merge master"`, current)
		ctx.Run().OutputWithVerbosity(true, []string{line1, line2})
	}
	return nil
}

// readLatestManifest reads the given manifest file into an in-memory
// data structure.
func readLatestManifest(ctx *Context, manifest string) (*Manifest, error) {
	root, err := VeyronRoot()
	if err != nil {
		return nil, err
	}
	// Update the manifest.
	project := Project{
		Path:     filepath.Join(root, ".manifest"),
		Protocol: "git",
	}
	if err := pullProject(ctx, project); err != nil {
		return nil, err
	}
	// Parse the manifest.
	path := filepath.Join(root, ".manifest", manifest+".xml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%v) failed: %v", path, err)
	}
	m := &Manifest{}
	if err := xml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	return m, err
}

// pullProject advances the local master branch of the given
// project, which is expected to exist locally at project.Path.
func pullProject(ctx *Context, project Project) error {
	pullFn := func() error {
		switch project.Protocol {
		case "git":
			return ctx.Git().Pull("origin", "master")
		case "hg":
			return ctx.Hg().Pull()
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
	}
	return applyToLocalMaster(ctx, project, pullFn)
}

// updateProjects updates all veyron projects.
func updateProjects(ctx *Context, remoteProjects Projects, gc bool) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	ops, err := computeOperations(localProjects, remoteProjects, gc)
	if err != nil {
		return err
	}
	if err := testOperations(ops); err != nil {
		return err
	}
	failed := false
	for _, op := range ops {
		updateFn := func() error { return runOperation(ctx, op) }
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		if err := ctx.Run().FunctionWithVerbosity(true, updateFn, "%v", op); err != nil {
			// TODO(jsimsa): Switch this to Run.Output()?
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// operation represents a project operation.
type operation struct {
	// project holds information about the project such as its
	// name, local path, and the protocol it uses for version
	// control.
	project Project
	// Destination is the new project path.
	destination string
	// Source is the current project path.
	source string
	// ty is the type of the operation.
	ty operationType
}

func (op operation) String() string {
	name := filepath.Base(op.project.Name)
	switch op.ty {
	case createOperation:
		return fmt.Sprintf("create project %q in %q", name, op.destination)
	case deleteOperation:
		return fmt.Sprintf("delete project %q from %q", name, op.source)
	case moveOperation:
		return fmt.Sprintf("move project %q from %q to %q and update it", name, op.source, op.destination)
	case updateOperation:
		return fmt.Sprintf("update project %q in %q", name, op.source)
	default:
		return fmt.Sprintf("unknown operation type: %v", op.ty)
	}
}

// operations is a sortable collection of operations
type operations []operation

// Len returns the length of the collection.
func (ops operations) Len() int {
	return len(ops)
}

// Less defines the order of operations. Operations are ordered first
// by their type and then by their project name.
func (ops operations) Less(i, j int) bool {
	if ops[i].ty != ops[j].ty {
		return ops[i].ty < ops[j].ty
	}
	return ops[i].project.Name < ops[j].project.Name
}

// Swap swaps two elements of the collection.
func (ops operations) Swap(i, j int) {
	ops[i], ops[j] = ops[j], ops[i]
}

type operationType int

const (
	// The order in which operation types are defined determines
	// the order in which operations are performed. For
	// correctness and also to minimize the chance of a conflict,
	// the delete operations should happen before move operations,
	// which should happen before create operations.
	deleteOperation operationType = iota
	moveOperation
	createOperation
	updateOperation
)

// computeOperations inputs a set of projects to update and the set of
// current and new projects (as defined by contents of the local file
// system and manifest file respectively) and outputs a collection of
// operations that describe the actions needed to update the target
// projects.
func computeOperations(localProjects, remoteProjects Projects, gc bool) (operations, error) {
	result := operations{}
	allProjects := map[string]struct{}{}
	for name, _ := range localProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range remoteProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range allProjects {
		if localProject, ok := localProjects[name]; ok {
			if remoteProject, ok := remoteProjects[name]; ok {
				if localProject.Path == remoteProject.Path {
					result = append(result, operation{localProject, remoteProject.Path, localProject.Path, updateOperation})
				} else {
					result = append(result, operation{localProject, remoteProject.Path, localProject.Path, moveOperation})
				}
			} else if gc {
				result = append(result, operation{localProject, "", localProject.Path, deleteOperation})
			}
		} else if remoteProject, ok := remoteProjects[name]; ok {
			result = append(result, operation{remoteProject, remoteProject.Path, "", createOperation})
		} else {
			return nil, fmt.Errorf("project %v does not exist", name)
		}
	}
	sort.Sort(result)
	return result, nil
}

// preCommitHook is a git hook installed to all new projects. It
// prevents accidental commits to the local master branch.

const preCommitHook = `
#!/bin/bash

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "${BRANCH}" == "master" ]
then
  echo "========================================================================="
  echo "Veyron code cannot be committed to master using the 'git commit' command."
  echo "Please make a feature branch and commit your code there."
  echo "========================================================================="
  exit 1
fi

exit 0
`

// prePushHook is a git hook installed to all new projects. It
// prevents accidental pushes to the remote master branch.
const prePushHook = `
#!/bin/bash

readonly REMOTE=$1

# Get the current branch name.
readonly BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "${REMOTE}" == "origin" ] && [ "${BRANCH}" == "master" ]
then
  echo "======================================================================="
  echo "Veyron code cannot be pushed to master using the 'git push' command."
  echo "Use the 'git veyron review' command to follow the code review workflow."
  echo "======================================================================="
  exit 1
fi

exit 0
`

// runOperation executes the given operation.
//
// TODO(jsimsa): Decide what to do in case we would want to update the
// commit hooks for existing repositories. Overwriting the existing
// hooks is not a good idea as developers might have customized the
// hooks.
func runOperation(ctx *Context, op operation) error {
	switch op.ty {
	case createOperation:
		path, perm := filepath.Dir(op.destination), os.FileMode(0755)
		if err := ctx.Run().Function(runutil.MkdirAll(path, perm)); err != nil {
			return err
		}
		switch op.project.Protocol {
		case "git":
			if err := ctx.Git().Clone(op.project.Name, op.destination); err != nil {
				return err
			}
			if strings.HasPrefix(op.project.Name, "https://veyron.googlesource.com/") {
				// Setup the repository for Gerrit code reviews.
				file := filepath.Join(op.destination, ".git", "hooks", "commit-msg")
				url := "https://gerrit-review.googlesource.com/tools/hooks/commit-msg"
				args := []string{"-Lo", file, url}
				var stderr bytes.Buffer
				if err := ctx.Run().Command(ioutil.Discard, &stderr, nil, "curl", args...); err != nil {
					return fmt.Errorf("failed to download commit message hook: %v\n%v", err, stderr.String())
				}
				if err := os.Chmod(file, perm); err != nil {
					return fmt.Errorf("Chmod(%v, %v) failed: %v", file, perm, err)
				}
				file = filepath.Join(op.destination, ".git", "hooks", "pre-commit")
				if err := ioutil.WriteFile(file, []byte(preCommitHook), perm); err != nil {
					return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
				}
				file = filepath.Join(op.destination, ".git", "hooks", "pre-push")
				if err := ioutil.WriteFile(file, []byte(prePushHook), perm); err != nil {
					return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
				}
			}
		case "hg":
			if err := ctx.Hg().Clone(op.project.Name, op.destination); err != nil {
				return err
			}
		default:
			return UnsupportedProtocolErr(op.project.Protocol)
		}
	case deleteOperation:
		if err := ctx.Run().Function(runutil.RemoveAll(op.source)); err != nil {
			return err
		}
	case moveOperation:
		if err := reportNonMaster(ctx, op.project); err != nil {
			return err
		}
		if err := pullProject(ctx, op.project); err != nil {
			return err
		}
		path, perm := filepath.Dir(op.destination), os.FileMode(0755)
		if err := ctx.Run().Function(runutil.MkdirAll(path, perm)); err != nil {
			return err
		}
		if err := ctx.Run().Function(runutil.Rename(op.source, op.destination)); err != nil {
			return err
		}
	case updateOperation:
		if err := reportNonMaster(ctx, op.project); err != nil {
			return err
		}
		if err := pullProject(ctx, op.project); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%v", op)
	}
	return nil
}

// testOperations checks if the target set of operations can be
// carried out given the current state of the local file system.
func testOperations(ops operations) error {
	for _, op := range ops {
		switch op.ty {
		case createOperation:
			// Check the local file system.
			if _, err := os.Stat(op.destination); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
				}
			} else {
				return fmt.Errorf("cannot create %q as it already exists", op.destination)
			}
		case deleteOperation:
			if _, err := os.Stat(op.source); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("cannot delete %q as it does not exist", op.source)
				}
				return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
			}
		case moveOperation:
			if _, err := os.Stat(op.source); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
				}
				return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
			}
			if _, err := os.Stat(op.destination); err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
				}
			} else {
				return fmt.Errorf("cannot move %q to %q as the destination already exists", op.source, op.destination)
			}
		case updateOperation:
			continue
		default:
			return fmt.Errorf("%v", op)
		}
	}
	return nil
}