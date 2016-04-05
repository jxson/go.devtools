// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
	"v.io/x/devtools/tooldata"
)

const (
	// numLinesToOutput identifies the number of lines to be included in
	// the error messsage of an xUnit report.
	numLinesToOutput = 50
)

const (
	dummyTestResult = `<?xml version="1.0" encoding="utf-8"?>
<!--
  This file will be used to generate a dummy test results file
  in case the presubmit tests produce no test result files.
-->
<testsuites>
  <testsuite name="NO_TESTS" tests="1" errors="0" failures="0" skip="0">
    <testcase classname="NO_TESTS" name="NO_TESTS" time="0">
    </testcase>
  </testsuite>
</testsuites>
`
)

// testNode represents a node of a test dependency graph.
type testNode struct {
	// deps is a list of its dependencies.
	deps []string
	// visited determines whether a DFS exploration of the test
	// dependency graph has visited this test.
	visited bool
	// stack determines whether this test is on the search stack
	// of a DFS exploration of the test dependency graph.
	stack bool
}

// testDepGraph captures the test dependency graph.
type testDepGraph map[string]*testNode

var testMock = func(*jiri.X, string, ...Opt) (*test.Result, error) {
	return &test.Result{Status: test.Passed}, nil
}

var testFunctions = map[string]func(*jiri.X, string, ...Opt) (*test.Result, error){
	// TODO(jsimsa,cnicolaou): consider getting rid of the vanadium- prefix.
	"ignore-this":                             testMock,
	"baku-android-build":                      bakuAndroidBuild,
	"baku-java-test":                          bakuJavaTest,
	"test-presubmit-test":                     testPresubmitTest,
	"third_party-go-build":                    thirdPartyGoBuild,
	"third_party-go-test":                     thirdPartyGoTest,
	"third_party-go-race":                     thirdPartyGoRace,
	"vanadium-android-build":                  vanadiumAndroidBuild,
	"vanadium-baku-test":                      vanadiumBakuTest,
	"vanadium-bootstrap":                      vanadiumBootstrap,
	"vanadium-browser-test":                   vanadiumBrowserTest,
	"vanadium-browser-test-web":               vanadiumBrowserTestWeb,
	"vanadium-chat-shell-test":                vanadiumChatShellTest,
	"vanadium-chat-web-test":                  vanadiumChatWebTest,
	"vanadium-chat-web-ui-test":               vanadiumChatWebUITest,
	"vanadium-copyright":                      vanadiumCopyright,
	"vanadium-create-instance-test":           vanadiumCreateInstanceTest,
	"vanadium-croupier-unit":                  vanadiumCroupierTestUnit,
	"vanadium-croupier-unit-go":               vanadiumCroupierTestUnitGo,
	"vanadium-github-mirror":                  vanadiumGitHubMirror,
	"vanadium-go-api":                         vanadiumGoAPI,
	"vanadium-go-bench":                       vanadiumGoBench,
	"vanadium-go-binaries":                    vanadiumGoBinaries,
	"vanadium-go-build":                       vanadiumGoBuild,
	"vanadium-go-cover":                       vanadiumGoCoverage,
	"vanadium-go-depcop":                      vanadiumGoDepcop,
	"vanadium-go-format":                      vanadiumGoFormat,
	"vanadium-go-generate":                    vanadiumGoGenerate,
	"vanadium-go-race":                        vanadiumGoRace,
	"vanadium-go-snapshot":                    vanadiumGoSnapshot,
	"vanadium-go-test":                        vanadiumGoTest,
	"vanadium-go-vdl":                         vanadiumGoVDL,
	"vanadium-go-vet":                         vanadiumGoVet,
	"vanadium-go-rpc-stress":                  vanadiumGoRPCStress,
	"vanadium-go-rpc-load":                    vanadiumGoRPCLoad,
	"vanadium-integration-test":               vanadiumIntegrationTest,
	"vanadium-java-test":                      vanadiumJavaTest,
	"vanadium-js-build-extension":             vanadiumJSBuildExtension,
	"vanadium-js-doc":                         vanadiumJSDoc,
	"vanadium-js-doc-deploy":                  vanadiumJSDocDeploy,
	"vanadium-js-doc-syncbase":                vanadiumJSDocSyncbase,
	"vanadium-js-doc-syncbase-deploy":         vanadiumJSDocSyncbaseDeploy,
	"vanadium-js-browser-integration":         vanadiumJSBrowserIntegration,
	"vanadium-js-node-integration":            vanadiumJSNodeIntegration,
	"vanadium-js-syncbase-browser":            vanadiumJSSyncbaseBrowser,
	"vanadium-js-syncbase-node":               vanadiumJSSyncbaseNode,
	"vanadium-js-unit":                        vanadiumJSUnit,
	"vanadium-js-vdl":                         vanadiumJSVdl,
	"vanadium-js-vdl-audit":                   vanadiumJSVdlAudit,
	"vanadium-js-vom":                         vanadiumJSVom,
	"vanadium-mojo-discovery-test":            vanadiumMojoDiscoveryTest,
	"vanadium-mojo-syncbase-test":             vanadiumMojoSyncbaseTest,
	"vanadium-mojo-v23proxy-unit-test":        vanadiumMojoV23ProxyUnitTest,
	"vanadium-mojo-v23proxy-integration-test": vanadiumMojoV23ProxyIntegrationTest,
	"vanadium-moments-test":                   vanadiumMomentsTest,
	"vanadium-nginx-deploy-production":        vanadiumNGINXDeployProduction,
	"vanadium-nginx-deploy-staging":           vanadiumNGINXDeployStaging,
	"vanadium-pipe2browser-test":              vanadiumPipe2BrowserTest,
	"vanadium-playground-test":                vanadiumPlaygroundTest,
	"vanadium-postsubmit-poll":                vanadiumPostsubmitPoll,
	"vanadium-presubmit-poll":                 vanadiumPresubmitPoll,
	"vanadium-presubmit-result":               vanadiumPresubmitResult,
	"vanadium-presubmit-test":                 vanadiumPresubmitTest,
	"vanadium-prod-services-test":             vanadiumProdServicesTest,
	"vanadium-reader-test":                    vanadiumReaderTest,
	"vanadium-regression-test":                vanadiumRegressionTest,
	"vanadium-release-candidate":              vanadiumReleaseCandidate,
	"vanadium-release-candidate-snapshot":     vanadiumReleaseCandidateSnapshot,
	"vanadium-release-production":             vanadiumReleaseProduction,
	"vanadium-release-kube-staging":           vanadiumReleaseKubeStaging,
	"vanadium-release-kube-production":        vanadiumReleaseKubeProduction,
	"vanadium-signup-github":                  vanadiumSignupGithub,
	"vanadium-signup-github-new":              vanadiumSignupGithubNew,
	"vanadium-signup-group":                   vanadiumSignupGroup,
	"vanadium-signup-group-new":               vanadiumSignupGroupNew,
	"vanadium-signup-discuss-new":             vanadiumSignupDiscussNew,
	"vanadium-signup-proxy":                   vanadiumSignupProxy,
	"vanadium-signup-proxy-new":               vanadiumSignupProxyNew,
	"vanadium-signup-welcome-1-new":           vanadiumSignupWelcomeStepOneNew,
	"vanadium-signup-welcome-2-new":           vanadiumSignupWelcomeStepTwoNew,
	"vanadium-travel-test":                    vanadiumTravelTest,
	"vanadium-website-deploy":                 vanadiumWebsiteDeploy,
	"vanadium-website-site":                   vanadiumWebsiteSite,
	"vanadium-website-tutorials-core":         vanadiumWebsiteTutorialsCore,
	"vanadium-website-tutorials-external":     vanadiumWebsiteTutorialsExternal,
	"vanadium-website-tutorials-java":         vanadiumWebsiteTutorialsJava,
}

