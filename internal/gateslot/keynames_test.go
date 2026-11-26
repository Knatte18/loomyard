// keynames_test.go covers the full key name a timed-out run's halt detail cites.

package gateslot_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestTimeoutKeyName pins that a tier's bound lies under gate.tiers and the build's bound does not.
func TestTimeoutKeyName(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		run  string
		want string
	}{
		{"Tier", "tier2", "gate.tiers.tier2.timeout_min"},
		{"Build", "build", "gate.build.timeout_min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := gateslot.TimeoutKeyName(tt.run); got != tt.want {
				t.Errorf("TimeoutKeyName(%q) = %q; want %q", tt.run, got, tt.want)
			}
		})
	}
}
