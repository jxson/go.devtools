// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
)

const (
	defaultWebsiteTestTimeout = 15 * time.Minute
)

// Runs the specified make target in the 'website' repo as a test.
func commonVanadiumWebsite(jirix *jiri.X, testName, makeTarget string, timeout time.Duration, extraDeps []string) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(jirix, testName, append([]string{"v23:base", "v23:nodejs"}, extraDeps...))
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	s := jirix.NewSeq()
	if err := s.Chdir(filepath.Join(jirix.Root, "website")).
		Run("make", "clean").
		Timeout(timeout).Last("make", makeTarget); err != nil {
		if runutil.IsTimeout(err) {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, newInternalError(err, "Make "+makeTarget)
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWebsiteDeploy(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWebsite(jirix, testName, "deploy", defaultWebsiteTestTimeout, nil)
}

func vanadiumWebsiteSite(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWebsite(jirix, testName, "test-site", defaultWebsiteTestTimeout, nil)
}

func vanadiumWebsiteTutorialsCore(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWebsite(jirix, testName, "test-tutorials-core", defaultWebsiteTestTimeout, nil)
}

func vanadiumWebsiteTutorialsExternal(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	// The external tutorial test installs both the jiri CLI tool and Vanadium
	// from scratch. This simulates a new user walking through the install and
	// tutorial process but can be time consuming. To help prevent this test from
	// timing out before it has finished a reasonably high timeout is set for this
	// test. The timeout should not be removed entirely, since tests do sometimes
	// completely hang.
	timeout := 60 * time.Minute
	return commonVanadiumWebsite(jirix, testName, "test-tutorials-external", timeout, nil)
}

func vanadiumWebsiteTutorialsJava(jirix *jiri.X, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWebsite(jirix, testName, "test-tutorials-java", defaultWebsiteTestTimeout, []string{"java"})
}

// vanadiumNGINXDeployHelper updates various configurations on the nginx
// instances and restarts all managed running services that are not nginx.
func vanadiumNGINXDeployHelper(jirix *jiri.X, testName string, env string, _ ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	dir := filepath.Join(jirix.Root, "infrastructure", "nginx")
	target := "deploy-" + env
	project := "vanadium-" + env
	if err := jirix.NewSeq().Chdir(dir).
		Run("make", target).
		Last("./restart.sh", project); err != nil {
		return &test.Result{Status: test.Failed}, err
	}
	return &test.Result{Status: test.Passed}, nil
}

func vanadiumNGINXDeployProduction(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumNGINXDeployHelper(jirix, testName, "production")
}
func vanadiumNGINXDeployStaging(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumNGINXDeployHelper(jirix, testName, "staging")
}
