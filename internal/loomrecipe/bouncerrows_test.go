package loomrecipe

import (
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// rolePrefixViolations returns one message per role-prefix violation in recipe:
// a row of a review segment, a segment holding a Bouncer row, whose engine is Bouncer, BurlerReview or BurlerFix and that sets no role_prefix;
// a segment whose rows set different values; and two segments that set the same value.
func rolePrefixViolations(recipe shedbuild.Recipe) []string {
	reviewSegments := map[string]bool{}
	for _, row := range recipe.Producers {
		if row.Engine == bouncerEngine && row.Segment != "" {
			reviewSegments[row.Segment] = true
		}
	}

	var violations []string
	var segments []string
	prefixOf := map[string]string{}
	for _, row := range recipe.Producers {
		if !reviewSegments[row.Segment] {
			continue
		}
		switch row.Engine {
		case bouncerEngine, "BurlerReview", "BurlerFix":
		default:
			continue
		}
		prefix, _ := row.Config["role_prefix"].(string)
		if prefix == "" {
			violations = append(violations, fmt.Sprintf("row %s of segment %s sets no role_prefix", row.Name, row.Segment))
			continue
		}
		first, seen := prefixOf[row.Segment]
		switch {
		case !seen:
			prefixOf[row.Segment] = prefix
			segments = append(segments, row.Segment)
		case first != prefix:
			violations = append(violations, fmt.Sprintf("row %s of segment %s sets role_prefix %q; the segment's other rows set %q", row.Name, row.Segment, prefix, first))
		}
	}

	owner := map[string]string{}
	for _, segment := range segments {
		prefix := prefixOf[segment]
		if other, taken := owner[prefix]; taken {
			violations = append(violations, fmt.Sprintf("segments %s and %s both set role_prefix %q", other, segment, prefix))
			continue
		}
		owner[prefix] = segment
	}
	return violations
}

// TestBouncerRowsCarryOverTheirOwnSegment pins both per-segment keys of the shipped recipes' review rows.
// Every Bouncer row sets carry_over to the segment the row itself belongs to, since a Bouncer learns the segment it files its carry-over entry under from that key alone.
// Every review segment's rows share one role_prefix that no other segment uses, which the role-prefix subtests check on the shipped recipes and on recipes mutated to break each rule.
func TestBouncerRowsCarryOverTheirOwnSegment(t *testing.T) {
	t.Run("RolePrefixes", func(t *testing.T) {
		parse := func(t *testing.T, source []byte) shedbuild.Recipe {
			t.Helper()
			recipe, err := shedbuild.Parse(source)
			if err != nil {
				t.Fatalf("shedbuild.Parse() = %v; want nil", err)
			}
			return recipe
		}
		for name, source := range map[string][]byte{"LoomRecipe": recipes.LoomRecipe, "DarnRecipe": recipes.DarnRecipe} {
			if got := rolePrefixViolations(parse(t, source)); len(got) != 0 {
				t.Errorf("%s: rolePrefixViolations = %v; want none", name, got)
			}
		}

		mutations := []struct {
			name   string
			mutate func(recipe shedbuild.Recipe)
		}{
			{"a review row without the key", func(recipe shedbuild.Recipe) {
				for _, row := range recipe.Producers {
					if row.Name == loomshed.NamePlanBurlerReview {
						delete(row.Config, "role_prefix")
					}
				}
			}},
			{"a fix row with another value", func(recipe shedbuild.Recipe) {
				for _, row := range recipe.Producers {
					if row.Name == loomshed.NameWebsterBurlerFix {
						row.Config["role_prefix"] = "other"
					}
				}
			}},
			{"two segments with the same value", func(recipe shedbuild.Recipe) {
				for _, row := range recipe.Producers {
					if row.Segment == "Plan-Review" {
						row.Config["role_prefix"] = "discussion"
					}
				}
			}},
		}
		for _, tt := range mutations {
			t.Run(tt.name, func(t *testing.T) {
				recipe := parse(t, recipes.LoomRecipe)
				tt.mutate(recipe)
				if got := rolePrefixViolations(recipe); len(got) != 1 {
					t.Errorf("rolePrefixViolations = %v; want exactly one violation", got)
				}
			})
		}
	})

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
