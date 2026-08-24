package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// wantSequenceOrder is the row 1-12 name sequence a clean Run over buildSequenceFixture must
// produce. Asserted against this literal expected list rather than a computed one, so a reordering
// in contracts/recipes/loom-recipe.yaml's row order is a test failure rather than a silently-agreeing
// derivation.
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
//
// Row 3 (Discussion-Write) passes too, now that it is a real shedadapters.SingleLLMProducer behind
// loomshed's commit decorator rather than a Stub: the fixture's fake shuttle writes both discussion
// output files and reports Done, so the decorator's injected commit closure fires and
// Discussion-Validate finds a complete pair.
//
// Row 6 (Plan-Write) passes for the same reason: it is now a real shedadapters.SingleLLMProducer
// behind loomshed's rotate-and-commit decorator, and the fixture's fake shuttle rewrites the whole
// plan directory on its "plan"-role branch, so Plan-Validate still finds a complete, approved,
// zero-findings plan after the decorator's rotation archived the seeded one away.
var wantSequenceOrder = []string{
	loomshed.NamePreflight,
	loomshed.NameLoomPreflight,
	loomshed.NameDiscussionWrite,
	loomshed.NameDiscussionValidate,
	loomshed.NameDiscussionReview,
	loomshed.NamePlanWrite,
	loomshed.NamePlanValidate,
	loomshed.NamePlanReview,
	loomshed.NameBatchifier,
	loomshed.NameWebster,
	loomshed.NameWebsterReview,
	loomshed.NamePublish,
}

// TestSequence_FullRunBlocksAtPublish is the task's own verify requirement: the 13-row list runs
// rows 1 through 12 (Preflight through Publish) and blocks on Publish's Stuck verdict, never
// reaching Finalize (row 13) -- see wantSequenceOrder's own doc comment for why.
func TestSequence_FullRunBlocksAtPublish(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)

	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunBlocked, result.Reason)
	}
	if result.HaltedProducer != loomshed.NamePublish {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NamePublish)
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
		if wantName == loomshed.NamePublish {
			wantOutcome = shedengine.Stuck
		}
		if entry.Outcome != wantOutcome {
			t.Errorf("History[%d] (%s).Outcome = %q; want %q", i, entry.Producer, entry.Outcome, wantOutcome)
		}
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after Run()")
	}
	if got.State != shedengine.StateBlocked {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StateBlocked)
	}
	if got.CurrentProducer != loomshed.NamePublish {
		t.Errorf("persisted CurrentProducer = %q; want %q -- current_producer must name the row the run blocked on", got.CurrentProducer, loomshed.NamePublish)
	}

	// This is the scenario check that a Done from row 3 genuinely reaches the Fabric-commit seam,
	// rather than the decorator being silently bypassed.
	loomShuttle := env.Shuttle.(*fakeLoomShuttle)
	if loomShuttle.commitDiscussionCalls != 1 {
		t.Errorf("fakeLoomShuttle.commitDiscussionCalls = %d; want exactly 1 after a clean run", loomShuttle.commitDiscussionCalls)
	}

	// The equivalent check for row 6: a Done from Plan-Write must reach its own commit seam too,
	// rather than the decorator being silently bypassed.
	if loomShuttle.commitPlanCalls != 1 {
		t.Errorf("fakeLoomShuttle.commitPlanCalls = %d; want exactly 1 after a clean run", loomShuttle.commitPlanCalls)
	}
}
