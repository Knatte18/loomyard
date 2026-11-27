package loomrecipe

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantSequenceEntry pairs one expected history-row name with its expected shedengine.Outcome.
type wantSequenceEntry struct {
	name    string
	outcome shedengine.Outcome
}

// wantSequenceOrder is the name and outcome sequence of the rows from 1 through Merge-In.
// A clean Run over buildSequenceFixture must produce exactly this sequence.
// It follows the row order of wantProducerTable, which TestNew_ShapeMatchesRecipe pins against New;
// a reordering of the rows in contracts/recipes/tapestry-recipe.yaml therefore fails the test.
//
// Every entry but the three review segments and the trailing Merge-In carries a Done outcome by rule.
// The segment entries carry no such rule, so they are spelled out explicitly.
// Each segment contributes four entries in the same shape:
// `NameXBouncer` with Stuck, the seed call, which spawns a focus-setting pass and never judges on its first call;
// `NameXBurlerReview` with Stuck, since the review row reports every successful review as Stuck by contract;
// `NameXBurlerFix` with Stuck, the fix row's hand-back to the Bouncer;
// and `NameXBouncer` with Done, the judge call;
// its fixture-scripted CONVERGED verdict advances the run past the segment.
// Stuck entries mid-run are therefore each segment doing its job, not a failure signal.
//
// The sequence stops at Merge-In because its OnStuck is "" (escalate), so a Stuck verdict blocks the run.
// The fixture's fake tips report the local parent ahead of its upstream, so Merge-In reports one before any merge.
// Driving the landing producers' real merge logic through a Shed run needs a genuine two-worktree pair;
// that requires git, which stays out of this package's untagged tier.
//
// Discussion-Write passes because the fake seat runner writes both discussion output files and reports Done.
// Discussion-Write's gate runs inside that fake's gated method, so it accepts the pair the fake just wrote;
// the decorator's injected commit closure then fires on the gate-passed Done.
//
// Plan-Write passes for the same reason.
// The fixture's fake shuttle rewrites the whole plan directory on its "plan"-role branch.
// Plan-Write's gate, run inside that same gated method, still finds a complete, zero-findings plan.
// The decorator's rotation had already archived the seeded plan away.
//
// It is derived from wantProducerTable's order and types.
// A Bouncer row expands to its four entries, and its review and fix rows follow it in the table.
// A review or fix row contributes only through its Bouncer.
// A Drift row is skipped since the kit's template answers drift.enabled false.
// A parts row is skipped since it answers tapestry.plan_split false.
// Merge-In is Stuck and ends the sequence, and every other row is Done.
var wantSequenceOrder = deriveSequenceOrder()

func deriveSequenceOrder() []wantSequenceEntry {
	var order []wantSequenceEntry
	for i, row := range wantProducerTable {
		switch {
		case row.producerType == bouncerType:
			review, fix := wantProducerTable[i+1], wantProducerTable[i+2]
			order = append(order,
				wantSequenceEntry{row.name, shedengine.Stuck},
				wantSequenceEntry{review.name, shedengine.Stuck},
				wantSequenceEntry{fix.name, shedengine.Stuck},
				wantSequenceEntry{row.name, shedengine.Done},
			)
		case row.producerType == burlerReviewType, row.producerType == burlerFixType:
		case row.name == loomshed.NameDriftBaseline, row.name == loomshed.NameDriftAwait, row.name == loomshed.NameDriftExpect, row.name == loomshed.NameDriftVerify, row.name == loomshed.NameDriftRecheck,
			row.name == loomshed.NamePartsWrite, row.name == loomshed.NamePartsPlanWrite:
			order = append(order, wantSequenceEntry{row.name, shedengine.OutcomeSkipped})
		case row.name == loomshed.NameParentAlign:
			return append(order, wantSequenceEntry{row.name, shedengine.Stuck})
		default:
			order = append(order, wantSequenceEntry{row.name, shedengine.Done})
		}
	}
	return order
}

