package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"tools/lib/cmdline"
	"tools/lib/util"
	"tools/lib/version"
)

const (
	defaultConfigFile                  = "$VEYRON_ROOT/tools/conf/presubmit"
	defaultGerritBaseUrl               = "https://veyron-review.googlesource.com"
	defaultLogFilePath                 = "$HOME/tmp/presubmit_log"
	defaultNetRcFilePath               = "$HOME/.netrc"
	defaultPresubmitTestJenkinsProject = "veyron-presubmit-test"
	defaultQueryString                 = "(status:open -project:experimental)"
	defaultTestScriptsBase             = "$VEYRON_ROOT/scripts/jenkins/"
	jenkinsBaseJobUrl                  = "http://www.envyor.com/jenkins/job"
	outputPrefix                       = "[VEYRON PRESUBMIT]"
)

type credential struct {
	username string
	password string
}

var (
	// flags
	configFileFlag                  string
	gerritBaseUrlFlag               string
	jenkinsBuildNumberFlag          int
	jenkinsHostFlag                 string
	jenkinsTokenFlag                string
	logFilePathFlag                 string
	manifestFlag                    string
	netRcFilePathFlag               string
	presubmitTestJenkinsProjectFlag string
	queryStringFlag                 string
	repoFlag                        string
	reviewMessageFlag               string
	reviewTargetRefFlag             string
	testScriptsBasePathFlag         string
	verboseFlag                     bool

	reURLUnsafeChars     *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
	veyronRoot           string
)

func init() {
	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&netRcFilePathFlag, "netrc", defaultNetRcFilePath, "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&jenkinsTokenFlag, "token", "", "The Jenkins API token.")
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log_file", defaultLogFilePath, "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.StringVar(&presubmitTestJenkinsProjectFlag, "project", defaultPresubmitTestJenkinsProject, "The name of the Jenkins project to add presubmit-test builds to.")
	cmdPost.Flags.StringVar(&reviewMessageFlag, "msg", "", "The review message to post to Gerrit.")
	cmdPost.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&configFileFlag, "conf", defaultConfigFile, "The config file for presubmit tests.")
	cmdTest.Flags.StringVar(&repoFlag, "repo", "", "The URL of the repository containing the CL pointed by the ref.")
	cmdTest.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testScriptsBasePathFlag, "tests_base_path", defaultTestScriptsBase, "The base path of all the test scripts.")
	cmdTest.Flags.StringVar(&manifestFlag, "manifest", "manifest/v1/default", "Name of the project manifest.")
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
}

// printf outputs the given message prefixed by outputPrefix.
func printf(out io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(out, "%s ", outputPrefix)
	fmt.Fprintf(out, format, args...)
}

// substituteVarsInFlags substitutes environment variables in default
// values of relevant flags.
func substituteVarsInFlags() {
	veyronRoot, err := util.VeyronRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	if configFileFlag == defaultConfigFile {
		configFileFlag = filepath.Join(veyronRoot, "tools", "conf", "presubmit")
	}
	if logFilePathFlag == defaultLogFilePath {
		logFilePathFlag = filepath.Join(os.Getenv("HOME"), "tmp", "presubmit_log")
	}
	if netRcFilePathFlag == defaultNetRcFilePath {
		netRcFilePathFlag = filepath.Join(os.Getenv("HOME"), ".netrc")
	}
	if testScriptsBasePathFlag == defaultTestScriptsBase {
		testScriptsBasePathFlag = filepath.Join(veyronRoot, "scripts", "jenkins")
	}
}

// root returns a command that represents the root of the presubmit tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the presubmit tool.
var cmdRoot = &cmdline.Command{
	Name:     "presubmit",
	Short:    "Tool for performing various presubmit related functions",
	Long:     "The presubmit tool performs various presubmit related functions.",
	Children: []*cmdline.Command{cmdQuery, cmdPost, cmdTest, cmdVersion},
}

// cmdVersion represent the 'version' command of the presubmit tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmit tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	printf(command.Stdout(), "presubmit tool version %v\n", version.Version)
	return nil
}
