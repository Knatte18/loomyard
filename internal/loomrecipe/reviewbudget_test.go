// reviewbudget_test.go asserts the review bounce budget comes from the caller, not the recipe:
// New and Routing set it on every row of a Bouncer-holding segment and leave PR-Gate alone.

package loomrecipe

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// loomReviewSegments lists the loom recipe's review segments, the keys of the per-segment budget map.
var loomReviewSegments = []string{"Discussion-Review", "Plan-Review", "Webster-Review", "Merge-Review"}

// setBudgets sets every env.Segments entry's MaxBounces to runWide, then each segment of own to its own value.
func setBudgets(env *shedrecipe.Env, runWide int, own map[string]int) {
	for segment, settings := range env.Segments {
		settings.MaxBounces = runWide
		if budget, ok := own[segment]; ok {
			settings.MaxBounces = budget
		}
		env.Segments[segment] = settings
	}
}

// budgetsOf returns env.Segments' MaxBounces keyed by segment, the map Routing takes.
func budgetsOf(env shedrecipe.Env) map[string]int {
	budgets := map[string]int{}
	for segment, settings := range env.Segments {
		budgets[segment] = settings.MaxBounces
	}

	return budgets
}

// assertReviewBudget fails unless every review-segment row carries its segment's entry in budgets, 0 included, and PR-Gate carries prGate.
func assertReviewBudget(t *testing.T, producers []shedengine.ProducerDef, budgets map[string]int, prGate int) {
	t.Helper()

	reviewRows := 0
	for _, def := range producers {
		isReview := slices.Contains(loomReviewSegments, def.Segment)
		switch {
		case isReview:
			reviewRows++
			want := budgets[def.Segment]
			if def.MaxBounces != want {
				t.Errorf("row %q (segment %q) MaxBounces = %d; want %d", def.Name, def.Segment, def.MaxBounces, want)
			}
		case def.Name == "PR-Gate":
			if def.MaxBounces != prGate {
				t.Errorf("PR-Gate MaxBounces = %d; want the recipe's %d", def.MaxBounces, prGate)
			}
		}
	}
	if want := 3 * len(loomReviewSegments); reviewRows != want {
		t.Errorf("review-segment rows = %d; want %d, a Bouncer, a review row and a fix row per segment", reviewRows, want)
	}
}

// TestReviewBudget_AppliedByNewAndRouting asserts both entry points stamp the budget on the review rows:
// the same budget on every segment, and a segment's own budget on its three rows alone.
func TestReviewBudget_AppliedByNewAndRouting(t *testing.T) {
	const budget = 7

	tests := []struct {
		name string
		own  map[string]int
	}{
		{"one budget on every segment", nil},
		{"a segment's budget leaves the others", map[string]int{"Discussion-Review": 1}},
		{"a zero entry is zero rounds", map[string]int{"Plan-Review": 0}},
		{"every segment's own budget", map[string]int{"Discussion-Review": 1, "Plan-Review": 2, "Webster-Review": 9, "Merge-Review": 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, paths := testEnv(t)
			setBudgets(&env, budget, tt.own)
			budgets := budgetsOf(env)

			routing, err := Routing(budgets)
			if err != nil {
				t.Fatalf("Routing(%v) = _, %v; want nil", budgets, err)
			}
			var prGate int
			for _, def := range routing.Producers {
				if def.Name == "PR-Gate" {
					prGate = def.MaxBounces
				}
			}
			if prGate == 0 {
				t.Fatal("PR-Gate MaxBounces = 0 in Routing; want the recipe's literal budget")
			}
			assertReviewBudget(t, routing.Producers, budgets, prGate)

			shed, err := New(env, paths)
			if err != nil {
				t.Fatalf("New() = _, %v; want nil", err)
			}
			assertReviewBudget(t, shed.Producers, budgets, prGate)
		})
	}
}

