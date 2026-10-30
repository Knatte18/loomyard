//go:build tier0

// pawlrecipe_test.go covers NewPawl and PawlRouting: the pawl recipe's rows, their edges, with Beam-Describe handing to PR-Review, and producer types, the Code-Review perch's budget, the absence of a reroute edge on Finalize, the refusal of an unfilled seam, and the hold boundary the recipe's rows pin against boardengine.RunHolds.

package loomrecipe

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// wantPawlRow is one row of the pawl recipe: its name, its engine's producer type and its edges.
type wantPawlRow struct {
	name         string
	producerType reflect.Type
	onDone       string
	onStuck      string
}

func wantPawlRows(t *testing.T) []wantPawlRow {
	t.Helper()
	env, _ := testEnv(t)
	describe, err := landingshed.NewBeamDescribe("Beam-Describe", env.BeamDescribe)
	if err != nil {
		t.Fatalf("landingshed.NewBeamDescribe() error = %v; want nil", err)
	}
	return []wantPawlRow{
		{loomshed.NamePreflight, reflect.TypeOf(preflightshed.NewPreflight("", "")), pawlengine.NameBeamUp, ""},
		{pawlengine.NameBeamUp, reflect.TypeOf(&pawlengine.BeamUp{}), pawlengine.NameBeamAwait, ""},
		{pawlengine.NameBeamAwait, reflect.TypeOf(&pawlengine.BeamAwait{}), loomshed.NameParentAlign, ""},
		{loomshed.NameParentAlign, reflect.TypeOf(&landingshed.ParentAlign{}), loomshed.NameCodeBouncer, ""},
		{loomshed.NameCodeBouncer, bouncerType, "Beam-Describe", loomshed.NameCodeBurlerReview},
		{loomshed.NameCodeBurlerReview, burlerReviewType, loomshed.NameCodeBurlerFix, loomshed.NameCodeBurlerFix},
		{loomshed.NameCodeBurlerFix, burlerFixType, loomshed.NameCodeBouncer, loomshed.NameCodeBouncer},
		{"Beam-Describe", reflect.TypeOf(describe), loomshed.NamePRReview, ""},
		{loomshed.NamePRReview, reflect.TypeOf(&landingshed.PRReview{}), loomshed.NameFinalize, ""},
		{loomshed.NameFinalize, reflect.TypeOf(&landingshed.Finalize{}), pawlengine.NameBeamDown, ""},
		{pawlengine.NameBeamDown, reflect.TypeOf(&pawlengine.BeamDown{}), "", ""},
	}
}

