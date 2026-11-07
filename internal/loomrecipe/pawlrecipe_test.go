// pawlrecipe_test.go covers the pawl recipe as a loom recipe.
// It pins the recipe's rows and their edges and producer types through New and Routing.
// It also pins the absence of a review segment and of a reroute edge.
// It pins the retired rows and the hold boundary against boardengine.RunHolds.

package loomrecipe

import (
	"reflect"
	"slices"
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

// wantPawlRow is one row of the pawl recipe: its name, its engine's producer type and the row its Done routes to.
type wantPawlRow struct {
	name         string
	producerType reflect.Type
	onDone       string
}

func wantPawlRows() []wantPawlRow {
	return []wantPawlRow{
		{loomshed.NamePreflight, reflect.TypeOf(preflightshed.NewPreflight("", "")), loomshed.NameRecordWrite},
		{loomshed.NameRecordWrite, reflect.TypeOf(loomshed.NewRecordWrite("", loomshed.RecordWriteDeps{})), pawlengine.NameBeamUp},
		{pawlengine.NameBeamUp, reflect.TypeOf(&pawlengine.BeamUp{}), pawlengine.NameBeamAwait},
		{pawlengine.NameBeamAwait, reflect.TypeOf(&pawlengine.BeamAwait{}), loomshed.NameFinalize},
		{loomshed.NameFinalize, reflect.TypeOf(&landingshed.Finalize{}), pawlengine.NameBeamDown},
		{pawlengine.NameBeamDown, reflect.TypeOf(&pawlengine.BeamDown{}), ""},
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

		want := wantPawlRows()
		if len(shed.Producers) != len(want) {
			t.Fatalf("New(pawl) built %d rows; want %d", len(shed.Producers), len(want))
		}
		for i, w := range want {
			got := shed.Producers[i]
			if got.Name != w.name {
				t.Errorf("row %d name = %q; want %q", i, got.Name, w.name)
			}
			if got.OnDone != w.onDone || got.OnStuck != "" || got.OnReroute != "" {
				t.Errorf("row %q OnDone/OnStuck/OnReroute = %q/%q/%q; want %q/empty/empty", w.name, got.OnDone, got.OnStuck, got.OnReroute, w.onDone)
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
		want := wantPawlRows()
		if len(routing.Producers) != len(want) {
			t.Fatalf("Routing(pawl) has %d rows; want %d", len(routing.Producers), len(want))
		}
		for i, w := range want {
			if got := routing.Producers[i]; got.Name != w.name || got.OnDone != w.onDone || got.OnReroute != "" {
				t.Errorf("Routing(pawl) row %d = %q -> %q, reroute %q; want %q -> %q, no reroute", i, got.Name, got.OnDone, got.OnReroute, w.name, w.onDone)
			}
		}
	})
}

// TestPawlRecipe_HoldLiftsOnlyBeforeFinalize pins boardengine.RunHolds against the recipe's rows.
// boardengine imports no recipe package.
// A beam project's run holds nothing of its entry at batten's claim row and at every pawl row before Finalize.
// It holds it at Finalize and every row after it.
// It also pins the recipe's retired rows, which a parked run may still stand at, as held.
func TestPawlRecipe_HoldLiftsOnlyBeforeFinalize(t *testing.T) {
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
	reachedFinalize := false
	for _, name := range names {
		if name == loomshed.NameFinalize {
			reachedFinalize = true
		}
		if got := held(name); got != reachedFinalize {
			t.Errorf("RunHolds at %s = %v; want %v", name, got, reachedFinalize)
		}
	}
	if !reachedFinalize {
		t.Fatalf("the pawl recipe's rows %v name no %s row", names, loomshed.NameFinalize)
	}

	retired, err := RetiredRowNames(shedrun.RecipePawl)
	if err != nil {
		t.Fatalf("RetiredRowNames(pawl) error = %v; want nil", err)
	}
	if want := []string{"Beam-Describe", loomshed.NamePRReview, loomshed.NameParentAlign}; !slices.Equal(retired, want) {
		t.Errorf("RetiredRowNames(pawl) = %v; want %v, sorted", retired, want)
	}
	for _, name := range retired {
		if !held(name) {
			t.Errorf("RunHolds at the retired row %s = false; want true, a parked run there holds its beam", name)
		}
	}
}

// TestPawlRecipe_BodyLocksFromRecordWrite pins boardengine's body lock of a beam project against the recipe's rows.
// boardengine imports no recipe package.
// The project's body takes an edit at batten's claim row and at Preflight.
// Record-Write writes the project record from the body, and the body takes no edit from Record-Write on.
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