// TestZeroBudget_BouncerSettlesAtOnce asserts a Bouncer whose segment budget is 0 settles Done with no shuttle start.
// That proves loomrecipe stamped zero_rounds on its row; the settle's seam order is TestBouncerEntry_SkipSeam's to pin.
// It also asserts New and Routing keep each perch's route: the writers and Merge-In still hand to their Bouncer and every Merge-Review row stays in the list.
func TestZeroBudget_BouncerSettlesAtOnce(t *testing.T) {
	tests := []struct {
		name    string
		runWide int
		own     map[string]int
		zeroed  []string
	}{
		{"Discussion-Review entry", 3, map[string]int{"Discussion-Review": 0}, []string{"Discussion-Bouncer"}},
		{"Plan-Review entry", 3, map[string]int{"Plan-Review": 0}, []string{"Plan-Bouncer"}},
		{"Webster-Review entry", 3, map[string]int{"Webster-Review": 0}, []string{"Webster-Bouncer"}},
		{"Merge-Review entry", 3, map[string]int{"Merge-Review": 0}, []string{"Merge-Bouncer"}},
		{"every segment at 0", 0, nil, []string{"Discussion-Bouncer", "Plan-Bouncer", "Webster-Bouncer", "Merge-Bouncer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, paths := testEnv(t)
			setBudgets(&env, tt.runWide, tt.own)
			routing, err := Routing(budgetsOf(env))
			if err != nil {
				t.Fatalf("Routing() = _, %v; want nil", err)
			}
			shed, err := New(env, paths)
			if err != nil {
				t.Fatalf("New() = _, %v; want nil", err)
			}

			for name, producers := range map[string][]shedengine.ProducerDef{"Routing": routing.Producers, "New": shed.Producers} {
				rows := map[string]shedengine.ProducerDef{}
				for _, def := range producers {
					rows[def.Name] = def
				}
				for write, bouncer := range map[string]string{
					loomshed.NameDiscussionWrite: loomshed.NameDiscussionBouncer,
					loomshed.NamePlanWrite:       loomshed.NamePlanBouncer,
					loomshed.NameWebster:         loomshed.NameWebsterBouncer,
					loomshed.NameMergeIn:         loomshed.NameMergeBouncer,
				} {
					if got := rows[write].OnDone; got != bouncer {
						t.Errorf("%s: %s OnDone = %q; want %q", name, write, got, bouncer)
					}
				}
				for _, row := range []string{loomshed.NameMergeBouncer, loomshed.NameMergeBurlerReview, loomshed.NameMergeBurlerFix} {
					if _, present := rows[row]; !present {
						t.Errorf("%s: no %s row; want the Merge-Review rows kept in the list", name, row)
					}
				}
			}

			shuttle := env.Shuttle.(*shedfake.Shuttle)
			for _, bouncer := range tt.zeroed {
				outcome, _, err := builtProducer(t, shed, bouncer).Call(context.Background())
				if err != nil || outcome != shedengine.Done {
					t.Errorf("%s Call() = %q, %v; want Done, nil", bouncer, outcome, err)
				}
			}
			if shuttle.Called {
				t.Errorf("shuttle started %d spec(s); want none from a zero-round Bouncer", len(shuttle.Specs))
			}
		})
	}
}

// builtProducer returns the producer of the named row of shed.
func builtProducer(t *testing.T, shed *shedengine.Shed, name string) shedengine.ShedProducer {
	t.Helper()

	for _, def := range shed.Producers {
		if def.Name == name {
			return def.Producer
		}
	}
	t.Fatalf("shed has no row %q", name)
	return nil
}

// TestReviewBudget_NegativeRefused asserts New and Routing refuse a negative segment budget and a review segment with no budget, each naming the segment and its way forward, and accept a budget of 0.
func TestReviewBudget_NegativeRefused(t *testing.T) {
	zeroes := map[string]int{"Discussion-Review": 0, "Plan-Review": 0, "Webster-Review": 0, "Merge-Review": 0}
	if _, err := Routing(zeroes); err != nil {
		t.Errorf("Routing(%v) error = %v; want nil", zeroes, err)
	}

	routingTests := []struct {
		name    string
		budgets map[string]int
		want    []string
	}{
		{"a negative entry", map[string]int{"Discussion-Review": 1, "Plan-Review": 1, "Webster-Review": -1, "Merge-Review": 1}, []string{"Webster-Review", "-1"}},
		{"a segment with no entry", map[string]int{"Discussion-Review": 1, "Plan-Review": 1, "Webster-Review": 1}, []string{"Merge-Review", "segment-to-perch table"}},
	}
	for _, tt := range routingTests {
		t.Run("Routing "+tt.name, func(t *testing.T) {
			_, err := Routing(tt.budgets)
			if err == nil {
				t.Fatalf("Routing(%v) error = nil; want one naming %v", tt.budgets, tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Routing(%v) error = %q; want it to contain %q", tt.budgets, err, want)
				}
			}
		})
	}

	newTests := []struct {
		name   string
		mutate func(env *shedrecipe.Env)
		want   []string
	}{
		{"a negative budget", func(env *shedrecipe.Env) {
			env.Segments["Plan-Review"] = shedrecipe.SegmentSettings{MaxBounces: -1, MaxBouncesKey: "plan_max_bounces", ConfigFile: "loom.yaml"}
		}, []string{"Plan-Review", "plan_max_bounces", "loom.yaml"}},
		{"a segment with no Env.Segments entry", func(env *shedrecipe.Env) {
			delete(env.Segments, "Webster-Review")
		}, []string{"Webster-Review", "segment-to-perch table"}},
	}
	for _, tt := range newTests {
		t.Run("New "+tt.name, func(t *testing.T) {
			env, paths := testEnv(t)
			tt.mutate(&env)
			_, err := New(env, paths)
			if err == nil {
				t.Fatalf("New() error = nil; want one naming %v", tt.want)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("New() error = %q; want it to contain %q", err, want)
				}
			}
		})
	}
}
