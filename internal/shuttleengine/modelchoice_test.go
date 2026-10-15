// modelchoice_test.go verifies NewModelChoice maps a resolved model spec onto the neutral choice.

package shuttleengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestNewModelChoice pins which Resolved fields and params reach ModelChoice and that an absent or unrelated param leaves nothing behind.
func TestNewModelChoice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved modelspec.Resolved
		want     ModelChoice
	}{
		{
			name:     "BothParamsMap",
			resolved: modelspec.Resolved{Engine: "claude", Model: "opus", Params: map[string]string{"effort": "high", "version": "5"}},
			want:     ModelChoice{Engine: "claude", Model: "opus", Effort: "high", Version: "5"},
		},
		{
			name:     "EmptyParamsLeaveEffortAndVersionEmpty",
			resolved: modelspec.Resolved{Engine: "claude", Model: "opus", Params: map[string]string{}},
			want:     ModelChoice{Engine: "claude", Model: "opus"},
		},
		{
			name:     "UnrelatedParamIgnored",
			resolved: modelspec.Resolved{Engine: "claude", Model: "opus", Params: map[string]string{"other": "x"}},
			want:     ModelChoice{Engine: "claude", Model: "opus"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NewModelChoice(tt.resolved); got != tt.want {
				t.Errorf("NewModelChoice(%+v) = %+v; want %+v", tt.resolved, got, tt.want)
			}
		})
	}
}