func newTestContext(jirix *jiri.X, env map[string]string) *jiri.X {
	tmpEnv := map[string]string{}
	for key, value := range jirix.Env() {
		tmpEnv[key] = value
	}
	for key, value := range env {
		tmpEnv[key] = value
	}
	return jirix.Clone(tool.ContextOpts{
		Env: tmpEnv,
	})
}

type Opt interface {
	Opt()
}

// BlessingsRootOpt is an option that specifies the blessings root of the
// services to check in VanadiumProdServicesTest.
type BlessingsRootOpt string

func (BlessingsRootOpt) Opt() {}

// CleanGoOpt is an option that specifies whether to remove Go object
// files and binaries before running the tests.
type CleanGoOpt bool

func (CleanGoOpt) Opt() {}

// NamespaceRootOpt is an option that specifies the namespace root of the
// services to check in VanadiumProdServicesTest.
type NamespaceRootOpt string

func (NamespaceRootOpt) Opt() {}

// NumWorkersOpt is an option to control the number of test workers used.
type NumWorkersOpt int

func (NumWorkersOpt) Opt() {}

// OutputDirOpt is an option that specifies where to output the test
// results.
type OutputDirOpt string

func (OutputDirOpt) Opt() {}

// PartOpt is an option that specifies which part of the test to run.
type PartOpt int

