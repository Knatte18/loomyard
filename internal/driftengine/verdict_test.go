// verdict_test.go — tier 0 tests for the regression check's section rendered from the scope record.

package driftengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestVerdict covers no record, scope off, a waiver, a compare not yet run, a passing and a drifted compare, and accepted drift by each acceptor.
func TestVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	compared := func(passed bool) ScopeRecord {
		return ScopeRecord{Scope: ScopeFull, BaselineHead: "base1", ComparedHead: "head2", ComparedAt: "2026-01-02T03:04:05Z", ComparePassed: passed}
	}
	tests := []struct {
		name  string
		scope *ScopeRecord
		want  string
	}{
		{"NoRecordCarriesNoSection", nil, ""},
		{"ScopeOffNamesOnlyTheScope", &ScopeRecord{Scope: ScopeOff}, "## Regression check\n\n- Scope: off; the task is not regression-checked."},
		{
			"WaivedNamesTheReason",
			&ScopeRecord{Scope: ScopePartial, Skip: []string{"slow", "flaky"}, WaivedAt: "2026-01-02T03:04:05Z", WaivedReason: "no baseline possible"},
			"## Regression check\n\n- Scope: partial, skipping `slow`, `flaky`.\n- Compare: waived: no baseline possible\n- Accepted drift: none.",
		},
		{"NotComparedSaysSo", &ScopeRecord{Scope: ScopeFull}, "## Regression check\n\n- Scope: full.\n- Compare: not run.\n- Accepted drift: none."},
		{
			"PassedNamesTheBaselineAndTheComparedHead",
			func() *ScopeRecord { s := compared(true); return &s }(),
			"## Regression check\n\n- Scope: full.\n- Baseline: captured at `base1`, the task's branch point.\n- Compare: passed at `head2`.\n- Accepted drift: none.",
		},
		{
			"DriftedSaysSo",
			func() *ScopeRecord { s := compared(false); return &s }(),
			"## Regression check\n\n- Scope: full.\n- Baseline: captured at `base1`, the task's branch point.\n- Compare: showed drift at `head2`.\n- Accepted drift: none.",
		},
		{
			"AcceptedDriftNamesEachTestWithItsAcceptorAndReason",
			func() *ScopeRecord {
				s := compared(true)
				s.Accepted = []Acceptance{
					{Test: "t1", Reason: "the model changed", By: AcceptedByCaller, Ruling: RulingRuled},
					{Test: "t2", Reason: "new rounding", By: AcceptedByOperator, Ruling: RulingRuled},
					{Test: "t3", Reason: "intended", By: AcceptedByAgent, Ruling: RulingUnruled},
				}
				return &s
			}(),
			"## Regression check\n\n- Scope: full.\n- Baseline: captured at `base1`, the task's branch point.\n- Compare: passed at `head2`.\n- Accepted drift:\n" +
				"  - `t1`, accepted by the run's parent: the model changed\n" +
				"  - `t2`, accepted by the operator: new rounding\n" +
				"  - `t3`, accepted by the Drift-Verify agent: intended (not yet ruled on by the run's parent)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := NewPaths(t.TempDir())
			if tt.scope != nil {
				err := UpdateScope(p, func(ScopeRecord, bool) (ScopeRecord, error) { return *tt.scope, nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := Verdict(p)
			if err != nil || got != tt.want {
				t.Errorf("Verdict() = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}

	t.Run("NoRecordCreatesNoDirectory", func(t *testing.T) {
		t.Parallel()
		p := NewPaths(t.TempDir())
		if _, err := Verdict(p); err != nil {
			t.Fatalf("Verdict() = _, %v; want nil", err)
		}
		if _, err := os.Stat(filepath.Dir(p.Capture)); err == nil {
			t.Errorf("Verdict() with no scope record created the ephemeral drift directory")
		}
	})
}
