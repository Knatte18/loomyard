// recipekeys.go holds the accessors that answer questions about a recipe, selected by name.
// They cover its contract, engines, Code-Review perch gate, record engine, verify source and retired rows.
// Consumers read a recipe's rows and keys through them instead of comparing the recipe name.

package loomrecipe

import (
	"fmt"
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

// EngineRow returns the name of the first row of the named recipe that runs the given engine.
// It returns false when no row does.
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

// GateFixerSetting returns the `fixer` value of the gate element named gate on the first row that runs engine.
// That value is the `<module>.<key>` setting that holds the row's cold fixer model-spec.
// It is empty when the row has no such element or the element carries no `fixer`.
// An empty recipe name reads as the tapestry recipe.
// A name that selects no embedded recipe, a recipe with no row running engine, and a non-string `fixer` are refused.
func GateFixerSetting(recipe, engine, gate string) (string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", err
	}
	for _, row := range parsed.Producers {
		if row.Engine != engine {
			continue
		}
		elements, _ := row.Config["gates"].([]any)
		for _, element := range elements {
			fields, _ := element.(map[string]any)
			if fields["name"] != gate {
				continue
			}
			value, present := fields["fixer"]
			if !present {
				return "", nil
			}
			setting, isString := value.(string)
			if !isString {
				return "", fmt.Errorf("loomrecipe: the %s recipe's %s row names a gate fixer on element %q that is %v, not a setting string", recipe, row.Name, gate, value)
			}
			return setting, nil
		}
		return "", nil
	}

	return "", fmt.Errorf("loomrecipe: the %s recipe has no row running the %s engine", recipe, engine)
}

// CodePerchGate returns the entry gate of the named recipe's Code-Review segment Bouncer row, built over env.
// It is nil for a recipe with no such row, and for a Bouncer row with no entry_gate.
// A nil gate leaves the perch's on or off to its budget alone.
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

// RecordEngine returns the engine of the named recipe's row that writes the decision record.
// It is empty when the recipe has no such row.
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

// RetiredRows maps each row name that an earlier recipe left in a run's status file onto the row that replaced it.
// It is nil for a recipe with none, and an empty name reads as the tapestry recipe.
func RetiredRows(recipe string) map[string]string {
	if recipe == "" {
		recipe = shedrun.RecipeTapestry
	}

	return retiredRows[recipe]
}
