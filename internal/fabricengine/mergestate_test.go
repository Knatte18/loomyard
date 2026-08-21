// mergestate_test.go is pure-logic (Tier 1) coverage for the merge record's two derived flags,
// landedConcludeCommit and bothSidesAlreadyUpToDate, which MergeResult.Committed and
// MergeResult.AlreadyUpToDate are read off.
// Both are two-sided predicates, and a two-sided predicate needs the mixed rows to be load-bearing:
// dropping the weft conjunct from bothSidesAlreadyUpToDate left every other test in the repo green,
// because nothing asserted a record with one side up_to_date and the other not.
// Struct-literal only — no git, no spawn — so this file stays untagged per the Test Tier Purity
// Invariant.

package fabricengine

import "testing"

func TestMergeState_LandedConcludeCommit(t *testing.T) {
	tests := []struct {
		name          string
		warpCommitted string
		weftCommitted string
		want          bool
	}{
		{name: "NeitherSideLanded", warpCommitted: "", weftCommitted: "", want: false},
		{name: "WarpOnlyLanded", warpCommitted: "abc123", weftCommitted: "", want: true},
		{name: "WeftOnlyLanded", warpCommitted: "", weftCommitted: "def456", want: true},
		{name: "BothSidesLanded", warpCommitted: "abc123", weftCommitted: "def456", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &mergeState{WarpCommitted: tt.warpCommitted, WeftCommitted: tt.weftCommitted}
			if got := st.landedConcludeCommit(); got != tt.want {
				t.Errorf("landedConcludeCommit() with (warp %q, weft %q) = %v; want %v",
					tt.warpCommitted, tt.weftCommitted, got, tt.want)
			}
		})
	}
}

// TestMergeState_BothSidesAlreadyUpToDate pins BOTH conjuncts of the AlreadyUpToDate predicate.
// The two mixed rows are the point: doc.go promises AlreadyUpToDate answers "whether the attempt
// found BOTH sides already carrying the resolved source", so a merge that really moved one side must
// report false. With one conjunct dropped it reported true, and no test noticed.
func TestMergeState_BothSidesAlreadyUpToDate(t *testing.T) {
	tests := []struct {
		name        string
		warpOutcome string
		weftOutcome string
		want        bool
	}{
		{name: "BothUpToDate", warpOutcome: mergeOutcomeAlreadyUpToDate, weftOutcome: mergeOutcomeAlreadyUpToDate, want: true},
		{name: "WarpUpToDateWeftStaged", warpOutcome: mergeOutcomeAlreadyUpToDate, weftOutcome: mergeOutcomeStaged, want: false},
		{name: "WarpStagedWeftUpToDate", warpOutcome: mergeOutcomeStaged, weftOutcome: mergeOutcomeAlreadyUpToDate, want: false},
		{name: "WarpUpToDateWeftFastForwarded", warpOutcome: mergeOutcomeAlreadyUpToDate, weftOutcome: mergeOutcomeFastForwarded, want: false},
		{name: "WarpFastForwardedWeftUpToDate", warpOutcome: mergeOutcomeFastForwarded, weftOutcome: mergeOutcomeAlreadyUpToDate, want: false},
		{name: "WarpUpToDateWeftConflicted", warpOutcome: mergeOutcomeAlreadyUpToDate, weftOutcome: mergeOutcomeConflicted, want: false},
		{name: "NeitherUpToDate", warpOutcome: mergeOutcomeStaged, weftOutcome: mergeOutcomeStaged, want: false},
		{
			// A record written before either MergeStart returned carries empty outcomes. Empty is not
			// up_to_date, and must never be read as one.
			name: "BothOutcomesEmpty", warpOutcome: "", weftOutcome: "", want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &mergeState{WarpOutcome: tt.warpOutcome, WeftOutcome: tt.weftOutcome}
			if got := st.bothSidesAlreadyUpToDate(); got != tt.want {
				t.Errorf("bothSidesAlreadyUpToDate() with (warp %q, weft %q) = %v; want %v",
					tt.warpOutcome, tt.weftOutcome, got, tt.want)
			}
		})
	}
}
