//go:build integration

// export_integration_test.go re-exports the seams loop_arming_integration_test.go drives:
// internal/shedverbs sits inside internal/hubforge's dependency set, so a hubforge-using test cannot live in-package without closing a compile cycle — the standard Go export_test.go idiom.

package shedverbs

import (
	"testing"

	"github.com/spf13/cobra"
)

// StepCmdForTest builds the step command over spec with the package's own help texts.
func StepCmdForTest(spec *Spec) *cobra.Command {
	return stepCmd(stepTexts(), spec)
}

// LoopOfForTest returns the loop object of env, failing the test when it has none.
func LoopOfForTest(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	return loopOf(t, env)
}

// AssertLoopKeysForTest fails the test unless loop carries exactly the loop object's closed key set.
func AssertLoopKeysForTest(t *testing.T, loop map[string]any, withRestep bool) {
	t.Helper()
	assertLoopKeys(t, loop, withRestep)
}
