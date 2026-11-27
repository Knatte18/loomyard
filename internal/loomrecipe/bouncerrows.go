// bouncerrows.go answers whether a row of the embedded tapestry, darn or arras recipe is a Bouncer.
// It also answers the run directory of such a row.
// It reads each recipe through parseEmbeddedRecipe, so the rows are the expanded ones.

package loomrecipe

import (
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// bouncerEngine is the recipe engine name a Bouncer row carries.
const bouncerEngine = "Bouncer"

// BouncerRunSubdir returns the `run_subdir` of the named row of the tapestry, darn or arras recipe.
// The bool is true only when that row's engine is Bouncer and it has a non-empty `run_subdir`.
// A recipe parse failure is a `loomrecipe:`-prefixed error.
func BouncerRunSubdir(row string) (string, bool, error) {
	for _, name := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn, shedrun.RecipeArras} {
		recipe, err := parseEmbeddedRecipe(name)
		if err != nil {
			return "", false, err
		}
		for _, r := range recipe.Producers {
			if r.Name != row {
				continue
			}
			if r.Engine != bouncerEngine {
				return "", false, nil
			}
			subdir, ok := r.Config["run_subdir"].(string)
			if !ok || subdir == "" {
				return "", false, nil
			}
			return subdir, true, nil
		}
	}
	return "", false, nil
}
