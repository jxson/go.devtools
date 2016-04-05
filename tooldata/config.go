// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tooldata

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/jiri"
	"v.io/jiri/project"
	"v.io/x/lib/envvar"
	"v.io/x/lib/set"
)

// Config holds configuration common to jiri tools.
type Config struct {
	// apiCheckProjects identifies the set of project names for which
	// the API check is required.
	apiCheckProjects map[string]struct{}
	// copyrightCheckProjects identifies the set of project names for
	// which the copyright check is required.
	copyrightCheckProjects map[string]struct{}
	// goWorkspaces identifies JIRI_ROOT subdirectories that contain a
	// Go workspace.
	goWorkspaces []string
	// jenkinsMatrixJobs identifies the set of matrix (multi-configutation) jobs
	// in Jenkins.
	jenkinsMatrixJobs map[string]JenkinsMatrixJobInfo
	// projectTests maps jiri projects to sets of tests that should be
	// executed to test changes in the given project.
	projectTests map[string][]string
	// testDependencies maps tests to sets of tests that the given test
	// depends on.
	testDependencies map[string][]string
	// testGroups maps test group labels to sets of tests that the label
	// identifies.
	testGroups map[string][]string
	// testParts maps test names to lists of strings that identify
	// different parts of a test. If a list L has n elements, then the
	// corresponding test has n+1 parts: the first n parts are identified
	// by L[0] to L[n-1]. The last part is whatever is left.
	testParts map[string][]string
	// vdlWorkspaces identifies JIRI_ROOT subdirectories that contain
	// a VDL workspace.
	vdlWorkspaces []string
}

// ConfigOpt is an interface for Config factory options.
type ConfigOpt interface {
	configOpt()
}

// APICheckProjectsOpt is the type that can be used to pass the Config
// factory a API check projects option.
type APICheckProjectsOpt map[string]struct{}

func (APICheckProjectsOpt) configOpt() {}

// CopyrightCheckProjectsOpt is the type that can be used to pass the
// Config factory a copyright check projects option.
type CopyrightCheckProjectsOpt map[string]struct{}

func (CopyrightCheckProjectsOpt) configOpt() {}

// GoWorkspacesOpt is the type that can be used to pass the Config
// factory a Go workspace option.
type GoWorkspacesOpt []string

func (GoWorkspacesOpt) configOpt() {}

// JenkinsMatrixJobsOpt is the type that can be used to pass the Config factory
// a Jenkins matrix jobs option.
type JenkinsMatrixJobsOpt map[string]JenkinsMatrixJobInfo

func (JenkinsMatrixJobsOpt) configOpt() {}

// ProjectTestsOpt is the type that can be used to pass the Config
// factory a project tests option.
type ProjectTestsOpt map[string][]string

func (ProjectTestsOpt) configOpt() {}

// TestDependenciesOpt is the type that can be used to pass the Config
// factory a test dependencies option.
type TestDependenciesOpt map[string][]string

func (TestDependenciesOpt) configOpt() {}

// TestGroupsOpt is the type that can be used to pass the Config
// factory a test groups option.
type TestGroupsOpt map[string][]string

func (TestGroupsOpt) configOpt() {}

// TestPartsOpt is the type that can be used to pass the Config
// factory a test parts option.
type TestPartsOpt map[string][]string

func (TestPartsOpt) configOpt() {}

// VDLWorkspacesOpt is the type that can be used to pass the Config
// factory a VDL workspace option.
type VDLWorkspacesOpt []string

func (VDLWorkspacesOpt) configOpt() {}

