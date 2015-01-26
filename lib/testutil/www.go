package testutil

import (
	"path/filepath"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

const (
	defaultWWWTestTimeout = 5 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(ctx *util.Context, testName, makeTarget string, timeout time.Duration) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, testResult, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, err
	} else if testResult != nil {
		return testResult, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := ctx.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Invoke the make target.
	makeTargetFunc := func(opts runutil.Opts) error {
		return ctx.Run().TimedCommand(timeout, "make", makeTarget)
	}
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "Make "+makeTarget, "failure", makeTargetFunc); err != nil {
		return nil, err
	} else if testResult != nil {
		if testResult.Status == TestTimedOut {
			testResult.TimeoutValue = timeout
		}
		return testResult, nil
	}

	return &TestResult{Status: TestPassed}, nil
}

func vanadiumWWWSite(ctx *util.Context, testName string) (*TestResult, error) {
	return commonVanadiumWWW(ctx, testName, "test-site", defaultWWWTestTimeout)
}

func vanadiumWWWTutorials(ctx *util.Context, testName string) (*TestResult, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials", defaultWWWTestTimeout)
}
