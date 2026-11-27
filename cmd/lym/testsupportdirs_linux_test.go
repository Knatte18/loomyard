// testsupportdirs_linux_test.go holds the plan check's test-support directories equal to the test-only code guard's list.
// The guard's list compiles on Linux only, so this file does too.

package main

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTestSupportDirectories_MatchTheTestOnlyCodeGuard fails when the two lists differ.
// A difference makes the plan check refuse a function the guard counts as reached, or pass one it does not.
func TestTestSupportDirectories_MatchTheTestOnlyCodeGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var guarded []string
	for _, entry := range testSupportPackages {
		guarded = append(guarded, entry.Key)
	}
	if !slices.Equal(planparser.TestSupportDirectories, guarded) {
		t.Errorf("planparser.TestSupportDirectories = %q; want testSupportPackages' keys %q", planparser.TestSupportDirectories, guarded)
	}
}
