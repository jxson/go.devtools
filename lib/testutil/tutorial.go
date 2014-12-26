package testutil

import (
	"path/filepath"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

// vanadiumTutorial runs the vanadium tutorial examples.
//
// TODO(jregan): Merge the mdrip logic into this package.
func vanadiumTutorial(ctx *util.Context, testName string) (_ *TestResult, e error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the mdrip tool.
	opts := ctx.Run().Opts()
	opts.Env["GOPATH"] = filepath.Join(root, "tutorial", "testing")
	if err := ctx.Run().CommandWithOpts(opts, "go", "install", "mdrip"); err != nil {
		return nil, err
	}

	// Run the tutorials.
	content := filepath.Join(root, "tutorial", "www", "content")
	mdrip := filepath.Join(root, "tutorial", "testing", "bin", "mdrip")
	args := []string{"--subshell", "1",
		filepath.Join(content, "docs", "installation", "index.md"),
		filepath.Join(content, "tutorials", "basics.md"),
		filepath.Join(content, "tutorials", "security", "principals_and_blessings.md"),
		filepath.Join(content, "tutorials", "security", "custom_authorizer.md"),
		filepath.Join(content, "tutorials", "security", "acl_authorizer.md"),
		filepath.Join(content, "tutorials", "security", "delegation_and_caveats.md"),
	}
	if err := ctx.Run().TimedCommandWithOpts(DefaultTestTimeout, opts, mdrip, args...); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
