// tmuxallowed_test.go pins which tier selections let a test binary's tests start tmux servers.
// The test sets LYM_TEST_TIER, a process-global variable, so it does not run in parallel.

package tmuxkit

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestTmuxAllowed_FollowsTheTierSelection(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"Unset", "", false},
		{"Tier0", "0", false},
		{"Tier1", "1", false},
		{"Tier2", "2", true},
		{"Tier3", "3", true},
		{"RangeReachingTier2", "0-2", true},
		{"RangeReachingTier3", "1-3", true},
		{"Malformed", "tier2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tiergate.EnvVar, tt.value)
			if got := tmuxAllowed(); got != tt.want {
				t.Errorf("tmuxAllowed() with %s=%q = %v; want %v", tiergate.EnvVar, tt.value, got, tt.want)
			}
		})
	}
}