// TestSequence_FullRunBlocksAtMergeIn runs the built row list from Preflight through Webster.
// It checks that the run blocks on Merge-In's Stuck verdict, never reaching the Code-Review perch.
// It also checks that the plan is left approved after the run.
// The fake writer seeds and rewrites the plan unapproved.
// A nil Env.ApprovePlan, a no-op closure, or a self-approving writer would let the history pass wrongly;
// the trailing planparser.ParsePlan check is what shows Plan-Bouncer's approve seam ran.
func TestSequence_FullRunBlocksAtMergeIn(t *testing.T) {
	tiergate.Require(t, 0)
	anchorPath, env, paths := buildSequenceFixture(t)

	var commitDiscussionCalls, commitPlanCalls, approveDiscussionCalls int
	env.ApproveDiscussion = func() error {
		approveDiscussionCalls++
		return nil
	}
	env.CommitDiscussion = func() error {
		commitDiscussionCalls++
		return nil
	}
	env.CommitPlan = func() error {
		commitPlanCalls++
		return nil
	}

	shed, err := New(shedrun.RecipeTapestry, env, paths)
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
	if result.HaltedProducer != loomshed.NameParentAlign {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NameParentAlign)
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

	// The fixture's open parent-review round, written by an older binary, does not hold Discussion-Write:
	// the row is Done straight to Discussion-Bouncer, and the round directory stays on disk unchanged.
	for i, entry := range result.History {
		if entry.Producer != loomshed.NameDiscussionWrite {
			continue
		}
		if entry.Outcome != shedengine.Done || i+1 >= len(result.History) || result.History[i+1].Producer != loomshed.NameDiscussionBouncer {
			t.Errorf("Discussion-Write history = %+v; want Done handing on to %s", result.History[i:], loomshed.NameDiscussionBouncer)
		}
	}
	roundRequest, err := os.ReadFile(filepath.Join(anchorPath, parentReviewRoundRel))
	if err != nil || string(roundRequest) != openParentReviewRequest {
		t.Errorf("open parent-review request = %q, %v; want it left on disk unchanged as %q", roundRequest, err, openParentReviewRequest)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Join(anchorPath, parentReviewRoundRel)), "verdict.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("parent-review verdict stat error = %v; want no verdict written", err)
	}

	got, found, err := state.ReadJSON[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after Run()")
	}
	if got.State != shedengine.StateBlocked {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StateBlocked)
	}
	if got.CurrentProducer != loomshed.NameParentAlign {
		t.Errorf("persisted CurrentProducer = %q; want %q -- current_producer must name the row the run blocked on", got.CurrentProducer, loomshed.NameParentAlign)
	}

	// Discussion-Write's own commit and Discussion-Bouncer's approval commit both invoke CommitDiscussion,
	// so a clean run calls it twice; a count of one means the Discussion-Bouncer commit seam never fired.
	if commitDiscussionCalls != 2 {
		t.Errorf("CommitDiscussion calls = %d; want exactly 2 after a clean run (Discussion-Write's commit plus Discussion-Bouncer's approval commit)", commitDiscussionCalls)
	}

	// The Discussion-Write row runs on the seat engine: one discussion table carries the row's gates,
	// and no single-agent discussion session runs.
	seats := env.Seats.(*shedfake.SeatRunner)
	if tables := countSeatRole(seats, "discussion"); tables != 1 {
		t.Fatalf("seat runner ran %d discussion tables; want exactly 1", tables)
	}
	var gateNames []string
	for _, entry := range seats.GotTables[0].Gate {
		gateNames = append(gateNames, entry.Name)
	}
	if want := []string{"discussion"}; !reflect.DeepEqual(gateNames, want) {
		t.Errorf("table gate = %v; want the row's gates %v", gateNames, want)
	}
	if spawns := countRole(env.Shuttle.(*shedfake.Shuttle), "discussion"); spawns != 0 {
		t.Errorf("loom shuttle ran %d discussion sessions; want none on the seat engine", spawns)
	}

	if approveDiscussionCalls != 1 {
		t.Errorf("ApproveDiscussion calls = %d; want exactly 1 after a clean run (Discussion-Bouncer's approved settle)", approveDiscussionCalls)
	}

	// Plan-Write's own commit and Plan-Bouncer's approval commit both invoke CommitPlan,
	// so a clean run calls it twice; a count of one means the Plan-Bouncer commit seam never fired.
	if commitPlanCalls != 2 {
		t.Errorf("CommitPlan calls = %d; want exactly 2 after a clean run (Plan-Write's commit plus Plan-Bouncer's approval commit)", commitPlanCalls)
	}

	// Both review segments before Merge-In must run, not be short-circuited:
	// the fake burler runs exactly one review and one fix per segment,
	// and the fake shuttle records exactly two bouncer-judge spawns;
	// their fixture-scripted CONVERGED verdicts advance the run past each segment.
	loomBurler := env.Burler.(*shedfake.BurlerHalfRunner)
	if loomBurler.ReviewCalls != 2 || loomBurler.FixCalls != 2 {
		t.Errorf("burler Review/Fix calls = %d/%d; want exactly 2/2 after a clean run", loomBurler.ReviewCalls, loomBurler.FixCalls)
	}
	if judgeCalls := countRole(env.Shuttle.(*shedfake.Shuttle), "bouncer-judge"); judgeCalls != 2 {
		t.Errorf("bouncer-judge spawns = %d; want exactly 2 after a clean run", judgeCalls)
	}

	// The parsed plan must be Approved.
	// The fake writer seeds and rewrites the plan unapproved on every "plan"-role Run,
	// so this passes only if Plan-Bouncer's approve_seam wrote the flag through env.ApprovePlan.
	// This assertion is the only check that proves the seam fired.
	plan, err := planparser.ParsePlan(planparser.PlanDir(env.AnchorPath))
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	if !plan.Approved {
		t.Errorf("ParsePlan().Approved = false; want true after a clean run")
	}
}
