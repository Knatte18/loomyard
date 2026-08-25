package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// wantSequenceEntry pairs one expected history-row name with its expected shedengine.Outcome.
type wantSequenceEntry struct {
	name    string
	outcome shedengine.Outcome
}

// wantSequenceOrder is the row 1-through-Publish name/outcome sequence a clean Run over
// buildSequenceFixture must produce. Asserted against this literal expected list rather than a
// computed one, so a reordering in contracts/recipes/loom-recipe.yaml's row order is a test failure
// rather than a silently-agreeing derivation.
//
// Every entry but the three review segments and the trailing Publish carries a Done outcome by
// rule; the segments themselves do not, so their entries are spelled out explicitly rather than
// derived. Each segment contributes exactly three entries in the same shape: NameXBouncer with
// Stuck (the seed call, which spawns a focus-setting pass and always reports Stuck, never judging
// anything on its first call), NameXBurler with Stuck (one completed review round --
// BurlerProducer reports every successful round as Stuck by contract, never Done, since its Stuck
// is a routine hand-off to the Bouncer rather than a real stuck condition), and NameXBouncer again
// with Done (the judge call, whose fixture-scripted APPROVED verdict is what advances the run past
// the segment). The Plan-Review segment carries a fourth, trailing entry neither the
// Discussion-Review nor the Webster-Review segment has a counterpart for: NamePlanRevalidate with
// Done, the post-segment mechanical re-check, which passes here because the fixture's fake burler
// leaves the plan untouched (buildSequenceFixture never sets fakeLoomBurler.corruptPlanOverview).
// Stuck entries mid-run are therefore not a failure signal here; they are each segment doing its
// job. The list runs to nineteen entries total.
//
// The sequence stops at Publish deliberately: Publish's OnStuck is "" (escalate), so a Stuck verdict
// blocks the run and Finalize is never invoked. Driving both producers' real merge logic through a
// Shed run needs a genuine two-worktree pair and therefore git, which this batch's own decision keeps
// out of this package's untagged tier.
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
var wantSequenceOrder = []wantSequenceEntry{
	{loomshed.NamePreflight, shedengine.Done},
	{loomshed.NameLoomPreflight, shedengine.Done},
	{loomshed.NameDiscussionWrite, shedengine.Done},
	{loomshed.NameDiscussionValidate, shedengine.Done},
	{loomshed.NameDiscussionBouncer, shedengine.Stuck},
	{loomshed.NameDiscussionBurler, shedengine.Stuck},
	{loomshed.NameDiscussionBouncer, shedengine.Done},
	{loomshed.NamePlanWrite, shedengine.Done},
	{loomshed.NamePlanValidate, shedengine.Done},
	{loomshed.NamePlanBouncer, shedengine.Stuck},
	{loomshed.NamePlanBurler, shedengine.Stuck},
	{loomshed.NamePlanBouncer, shedengine.Done},
	{loomshed.NamePlanRevalidate, shedengine.Done},
	{loomshed.NameBatchifier, shedengine.Done},
	{loomshed.NameWebster, shedengine.Done},
	{loomshed.NameWebsterBouncer, shedengine.Stuck},
	{loomshed.NameWebsterBurler, shedengine.Stuck},
	{loomshed.NameWebsterBouncer, shedengine.Done},
	{loomshed.NamePublish, shedengine.Stuck},
}

// TestSequence_FullRunBlocksAtPublish is the task's own verify requirement: the seventeen-row list
// runs Preflight through Publish and blocks on Publish's Stuck verdict, never reaching Finalize --
// see wantSequenceOrder's own doc comment for why, including for all three review segments' entry
// shapes.
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
	for i, want := range wantSequenceOrder {
		entry := result.History[i]
		if entry.Producer != want.name {
			t.Errorf("History[%d].Producer = %q; want %q", i, entry.Producer, want.name)
		}
		if entry.Outcome != want.outcome {
			t.Errorf("History[%d] (%s).Outcome = %q; want %q", i, entry.Producer, entry.Outcome, want.outcome)
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
	// rather than the decorator being silently bypassed. Only one segment (Discussion-Review)
	// commits through this seam, so the count stays 1 even though the Plan-Review segment also ran.
	loomShuttle := env.Shuttle.(*fakeLoomShuttle)
	if loomShuttle.commitDiscussionCalls != 1 {
		t.Errorf("fakeLoomShuttle.commitDiscussionCalls = %d; want exactly 1 after a clean run", loomShuttle.commitDiscussionCalls)
	}

	// This is the scenario proof that the Plan-Bouncer commit_seam is genuinely reached through a
	// real Shed run: both the Plan-Write row's own commit and the Plan-Bouncer row's approval commit
	// invoke CommitPlan, so the count is 2, not 1.
	if loomShuttle.commitPlanCalls != 2 {
		t.Errorf("fakeLoomShuttle.commitPlanCalls = %d; want exactly 2 after a clean run (Plan-Write's commit plus Plan-Bouncer's approval commit)", loomShuttle.commitPlanCalls)
	}

	// The scenario checks that all three review segments genuinely ran rather than being silently
	// short-circuited: the fake burler ran exactly three rounds (one per segment), and the fake
	// shuttle recorded exactly three bouncer-judge spawns -- the judge calls whose fixture-scripted
	// APPROVED verdicts are what advanced the run past each segment.
	loomBurler := env.Burler.(*fakeLoomBurler)
	if loomBurler.calls != 3 {
		t.Errorf("fakeLoomBurler.calls = %d; want exactly 3 after a clean run", loomBurler.calls)
	}
	if loomShuttle.bouncerJudgeCalls != 3 {
		t.Errorf("fakeLoomShuttle.bouncerJudgeCalls = %d; want exactly 3 after a clean run", loomShuttle.bouncerJudgeCalls)
	}
}