// NewConfig is the Config factory.
func NewConfig(opts ...ConfigOpt) *Config {
	var c Config
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case APICheckProjectsOpt:
			c.apiCheckProjects = map[string]struct{}(typedOpt)
		case CopyrightCheckProjectsOpt:
			c.copyrightCheckProjects = map[string]struct{}(typedOpt)
		case GoWorkspacesOpt:
			c.goWorkspaces = []string(typedOpt)
		case JenkinsMatrixJobsOpt:
			c.jenkinsMatrixJobs = map[string]JenkinsMatrixJobInfo(typedOpt)
		case ProjectTestsOpt:
			c.projectTests = map[string][]string(typedOpt)
		case TestDependenciesOpt:
			c.testDependencies = map[string][]string(typedOpt)
		case TestGroupsOpt:
			c.testGroups = map[string][]string(typedOpt)
		case TestPartsOpt:
			c.testParts = map[string][]string(typedOpt)
		case VDLWorkspacesOpt:
			c.vdlWorkspaces = []string(typedOpt)
		}
	}
	return &c
}

// APICheckProjects returns the set of project names for which the API
// check is required.
func (c Config) APICheckProjects() map[string]struct{} {
	return c.apiCheckProjects
}

// CopyrightCheckProjects returns the set of project names for which
// the copyright check is required.
func (c Config) CopyrightCheckProjects() map[string]struct{} {
	return c.copyrightCheckProjects
}

// GroupTests returns a list of Jenkins tests associated with the
// given test groups.
func (c Config) GroupTests(groups []string) []string {
	testSet := map[string]struct{}{}
	testGroups := c.testGroups
	for _, group := range groups {
		if testGroup, ok := testGroups[group]; ok {
			set.String.Union(testSet, set.String.FromSlice(testGroup))
		}
	}
	tests := set.String.ToSlice(testSet)
	sort.Strings(tests)
	return tests
}

// GoWorkspaces returns the Go workspaces included in the config.
func (c Config) GoWorkspaces() []string {
	return c.goWorkspaces
}

// JenkinsMatrixJobs returns the set of Jenkins matrix jobs.
func (c Config) JenkinsMatrixJobs() map[string]JenkinsMatrixJobInfo {
	return c.jenkinsMatrixJobs
}

// Projects returns a list of projects included in the config.
func (c Config) Projects() []string {
	var projects []string
	for project, _ := range c.projectTests {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

// ProjectTests returns a list of Jenkins tests associated with the
// given projects by the config.
func (c Config) ProjectTests(projects []string) []string {
	testSet := map[string]struct{}{}
	testGroups := c.testGroups
	for _, project := range projects {
		for _, test := range c.projectTests[project] {
			if testGroup, ok := testGroups[test]; ok {
				set.String.Union(testSet, set.String.FromSlice(testGroup))
			} else {
				testSet[test] = struct{}{}
			}
		}
	}
	tests := set.String.ToSlice(testSet)
	sort.Strings(tests)
	return tests
}

// TestDependencies returns a list of dependencies for the given test.
func (c Config) TestDependencies(test string) []string {
	return c.testDependencies[test]
}

// TestParts returns a list of strings that identify different test parts.
func (c Config) TestParts(test string) []string {
	return c.testParts[test]
}

// VDLWorkspaces returns the VDL workspaces included in the config.
func (c Config) VDLWorkspaces() []string {
	return c.vdlWorkspaces
}

// GoPath computes and returns the GOPATH environment variable based on the
// current jiri configuration.
func (c Config) GoPath(jirix *jiri.X) string {
	projects, err := project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		return ""
	}
	path := pathHelper(jirix, projects, c.goWorkspaces, "")
	return "GOPATH=" + envvar.JoinTokens(path, ":")
}

// VDLPath computes and returns the VDLPATH environment variable based on the
// current jiri configuration.
func (c Config) VDLPath(jirix *jiri.X) string {
	projects, err := project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		return ""
	}
	path := pathHelper(jirix, projects, c.vdlWorkspaces, "src")
	return "VDLPATH=" + envvar.JoinTokens(path, ":")
}

