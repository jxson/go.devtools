// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"time"

	"v.io/jiri/tool"
)

// DefaultTimeout identifies the maximum time each test is allowed to
// run before being forcefully terminated.
var DefaultTimeout = 10 * time.Minute

// FailedExitCode is the error code that "jiri test" exits with if any
// of the tests it runs fails.
const FailedExitCode = 3

type Status int

type Result struct {
	Status               Status
	TimeoutValue         time.Duration       // Used when Status == TimedOut
	MergeConflictCL      string              // Used when Status == MergeConflict
	ToolsBuildFailureMsg string              // Used when Status == ToolsBuildFailure
	ExcludedTests        map[string][]string // Tests that are excluded within packages keyed by package name
	SkippedTests         map[string][]string // Tests that are skipped within packages keyed by package name
}

const (
	Pending Status = iota
	Skipped
	Passed
	Failed
	MergeConflict
	ToolsBuildFailure
	TimedOut
)

func (s Status) String() string {
	switch s {
	case Skipped:
		return "SKIPPED"
	case Passed:
		return "PASSED"
	case Failed:
		return "FAILED"
	case MergeConflict:
		return "MERGE CONFLICT"
	case TimedOut:
		return "TIMED OUT"
	default:
		return "UNKNOWN"
	}
}

func Pass(ctx *tool.Context, format string, a ...interface{}) {
	strOK := "ok"
	if ctx.Color() {
		strOK = ColorString("ok", Green)
	}
	fmt.Fprintf(ctx.Stdout(), "%s   ", strOK)
	fmt.Fprintf(ctx.Stdout(), format, a...)
}

func Fail(ctx *tool.Context, format string, a ...interface{}) {
	strFail := "fail"
	if ctx.Color() {
		strFail = ColorString("fail", Red)
	}
	fmt.Fprintf(ctx.Stderr(), "%s ", strFail)
	fmt.Fprintf(ctx.Stderr(), format, a...)
}

func Warn(ctx *tool.Context, format string, a ...interface{}) {
	strWarn := "warn"
	if ctx.Color() {
		strWarn = ColorString("warn", Yellow)
	}
	fmt.Fprintf(ctx.Stderr(), "%s ", strWarn)
	fmt.Fprintf(ctx.Stderr(), format, a...)
}
