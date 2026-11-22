// reinterpret_test.go covers RetireHardenLiveCopies over hub gate.yaml bytes.

package gateslot_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRetireHardenLiveCopies pins the three outcomes of the rewrite.
// An entry equal to harden-live's one tier is removed.
// A differing entry is flagged and left.
// A file with no entry is unchanged.
func TestRetireHardenLiveCopies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const wayForward = "`lym config gate --set 'gates.harden-live={tiers: [<live tier>], scope: diff}'`"
	tests := []struct {
		name          string
		input         string
		wantUnchanged bool
		wantGates     map[string]gateslot.GateKey
		wantRewrote   []string
		wantFlagged   []string
	}{
		{
			name:  "EqualOneTierIsRemoved",
			input: "slots: 2\ngates:\n  harden-live: {tiers: [1], scope: diff}\n  harden-live-copies: {tiers: [1], scope: diff}\n",
			wantGates: map[string]gateslot.GateKey{
				"harden-live": {Tiers: []int{1}, Scope: "diff"},
			},
			wantRewrote: []string{"removed gates.harden-live-copies, which names the tier harden-live already names, tier 1"},
		},
		{
			name:        "AbsentLiveKeyReadsTheTemplateTierZero",
			input:       "gates:\n  harden-live-copies: {tiers: [0], scope: diff}\n",
			wantGates:   map[string]gateslot.GateKey{},
			wantRewrote: []string{"removed gates.harden-live-copies, which names the tier harden-live already names, tier 0"},
		},
		{
			name:          "SeveralLiveTiersAreFlaggedAndLeft",
			input:         "gates:\n  harden-live: {tiers: [0, 1], scope: diff}\n  harden-live-copies: {tiers: [1], scope: diff}\n",
			wantUnchanged: true,
			wantFlagged:   []string{"gates.harden-live-copies is retired and gates.harden-live names the tiers [0 1]"},
		},
		{
			name:          "BilledTierIsFlaggedAndLeft",
			input:         "gates:\n  harden-live: {tiers: [3], scope: diff}\n  harden-live-copies: {tiers: [3], scope: diff}\n",
			wantUnchanged: true,
			wantFlagged:   []string{"gates.harden-live-copies is retired and gates.harden-live names the tiers [3]"},
		},
		{
			name:          "NoCopiesKeyIsUnchanged",
			input:         "gates:\n  harden-live: {tiers: [0, 1], scope: diff}\n",
			wantUnchanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := gateslot.RetireHardenLiveCopies([]byte(tt.input), "", "")
			if err != nil {
				t.Fatalf("RetireHardenLiveCopies() error = %v; want nil", err)
			}
			if tt.wantUnchanged {
				if string(got) != tt.input {
					t.Errorf("RetireHardenLiveCopies() rewrote the file to %q; want it unchanged", got)
				}
			} else {
				var decoded struct {
					Gates map[string]gateslot.GateKey `yaml:"gates"`
				}
				if err := yaml.Unmarshal(got, &decoded); err != nil {
					t.Fatalf("rewritten file does not parse: %v", err)
				}
				if diff := cmp.Diff(tt.wantGates, decoded.Gates); diff != "" {
					t.Errorf("RetireHardenLiveCopies() gates mismatch (-want +got):\n%s", diff)
				}
			}
			if diff := cmp.Diff(tt.wantRewrote, rewrote); diff != "" {
				t.Errorf("RetireHardenLiveCopies() rewrote mismatch (-want +got):\n%s", diff)
			}
			if len(flagged) != len(tt.wantFlagged) {
				t.Fatalf("RetireHardenLiveCopies() flagged %d lines; want %d: %v", len(flagged), len(tt.wantFlagged), flagged)
			}
			for i, want := range tt.wantFlagged {
				if !strings.Contains(flagged[i], want) || !strings.Contains(flagged[i], wayForward) {
					t.Errorf("flagged line = %q; want it to hold %q and %q", flagged[i], want, wayForward)
				}
			}
		})
	}
}
