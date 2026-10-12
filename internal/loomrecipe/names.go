// names.go declares RecipeEngines, the derived engine set the cross-consumer coverage guard in
// internal/shedrecipe trusts, and RowNames, a recipe's row names in file order.

package loomrecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// discussionSeatsEngine is the engine New gives the Discussion-Write row when env.DiscussionSeats is true.
const discussionSeatsEngine = "DiscussionSeats"

// RecipeEngines parses recipes.LoomRecipe and recipes.DarnRecipe, collects each row's engine name, adds discussionSeatsEngine, de-duplicates, and returns the result sorted.
// discussionSeatsEngine joins the set because New substitutes it for the Discussion-Write row's engine, so a loom row can reach it.
// Both recipes are the loom module's, so the set is their union.
// It exists as the input to the cross-consumer coverage guard, which
// unions every recipe consumer's engine set -- deriving the set from the recipe rather than
// writing it down is what keeps that union honest without a second hand-maintained table alongside
// the one row table in shape_test.go.
//
// RecipeEngines never returns a nil slice alongside an error: a parse failure panics, naming this
// package, since a recipe that fails to parse is a build-time defect in an embedded file rather
// than a runtime condition.
func RecipeEngines() []string {
	seen := map[string]bool{discussionSeatsEngine: true}
	engines := []string{discussionSeatsEngine}
	for _, source := range [][]byte{recipes.LoomRecipe, recipes.DarnRecipe} {
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			panic(fmt.Sprintf("loomrecipe: RecipeEngines: %v", err))
		}
		for _, row := range recipe.Producers {
			if seen[row.Engine] {
				continue
			}
			seen[row.Engine] = true
			engines = append(engines, row.Engine)
		}
	}

	sort.Strings(engines)
	return engines
}

// RowNames returns the row names of the named recipe in file order: shedrun.RecipeLoom is the loom recipe and shedrun.RecipeDarn the darn recipe.
// Any other recipe name is refused with an error naming it.
func RowNames(recipe string) ([]string, error) {
	var source []byte
	switch recipe {
	case shedrun.RecipeLoom:
		source = recipes.LoomRecipe
	case shedrun.RecipeDarn:
		source = recipes.DarnRecipe
	default:
		return nil, fmt.Errorf("loomrecipe: no row names for recipe %q", recipe)
	}
	parsed, err := shedbuild.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: parse the %s recipe: %w", recipe, err)
	}
	names := make([]string, 0, len(parsed.Producers))
	for _, row := range parsed.Producers {
		names = append(names, row.Name)
	}
	return names, nil
}
