// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package one_test

import "testing"

import "v.io/core/veyron/lib/modules"
import "v.io/core/veyron/lib/testutil/v23tests"

func init() {
	modules.RegisterChild("modulesOneExt", ``, modulesOneExt)
	modules.RegisterChild("modulesTwoExt", ``, modulesTwoExt)
}

func TestV23OneA(t *testing.T) {
	v23tests.RunTest(t, V23TestOneA)
}

func TestV23OneB(t *testing.T) {
	v23tests.RunTest(t, V23TestOneB)
}