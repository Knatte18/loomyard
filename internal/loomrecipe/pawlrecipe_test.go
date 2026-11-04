// pawlrecipe_test.go covers the pawl recipe as a loom recipe: its rows, their edges and producer types through New and Routing, the absence of a review segment, PR-Review's one reroute edge back to Beam-Await, and the hold boundary the recipe's rows pin against boardengine.RunHolds.

package loomrecipe

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantPawlRow is one row of the pawl recipe: its name, its engine's producer type and its edges.
type wantPawlRow struct {
	name         string
	producerType reflect.Type
	onDone       string
	onReroute    string
}

func wantPawlRows(t *testing.T) []wantPawlRow {
	t.Helper()
	env, _ := testEnv(t)
	describe, err := landingshed.NewBeamDescribe("Beam-Describe", env.BeamDescribe)
	if err != nil {
		t.Fatalf("landingshed.NewBeamDescribe() error = %v; want nil", err)
	}
	return []wantPawlRow{
		{loomshed.NamePreflight, reflect.TypeOf(preflightshed.NewPreflight("", "")), loomshed.NameRecordWrite, ""},
		{loomshed.NameRecordWrite, reflect.TypeOf(loomshed.NewRecordWrite("", loomshed.RecordWriteDeps{})), pawlengine.NameBeamUp, ""},
		{pawlengine.NameBeamUp, reflect.TypeOf(&pawlengine.BeamUp{}), pawlengine.NameBeamAwait, ""},
		{pawlengine.NameBeamAwait, reflect.TypeOf(&pawlengine.BeamAwait{}), loomshed.NameParentAlign, ""},
		{loomshed.NameParentAlign, reflect.TypeOf(&landingshed.ParentAlign{}), loomshed.NameBeamDescribe, ""},
		{loomshed.NameBeamDescribe, reflect.TypeOf(describe), loomshed.NamePRReview, ""},
		{loomshed.NamePRReview, reflect.TypeOf(&landingshed.PRReview{}), loomshed.NameFinalize, pawlengine.NameBeamAwait},
		{loomshed.NameFinalize, reflect.TypeOf(&landingshed.Finalize{}), pawlengine.NameBeamDown, ""},
		{pawlengine.NameBeamDown, reflect.TypeOf(&pawlengine.BeamDown{}), "", ""},
	}
}

func TestPawlRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("NewBuildsTheRowsAndTheirEdges", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		delete(env.Segments, codeReviewSegment)
		shed, err := New(shedrun.RecipePawl, env, paths)
		if err != nil {
			t.Fatalf("New(pawl) error = %v; want nil, with no review segment to budget", err)
		}

		want := wantPawlRows(t)
		if len(shed.Producers) != len(want) {
			t.Fatalf("New(pawl) built %d rows; want %d", len(shed.Producers), len(want))
		}
		for i, w := range want {
			got := shed.Producers[i]
			if got.Name != w.name {
				t.Errorf("row %d name = %q; want %q", i, got.Name, w.name)
			}
			if got.OnDone != w.onDone || got.OnStuck != "" || got.OnReroute != w.onReroute {
				t.Errorf("row %q OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/empty/%q", w.name, got.OnDone, got.OnStuck, got.OnReroute, w.onDone, w.onReroute)
			}
			if got.Segment != "" || got.MaxBounces != 0 {
				t.Errorf("row %q Segment/MaxBounces = %q/%d; want no review segment", w.name, got.Segment, got.MaxBounces)
			}
			if reflect.TypeOf(got.Producer) != w.producerType {
				t.Errorf("row %q producer type = %v; want %v", w.name, reflect.TypeOf(got.Producer), w.producerType)
			}
		}
	})

	t.Run("RoutingProjectsTheSameRows", func(t *testing.T) {
		t.Parallel()
		routing, err := Routing(shedrun.RecipePawl, map[string]int{})
		if err != nil {
			t.Fatalf("Routing(pawl) error = %v; want nil", err)
		}
		if routing.Entry != loomshed.NamePreflight {
			t.Errorf("Routing(pawl).Entry = %q; want %q", routing.Entry, loomshed.NamePreflight)
		}
		want := wantPawlRows(t)
		if len(routing.Producers) != len(want) {
			t.Fatalf("Routing(pawl) has %d rows; want %d", len(routing.Producers), len(want))
		}
		for i, w := range want {
			if got := routing.Producers[i]; got.Name != w.name || got.OnDone != w.onDone || got.OnReroute != w.onReroute {
				t.Errorf("Routing(pawl) row %d = %q -> %q, reroute %q; want %q -> %q, reroute %q", i, got.Name, got.OnDone, got.OnReroute, w.name, w.onDone, w.onReroute)
			}
		}
	})
}

// TestPawlRecipe_HoldLiftsOnlyBeforeParentAlign pins boardengine.RunHolds, which imports no recipe package, against the recipe's rows:
// a beam project's run holds nothing of its entry at batten's claim row and at every pawl row before Parent-Align, and holds it at Parent-Align and every row after it.
func TestPawlRecipe_HoldLiftsOnlyBeforeParentAlign(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	names, err := RowNames(shedrun.RecipePawl)
	if err != nil {
		t.Fatalf("RowNames(pawl) error = %v; want nil", err)
	}

	held := func(producer string) bool {
		status := boardengine.RunStatus("running", producer)
		return boardengine.RunHolds(boardengine.Task{Slug: "beam", Kind: boardengine.KindProject, Beam: true, Status: &status})
	}

	if held(battenrecipe.NameWorktreeCreate) {
		t.Errorf("RunHolds at %s = true; want false, the claim row holds nothing", battenrecipe.NameWorktreeCreate)
	}
	reachedParentAlign := false
	for _, name := range names {
		if name == loomshed.NameParentAlign {
			reachedParentAlign = true
		}
		if got := held(name); got != reachedParentAlign {
			t.Errorf("RunHolds at %s = %v; want %v", name, got, reachedParentAlign)
		}
	}
	if !reachedParentAlign {
		t.Fatalf("the pawl recipe's rows %v name no %s row", names, loomshed.NameParentAlign)
	}
}

// TestPawlRecipe_BodyLocksFromRecordWrite pins boardengine's body lock of a beam project, which imports no recipe package, against the recipe's rows:
// the project's body takes an edit at batten's claim row and at Preflight, before Record-Write writes the project record from it, and none from Record-Write on.
func TestPawlRecipe_BodyLocksFromRecordWrite(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	names, err := RowNames(shedrun.RecipePawl)
	if err != nil {
		t.Fatalf("RowNames(pawl) error = %v; want nil", err)
	}

	locked := func(producer string) bool {
		status := boardengine.RunStatus("running", producer)
		return boardengine.ProjectRecordHeld(boardengine.Task{Slug: "beam", Kind: boardengine.KindProject, Beam: true, Status: &status})
	}

	if locked(battenrecipe.NameWorktreeCreate) {
		t.Errorf("ProjectRecordHeld at %s = true; want false, the claim row precedes the record", battenrecipe.NameWorktreeCreate)
	}
	reachedRecordWrite := false
	for _, name := range names {
		if name == loomshed.NameRecordWrite {
			reachedRecordWrite = true
		}
		if got := locked(name); got != reachedRecordWrite {
			t.Errorf("ProjectRecordHeld at %s = %v; want %v", name, got, reachedRecordWrite)
		}
	}
	if !reachedRecordWrite {
		t.Fatalf("the pawl recipe's rows %v name no %s row", names, loomshed.NameRecordWrite)
	}
}
