// recipes.go is the single Go file at the top-level recipes/ package root: //go:embed reaches only
// files at or below its own directory, so every recipe's shipped-default byte var and its
// //go:embed directive must live here, one per recipe.

package recipes

import (
	_ "embed"
)

// TapestryRecipe is loom's shipped-default producer graph, in internal/shedbuild's recipe format.
//
//go:embed tapestry-recipe.yaml
var TapestryRecipe []byte

// DarnRecipe is the darn task recipe's producer graph, in internal/shedbuild's recipe format.
//
//go:embed darn-recipe.yaml
var DarnRecipe []byte

// PawlRecipe is the pawl recipe's producer graph, a beam's life, in internal/shedbuild's recipe format.
//
//go:embed pawl-recipe.yaml
var PawlRecipe []byte

// SamplerRecipe is the sampler recipe's producer graph, one interactive agent session between the decision record and the landing, in internal/shedbuild's recipe format.
//
//go:embed sampler-recipe.yaml
var SamplerRecipe []byte

// MotifRecipe is the motif recipe's producer graph, the rows that make a sketch, the decision record, from a task, in internal/shedbuild's recipe format.
// The tapestry recipe runs it inline as its Motif block, and a task may run it alone.
//
//go:embed motif-recipe.yaml
var MotifRecipe []byte

// BattenRecipe is the task-worktree batten's producer graph, in internal/shedbuild's recipe
// format.
//
//go:embed batten-recipe.yaml
var BattenRecipe []byte
