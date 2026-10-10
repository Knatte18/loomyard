package loomrecipe

import (
	"context"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// wantSequenceEntry pairs one expected history-row name with its expected shedengine.Outcome.
type wantSequenceEntry struct {
	name    string
	outcome shedengine.Outcome
}

// wantSequenceOrder is the row 1-through-Publish name/outcome sequence a clean Run over buildSequenceFixture must produce.
// It follows wantProducerTable's row order, which TestNew_ShapeMatchesRecipe pins against New,
// so a reordering in contracts/recipes/loom-recipe.yaml's row order is a test failure.
//
// Every entry but the three review segments and the trailing Publish carries a Done outcome by
// rule; the segments themselves do not, so their entries are spelled out explicitly rather than
// derived.
// Each segment contributes exactly three entries in the same shape: NameXBouncer with Stuck (the seed call, which spawns a focus-setting pass and always reports Stuck, never judging anything on its first call), NameXBurler with Stuck (one completed review round -- BurlerProducer reports every successful round as Stuck by contract, never Done, since its Stuck is a routine hand-off to the Bouncer rather than a real stuck condition), and NameXBouncer again with Done (the judge call, whose fixture-scripted CONVERGED verdict is what advances the run past the segment).
// With the standalone validate rows gone, the Plan-Review segment no longer carries
// the trailing post-segment mechanical re-check the other two segments never had a counterpart
// for either -- all three segments now share the identical three-entry shape.
// Stuck entries mid-run are therefore not a failure signal here; they are each segment doing its
// job.
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
// Row 3 (Discussion-Write) passes because the fixture's fake shuttle writes both discussion output
// files and reports Done, and Discussion-Write's own gate -- run inside that fake's gated method
// rather than as a separate row below it -- accepts the pair it just wrote; the decorator's
// injected commit closure then fires on the gate-passed Done.
//
// Row 6 (Plan-Write) passes for the same reason: the fixture's fake shuttle rewrites the whole plan
// directory on its "plan"-role branch, and Plan-Write's own gate, run inside that same gated
// method, still finds a complete, zero-findings plan after the decorator's rotation archived the
// seeded one away.
//
// It is derived from wantProducerTable's order and types: a Bouncer row expands to its three entries (the Burler row follows it in the table), a Burler row contributes only through its Bouncer, Publish is Stuck and ends the sequence, and every other row is Done.
var wantSequenceOrder = deriveSequenceOrder()

func deriveSequenceOrder() []wantSequenceEntry {
	var order []wantSequenceEntry
	for i, row := range wantProducerTable {
		switch {
		case row.producerType == bouncerType:
			burler := wantProducerTable[i+1]
			order = append(order,
				wantSequenceEntry{row.name, shedengine.Stuck},
				wantSequenceEntry{burler.name, shedengine.Stuck},
				wantSequenceEntry{row.name, shedengine.Done},
			)
		case row.producerType == burlerType:
		case row.name == loomshed.NamePublish:
			return append(order, wantSequenceEntry{row.name, shedengine.Stuck})
		default:
			order = append(order, wantSequenceEntry{row.name, shedengine.Done})
		}
	}
	return order
}

// TestSequence_FullRunBlocksAtPublish is the task's own verify requirement: the built row list
// runs Preflight through Publish and blocks on Publish's Stuck verdict, never reaching Finalize --
// see wantSequenceOrder's own doc comment for why, including for all three review segments' entry
// shapes. It also asserts the plan is left approved after the run: under the pre-fix code the fake
// writer self-approved the plan it wrote, which masked Plan-Bouncer's approve_seam ever firing at
// all -- a nil Env.ApprovePlan, a no-op closure, or a fake writer that started self-approving again
// would each leave this history list passing for the wrong reason, and with the row-removal batch's
// deletion of the post-segment mechanical re-check that used to confirm the approval flag survived,
// the trailing planparser.ParsePlan check below is the ONLY standing guard left anywhere that
// Plan-Bouncer's approve seam genuinely ran.
func TestSequence_FullRunBlocksAtPublish(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)

	var commitDiscussionCalls, commitPlanCalls int
	env.CommitDiscussion = func() error {
		commitDiscussionCalls++
		return nil
	}
	env.CommitPlan = func() error {
		commitPlanCalls++
		return nil
	}

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

	// This is the scenario proof that the Discussion-Bouncer commit_seam is genuinely reached through
	// a real Shed run: both the Discussion-Write row's own commit and the Discussion-Bouncer row's
	// approval commit invoke CommitDiscussion, so the count is 2, not 1.
	if commitDiscussionCalls != 2 {
		t.Errorf("CommitDiscussion calls = %d; want exactly 2 after a clean run (Discussion-Write's commit plus Discussion-Bouncer's approval commit)", commitDiscussionCalls)
	}

	// This is the scenario proof that the Plan-Bouncer commit_seam is genuinely reached through a
	// real Shed run: both the Plan-Write row's own commit and the Plan-Bouncer row's approval commit
	// invoke CommitPlan, so the count is 2, not 1.
	if commitPlanCalls != 2 {
		t.Errorf("CommitPlan calls = %d; want exactly 2 after a clean run (Plan-Write's commit plus Plan-Bouncer's approval commit)", commitPlanCalls)
	}

	// The scenario checks that all three review segments genuinely ran rather than being silently short-circuited:
	// the fake burler ran exactly three rounds (one per segment),
	// and the fake shuttle recorded exactly three bouncer-judge spawns -- the judge calls whose fixture-scripted CONVERGED verdicts are what advanced the run past each segment.
	loomBurler := env.Burler.(*shedfake.BurlerRunner)
	if loomBurler.Calls != 3 {
		t.Errorf("burler Calls = %d; want exactly 3 after a clean run", loomBurler.Calls)
	}
	if judgeCalls := countRole(env.Shuttle.(*shedfake.Shuttle), "bouncer-judge"); judgeCalls != 3 {
		t.Errorf("bouncer-judge spawns = %d; want exactly 3 after a clean run", judgeCalls)
	}

	// This is the regression proof for F7: parse the fixture's own plan and assert Approved is
	// true. The fake writer seeds and rewrites the plan unapproved on every "plan"-role Run, so this
	// can only pass because Plan-Bouncer's approve_seam wrote the flag through env.ApprovePlan --
	// and, with the removed Plan-Revalidate row's own re-check gone, this assertion is the only
	// thing anywhere that still proves the seam fired at all.
	plan, err := planparser.ParsePlan(planparser.PlanDir(env.AnchorPath))
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	if !plan.Approved {
		t.Errorf("ParsePlan().Approved = false; want true after a clean run")
	}
}