// pathHelper is a utility function for determining paths for project workspaces.
func pathHelper(jirix *jiri.X, projects project.Projects, workspaces []string, suffix string) []string {
	path := []string{}
	for _, workspace := range workspaces {
		absWorkspace := filepath.Join(jirix.Root, workspace, suffix)
		// Only append an entry to the path if the workspace is rooted
		// under a jiri project that exists locally or vice versa.
		for _, project := range projects {
			// We check if <project.Path> is a prefix of <absWorkspace> to
			// account for Go workspaces nested under a single jiri project,
			// such as: $JIRI_ROOT/release/projects/chat/go.
			//
			// We check if <absWorkspace> is a prefix of <project.Path> to
			// account for Go workspaces that span multiple jiri projects,
			// such as: $JIRI_ROOT/release/go.
			if strings.HasPrefix(absWorkspace, project.Path) || strings.HasPrefix(project.Path, absWorkspace) {
				if _, err := jirix.NewSeq().Stat(filepath.Join(absWorkspace)); err == nil {
					path = append(path, absWorkspace)
					break
				}
			}
		}
	}
	return path
}

type configSchema struct {
	APICheckProjects       []string                `xml:"apiCheckProjects>project"`
	CopyrightCheckProjects []string                `xml:"copyrightCheckProjects>project"`
	GoWorkspaces           []string                `xml:"goWorkspaces>workspace"`
	JenkinsMatrixJobs      jenkinsMatrixJobsSchema `xml:"jenkinsMatrixJobs>job"`
	ProjectTests           testGroupSchemas        `xml:"projectTests>project"`
	TestDependencies       dependencyGroupSchemas  `xml:"testDependencies>test"`
	TestGroups             testGroupSchemas        `xml:"testGroups>group"`
	TestParts              partGroupSchemas        `xml:"testParts>test"`
	VDLWorkspaces          []string                `xml:"vdlWorkspaces>workspace"`
	XMLName                xml.Name                `xml:"config"`
}

type dependencyGroupSchema struct {
	Name         string   `xml:"name,attr"`
	Dependencies []string `xml:"dependency"`
}

type dependencyGroupSchemas []dependencyGroupSchema

func (d dependencyGroupSchemas) Len() int           { return len(d) }
func (d dependencyGroupSchemas) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d dependencyGroupSchemas) Less(i, j int) bool { return d[i].Name < d[j].Name }

type JenkinsMatrixJobInfo struct {
	HasArch  bool `xml:"arch,attr"`
	HasOS    bool `xml:"OS,attr"`
	HasParts bool `xml:"parts,attr"`
	// ShowOS determines whether to show OS label in job summary.
	// It is possible that a job (e.g. jiri-go-race) has an OS axis but
	// the axis only has a single value in order to constrain where its
	// sub-builds run. In such cases, we do not want to show the OS label.
	ShowOS bool   `xml:"showOS,attr"`
	Name   string `xml:",chardata"`
}

type jenkinsMatrixJobsSchema []JenkinsMatrixJobInfo

func (jobs jenkinsMatrixJobsSchema) Len() int           { return len(jobs) }
func (jobs jenkinsMatrixJobsSchema) Swap(i, j int)      { jobs[i], jobs[j] = jobs[j], jobs[i] }
func (jobs jenkinsMatrixJobsSchema) Less(i, j int) bool { return jobs[i].Name < jobs[j].Name }

type partGroupSchema struct {
	Name  string   `xml:"name,attr"`
	Parts []string `xml:"part"`
}

type partGroupSchemas []partGroupSchema

func (p partGroupSchemas) Len() int           { return len(p) }
func (p partGroupSchemas) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p partGroupSchemas) Less(i, j int) bool { return p[i].Name < p[j].Name }

type testGroupSchema struct {
	Name  string   `xml:"name,attr"`
	Tests []string `xml:"test"`
}

type testGroupSchemas []testGroupSchema

func (p testGroupSchemas) Len() int           { return len(p) }
func (p testGroupSchemas) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p testGroupSchemas) Less(i, j int) bool { return p[i].Name < p[j].Name }

// LoadConfig returns the configuration stored in the tools
// configuration file.
func LoadConfig(jirix *jiri.X) (*Config, error) {
	configPath, err := ConfigFilePath(jirix)
	if err != nil {
		return nil, err
	}
	return loadConfig(jirix, configPath)
}

