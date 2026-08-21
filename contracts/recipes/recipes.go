// recipes.go is the single Go file at the top-level recipes/ package root: //go:embed reaches only
// files at or below its own directory, so every recipe's shipped-default byte var and its
// //go:embed directive must live here, one per recipe.

package recipes

import (
	_ "embed"
)

// LoomRecipe is loom's shipped-default producer graph, in internal/shedbuild's recipe format.
//
//go:embed loom-recipe.yaml
var LoomRecipe []byte
