// pairpush_test.go tests the rules of a pair's push: the records side's landing and the readings of its branches.

package fabriccore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecordsSideLanding drives the rule with a backoff of two delays.
// Every push ends in the row's class and every reading after it finds the row's facts.
func TestRecordsSideLanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	once := []string{"push_side"}
	tests := []struct {
		name      string
		class     fabriccore.OutcomeClass
		facts     fabriccore.RecordsLandingFacts
		wantTrace []string
		// wantVerdict is asked only of a side left unpushed.
		wantVerdict fabriclayout.PushFailureClass
	}{
		{"LandedSideEnds", fabriccore.OutcomeDone, fabriccore.RecordsLandingFacts{}, once, ""},
		{"FailedPushEnds", fabriccore.OutcomeFailed, fabriccore.RecordsLandingFacts{}, once, ""},
		{"UnpushedSideOnOriginIsPushedOncePerDelayAndDeferred", fabriccore.OutcomeDone,
			fabriccore.RecordsLandingFacts{Unpushed: true, OnOrigin: true},
			[]string{"push_side", "wait 1s", "push_side", "wait 2s", "push_side"}, fabriclayout.PushFailureDeferred},
		{"DeferralSinceTheStartEndsAtOnce", fabriccore.OutcomeDone,
			fabriccore.RecordsLandingFacts{Unpushed: true, DeferredSinceStart: true}, once, fabriclayout.PushFailureDeferred},
		{"TransportLookupFailureIsOwed", fabriccore.OutcomeDone,
			fabriccore.RecordsLandingFacts{Unpushed: true, LookupTransport: true}, once, fabriclayout.PushFailureTransport},
		{"OtherLookupFailureIsUnclassified", fabriccore.OutcomeDone,
			fabriccore.RecordsLandingFacts{Unpushed: true, LookupFailed: true}, once, ""},
		{"GoneBranchOwesNoRecord", fabriccore.OutcomeDone, fabriccore.RecordsLandingFacts{Unpushed: true}, once, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := fabriccore.Snapshot{Backoff: []time.Duration{time.Second, 2 * time.Second}}
			var trace []string

			for range 10 {
				answer := fabriccore.RecordsSideLanding(snapshot)
				if len(answer.Actions) == 0 {
					break
				}
				for _, action := range answer.Actions {
					class, step := fabriccore.OutcomeDone, "wait "+action.Delay.String()
					if action.Kind == fabriccore.ActionSeamCall {
						class, step = tt.class, action.Seam
					}
					trace = append(trace, step)
					snapshot.Outcomes = append(snapshot.Outcomes, fabriccore.Outcome{Action: action, Class: class})
				}
				snapshot.RecordsLanding = tt.facts
			}

			if !slices.Equal(trace, tt.wantTrace) {
				t.Errorf("RecordsSideLanding() ran %q; want %q", trace, tt.wantTrace)
			}
			if !tt.facts.Unpushed {
				return
			}
			if got := fabriccore.RecordsLandingVerdict(snapshot); got != tt.wantVerdict {
				t.Errorf("RecordsLandingVerdict() = %q; want %q", got, tt.wantVerdict)
			}
		})
	}
}

// TestRunAhead covers RunAhead and TipsHeldOnOrigin over the same readings of a worktree's branches.
func TestRunAhead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tracked := fabriccore.PushUpstreamTracked
	level := fabriccore.BranchReading{Tip: "t1", Upstream: tracked, HoldsTip: true}
	unpushed := fabriccore.BranchReading{Tip: "t2", Upstream: tracked, LacksCommit: true}
	noUpstream := fabriccore.BranchReading{Tip: "t1", Upstream: fabriccore.PushUpstreamNone}
	pruned := fabriccore.BranchReading{Tip: "t2", Upstream: fabriccore.PushUpstreamPruned}
	noCommits := fabriccore.BranchReading{Upstream: fabriccore.PushUpstreamNone}
	heldOnly := fabriccore.BranchReading{Tip: "t2", Upstream: tracked, LacksCommit: true, Ceiling: "c1"}
	belowHeld := heldOnly
	belowHeld.LacksCeiling = true

	tests := []struct {
		name          string
		prime         bool
		records, code fabriccore.BranchReading
		wantAhead     bool
		wantHeld      bool
	}{
		{"PrimeRecordsCommitUnpushed", true, unpushed, level, true, false},
		{"LevelPrime", true, level, level, false, true},
		{"PrimeCodeCommitAlone", true, level, unpushed, false, true},
		{"RecordsBranchWithNoUpstream", true, noUpstream, level, true, false},
		{"RecordsBranchWithNoCommits", true, noCommits, level, false, true},
		{"RecordsTrackingRefDeleted", true, pruned, level, false, true},
		{"BothPairTipsPushed", false, level, level, false, true},
		{"PairRecordsCommitUnpushed", false, unpushed, level, true, false},
		{"PairCodeCommitUnpushed", false, level, unpushed, true, false},
		{"PairCodeBranchWithNoUpstream", false, level, noUpstream, true, false},
		{"PairCodeTrackingRefDeleted", false, level, pruned, false, true},
		{"PairRecordsAheadOnlyByAHeldCommit", false, heldOnly, level, false, false},
		{"PairRecordsCommitBelowTheHeldOne", false, belowHeld, level, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := fabriccore.RunAhead(tt.prime, tt.records, tt.code); got != tt.wantAhead {
				t.Errorf("RunAhead() = %v; want %v", got, tt.wantAhead)
			}
			if got := fabriccore.TipsHeldOnOrigin(tt.prime, tt.records, tt.code); got != tt.wantHeld {
				t.Errorf("TipsHeldOnOrigin() = %v; want %v", got, tt.wantHeld)
			}
		})
	}
}
