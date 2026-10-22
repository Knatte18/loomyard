// finalizerow.go answers whether a row of the embedded loom and darn recipes is the Finalize row.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// finalizeEngine is the recipe engine name the Finalize row carries.
const finalizeEngine = "Finalize"

// IsFinalizeRow reports whether the named row of the loom or darn recipe has engine Finalize.
// The row is identified by its engine, never its name; an unknown name is false.
// A recipe parse failure is a `loomrecipe:`-prefixed error.
func IsFinalizeRow(row string) (bool, error) {
	for _, source := range [][]byte{recipes.LoomRecipe, recipes.DarnRecipe} {
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			return false, fmt.Errorf("loomrecipe: IsFinalizeRow: %w", err)
		}
		for _, r := range recipe.Producers {
			if r.Name == row {
				return r.Engine == finalizeEngine, nil
			}
		}
	}
	return false, nil
}
