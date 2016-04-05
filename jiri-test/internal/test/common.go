// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"v.io/jiri"
	"v.io/jiri/runutil"
)

var (
	// The name of the profile database file to use. It is intended
	// to be set via a command line flag.
	ProfilesDBFilename = jiri.ProfilesDBDir

	// cleanGo is used to control whether the initTest function removes
	// all stale Go object files and binaries. It is used to prevent the
	// test of this package from interfering with other concurrently
	// running tests that might be sharing the same object files.
	cleanGo = true

	// Regexp to match javascript result files.
	reJSResult = regexp.MustCompile(`.*_(integration|spec)\.out$`)

	// Regexp to match common test result files.
	reTestResult = regexp.MustCompile(`^((tests_.*\.xml)|(status_.*\.json))$`)
)

// internalTestError represents an internal test error.
type internalTestError struct {
	err    error
	name   string
	caller string
}

func (e internalTestError) Error() string {
	return fmt.Sprintf("%s: %s\n%s\n", e.name, e.caller, e.err.Error())
}

func newInternalError(err error, name string) internalTestError {
	_, file, line, _ := runtime.Caller(1)
	caller := fmt.Sprintf("%s:%d", filepath.Base(file), line)
	return internalTestError{err: err, name: name, caller: caller}
}

var testTmpDir = ""

type initTestOpt interface {
	initTestOpt()
}

type rootDirOpt string

func (rootDirOpt) initTestOpt() {}

// binDirPath returns the path to the directory for storing temporary
// binaries.
func binDirPath() string {
	if len(testTmpDir) == 0 {
		panic("binDirPath() shouldn't be called before initTest()")
	}
	return filepath.Join(testTmpDir, "bin")
}

// regTestBinDirPath returns the path to the directory for storing
// regression test binaries.
func regTestBinDirPath() string {
	if len(testTmpDir) == 0 {
		panic("regTestBinDirPath() shouldn't be called before initTest()")
	}
	return filepath.Join(testTmpDir, "regtest")
}

// initTest carries out the initial actions for the given test.
func initTest(jirix *jiri.X, testName string, profileNames []string, opts ...initTestOpt) (func() error, error) {
	return initTestImpl(jirix, false, true, true, testName, profileNames, "", opts...)
}

// initTestForTarget carries out the initial actions for the given test using
// a specific profile Target..
func initTestForTarget(jirix *jiri.X, testName string, profileNames []string, target string, opts ...initTestOpt) (func() error, error) {
	return initTestImpl(jirix, false, true, true, testName, profileNames, target, opts...)
}

func initTestImpl(jirix *jiri.X, needCleanup, updateProfiles, printProfiles bool, testName string, profileNames []string, target string, opts ...initTestOpt) (func() error, error) {
	// Output the hostname.
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Hostname() failed: %v", err)
	}
	fmt.Fprintf(jirix.Stdout(), "hostname = %q\n", hostname)

	// Create a working test directory under $HOME/tmp and set the
	// TMPDIR environment variable to it.
	rootDir := filepath.Join(os.Getenv("HOME"), "tmp", testName)
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case rootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	s := jirix.NewSeq()
	workDir, err := s.MkdirAll(rootDir, os.FileMode(0755)).
		TempDir(rootDir, "")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv("TMPDIR", workDir); err != nil {
		return nil, err
	}
	testTmpDir = workDir
	fmt.Fprintf(jirix.Stdout(), "workdir = %q\n", workDir)
	fmt.Fprintf(jirix.Stdout(), "bin dir = %q\n", binDirPath())

	// Create a directory for storing built binaries.
	if err := s.MkdirAll(binDirPath(), os.FileMode(0755)).
		MkdirAll(regTestBinDirPath(), os.FileMode(0755)).Done(); err != nil {
		return nil, err
	}

	if updateProfiles {
		insertTarget := func(profile string) []string {
			if len(target) > 0 {
				return []string{"--target=" + target, profile}
			}
			return []string{profile}
		}
		if needCleanup {
			if err := cleanupProfiles(jirix); err != nil {
				return nil, newInternalError(err, "Init")
			}
		}
		// Install profiles.
		args := []string{"-v", "profile-v23", "install"}
		for _, profile := range profileNames {
			clargs := append(args, insertTarget(profile)...)
			fmt.Fprintf(jirix.Stdout(), "Running: jiri %s\n", strings.Join(clargs, " "))
			if err := s.Last("jiri", clargs...); err != nil {
				return nil, fmt.Errorf("jiri %v: %v", strings.Join(clargs, " "), err)
			}
			fmt.Fprintf(jirix.Stdout(), "jiri %v: success\n", strings.Join(clargs, " "))
		}

		// Update profiles.
		args = []string{"profile", "update"}

		if err := s.Capture(os.Stdout, os.Stderr).Last("jiri", args...); err != nil {
			return nil, fmt.Errorf("jiri %v: %v", strings.Join(args, " "), err)
		}
		fmt.Fprintf(jirix.Stdout(), "jiri %v: success\n", strings.Join(args, " "))
	}

	if printProfiles {
		displayProfiles(jirix, "initTest:")
	}

	// Descend into the working directory (unless doing a "dry
	// run" in which case the working directory does not exist).
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := s.Chdir(workDir).Done(); err != nil {
		return nil, fmt.Errorf("Chdir(%s): %v", workDir, err)
	}

	// Remove all stale Go object files and binaries.
	if cleanGo {
		if err := s.Last("jiri", "goext", "distclean"); err != nil {
			return nil, fmt.Errorf("jiri goext distclean: %v", err)
		}
	}

	// Cleanup the test results possibly left behind by the
	// previous test.
	testResultFiles, err := findTestResultFiles(jirix, testName)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		if err := s.RemoveAll(file).Done(); err != nil {
			return nil, fmt.Errorf("RemoveAll(%s): %v", file, err)
		}
	}

	return func() error {
		if err := jirix.NewSeq().Chdir(cwd).Done(); err != nil {
			return fmt.Errorf("Chdir(%s): %v", cwd, err)
		}
		return nil
	}, nil
}

// findTestResultFiles returns a slice of paths to test result related files.
func findTestResultFiles(jirix *jiri.X, testName string) ([]string, error) {
	result := []string{}
	// Collect javascript test results.
	jsDir := filepath.Join(jirix.Root, "release", "javascript", "core", "test_out")
	s := jirix.NewSeq()
	if _, err := s.Stat(jsDir); err == nil {
		fileInfoList, err := s.ReadDir(jsDir)
		if err != nil {
			return nil, err
		}
		for _, fileInfo := range fileInfoList {
			name := fileInfo.Name()
			if reJSResult.MatchString(name) {
				result = append(result, filepath.Join(jsDir, name))
			}
		}
	} else {
		if !runutil.IsNotExist(err) {
			return nil, err
		}
	}

	// Collect xUnit xml files and test status json files.
	workspaceDir := os.Getenv("WORKSPACE")
	if workspaceDir == "" {
		workspaceDir = filepath.Join(os.Getenv("HOME"), "tmp", testName)
	}
	fileInfoList, err := s.ReadDir(workspaceDir)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfoList {
		fileName := fileInfo.Name()
		if reTestResult.MatchString(fileName) {
			result = append(result, filepath.Join(workspaceDir, fileName))
		}
	}
	return result, nil
}
