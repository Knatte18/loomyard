// names.go declares RecipeEngines, the derived engine set the cross-consumer coverage guard in
// internal/shedrecipe trusts.

package loomrecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// RecipeEngines parses recipes.LoomRecipe, collects each row's engine name, de-duplicates, and
// returns the result sorted. It exists as the input to the cross-consumer coverage guard, which
// unions every recipe consumer's engine set -- deriving the set from the recipe rather than
// writing it down is what keeps that union honest without a second hand-maintained table alongside
// loomRowEngines.
//
// RecipeEngines never returns a nil slice alongside an error: a parse failure panics, naming this
// package, since a recipe that fails to parse is a build-time defect in an embedded file rather
// than a runtime condition.
func RecipeEngines() []string {
	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		panic(fmt.Sprintf("loomrecipe: RecipeEngines: %v", err))
	}

	seen := make(map[string]bool, len(recipe.Producers))
	engines := make([]string, 0, len(recipe.Producers))
	for _, row := range recipe.Producers {
		if seen[row.Engine] {
			continue
		}
		seen[row.Engine] = true
		engines = append(engines, row.Engine)
	}

	sort.Strings(engines)
	return engines
}
