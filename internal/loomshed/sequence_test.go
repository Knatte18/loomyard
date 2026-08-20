package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// wantSequenceOrder is the row 1-12 name sequence a clean Run over buildSequenceFixture must
// produce. Asserted against this literal expected list rather than a computed one, so a reordering
// in loomshed.go's producer table is a test failure rather than a silently-agreeing derivation.
//
// The sequence stops at Publish (row 12) deliberately: Publish's OnStuck is "" (escalate), so a
// Stuck verdict blocks the run and row 13 (Finalize) is never invoked. Driving both producers'
// real merge logic through a Shed run needs a genuine two-worktree pair and therefore git, which
// this batch's own decision keeps out of this package's untagged tier.
//
// The real row 2 (Loom-Preflight) passes against this fixture rather than needing a substituted
// fake because buildSequenceFixture seeds through the production Seed, which writes a coherent
// fresh seed, and by the instant row 2 runs, shedengine.Run has already persisted
// current_producer: "Loom-Preflight" alongside a single Preflight Done history entry -- exactly the
// shape row 2's told expected name and tolerated set accept.
var wantSequenceOrder = []string{
	NamePreflight,
	NameLoomPreflight,
	NameDiscussionWrite,
	NameDiscussionValidate,
	NameDiscussionReview,
	NamePlanWrite,
	NamePlanValidate,
	NamePlanReview,
	NameBatchifier,
	NameWebster,
	NameWebsterReview,
	NamePublish,
}

// TestSequence_FullRunBlocksAtPublish is the task's own verify requirement: the 13-row list runs
// rows 1 through 12 (Preflight through Publish) and blocks on Publish's Stuck verdict, never
// reaching Finalize (row 13) -- see wantSequenceOrder's own doc comment for why.
func TestSequence_FullRunBlocksAtPublish(t *testing.T) {
	_, deps := buildSequenceFixture(t)

	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunBlocked, result.Reason)
	}
	if result.HaltedProducer != NamePublish {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, NamePublish)
	}

	if len(result.History) != len(wantSequenceOrder) {
		t.Fatalf("Run() History has %d entries; want %d: %+v", len(result.History), len(wantSequenceOrder), result.History)
	}
	for i, wantName := range wantSequenceOrder {
		entry := result.History[i]
		if entry.Producer != wantName {
			t.Errorf("History[%d].Producer = %q; want %q", i, entry.Producer, wantName)
		}
		wantOutcome := shedengine.Done
		if wantName == NamePublish {
			wantOutcome = shedengine.Stuck
		}
		if entry.Outcome != wantOutcome {
			t.Errorf("History[%d] (%s).Outcome = %q; want %q", i, entry.Producer, entry.Outcome, wantOutcome)
		}
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](deps.StatusPath, deps.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after Run()")
	}
	if got.State != shedengine.StateBlocked {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StateBlocked)
	}
	if got.CurrentProducer != NamePublish {
		t.Errorf("persisted CurrentProducer = %q; want %q -- current_producer must name the row the run blocked on", got.CurrentProducer, NamePublish)
	}
}
