// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func tempFilename(t *testing.T) string {
	f, err := ioutil.TempFile("", "madb_test")
	if err != nil {
		t.Fatalf("could not open a temp file: %v", err)
	}
	f.Close()

	return f.Name()
}

func TestParseDevicesOutput(t *testing.T) {
	var output string

	// Normal case
	output = `List of devices attached
deviceid01          device usb:3-3.4.3 product:bullhead model:Nexus_5X device:bullhead
emulator-5554       device product:sdk_phone_armv7 model:sdk_phone_armv7 device:generic

`

	got, err := parseDevicesOutput(output, nil)
	if err != nil {
		t.Fatalf("failed to parse the output: %v", err)
	}

	want := []device{
		device{
			Serial:     "deviceid01",
			Type:       realDevice,
			Qualifiers: []string{"usb:3-3.4.3", "product:bullhead", "model:Nexus_5X", "device:bullhead"},
			Nickname:   "",
			Index:      1,
		},
		device{
			Serial:     "emulator-5554",
			Type:       emulator,
			Qualifiers: []string{"product:sdk_phone_armv7", "model:sdk_phone_armv7", "device:generic"},
			Nickname:   "",
			Index:      2,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}

	// No devices at all
	output = `List of devices attached

`

	got, err = parseDevicesOutput(output, nil)
	if err != nil {
		t.Fatalf("failed to parse the output: %v", err)
	}
	if want = []device{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}

	// Offline devices should be excluded
	output = `List of devices attached
deviceid01       offline usb:3-3.4.3 product:bullhead model:Nexus_5X device:bullhead
deviceid02       device product:sdk_phone_armv7 model:sdk_phone_armv7 device:generic

`
	got, err = parseDevicesOutput(output, nil)
	if err != nil {
		t.Fatalf("failed to parse the output: %v", err)
	}

	want = []device{
		device{
			Serial:     "deviceid02",
			Type:       realDevice,
			Qualifiers: []string{"product:sdk_phone_armv7", "model:sdk_phone_armv7", "device:generic"},
			Nickname:   "",
			Index:      2,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}

	// In case some nicknames are defined.
	output = `List of devices attached
deviceid01          device usb:3-3.4.3 product:bullhead model:Nexus_5X device:bullhead
emulator-5554       device product:sdk_phone_armv7 model:sdk_phone_armv7 device:generic

	`

	nsm := map[string]string{
		"MyPhone": "deviceid01",
		"ARMv7":   "model:sdk_phone_armv7",
	}

	got, err = parseDevicesOutput(output, nsm)
	if err != nil {
		t.Fatalf("failed to parse the output: %v", err)
	}

	want = []device{
		device{
			Serial:     "deviceid01",
			Type:       realDevice,
			Qualifiers: []string{"usb:3-3.4.3", "product:bullhead", "model:Nexus_5X", "device:bullhead"},
			Nickname:   "MyPhone",
			Index:      1,
		},
		device{
			Serial:     "emulator-5554",
			Type:       emulator,
			Qualifiers: []string{"product:sdk_phone_armv7", "model:sdk_phone_armv7", "device:generic"},
			Nickname:   "ARMv7",
			Index:      2,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}
}

func TestGetSpecifiedDevices(t *testing.T) {
	// First, define some devices (three real devices, and two emulators).
	d1 := device{
		Serial:     "deviceid01",
		Type:       realDevice,
		Qualifiers: []string{"usb:3-3.4.3", "product:bullhead", "model:Nexus_5X", "device:bullhead"},
		Nickname:   "MyPhone",
		Index:      1,
	}

	d2 := device{
		Serial:     "deviceid02",
		Type:       realDevice,
		Qualifiers: []string{"usb:3-3.4.1", "product:volantisg", "model:Nexus_9", "device:flounder_lte"},
		Nickname:   "",
		Index:      2,
	}

	e1 := device{
		Serial:     "emulator-5554",
		Type:       emulator,
		Qualifiers: []string{"product:sdk_phone_armv7", "model:sdk_phone_armv7", "device:generic"},
		Nickname:   "ARMv7",
		Index:      3,
	}

	d3 := device{
		Serial:     "deviceid03",
		Type:       realDevice,
		Qualifiers: []string{"usb:3-3.3", "product:bullhead", "model:Nexus_5X", "device:bullhead"},
		Nickname:   "SecondPhone",
		Index:      4,
	}

	e2 := device{
		Serial:     "emulator-5555",
		Type:       emulator,
		Qualifiers: []string{"product:sdk_phone_armv7", "model:sdk_phone_armv7", "device:generic"},
		Nickname:   "",
		Index:      5,
	}

	allDevices := []device{d1, d2, e1, d3, e2}

	type deviceFlags struct {
		allDevices   bool
		allEmulators bool
		devices      string
	}

	testCases := []struct {
		flags deviceFlags
		want  []device
	}{
		{deviceFlags{false, false, ""}, allDevices},                        // Nothing is specified
		{deviceFlags{true, true, ""}, allDevices},                          // Both -d and -e are specified
		{deviceFlags{true, false, ""}, []device{d1, d2, d3}},               // Only -d is specified
		{deviceFlags{false, true, ""}, []device{e1, e2}},                   // Only -e is specified
		{deviceFlags{false, false, "device:bullhead"}, []device{d1, d3}},   // Device qualifier
		{deviceFlags{false, false, "ARMv7,SecondPhone"}, []device{e1, d3}}, // Nicknames
		{deviceFlags{false, false, "@2,@4"}, []device{d2, d3}},             // Device Indices
		{deviceFlags{true, false, "ARMv7"}, []device{d1, d2, e1, d3}},      // Combinations
		{deviceFlags{false, true, "model:Nexus_9"}, []device{d2, e1, e2}},  // Combinations
		{deviceFlags{false, false, "@1,SecondPhone"}, []device{d1, d3}},    // Combinations
	}

	for i, testCase := range testCases {
		allDevicesFlag = testCase.flags.allDevices
		allEmulatorsFlag = testCase.flags.allEmulators
		devicesFlag = testCase.flags.devices

		got, err := filterSpecifiedDevices(allDevices)
		if err != nil {
			t.Fatalf(err.Error())
		}

		if !reflect.DeepEqual(got, testCase.want) {
			t.Fatalf("unmatched results for testCases[%v]: got %v, want %v", i, got, testCase.want)
		}
	}
}

func TestIsFlutterProject(t *testing.T) {
	testCases := []struct {
		projectDir string
		want       bool
	}{
		{"testMultiPlatform", false},
		{"testMultiPlatform/android", false},
		{"testMultiPlatform/android/app", false},
		{"testMultiPlatform/flutter", true},
	}

	for i, testCase := range testCases {
		dir := filepath.Join("testdata", "projects", testCase.projectDir)
		if got := isFlutterProject(dir); got != testCase.want {
			t.Fatalf("unmatched results for testCases[%v]: got %v, want %v", i, got, testCase.want)
		}
	}
}

func TestIsGradleProject(t *testing.T) {
	testCases := []struct {
		projectDir string
		want       bool
	}{
		{"testMultiPlatform", false},
		{"testMultiPlatform/android", true},
		{"testMultiPlatform/android/app", true},
		{"testMultiPlatform/flutter", false},
	}

	for i, testCase := range testCases {
		dir := filepath.Join("testdata", "projects", testCase.projectDir)
		if got := isGradleProject(dir); got != testCase.want {
			t.Fatalf("unmatched results for testCases[%v]: got %v, want %v", i, got, testCase.want)
		}
	}
}

func TestExtractIdsFromGradle(t *testing.T) {
	testCases := []struct {
		key  variantKey
		want projectIds
	}{
		{
			variantKey{"testMultiPlatform/android", "", ""},
			projectIds{"io.v.testProjectId", "io.v.testProjectPackage.LauncherActivity"},
		},
		{
			variantKey{"testMultiPlatform/android", "app", "debug"},
			projectIds{"io.v.testProjectId", "io.v.testProjectPackage.LauncherActivity"},
		},
		{
			variantKey{"testMultiPlatform/android/app", "", ""},
			projectIds{"io.v.testProjectId", "io.v.testProjectPackage.LauncherActivity"},
		},
		{
			variantKey{"testAndroidMultiFlavor", "", ""},
			projectIds{"io.v.testProjectId.lite", "io.v.testProjectPackage.LauncherActivity"},
		},
		{
			variantKey{"testAndroidMultiFlavor", "app", "liteDebug"},
			projectIds{"io.v.testProjectId.lite.debug", "io.v.testProjectPackage.LauncherActivity"},
		},
		{
			variantKey{"testAndroidMultiFlavor/app", "", "proRelease"},
			projectIds{"io.v.testProjectId.pro", "io.v.testProjectPackage.LauncherActivity"},
		},
	}

	for i, testCase := range testCases {
		testCase.key.Dir = filepath.Join("testdata", "projects", testCase.key.Dir)
		got, err := extractIdsFromGradle(testCase.key)
		if err != nil {
			t.Fatalf("error occurred while extracting ids for testCases[%v]: %v", i, err)
		}

		if !reflect.DeepEqual(got, testCase.want) {
			t.Fatalf("unmatched results for testCases[%v]: got %v, want %v", i, got, testCase.want)
		}
	}
}

func TestGetProjectIds(t *testing.T) {
	cacheFile := tempFilename(t)
	defer os.Remove(cacheFile)

	called := false

	// See if it runs the extractor for the first time.
	extractor := func(key variantKey) (projectIds, error) {
		called = true
		return projectIds{"testAppID", "Activity"}, nil
	}

	want := projectIds{"testAppID", "Activity"}
	got, err := getProjectIds(extractor, variantKey{"testDir", "mod", "var"}, false, cacheFile)

	if err != nil {
		t.Fatalf(err.Error())
	}

	if !called {
		t.Fatalf("extractor was not called when expected to be called.")
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}

	// The second run should not invoke the extractor.
	called = false
	got, err = getProjectIds(extractor, variantKey{"testDir", "mod", "var"}, false, cacheFile)

	if err != nil {
		t.Fatalf(err.Error())
	}

	if called {
		t.Fatalf("extracted was called when not expected.")
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}

	// Run with clear cache flag.
	called = false
	got, err = getProjectIds(extractor, variantKey{"testDir", "mod", "var"}, true, cacheFile)

	if err != nil {
		t.Fatalf(err.Error())
	}

	if !called {
		t.Fatalf("extractor was not called when expected to be called.")
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmatched results: got %v, want %v", got, want)
	}
}
