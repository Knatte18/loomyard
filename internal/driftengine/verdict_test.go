// verdict_test.go — tier 0 tests for the regression check's section rendered from the committed drift records.

package driftengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeVerdictRecords commits scope and judged as the records of d.
func writeVerdictRecords(t *testing.T, d Deps, scope ScopeRecord, judged JudgedRecord) {
	t.Helper()
	err := WriteRecords(d, func(s *ScopeRecord, j *JudgedRecord) error {
		*s, *j = scope, judged
		return nil
	})
	if err != nil {
		t.Fatalf("WriteRecords() error = %v; want nil", err)
	}
}

// TestVerdict covers the section for a missing record, an off scope, a deferral and a full check's counts.
// It also covers a command-mode check, an old partial scope, and a hand-edited scope record after a records commit.
func TestVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	join := func(lines ...string) string { return strings.Join(lines, "\n") }
	tests := []struct {
		name   string
		scope  *ScopeRecord
		judged JudgedRecord
		want   string
	}{
		{"NoRecordCarriesNoSection", nil, JudgedRecord{}, ""},
		{
			"ScopeOffNamesItsSource",
			&ScopeRecord{Scope: ScopeOff, ScopeSource: "drift.scope"},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: off, set by `drift.scope`; the task is not regression-checked."),
		},
		{
			"DeferredSaysNoCompareRan",
			&ScopeRecord{Scope: ScopeFull, Deferred: true, AwaitedAt: "2026-01-02T03:04:05Z"},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: full.", "- Capture: deferred, so no compare ran."),
		},
		{
			"FullCheckNamesCountsUncoveredEntriesRangesAndAmendments",
			&ScopeRecord{
				Scope: ScopeFull,
				Diff:  DiffLines,
				LastCheck: CheckResult{
					Head: "head2", Passed: true, At: "2026-01-02T03:04:05Z", Mode: DiffLines,
					Changed: 5, Expected: 4, Matched: 3, Reused: 1, Unexpected: 0,
					UncoveredEntries: []string{"a.txt Total"},
				},
				Amendments: []Amendment{
					{By: "caller", At: "2026-01-02T03:05:00Z", Kind: "entries", Entries: "cases:\n  a.txt:\n    why: new\n"},
					{By: "caller", At: "2026-01-02T03:06:00Z", Kind: "restore"},
				},
			},
			JudgedRecord{Ranges: []JudgedRange{{Case: "report.txt", Range: "12-", Why: "the table was rewritten", Checked: "every row kept its column order"}}},
			join("## Regression check", "", "- Scope: full.", "- Diff: lines.",
				"- Last check: passed at `head2`. 5 changed cells, 4 expected, 3 judged as matching, 1 reused from stored rulings, 0 unexpected.",
				"- Expectation entries that covered no cell:",
				"  - `a.txt Total`",
				"- Open ranges:",
				"  - `12-` of `report.txt`: why: the table was rewritten; checked: every row kept its column order",
				"- Amendments by the run's parent:",
				"  - `entries` by caller:",
				"    ```yaml",
				"    cases:",
				"      a.txt:",
				"        why: new",
				"    ```",
				"  - `restore` by caller"),
		},
		{
			"CommandCheckSaysContainmentWasTheJudges",
			&ScopeRecord{Scope: ScopeFull, LastCheck: CheckResult{Head: "head3", At: "2026-01-02T03:04:05Z", Mode: DiffCommand, CompareExit: 3}},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: full.",
				"- Diff: command; containment was the judge's, which ruled each failing case of the compare.",
				"- Last check: did not pass at `head3`. The compare exited 3."),
		},
		{
			"OldPartialScopeNamesTheLeftOutCases",
			&ScopeRecord{Scope: ScopePartial, Skip: []string{"slow", "flaky"}, LastCheck: CheckResult{Head: "head2", Passed: true, At: "2026-01-02T03:04:05Z", Mode: DiffLines}},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: partial, an old scope that left out the cases `slow`, `flaky`.", "- Diff: lines.",
				"- Last check: passed at `head2`. 0 changed cells, 0 expected, 0 judged as matching, 0 reused from stored rulings, 0 unexpected."),
		},
		{
			"OldRecordRendersCompareAndAcceptanceFieldsUnderTheOldDriftLine",
			&ScopeRecord{
				Scope: ScopeFull, BaselineHead: "base1", ComparedHead: "head2", ComparedAt: "2026-01-02T03:04:05Z",
				Accepted: []Acceptance{
					{Test: "t1", Reason: "the model changed", By: AcceptedByCaller, Ruling: RulingRuled},
					{Test: "t3", Reason: "intended", By: AcceptedByAgent, Ruling: RulingUnruled},
				},
			},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: full.",
				"- The check ran under the old Drift, which compared whole profiles.",
				"- Baseline: captured at `base1`, the task's branch point.",
				"- Compare: showed drift at `head2`.",
				"- Accepted drift:",
				"  - `t1`, accepted by caller: the model changed",
				"  - `t3`, accepted by agent: intended (not yet ruled on by the run's parent)"),
		},
		{
			"OldWaiverNamesTheReason",
			&ScopeRecord{Scope: ScopeFull, WaivedAt: "2026-01-02T03:04:05Z", WaivedReason: "no baseline possible"},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: full.",
				"- The check ran under the old Drift, which compared whole profiles.",
				"- Compare: waived: no baseline possible",
				"- Accepted drift: none."),
		},
		{
			"NoCheckSaysNoneRan",
			&ScopeRecord{Scope: ScopeFull},
			JudgedRecord{},
			join("## Regression check", "", "- Scope: full.", "- Check: not run."),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newCommitStore(t)
			d := store.deps(NewPaths(store.root))
			if tt.scope != nil {
				writeVerdictRecords(t, d, *tt.scope, tt.judged)
			}
			got, err := Verdict(d)
			if err != nil || got != tt.want {
				t.Errorf("Verdict() = %q, %v; want %q, nil", got, err, tt.want)
			}
			if tt.scope == nil {
				if _, err := os.Stat(filepath.Dir(d.Paths.Capture)); err == nil {
					t.Errorf("Verdict() with no scope record created the ephemeral drift directory")
				}
			}
		})
	}

	t.Run("HandEditedScopeRecordRendersTheCommittedCopy", func(t *testing.T) {
		t.Parallel()
		store := newCommitStore(t)
		d := store.deps(NewPaths(store.root))
		writeVerdictRecords(t, d, ScopeRecord{Scope: ScopeFull}, JudgedRecord{})
		if err := os.WriteFile(d.Paths.Scope, []byte(`{"scope":"off"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		const want = "## Regression check\n\n- Scope: full.\n- Check: not run."
		if got, err := Verdict(d); err != nil || got != want {
			t.Errorf("Verdict() after a hand edit = %q, %v; want %q, nil", got, err, want)
		}
	})
}
