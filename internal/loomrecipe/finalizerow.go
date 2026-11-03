// finalizerow.go answers whether a row of the embedded tapestry and darn recipes is the Finalize row.
// It reads each recipe through parseEmbeddedRecipe, so the rows are the expanded ones.

package loomrecipe

import (
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// finalizeEngine is the recipe engine name the Finalize row carries.
const finalizeEngine = "Finalize"

// IsFinalizeRow reports whether the named row of the tapestry or darn recipe has engine Finalize.
// The row is identified by its engine, never its name; an unknown name is false.
// A recipe parse failure is a `loomrecipe:`-prefixed error.
func IsFinalizeRow(row string) (bool, error) {
	for _, name := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn} {
		recipe, err := parseEmbeddedRecipe(name)
		if err != nil {
			return false, err
		}
		for _, r := range recipe.Producers {
			if r.Name == row {
				return r.Engine == finalizeEngine, nil
			}
		}
	}
	return false, nil
}
