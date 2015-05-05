// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build"
	"go/token"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/types"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	failingPrefix        = "failschecks"
	withArgsPrefix       = "withargs"
	failingPackageCount  = 7
	withArgsPackageCount = 2
	testPackagePrefix    = "v.io/x/devtools/logcop/testdata"
)

func TestValidPackages(t *testing.T) {
	pkg := path.Join(testPackagePrefix, "passeschecks")
	_, methods := doTest(t, []string{pkg})
	if len(methods) > 0 {
		t.Fatalf("Test package %q failed to pass the log checks", pkg)
	}
}

func TestInvalidPackages(t *testing.T) {
	for i := 1; i <= failingPackageCount; i++ {
		pkg := path.Join(testPackagePrefix, failingPrefix, "test"+strconv.Itoa(i))
		_, methods := doTest(t, []string{pkg})
		if len(methods) == 0 {
			t.Fatalf("Test package %q passes log checks but it should not", pkg)
		}
	}
}

func TestRemove(t *testing.T) {
	stdout := bytes.NewBuffer(nil)
	ctx := tool.NewDefaultContext()
	ctx = ctx.Clone(tool.ContextOpts{Stdout: stdout})
	if _, err := configureDefaultBuildConfig(ctx, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	pkg := path.Join(testPackagePrefix, "passeschecks")

	diffOnlyFlag = true
	if err := runRemover(ctx, []string{pkg}); err != nil {
		t.Fatal(err)
	}
	diffs := []string{}
	scanner := bufio.NewScanner(bytes.NewBufferString(stdout.String()))
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, "] >>") {
			continue
		}
		diffs = append(diffs, text)
	}

	diffFilename := filepath.Join("testdata", "passeschecks.diff")
	want := ""
	if buf, err := ioutil.ReadFile(diffFilename); err != nil {
		t.Fatal(err)
	} else {
		want = strings.TrimRight(string(buf), "\n")
	}
	if got := strings.Join(diffs, "\n"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInject(t *testing.T) {
	testInject(t, "iface", failingPrefix, failingPackageCount)
}

func TestInjectWithArgs(t *testing.T) {
	testInject(t, "iface2", withArgsPrefix, withArgsPackageCount)
}

func testInject(t *testing.T, iface, prefix string, testPackageCount int) {
	ctx := tool.NewDefaultContext()
	if _, err := configureDefaultBuildConfig(ctx, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	ifc := path.Join(testPackagePrefix, iface)

	diffOnlyFlag = true
	for i := 1; i <= testPackageCount; i++ {
		stdout := bytes.NewBuffer(nil)
		ctx = ctx.Clone(tool.ContextOpts{Stdout: stdout})
		testPkg := "test" + strconv.Itoa(i)
		pkg := path.Join(testPackagePrefix, prefix, testPkg)
		if err := runInjector(ctx, []string{ifc}, []string{pkg}, false); err != nil {
			t.Fatal(err)
		}
		diffs := []string{}
		scanner := bufio.NewScanner(bytes.NewBufferString(stdout.String()))
		re := regexp.MustCompile(".*Warning: [[:^space:]]+: (.*)")
		for scanner.Scan() {
			text := scanner.Text()
			if strings.Contains(text, "] >>") {
				continue
			}
			if parts := re.FindStringSubmatch(text); len(parts) == 2 {
				text = parts[1]
			}
			diffs = append(diffs, text)
		}
		diffFilename := filepath.Join("testdata", prefix, testPkg+".diff")
		want := ""
		if buf, err := ioutil.ReadFile(diffFilename); err != nil {
			t.Fatal(err)
		} else {
			want = strings.TrimRight(string(buf), "\n")
		}
		if got := strings.Join(diffs, "\n"); got != want {
			t.Errorf("%s: got %v, want %v", testPkg, got, want)
		}
	}
}

func configureDefaultBuildConfig(ctx *tool.Context, tags []string) (cleanup func(), err error) {
	env, err := util.VanadiumEnvironment(ctx, util.HostPlatform())
	if err != nil {
		return nil, fmt.Errorf("failed to obtain the Vanadium environment: %v", err)
	}
	prevGOPATH := build.Default.GOPATH
	prevBuildTags := build.Default.BuildTags
	cleanup = func() {
		build.Default.GOPATH = prevGOPATH
		build.Default.BuildTags = prevBuildTags
	}
	build.Default.GOPATH = env.Get("GOPATH")
	build.Default.BuildTags = tags
	return cleanup, nil
}

func doTest(t *testing.T, packages []string) (*token.FileSet, map[funcDeclRef]error) {
	ctx := tool.NewDefaultContext()
	if _, err := configureDefaultBuildConfig(ctx, []string{"testpackage"}); err != nil {
		t.Fatal(err)
	}
	interfaceList := []string{path.Join(testPackagePrefix, "iface")}

	ifcs, err := importPkgs(ctx, interfaceList)
	if err != nil {
		t.Fatal(err)
	}

	impls, err := importPkgs(ctx, packages)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(impls), 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}

	ps := newState(ctx)

	ifc := ifcs[0]
	_, ifcpkg, err := ps.parseAndTypeCheckPackage(ifc)
	if err != nil {
		t.Fatal(err)
	}

	interfaces := findPublicInterfaces(ctx, []*types.Package{ifcpkg})
	if len(interfaces) == 0 {
		t.Fatalf("Log injector did not find any interfaces in %s for %s", interfaceList, ifcpkg.Path())
	}

	impl := impls[0]
	asts, tpkg, err := ps.parseAndTypeCheckPackage(impl)
	if err != nil {
		t.Fatal(err)
	}

	methods := findMethodsImplementing(ctx, ps.fset, tpkg, interfaces)
	if len(methods) == 0 {
		t.Fatalf("Log injector could not find any methods implementing the test interfaces in %v", impls)
	}
	methodPositions, err := functionDeclarationsAtPositions(ps.fset, asts, nil, methods)
	if err != nil {
		t.Fatal(err)
	}
	return ps.fset, checkMethods(methodPositions)
}