func TestNewPawl(t *testing.T) {
	t.Parallel()

	t.Run("BuildsTheRowsAndTheirEdges", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		setCodeBudget(&env, 3)
		shed, err := NewPawl(env, paths)
		if err != nil {
			t.Fatalf("NewPawl() error = %v; want nil", err)
		}

		want := wantPawlRows(t)
		if len(shed.Producers) != len(want) {
			t.Fatalf("NewPawl() built %d rows; want %d", len(shed.Producers), len(want))
		}
		for i, w := range want {
			got := shed.Producers[i]
			if got.Name != w.name {
				t.Errorf("row %d name = %q; want %q", i, got.Name, w.name)
			}
			if got.OnDone != w.onDone || got.OnStuck != w.onStuck {
				t.Errorf("row %q OnDone/OnStuck = %q/%q; want %q/%q", w.name, got.OnDone, got.OnStuck, w.onDone, w.onStuck)
			}
			if got.OnReroute != "" {
				t.Errorf("row %q OnReroute = %q; want none", w.name, got.OnReroute)
			}
			if reflect.TypeOf(got.Producer) != w.producerType {
				t.Errorf("row %q producer type = %v; want %v", w.name, reflect.TypeOf(got.Producer), w.producerType)
			}
			wantBudget := 0
			if got.Segment == codeReviewSegment {
				wantBudget = 3
			}
			if got.MaxBounces != wantBudget {
				t.Errorf("row %q MaxBounces = %d; want %d", w.name, got.MaxBounces, wantBudget)
			}
		}
	})

	t.Run("ABudgetOfZeroSettlesCodeBouncerAtOnce", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		setCodeBudget(&env, 0)
		shed, err := NewPawl(env, paths)
		if err != nil {
			t.Fatalf("NewPawl() error = %v; want nil", err)
		}
		for _, def := range shed.Producers {
			if def.Name != loomshed.NameCodeBouncer {
				continue
			}
			if def.OnDone != "Beam-Describe" || def.OnStuck != loomshed.NameCodeBurlerReview {
				t.Errorf("Code-Bouncer OnDone/OnStuck = %q/%q; want every edge kept at a budget of 0", def.OnDone, def.OnStuck)
			}
		}
		outcome, _, err := builtProducer(t, shed, loomshed.NameCodeBouncer).Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Errorf("Code-Bouncer Call() at a budget of 0 = %q, %v; want Done, nil", outcome, err)
		}
		if shuttle := env.Shuttle.(*shedfake.Shuttle); shuttle.Called {
			t.Errorf("shuttle started %d spec(s); want none from a zero-round Bouncer", len(shuttle.Specs))
		}
	})

	t.Run("RefusesAMissingCodeReviewBudget", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		delete(env.Segments, codeReviewSegment)
		shed, err := NewPawl(env, paths)
		if err == nil || shed != nil {
			t.Fatalf("NewPawl() = %v, %v; want nil and an error", shed, err)
		}
		for _, want := range []string{codeReviewSegment, loomSegmentTableWayForward} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("NewPawl() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("RefusesAMissingBeamDescribeSeam", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		env.BeamDescribe.Entry = nil
		shed, err := NewPawl(env, paths)
		if err == nil || !strings.Contains(err.Error(), "BeamDescribe") {
			t.Fatalf("NewPawl() error = %v; want one naming BeamDescribe", err)
		}
		if shed != nil {
			t.Errorf("NewPawl() shed = %+v; want nil alongside a non-nil error", shed)
		}
	})

	t.Run("RefusesDivergentStatusPaths", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		paths.StatusPath += ".other"
		if _, err := NewPawl(env, paths); err == nil || !strings.Contains(err.Error(), paths.StatusPath) {
			t.Errorf("NewPawl() error = %v; want one naming the divergent StatusPath", err)
		}
	})
}

func TestPawlRouting(t *testing.T) {
	t.Parallel()

	t.Run("AppliesTheBudgetToThePerchAndKeepsTheEdges", func(t *testing.T) {
		t.Parallel()
		routing, err := PawlRouting(map[string]int{codeReviewSegment: 2})
		if err != nil {
			t.Fatalf("PawlRouting() error = %v; want nil", err)
		}
		if routing.Entry != loomshed.NamePreflight {
			t.Errorf("PawlRouting().Entry = %q; want %q", routing.Entry, loomshed.NamePreflight)
		}
		want := wantPawlRows(t)
		if len(routing.Producers) != len(want) {
			t.Fatalf("PawlRouting() has %d rows; want %d", len(routing.Producers), len(want))
		}
		for i, w := range want {
			got := routing.Producers[i]
			if got.Name != w.name || got.OnDone != w.onDone || got.OnStuck != w.onStuck || got.OnReroute != "" {
				t.Errorf("PawlRouting() row %d = %q %q/%q/%q; want %q %q/%q/empty", i, got.Name, got.OnDone, got.OnStuck, got.OnReroute, w.name, w.onDone, w.onStuck)
			}
			wantBudget := 0
			if got.Segment == codeReviewSegment {
				wantBudget = 2
			}
			if got.MaxBounces != wantBudget {
				t.Errorf("PawlRouting() row %q MaxBounces = %d; want %d", w.name, got.MaxBounces, wantBudget)
			}
		}
	})

	t.Run("RefusesAMissingBudget", func(t *testing.T) {
		t.Parallel()
		if _, err := PawlRouting(map[string]int{}); err == nil || !strings.Contains(err.Error(), codeReviewSegment) {
			t.Errorf("PawlRouting() error = %v; want one naming %s", err, codeReviewSegment)
		}
	})
}

// TestPawlRecipe_HoldLiftsOnlyBeforeParentAlign pins boardengine.RunHolds, which imports no recipe package, against the recipe's rows:
// a beam project's run holds nothing of its entry at batten's claim row and at every pawl row before Parent-Align, and holds it at Parent-Align and every row after it.
func TestPawlRecipe_HoldLiftsOnlyBeforeParentAlign(t *testing.T) {
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
