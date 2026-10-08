package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestBouncerRowsCarryOverTheirOwnSegment pins that every Bouncer row of the shipped recipe sets carry_over to the segment the row itself belongs to.
// A Bouncer learns the segment it files its carry-over entry under from that key alone.
func TestBouncerRowsCarryOverTheirOwnSegment(t *testing.T) {
	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(LoomRecipe) = %v; want nil", err)
	}
	bouncers := 0
	for _, row := range recipe.Producers {
		if row.Engine != bouncerEngine {
			continue
		}
		bouncers++
		if got := row.Config["carry_over"]; row.Segment == "" || got != row.Segment {
			t.Errorf("row %s: carry_over = %v, segment = %q; want carry_over equal to a non-empty segment", row.Name, got, row.Segment)
		}
	}
	if bouncers == 0 {
		t.Error("the shipped recipe has no Bouncer row; want at least one")
	}
}

func TestBouncerRunSubdir(t *testing.T) {
	bouncers := map[string]string{
		loomshed.NameDiscussionBouncer: "discussion",
		loomshed.NamePlanBouncer:       "plan",
		loomshed.NameWebsterBouncer:    "webster",
	}
	for row, want := range bouncers {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || !ok || got != want {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want %q, true, nil", row, got, ok, err, want)
		}
	}

	for _, row := range []string{loomshed.NameDiscussionBurler, loomshed.NamePRGate, "No-Such-Row"} {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || ok || got != "" {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want empty, false, nil", row, got, ok, err)
		}
	}
}
