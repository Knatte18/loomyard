// bouncerrows.go answers which of the embedded loom recipe's rows is a Bouncer and where its run directory lives.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// bouncerEngine is the recipe engine name a Bouncer row carries.
const bouncerEngine = "Bouncer"

// BouncerRunSubdir returns the `run_subdir` of the named row when that row's engine is Bouncer.
// The bool is false for any other row and for an unknown name.
// A recipe parse failure is a `loomrecipe:`-prefixed error.
func BouncerRunSubdir(row string) (string, bool, error) {
	recipe, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		return "", false, fmt.Errorf("loomrecipe: BouncerRunSubdir: %w", err)
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
	return "", false, nil
}