// TestNew_DiscussionSeatsRunsTheWriterRowOnSeats runs the whole sequence with the Discussion-Write row on the seat engine and asserts the row still ends Done, runs its own gates on the chair and spawns no single-agent discussion session.
// With the choice off it asserts only that the seat runner saw no table; the default build's shape and its discussion shuttle role are pinned by the sequence tests above.
func TestNew_DiscussionSeatsRunsTheWriterRowOnSeats(t *testing.T) {
	tests := []struct {
		name  string
		seats bool
	}{
		{"SeatsChosen", true},
		{"SingleChosen", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, env, paths := buildSequenceFixture(t)

			commitDiscussionCalls := 0
			env.CommitDiscussion = func() error {
				commitDiscussionCalls++
				return nil
			}
			seatFake := newLoomSeats(true)
			env.Seats = seatFake
			env.DiscussionSeats = tt.seats
			env.DiscussionTable = func() (seatengine.Table, error) {
				return seatengine.Table{
					RolePrefix: "discussion",
					Segment:    segmentcolor.Discussion,
					Seats:      []seatengine.Seat{{Name: seatengine.RoleChair, Stencil: "loom-template-discussion-chair", Outputs: []string{env.DecisionRecordPath, env.SupportLogPath}}},
				}, nil
			}

			shed, err := New(env, paths)
			if err != nil {
				t.Fatalf("New() error = %v; want nil", err)
			}
			shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
			result, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}

			if !tt.seats {
				if len(seatFake.GotTables) != 0 {
					t.Errorf("seat runner saw %d tables; want none while the single-agent producer is chosen", len(seatFake.GotTables))
				}
				return
			}

			var writeOutcome shedengine.Outcome
			for _, entry := range result.History {
				if entry.Producer == loomshed.NameDiscussionWrite {
					writeOutcome = entry.Outcome
					break
				}
			}
			if writeOutcome != shedengine.Done {
				t.Fatalf("Discussion-Write history outcome = %q; want %q (History: %+v)", writeOutcome, shedengine.Done, result.History)
			}
			if len(seatFake.GotTables) != 1 {
				t.Fatalf("seat runner saw %d tables; want exactly 1", len(seatFake.GotTables))
			}
			var gateNames []string
			for _, entry := range seatFake.GotTables[0].Gate {
				gateNames = append(gateNames, entry.Name)
			}
			if want := []string{"discussion", "parent-review"}; !reflect.DeepEqual(gateNames, want) {
				t.Errorf("table gate = %v; want the row's gates %v", gateNames, want)
			}
			if spawns := countRole(env.Shuttle.(*shedfake.Shuttle), "discussion"); spawns != 0 {
				t.Errorf("loom shuttle ran %d discussion sessions; want none on the seat engine", spawns)
			}
			if commitDiscussionCalls != 2 {
				t.Errorf("CommitDiscussion calls = %d; want 2 (Discussion-Write's commit plus Discussion-Bouncer's approval commit)", commitDiscussionCalls)
			}
		})
	}
}
