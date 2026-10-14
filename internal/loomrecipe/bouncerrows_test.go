package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// TestBouncerRowsCarryOverTheirOwnSegment pins that every Bouncer row of both shipped recipes sets carry_over to the segment the row itself belongs to.
// A Bouncer learns the segment it files its carry-over entry under from that key alone.
func TestBouncerRowsCarryOverTheirOwnSegment(t *testing.T) {
	for name, source := range map[string][]byte{"LoomRecipe": recipes.LoomRecipe, "DarnRecipe": recipes.DarnRecipe} {
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			t.Fatalf("shedbuild.Parse(%s) = %v; want nil", name, err)
		}
		bouncers := 0
		for _, row := range recipe.Producers {
			if row.Engine != bouncerEngine {
				continue
			}
			bouncers++
			if got := row.Config["carry_over"]; row.Segment == "" || got != row.Segment {
				t.Errorf("%s row %s: carry_over = %v, segment = %q; want carry_over equal to a non-empty segment", name, row.Name, got, row.Segment)
			}
		}
		if bouncers == 0 {
			t.Errorf("%s has no Bouncer row; want at least one", name)
		}
	}
}

func TestBouncerRunSubdir(t *testing.T) {
	bouncers := map[string]string{
		loomshed.NameDiscussionBouncer: "discussion",
		loomshed.NamePlanBouncer:       "plan",
		loomshed.NameWebsterBouncer:    "webster",
		loomshed.NameDarnBouncer:       "darn",
	}
	for row, want := range bouncers {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || !ok || got != want {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want %q, true, nil", row, got, ok, err, want)
		}
	}

	for _, row := range []string{loomshed.NameDiscussionBurlerReview, loomshed.NameDiscussionBurlerFix, loomshed.NamePRGate, "No-Such-Row"} {
		got, ok, err := BouncerRunSubdir(row)
		if err != nil || ok || got != "" {
			t.Errorf("BouncerRunSubdir(%q) = %q, %v, %v; want empty, false, nil", row, got, ok, err)
		}
	}
}
