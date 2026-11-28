// mergestate_test.go holds the pure-logic tests for the merge record's two derived flags,
// landedConcludeCommit and bothSidesAlreadyUpToDate, which MergeResult.Committed and
// MergeResult.AlreadyUpToDate are read off.
// Both are two-sided predicates, so the tests need rows where exactly one side differs.
// The tests use struct literals only, with no git and no spawn,
// so the file stays tier0 under the Test Tier Purity Invariant.

package fabricengine

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

//testtiming:keep the landed-conclude predicate over every landed combination; coverage of its blocks by other tests does not show an assertion of this
func TestMergeState_LandedConcludeCommit(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestMergeState_BothSidesAlreadyUpToDate pins both conjuncts of the AlreadyUpToDate predicate.
// The mixed rows check each conjunct on its own.
// AlreadyUpToDate reports whether the attempt found both sides already carrying the resolved source.
// A merge that moved only one side must report false.
//
//testtiming:keep both conjuncts of the AlreadyUpToDate predicate, with the mixed rows; coverage of its blocks by other tests does not show an assertion of this
func TestMergeState_BothSidesAlreadyUpToDate(t *testing.T) {
	tiergate.Require(t, 0)
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
