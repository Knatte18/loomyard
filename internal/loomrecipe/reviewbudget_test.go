// reviewbudget_test.go asserts the review bounce budget comes from the caller, not the recipe:
// New and Routing set it on every row of a Bouncer-holding segment and leave PR-Gate alone.

package loomrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reviewSegmentBouncers maps each review segment to its Bouncer row, the key of the per-perch budget map.
var reviewSegmentBouncers = map[string]string{
	"Discussion-Review": "Discussion-Bouncer",
	"Plan-Review":       "Plan-Bouncer",
	"Webster-Review":    "Webster-Bouncer",
	"Merge-Review":      "Merge-Bouncer",
}

// assertReviewBudget fails unless every review-segment row carries its Bouncer's rowBudgets entry, or budget where that is absent or 0, and PR-Gate carries prGate.
func assertReviewBudget(t *testing.T, producers []shedengine.ProducerDef, budget int, rowBudgets map[string]int, prGate int) {
	t.Helper()

	reviewRows := 0
	for _, def := range producers {
		bouncer, isReview := reviewSegmentBouncers[def.Segment]
		switch {
		case isReview:
			reviewRows++
			want := budget
			if rowBudgets[bouncer] > 0 {
				want = rowBudgets[bouncer]
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
		{"zero entries take the run-wide budget", map[string]int{"Plan-Bouncer": 0}},
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

// TestReviewBudget_BelowOneRefused asserts New and Routing refuse a run-wide budget of 0 and name it, and a negative per-perch budget and name its Bouncer row.
func TestReviewBudget_BelowOneRefused(t *testing.T) {
	if _, err := Routing(0, nil); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("Routing(0) error = %v; want one naming ReviewMaxBounces", err)
	}
	negative := map[string]int{"Webster-Bouncer": -1}
	if _, err := Routing(3, negative); err == nil || !strings.Contains(err.Error(), "Webster-Bouncer") {
		t.Errorf("Routing(3, %v) error = %v; want one naming Webster-Bouncer", negative, err)
	}

	env, paths := testEnv(t)
	env.ReviewMaxBounces = 0
	if _, err := New(env, paths); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("New() with ReviewMaxBounces 0 error = %v; want one naming ReviewMaxBounces", err)
	}

	env, paths = testEnv(t)
	env.RowMaxBounces = negative
	if _, err := New(env, paths); err == nil || !strings.Contains(err.Error(), "Webster-Bouncer") {
		t.Errorf("New() with a negative RowMaxBounces error = %v; want one naming Webster-Bouncer", err)
	}
}
