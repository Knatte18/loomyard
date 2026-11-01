// recipekeys.go implements the accessors that answer a recipe's contract, engines, Code-Review perch gate, record engine, verify source and retired rows by recipe name.
// Consumers read a recipe's rows and keys through them instead of comparing the recipe name.

package loomrecipe

import (
	"slices"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// loomContract is the value of a recipe's top-level contract key that declares it a loom recipe.
const loomContract = "loom"

// retiredRows holds each recipe's retired-row map, keyed by recipe name.
// A recipe absent from it has no retired rows.
var retiredRows = map[string]map[string]string{
	shedrun.RecipeTapestry: {
		"Merge-In":           loomshed.NameParentAlign,
		"Loom-Preflight":     loomshed.NamePreflight,
		"Tapestry-Preflight": loomshed.NamePreflight,
	},
	shedrun.RecipeDarn: {
		"Darn": loomshed.NameDarnWrite,
	},
}

// IsLoom reports whether the named recipe declares `contract: loom`.
// An empty name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func IsLoom(recipe string) (bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return false, err
	}

	return parsed.Contract == loomContract, nil
}

// HasEngine reports whether some row of the named recipe runs the given engine.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func HasEngine(recipe, engine string) (bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return false, err
	}
	for _, row := range parsed.Producers {
		if row.Engine == engine {
			return true, nil
		}
	}

	return false, nil
}

// EngineRow returns the name of the first row of the named recipe that runs the given engine, and false when no row does.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func EngineRow(recipe, engine string) (string, bool, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", false, err
	}
	for _, row := range parsed.Producers {
		if row.Engine == engine {
			return row.Name, true, nil
		}
	}

	return "", false, nil
}

// CodePerchGate returns the entry gate of the named recipe's Code-Review segment Bouncer row, built over env.
// It is nil for a recipe with no such row and for a Bouncer row with no entry_gate, which leaves the perch's on or off to its budget alone.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func CodePerchGate(recipe string, env shedrecipe.Env) (shedengine.EntryGate, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	for _, row := range parsed.Producers {
		if row.Engine == bouncerEngine && row.Segment == codeReviewSegment {
			return shedbuild.RowEntryGate(row, env)
		}
	}

	return nil, nil
}

// RecordEngine returns the engine of the named recipe's row that writes the decision record, empty when the recipe has none.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func RecordEngine(recipe string) (string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", err
	}
	for _, row := range parsed.Producers {
		if slices.Contains(shedrecipe.LoomRecordEngines, row.Engine) {
			return row.Engine, nil
		}
	}

	return "", nil
}

// VerifySource returns the named recipe's top-level verify_source, empty when it sets none.
// An empty recipe name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
func VerifySource(recipe string) (string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", err
	}

	return parsed.VerifySource, nil
}

// RetiredRows maps each row name a run of the named recipe may still carry in its status file from an earlier recipe onto the row that replaced it.
// It is nil for a recipe with none, and an empty name reads as the tapestry recipe.
func RetiredRows(recipe string) map[string]string {
	if recipe == "" {
		recipe = shedrun.RecipeTapestry
	}

	return retiredRows[recipe]
}
