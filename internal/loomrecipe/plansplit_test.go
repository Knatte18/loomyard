// plansplit_test.go covers the Parts-Write and Parts-Plan-Write rows of the tapestry recipe:
// their entry gates against tapestry.plan_split, their edges, and a flag-on run through them.

package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// planSplitSetting answers tapestry.plan_split as planSplit and every other setting from the templates.
func planSplitSetting(planSplit bool) func(module, key string) (any, error) {
	return func(module, key string) (any, error) {
		if module == "tapestry" && key == "plan_split" {
			return planSplit, nil
		}
		return envkit.TemplateSetting(module, key)
	}
}

// TestPlanSplitRows asserts, for tapestry.plan_split false and true, that both parts rows' entry gates hold exactly when the flag is true and Plan-Write's exactly when it is false.
// It also asserts the edges Drift-Baseline to Parts-Write to Parts-Plan-Write to Plan-Write to Plan-Bouncer, and that neither parts row has an on_stuck.
func TestPlanSplitRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, planSplit := range []bool{false, true} {
		env, paths := testEnv(t)
		env.Setting = planSplitSetting(planSplit)
		shed, err := New(shedrun.RecipeTapestry, env, paths)
		if err != nil {
			t.Fatalf("New() with plan_split %v error = %v; want nil", planSplit, err)
		}
		for _, row := range []struct {
			name      string
			wantHolds bool
		}{
			{loomshed.NamePartsWrite, planSplit},
			{loomshed.NamePartsPlanWrite, planSplit},
			{loomshed.NamePlanWrite, !planSplit},
		} {
			def := producerDef(t, shed, row.name)
			if def.EntryGate == nil {
				t.Fatalf("%s EntryGate = nil; want a gate on tapestry.plan_split", row.name)
			}
			if holds, _, err := def.EntryGate(); err != nil || holds != row.wantHolds {
				t.Errorf("%s EntryGate() with plan_split %v = %v, %v; want %v, nil", row.name, planSplit, holds, err, row.wantHolds)
			}
		}
	}

	env, paths := testEnv(t)
	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	for _, edge := range []struct{ from, to string }{
		{loomshed.NameDriftBaseline, loomshed.NamePartsWrite},
		{loomshed.NamePartsWrite, loomshed.NamePartsPlanWrite},
		{loomshed.NamePartsPlanWrite, loomshed.NamePlanWrite},
		{loomshed.NamePlanWrite, loomshed.NamePlanBouncer},
	} {
		if got := producerDef(t, shed, edge.from).OnDone; got != edge.to {
			t.Errorf("%s OnDone = %q; want %q", edge.from, got, edge.to)
		}
	}
	for _, name := range []string{loomshed.NamePartsWrite, loomshed.NamePartsPlanWrite} {
		if got := producerDef(t, shed, name).OnStuck; got != "" {
			t.Errorf("%s OnStuck = %q; want none", name, got)
		}
	}
}

// TestSequence_PlanSplitRunsThePartsRows drives a flag-on run over buildSequenceFixture.
// The history runs Parts-Write Done, Parts-Plan-Write Done and Plan-Write Skipped, then enters Plan-Bouncer.
// A goto onto Plan-Write then lets its producer run and record Done although its entry gate does not hold, since a goto bypasses only its target's gate.
func TestSequence_PlanSplitRunsThePartsRows(t *testing.T) {
	tiergate.Require(t, 0)
	anchorPath, env, paths := buildSequenceFixture(t)
	env.Setting = planSplitSetting(true)
	env.PartsTable = func() (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "parts",
			Segment:    segmentcolor.Plan,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-parts", Outputs: []string{partsparser.File(anchorPath)}}},
		}, nil
	}
	env.PartsPlanTable = func(shedrecipe.PartSeats) (seatengine.Table, error) {
		return seatengine.Table{
			RolePrefix: "parts-plan",
			Segment:    segmentcolor.Plan,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-parts-chair", Outputs: []string{partsparser.JoinRecord(anchorPath)}}},
		}, nil
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
	if result.Outcome != shedengine.RunBlocked || result.HaltedProducer != loomshed.NameParentAlign {
		t.Fatalf("Run() = %q halted on %q (reason: %s); want %q halted on %q", result.Outcome, result.HaltedProducer, result.Reason, shedengine.RunBlocked, loomshed.NameParentAlign)
	}

	start := -1
	for i, entry := range result.History {
		if entry.Producer == loomshed.NamePartsWrite {
			start = i
			break
		}
	}
	want := []wantSequenceEntry{
		{loomshed.NamePartsWrite, shedengine.Done},
		{loomshed.NamePartsPlanWrite, shedengine.Done},
		{loomshed.NamePlanWrite, shedengine.OutcomeSkipped},
		{loomshed.NamePlanBouncer, shedengine.Stuck},
	}
	if start < 0 || start+len(want) > len(result.History) {
		t.Fatalf("History = %+v; want it to run %s", result.History, loomshed.NamePartsWrite)
	}
	for i, wantEntry := range want {
		if entry := result.History[start+i]; entry.Producer != wantEntry.name || entry.Outcome != wantEntry.outcome {
			t.Errorf("History[%d] = (%s, %s); want (%s, %s)", start+i, entry.Producer, entry.Outcome, wantEntry.name, wantEntry.outcome)
		}
	}

	if _, err := shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     shed.StatusPath,
		LockPath:       shed.LockPath,
		StatusLockPath: shed.StatusLockPath,
		Producers:      shed.Producers,
		Target:         loomshed.NamePlanWrite,
	}); err != nil {
		t.Fatalf("Goto(%s) error = %v; want nil", loomshed.NamePlanWrite, err)
	}
	step, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step() error = %v; want nil", err)
	}
	if step.Producer != loomshed.NamePlanWrite || step.Outcome != shedengine.Done {
		t.Errorf("Step() called %q with outcome %q; want %q called with outcome %q although its entry gate does not hold", step.Producer, step.Outcome, loomshed.NamePlanWrite, shedengine.Done)
	}
}