func loadConfig(jirix *jiri.X, path string) (*Config, error) {
	configBytes, err := jirix.NewSeq().ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data configSchema
	if err := xml.Unmarshal(configBytes, &data); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(configBytes), err)
	}
	config := &Config{
		apiCheckProjects:       map[string]struct{}{},
		copyrightCheckProjects: map[string]struct{}{},
		goWorkspaces:           []string{},
		jenkinsMatrixJobs:      map[string]JenkinsMatrixJobInfo{},
		projectTests:           map[string][]string{},
		testDependencies:       map[string][]string{},
		testGroups:             map[string][]string{},
		testParts:              map[string][]string{},
		vdlWorkspaces:          []string{},
	}
	config.apiCheckProjects = set.String.FromSlice(data.APICheckProjects)
	config.copyrightCheckProjects = set.String.FromSlice(data.CopyrightCheckProjects)
	for _, workspace := range data.GoWorkspaces {
		config.goWorkspaces = append(config.goWorkspaces, workspace)
	}
	sort.Strings(config.goWorkspaces)
	for _, job := range data.JenkinsMatrixJobs {
		config.jenkinsMatrixJobs[job.Name] = job
	}
	for _, project := range data.ProjectTests {
		config.projectTests[project.Name] = project.Tests
	}
	for _, test := range data.TestDependencies {
		config.testDependencies[test.Name] = test.Dependencies
	}
	for _, group := range data.TestGroups {
		config.testGroups[group.Name] = group.Tests
	}
	for _, test := range data.TestParts {
		config.testParts[test.Name] = test.Parts
	}
	for _, workspace := range data.VDLWorkspaces {
		config.vdlWorkspaces = append(config.vdlWorkspaces, workspace)
	}
	sort.Strings(config.vdlWorkspaces)
	return config, nil
}

// SaveConfig writes the given configuration to the tools
// configuration file.
func SaveConfig(jirix *jiri.X, config *Config) error {
	configPath, err := ConfigFilePath(jirix)
	if err != nil {
		return err
	}
	return saveConfig(jirix, config, configPath)
}

func saveConfig(jirix *jiri.X, config *Config, path string) error {
	var data configSchema
	data.APICheckProjects = set.String.ToSlice(config.apiCheckProjects)
	sort.Strings(data.APICheckProjects)
	data.CopyrightCheckProjects = set.String.ToSlice(config.copyrightCheckProjects)
	sort.Strings(data.CopyrightCheckProjects)
	for _, workspace := range config.goWorkspaces {
		data.GoWorkspaces = append(data.GoWorkspaces, workspace)
	}
	sort.Strings(data.GoWorkspaces)
	for _, job := range config.jenkinsMatrixJobs {
		data.JenkinsMatrixJobs = append(data.JenkinsMatrixJobs, job)
	}
	sort.Sort(data.JenkinsMatrixJobs)
	for name, tests := range config.projectTests {
		data.ProjectTests = append(data.ProjectTests, testGroupSchema{
			Name:  name,
			Tests: tests,
		})
	}
	sort.Sort(data.ProjectTests)
	for name, dependencies := range config.testDependencies {
		data.TestDependencies = append(data.TestDependencies, dependencyGroupSchema{
			Name:         name,
			Dependencies: dependencies,
		})
	}
	sort.Sort(data.TestDependencies)
	for name, tests := range config.testGroups {
		data.TestGroups = append(data.TestGroups, testGroupSchema{
			Name:  name,
			Tests: tests,
		})
	}
	sort.Sort(data.TestGroups)
	for name, parts := range config.testParts {
		data.TestParts = append(data.TestParts, partGroupSchema{
			Name:  name,
			Parts: parts,
		})
	}
	sort.Sort(data.TestParts)
	for _, workspace := range config.vdlWorkspaces {
		data.VDLWorkspaces = append(data.VDLWorkspaces, workspace)
	}
	sort.Strings(data.VDLWorkspaces)
	bytes, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", data, err)
	}
	s := jirix.NewSeq()
	if err := s.MkdirAll(filepath.Dir(path), os.FileMode(0755)).
		WriteFile(path, bytes, os.FileMode(0644)).Done(); err != nil {
		return err
	}
	return nil
}
