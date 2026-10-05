// reviewbudget_test.go asserts the review bounce budget comes from the caller, not the recipe:
// New and Routing set it on every row of a Bouncer-holding segment and leave PR-Gate alone.

package loomrecipe

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reviewSegmentNames are the segments whose rows carry the configured review budget.
var reviewSegmentNames = map[string]bool{
	"Discussion-Review": true,
	"Plan-Review":       true,
	"Webster-Review":    true,
}

// assertReviewBudget fails unless every review-segment row carries budget and PR-Gate carries prGate.
func assertReviewBudget(t *testing.T, producers []shedengine.ProducerDef, budget, prGate int) {
	t.Helper()

	reviewRows := 0
	for _, def := range producers {
		switch {
		case reviewSegmentNames[def.Segment]:
			reviewRows++
			if def.MaxBounces != budget {
				t.Errorf("row %q (segment %q) MaxBounces = %d; want %d", def.Name, def.Segment, def.MaxBounces, budget)
			}
		case def.Name == "PR-Gate":
			if def.MaxBounces != prGate {
				t.Errorf("PR-Gate MaxBounces = %d; want the recipe's %d", def.MaxBounces, prGate)
			}
		}
	}
	if reviewRows != 6 {
		t.Errorf("review-segment rows = %d; want 6", reviewRows)
	}
}

// TestReviewBudget_AppliedByNewAndRouting asserts both entry points stamp the budget on the review rows.
func TestReviewBudget_AppliedByNewAndRouting(t *testing.T) {
	const budget = 7

	routing, err := Routing(budget)
	if err != nil {
		t.Fatalf("Routing(%d) = _, %v; want nil", budget, err)
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
	assertReviewBudget(t, routing.Producers, budget, prGate)

	env, paths := testEnv(t)
	env.ReviewMaxBounces = budget
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() = _, %v; want nil", err)
	}
	assertReviewBudget(t, shed.Producers, budget, prGate)
}

// TestReviewBudget_BelowOneRefused asserts New and Routing refuse a budget of 0 and name it.
func TestReviewBudget_BelowOneRefused(t *testing.T) {
	if _, err := Routing(0); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("Routing(0) error = %v; want one naming ReviewMaxBounces", err)
	}

	env, paths := testEnv(t)
	env.ReviewMaxBounces = 0
	if _, err := New(env, paths); err == nil || !strings.Contains(err.Error(), "ReviewMaxBounces") {
		t.Errorf("New() with ReviewMaxBounces 0 error = %v; want one naming ReviewMaxBounces", err)
	}
}
