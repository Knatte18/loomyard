// reviewbudget_test.go asserts the review bounce budget comes from the caller, not the recipe:
// New and Routing set it on every row of a Bouncer-holding segment and leave PR-Gate alone.

package loomrecipe

import (
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// reviewSegmentBouncers maps each review segment to its Bouncer row, the key of the per-perch budget map.
var reviewSegmentBouncers = map[string]string{
	"Discussion-Review": "Discussion-Bouncer",
	"Plan-Review":       "Plan-Bouncer",
	"Webster-Review":    "Webster-Bouncer",
	"Merge-Review":      "Merge-Bouncer",
}

// assertReviewBudget fails unless every review-segment row carries its Bouncer's rowBudgets entry, 0 included, or budget where that is absent, and PR-Gate carries prGate.
func assertReviewBudget(t *testing.T, producers []shedengine.ProducerDef, budget int, rowBudgets map[string]int, prGate int) {
	t.Helper()

	reviewRows := 0
	for _, def := range producers {
		bouncer, isReview := reviewSegmentBouncers[def.Segment]
		switch {
		case isReview:
			reviewRows++
			want := budget
			if own, ok := rowBudgets[bouncer]; ok {
				want = own
			}
			if def.MaxBounces != want {
				t.Errorf("row %q (segment %q) MaxBounces = %d; want %d", def.Name, def.Segment, def.MaxBounces, want)
			}
		case def.Name == "PR-Gate":
			if def.MaxBounces != prGate {
				t.Errorf("PR-Gate MaxBounces = %d; want the recipe's %d", def.MaxBounces, prGate)
			}
		}
	}
	if want := 3 * len(reviewSegmentBouncers); reviewRows != want {
		t.Errorf("review-segment rows = %d; want %d, a Bouncer, a review row and a fix row per segment", reviewRows, want)
	}
}

// TestReviewBudget_AppliedByNewAndRouting asserts both entry points stamp the budget on the review rows:
// the run-wide one on every segment, and a perch's own budget on that segment's three rows alone.
func TestReviewBudget_AppliedByNewAndRouting(t *testing.T) {
	const budget = 7

	tests := []struct {
		name       string
		rowBudgets map[string]int
	}{
		{"run-wide budget on every segment", nil},
		{"an absent entry takes the run-wide budget", map[string]int{"Discussion-Bouncer": 1}},
		{"a zero entry is zero rounds", map[string]int{"Plan-Bouncer": 0}},
		{"one perch's own budget", map[string]int{"Plan-Bouncer": 2}},
		{"every perch's own budget", map[string]int{"Discussion-Bouncer": 1, "Plan-Bouncer": 2, "Webster-Bouncer": 9, "Merge-Bouncer": 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routing, err := Routing(budget, tt.rowBudgets)
			if err != nil {
				t.Fatalf("Routing(%d, %v) = _, %v; want nil", budget, tt.rowBudgets, err)
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
			assertReviewBudget(t, routing.Producers, budget, tt.rowBudgets, prGate)

			env, paths := testEnv(t)
			env.ReviewMaxBounces = budget
			env.RowMaxBounces = tt.rowBudgets
			shed, err := New(env, paths)
			if err != nil {
				t.Fatalf("New() = _, %v; want nil", err)
			}
			assertReviewBudget(t, shed.Producers, budget, tt.rowBudgets, prGate)
		})
	}
}

// TestZeroBudget_BouncerSettlesAtOnce asserts a Bouncer at budget 0, through its own RowMaxBounces entry or through a run-wide 0 with no entries, settles Done with no shuttle start.
// That proves loomrecipe stamped zero_rounds on its row; the settle's seam order is TestBouncerEntry_SkipSeam's to pin.
// It also asserts New and Routing keep each perch's route: the writers and Merge-In still hand to their Bouncer and every Merge-Review row stays in the list.
func TestZeroBudget_BouncerSettlesAtOnce(t *testing.T) {
	tests := []struct {
		name       string
		runWide    int
		rowBudgets map[string]int
		zeroed     []string
	}{
		{"Discussion-Bouncer entry", 3, map[string]int{"Discussion-Bouncer": 0}, []string{"Discussion-Bouncer"}},
		{"Plan-Bouncer entry", 3, map[string]int{"Plan-Bouncer": 0}, []string{"Plan-Bouncer"}},
		{"Webster-Bouncer entry", 3, map[string]int{"Webster-Bouncer": 0}, []string{"Webster-Bouncer"}},
		{"Merge-Bouncer entry", 3, map[string]int{"Merge-Bouncer": 0}, []string{"Merge-Bouncer"}},
		{"run-wide 0 with no entries", 0, nil, []string{"Discussion-Bouncer", "Plan-Bouncer", "Webster-Bouncer", "Merge-Bouncer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routing, err := Routing(tt.runWide, tt.rowBudgets)
			if err != nil {
				t.Fatalf("Routing() = _, %v; want nil", err)
			}
			env, paths := testEnv(t)
			env.ReviewMaxBounces = tt.runWide
			env.RowMaxBounces = tt.rowBudgets
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

// TestReviewBudget_NegativeRefused asserts New and Routing refuse a negative run-wide budget and name it, accept a run-wide 0, and refuse a negative per-perch budget naming its Bouncer row.
func TestReviewBudget_NegativeRefused(t *testing.T) {
	if _, err := Routing(-1, nil); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("Routing(-1) error = %v; want one naming ReviewMaxBounces", err)
	}
	if _, err := Routing(0, nil); err != nil {
		t.Errorf("Routing(0) error = %v; want nil", err)
	}
	negative := map[string]int{"Webster-Bouncer": -1}
	if _, err := Routing(3, negative); err == nil || !strings.Contains(err.Error(), "Webster-Bouncer") {
		t.Errorf("Routing(3, %v) error = %v; want one naming Webster-Bouncer", negative, err)
	}

	env, paths := testEnv(t)
	env.ReviewMaxBounces = -1
	if _, err := New(env, paths); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("New() with ReviewMaxBounces -1 error = %v; want one naming ReviewMaxBounces", err)
	}

	env, paths = testEnv(t)
	env.RowMaxBounces = negative
	if _, err := New(env, paths); err == nil || !strings.Contains(err.Error(), "Webster-Bouncer") {
		t.Errorf("New() with a negative RowMaxBounces error = %v; want one naming Webster-Bouncer", err)
	}
}
