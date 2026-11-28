// recipes.go is the single Go file at the top-level recipes/ package root.
// The //go:embed directive reaches only files at or below its own directory,
// so every recipe's shipped-default byte var and its //go:embed directive must live here, one per recipe.

package recipes

import (
	_ "embed"
)

// TapestryRecipe is loom's shipped-default producer graph, in internal/shedbuild's recipe format.
// Its block rows run the motif recipe and then the jacquard recipe inline.
//
//go:embed tapestry-recipe.yaml
var TapestryRecipe []byte

// JacquardRecipe is the jacquard recipe's producer graph, in internal/shedbuild's recipe format.
// Its rows plan, build and land a task from a decision record approved before the run.
// A recipe may run it inline as a block, and a task may run it alone.
//
//go:embed jacquard-recipe.yaml
var JacquardRecipe []byte

// DarnRecipe is the darn task recipe's producer graph, in internal/shedbuild's recipe format.
//
//go:embed darn-recipe.yaml
var DarnRecipe []byte

// PawlRecipe is the pawl recipe's producer graph, a beam's life, in internal/shedbuild's recipe format.
//
//go:embed pawl-recipe.yaml
var PawlRecipe []byte

// SamplerRecipe is the sampler recipe's producer graph, in internal/shedbuild's recipe format.
// It runs one interactive agent session between the decision record and the landing.
//
//go:embed sampler-recipe.yaml
var SamplerRecipe []byte

// MotifRecipe is the motif recipe's producer graph, in internal/shedbuild's recipe format.
// Its rows make the decision record from a task.
// A recipe may run it inline as a block, and a task may run it alone.
//
//go:embed motif-recipe.yaml
var MotifRecipe []byte

// BattenRecipe is the task-worktree batten's producer graph, in internal/shedbuild's recipe format.
//
//go:embed batten-recipe.yaml
var BattenRecipe []byte

// RaddleRecipe is the raddle recipe's producer graph, in internal/shedbuild's recipe format.
// Its rows split a decision record into the board entries the operator approves, and file them.
//
//go:embed raddle-recipe.yaml
var RaddleRecipe []byte

// ArrasRecipe is the arras recipe's producer graph, in internal/shedbuild's recipe format.
// Its rows discuss a task, split its decision record into board entries under the split perch's review, and file them.
//
//go:embed arras-recipe.yaml
var ArrasRecipe []byte

// HardenerRecipe is the hardener recipe's producer graph, in internal/shedbuild's recipe format.
// Its Harden-Review ring drives a lym module against throwaway sandbox hubs round after round and lands the fixes.
//
//go:embed hardener-recipe.yaml
var HardenerRecipe []byte
