// recipeengines.go implements RecipeEngines, the engine set a recipe file's rows name.

package shedcheckkit

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// RecipeEngines returns the distinct engine names of the rows of the recipe source, sorted.
// It derives the engines from the recipe rather than a hand-maintained list.
// A parse failure panics, naming shedcheckkit.
// A recipe that does not parse is a build-time defect, not a runtime condition.
func RecipeEngines(source []byte) []string {
	recipe, err := shedbuild.Parse(source)
	if err != nil {
		panic(fmt.Sprintf("shedcheckkit: RecipeEngines: %v", err))
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
