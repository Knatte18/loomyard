package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// wantSequenceOrder is the full 12-row name sequence a clean Run over buildSequenceFixture must
// produce. Asserted against this literal expected list rather than a computed one, so a reordering
// in loomshed.go's producer table is a test failure rather than a silently-agreeing derivation.
var wantSequenceOrder = []string{
	NamePreflight,
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
	NameFinalize,
}

// TestSequence_FullRunReachesDone is the task's own verify requirement: the full 12-row sequence
// running to completion against the real list.
func TestSequence_FullRunReachesDone(t *testing.T) {
	_, deps := buildSequenceFixture(t)

	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunDone {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunDone, result.Reason)
	}
	if result.HaltedProducer != NameFinalize {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, NameFinalize)
	}

	if len(result.History) != len(wantSequenceOrder) {
		t.Fatalf("Run() History has %d entries; want %d: %+v", len(result.History), len(wantSequenceOrder), result.History)
	}
	for i, wantName := range wantSequenceOrder {
		entry := result.History[i]
		if entry.Producer != wantName {
			t.Errorf("History[%d].Producer = %q; want %q", i, entry.Producer, wantName)
		}
		if entry.Outcome != shedengine.Done {
			t.Errorf("History[%d] (%s).Outcome = %q; want %q", i, entry.Producer, entry.Outcome, shedengine.Done)
		}
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](deps.StatusPath, deps.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after Run()")
	}
	if got.State != shedengine.StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StateDone)
	}
	if got.CurrentProducer != NameFinalize {
		t.Errorf("persisted CurrentProducer = %q; want %q -- current_producer must still name the final row on the happy-path terminal", got.CurrentProducer, NameFinalize)
	}
}