func (PartOpt) Opt() {}

// PkgsOpt is an option that specifies which Go tests to run using a
// list of Go package expressions.
type PkgsOpt []string

func (PkgsOpt) Opt() {}

// MergePoliciesOpt is an option that specifies merge policies for use
// when merging environment variables from the environment and from profiles.
type MergePoliciesOpt profilesreader.MergePolicies

func (MergePoliciesOpt) Opt() {}

// ListTests returns a list of all tests known by the test package.
func ListTests() ([]string, error) {
	result := []string{}
	for name := range testFunctions {
		if !strings.HasPrefix(name, "ignore") {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// RunProjectTests runs all tests associated with the given projects.
func RunProjectTests(jirix *jiri.X, env map[string]string, projects []string, opts ...Opt) (map[string]*test.Result, error) {
	testCtx := newTestContext(jirix, env)

	// Parse tests and dependencies from config file.
	config, err := tooldata.LoadConfig(jirix)
	if err != nil {
		return nil, err
	}
	tests := config.ProjectTests(projects)
	if len(tests) == 0 {
		return nil, nil
	}
	sort.Strings(tests)
	graph, err := createTestDepGraph(config, tests)
	if err != nil {
		return nil, err
	}

	// Run tests.
	//
	// TODO(jingjin) parallelize the top-level scheduling loop so
	// that tests do not need to run serially.
	results := make(map[string]*test.Result, len(tests))
	for _, t := range tests {
		results[t] = &test.Result{}
	}
run:
	for i := 0; i < len(graph); i++ {
		// Find a test that can execute.
		for _, t := range tests {
			result, node := results[t], graph[t]
			if result.Status != test.Pending {
				continue
			}
			ready := true
			for _, dep := range node.deps {
				switch results[dep].Status {
				case test.Skipped, test.Failed, test.TimedOut:
					results[t].Status = test.Skipped
					continue run
				case test.Pending:
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			if err := runTests(testCtx, []string{t}, results, opts...); err != nil {
				return nil, err
			}
			continue run
		}
		// The following line should be never reached.
		return nil, fmt.Errorf("erroneous test running logic")
	}

	return results, nil
}

// RunTests executes the given tests and reports the test results.
func RunTests(jirix *jiri.X, env map[string]string, tests []string, opts ...Opt) (map[string]*test.Result, error) {
	results := make(map[string]*test.Result, len(tests))
	for _, t := range tests {
		results[t] = &test.Result{}
	}
	testCtx := newTestContext(jirix, env)
	if err := runTests(testCtx, tests, results, opts...); err != nil {
		return nil, err
	}
	return results, nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Close() error {
	return nil
}

func (nopWriteCloser) Write([]byte) (int, error) {
	return 0, nil
}

// runTests runs the given tests, populating the results map.
func runTests(jirix *jiri.X, tests []string, results map[string]*test.Result, opts ...Opt) (e error) {
	outputDir := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case OutputDirOpt:
			outputDir = string(typedOpt)
		case CleanGoOpt:
			cleanGo = bool(typedOpt)
		}
	}

	var outputFile io.WriteCloser = &nopWriteCloser{}
	if outputDir != "" {
		var err error
		// Create a file for aggregating all of the test output.
		fileName := filepath.Join(outputDir, "output")
		outputFile, err = os.Create(fileName)
		if err != nil {
			return fmt.Errorf("Create(%v) failed: %v", fileName, err)
		}
		defer collect.Error(func() error { return outputFile.Close() }, &e)
	}

	// Validate all tests before running any tests.
	for _, t := range tests {
		if _, ok := testFunctions[t]; !ok {
			return fmt.Errorf("test %v does not exist", t)
		}
	}

	for _, t := range tests {
		testFn := testFunctions[t]
		fmt.Fprintf(jirix.Stdout(), "##### Running test %q #####\n", t)

		// Create a 1MB buffer to capture the test function output.
		var out bytes.Buffer
		const largeBufferSize = 1 << 20
		out.Grow(largeBufferSize)
		newX := jirix.Clone(tool.ContextOpts{
			Stdout: io.MultiWriter(&out, jirix.Stdout()),
			Stderr: io.MultiWriter(&out, jirix.Stderr()),
		})

		// Run the test and collect the test results.
		result, err := testFn(newX, t, opts...)
		if result != nil && result.Status == test.TimedOut {
			writeTimedOutTestReport(newX, t, *result)
		}
		if err == nil {
			err = checkTestReportFile(newX, t)
		}
		if err != nil {
			fmt.Fprintf(newX.Stderr(), "%v\n", err)
			r, err := generateXUnitReportForError(newX, t, err, out.String())
			if err != nil {
				return err
			}
			result = r
		}
		results[t] = result
		if _, err := outputFile.Write(out.Bytes()); err != nil {
			return err
		}
		fmt.Fprintf(jirix.Stdout(), "##### %s #####\n", results[t].Status)
	}

	if outputDir != "" {
		// Write the test results to the given output directory.
		bytes, err := json.Marshal(results)
		if err != nil {
			return fmt.Errorf("Marshal(%v) failed: %v", results, err)
		}
		resultsFile := filepath.Join(outputDir, "results")
		if err := jirix.NewSeq().WriteFile(resultsFile, bytes, os.FileMode(0644)).Done(); err != nil {
			return err
		}
	}

	return nil
}

// writeTimedOutTestReport writes a xUnit test report for the given timed-out test.
func writeTimedOutTestReport(jirix *jiri.X, testName string, result test.Result) {
	timeoutValue := test.DefaultTimeout
	if result.TimeoutValue != 0 {
		timeoutValue = result.TimeoutValue
	}
	errorMessage := fmt.Sprintf("The test timed out after %s.", timeoutValue)
	if err := xunit.CreateFailureReport(jirix, testName, testName, "Timeout", errorMessage, errorMessage); err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	}
}

// checkTestReportFile checks that the test report file exists, contains a
// valid xUnit test report, and the set of test cases is non-empty. If any of
// these is not true, the function generates a dummy test report file that
// meets these requirements.
func checkTestReportFile(jirix *jiri.X, testName string) error {
	// Skip the checks for presubmit-test itself.
	if testName == "vanadium-presubmit-test" {
		return nil
	}

	s := jirix.NewSeq()
	xUnitReportFile := xunit.ReportPath(testName)
	if _, err := s.Stat(xUnitReportFile); err != nil {
		if !runutil.IsNotExist(err) {
			return err
		}
		// No test report.
		dummyFile, perm := filepath.Join(filepath.Dir(xUnitReportFile), "tests_dummy.xml"), os.FileMode(0644)
		if err := s.WriteFile(dummyFile, []byte(dummyTestResult), perm).Done(); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", dummyFile, err)
		}
		return nil
	}

	// Invalid xUnit file.
	bytes, err := ioutil.ReadFile(xUnitReportFile)
	if err != nil {
		return fmt.Errorf("ReadFile(%s) failed: %v", xUnitReportFile, err)
	}
	var suites xunit.TestSuites
	if err := xml.Unmarshal(bytes, &suites); err != nil {
		jirix.NewSeq().RemoveAll(xUnitReportFile)
		if err := xunit.CreateFailureReport(jirix, testName, testName, "Invalid xUnit Report", "Invalid xUnit Report", err.Error()); err != nil {
			return err
		}
		return nil
	}

	// No test cases.
	numTestCases := 0
	for _, suite := range suites.Suites {
		numTestCases += len(suite.Cases)
	}
	if numTestCases == 0 {
		s.RemoveAll(xUnitReportFile)
		if err := xunit.CreateFailureReport(jirix, testName, testName, "No Test Cases", "No Test Cases", ""); err != nil {
			return err
		}
		return nil
	}

	return nil
}

// generateXUnitReportForError generates an xUnit test report for the
// given (internal) error.
func generateXUnitReportForError(jirix *jiri.X, testName string, err error, output string) (*test.Result, error) {
	// Skip the report generation for presubmit-test itself.
	if testName == "vanadium-presubmit-test" {
		return &test.Result{Status: test.Passed}, nil
	}
	s := jirix.NewSeq()

	xUnitFilePath := xunit.ReportPath(testName)

	// Only create the report when the xUnit file doesn't exist, is
	// invalid, or exist but doesn't have failed test cases.
	createXUnitFile := false
	if _, err := s.Stat(xUnitFilePath); err != nil {
		if runutil.IsNotExist(err) {
			createXUnitFile = true
		} else {
			return nil, err
		}
	} else {
		bytes, err := s.ReadFile(xUnitFilePath)
		if err != nil {
			return nil, err
		}
		var existingSuites xunit.TestSuites
		if err := xml.Unmarshal(bytes, &existingSuites); err != nil {
			createXUnitFile = true
		} else {
			createXUnitFile = true
			for _, curSuite := range existingSuites.Suites {
				if curSuite.Failures > 0 || curSuite.Errors > 0 {
					createXUnitFile = false
					break
				}
			}
		}
	}

	if createXUnitFile {
		errType := "Internal Error"
		if internalErr, ok := err.(internalTestError); ok {
			errType = internalErr.name
			err = internalErr.err
		}
		// Create a test suite to encapsulate the error. Include last
		// <numLinesToOutput> lines of the output in the error message.
		lines := strings.Split(output, "\n")
		startLine := int(math.Max(0, float64(len(lines)-numLinesToOutput)))
		consoleOutput := "......\n" + strings.Join(lines[startLine:], "\n")
		errMsg := fmt.Sprintf("Error message:\n%s:\n%s\n\n\nConsole output:\n%s\n", errType, err.Error(), consoleOutput)
		if err := xunit.CreateFailureReport(jirix, testName, testName, errType, errType, errMsg); err != nil {
			return nil, err
		}

		if runutil.IsTimeout(err) {
			return &test.Result{Status: test.TimedOut}, nil
		}
	}
	return &test.Result{Status: test.Failed}, nil
}

// createTestDepGraph creates a test dependency graph given a map of
// dependencies and a list of tests.
func createTestDepGraph(config *tooldata.Config, tests []string) (testDepGraph, error) {
	// For the given list of tests, build a map from the test name
	// to its testInfo object using the dependency data extracted
	// from the given dependency config data "dep".
	depGraph := testDepGraph{}
	for _, test := range tests {
		// Make sure the test dependencies are included in <tests>.
		deps := []string{}
		for _, curDep := range config.TestDependencies(test) {
			isDepInTests := false
			for _, test := range tests {
				if curDep == test {
					isDepInTests = true
					break
				}
			}
			if isDepInTests {
				deps = append(deps, curDep)
			}
		}
		depGraph[test] = &testNode{
			deps: deps,
		}
	}

	// Detect dependency loop using depth-first search.
	for name, info := range depGraph {
		if info.visited {
			continue
		}
		if findCycle(name, depGraph) {
			return nil, fmt.Errorf("found dependency loop: %v", depGraph)
		}
	}
	return depGraph, nil
}

// findCycle checks whether there are any cycles in the test
// dependency graph.
func findCycle(name string, depGraph testDepGraph) bool {
	node := depGraph[name]
	node.visited = true
	node.stack = true
	for _, dep := range node.deps {
		depNode := depGraph[dep]
		if depNode.stack {
			return true
		}
		if depNode.visited {
			continue
		}
		if findCycle(dep, depGraph) {
			return true
		}
	}
	node.stack = false
	return false
}
