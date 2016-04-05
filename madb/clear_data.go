// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"v.io/x/lib/cmdline"
	"v.io/x/lib/gosh"
)

func init() {
	initializePropertyCacheFlags(&cmdMadbClearData.Flags)
}

var cmdMadbClearData = &cmdline.Command{
	Runner: subCommandRunner{initMadbClearData, runMadbClearDataForDevice, true},
	Name:   "clear-data",
	Short:  "Clear your app data from all devices",
	Long: `
Clears your app data from all devices.

To specify which user's data should be cleared, use 'madb user set' command to set the default user
ID for that device. (See 'madb help user' for more details.)

`,
	ArgsName: "[<application_id>]",
	ArgsLong: `
<application_id> is usually the package name where the activities are defined.
(See: http://tools.android.com/tech-docs/new-build-system/applicationid-vs-packagename)


If the application ID is not specified, madb automatically determines which app to be cleared, based
on the build scripts found in the current working directory.

If the working directory contains a Gradle Android project (i.e., has "build.gradle"), run a small
Gradle script to extract the application ID. In this case, the extracted ID is cached, so that
"madb clear-data" can be repeated without even running the Gradle script again. The ID can be
re-extracted by clearing the cache by providing "-clear-cache" flag.
`,
}

func initMadbClearData(env *cmdline.Env, args []string, properties variantProperties) ([]string, error) {
	return initMadbCommand(env, args, properties, false, false)
}

func runMadbClearDataForDevice(env *cmdline.Env, args []string, d device, properties variantProperties) error {
	sh := gosh.NewShell(nil)
	defer sh.Cleanup()

	sh.ContinueOnError = true

	if len(args) == 1 {
		appID := args[0]

		// TODO(youngseokyoon): maybe do something equivalent for flutter?
		cmdArgs := []string{"-s", d.Serial, "shell", "pm", "clear"}

		// Specify the user ID if applicable.
		if d.UserID != "" {
			cmdArgs = append(cmdArgs, "--user", d.UserID)
		}
		cmdArgs = append(cmdArgs, appID)

		cmd := sh.Cmd("adb", cmdArgs...)
		return runGoshCommandForDevice(cmd, d, true)
	}

	return fmt.Errorf("No arguments are provided and failed to extract the id from the build scripts.")
}
