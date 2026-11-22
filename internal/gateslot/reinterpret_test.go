// reinterpret_test.go covers RetireHardenLiveCopies over hub gate.yaml bytes.

package gateslot_test

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRetireHardenLiveCopies pins the outcomes of the rewrite.
// An entry equal to harden-live's one tier is removed.
// A differing entry that stops the gate config loading is removed and flagged.
// Such an entry has no scope, an invalid scope, or tiers that do not decode.
// Any other differing entry is flagged and left, with the step that removes it.
// A file with no entry is unchanged.
func TestRetireHardenLiveCopies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const wayForward = "`lym config gate --set 'gates.harden-live={tiers: [<live tier>], scope: diff}'`"
	billedTierClause := fmt.Sprintf("one tier below tier%d", gateslot.BilledTier)
	const unsetStep = "`lym config gate --unset gates.harden-live-copies`"
	tests := []struct {
		name          string
		input         string
		wantUnchanged bool
		wantGates     map[string]gateslot.GateKey
		wantRewrote   []string
		wantFlagged   []string
		// wantUnsetStep says whether every flagged line names the step that removes the left entry.
		wantUnsetStep bool
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
			name:  "ScopelessDifferingEntryIsRemovedAndFlagged",
			input: "gates:\n  harden-live:\n    tiers: [0, 1]\n  harden-live-copies:\n    tiers: [1]\n",
			wantGates: map[string]gateslot.GateKey{
				"harden-live": {Tiers: []int{0, 1}},
			},
			wantRewrote: []string{"removed gates.harden-live-copies, which stopped the gate config loading"},
			wantFlagged: []string{"the removed gates.harden-live-copies named the tiers [1] and gates.harden-live names the tiers [0 1]"},
		},
		{
			name:  "UndecodableTiersWithAScopeAreRemovedAndFlagged",
			input: "gates:\n  harden-live: {tiers: [0, 1], scope: diff}\n  harden-live-copies: {tiers: [oops], scope: diff}\n",
			wantGates: map[string]gateslot.GateKey{
				"harden-live": {Tiers: []int{0, 1}, Scope: "diff"},
			},
			wantRewrote: []string{"removed gates.harden-live-copies, which stopped the gate config loading"},
			wantFlagged: []string{"the removed gates.harden-live-copies did not decode and gates.harden-live names the tiers [0 1]"},
		},
		{
			name:        "RetiredScopeIsRemovedAndFlagged",
			input:       "gates:\n  harden-live-copies: {tiers: [1], scope: coverage}\n",
			wantGates:   map[string]gateslot.GateKey{},
			wantRewrote: []string{"removed gates.harden-live-copies, which stopped the gate config loading"},
			wantFlagged: []string{"the removed gates.harden-live-copies named the tiers [1] and gates.harden-live names the tiers [0]"},
		},
		{
			name:          "SeveralLiveTiersAreFlaggedAndLeft",
			input:         "gates:\n  harden-live: {tiers: [0, 1], scope: diff}\n  harden-live-copies: {tiers: [1], scope: diff}\n",
			wantUnchanged: true,
			wantFlagged:   []string{"gates.harden-live-copies is retired and gates.harden-live names the tiers [0 1]"},
			wantUnsetStep: true,
		},
		{
			name:          "BilledTierIsFlaggedAndLeft",
			input:         "gates:\n  harden-live: {tiers: [3], scope: diff}\n  harden-live-copies: {tiers: [3], scope: diff}\n",
			wantUnchanged: true,
			wantFlagged:   []string{"gates.harden-live-copies is retired and gates.harden-live names the tiers [3]"},
			wantUnsetStep: true,
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
				if !strings.Contains(flagged[i], billedTierClause) {
					t.Errorf("flagged line = %q; want it to hold %q", flagged[i], billedTierClause)
				}
				if got := strings.Contains(flagged[i], unsetStep); got != tt.wantUnsetStep {
					t.Errorf("flagged line = %q; names the unset step = %v; want %v", flagged[i], got, tt.wantUnsetStep)
				}
			}
		})
	}
}
