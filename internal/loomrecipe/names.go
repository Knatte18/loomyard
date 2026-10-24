// names.go declares RecipeEngines, the derived engine set the cross-consumer coverage guard in internal/shedrecipe trusts.
// It also declares RowNames and Rows, a recipe's row names and rows in file order, and EntryNumbers, the entry number of each row.

package loomrecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// RecipeEngines parses recipes.LoomRecipe, recipes.DarnRecipe and recipes.BeamRecipe, collects each row's engine name, de-duplicates, and returns the result sorted.
// All three recipes are the loom module's, so the set is their union.
// It exists as the input to the cross-consumer coverage guard, which
// unions every recipe consumer's engine set -- deriving the set from the recipe rather than
// writing it down is what keeps that union honest without a second hand-maintained table alongside
// the one row table in shape_test.go.
//
// RecipeEngines never returns a nil slice alongside an error: a parse failure panics, naming this
// package, since a recipe that fails to parse is a build-time defect in an embedded file rather
// than a runtime condition.
func RecipeEngines() []string {
	seen := map[string]bool{}
	var engines []string
	for _, source := range [][]byte{recipes.LoomRecipe, recipes.DarnRecipe, recipes.BeamRecipe} {
		recipe, err := shedbuild.Parse(source)
		if err != nil {
			panic(fmt.Sprintf("loomrecipe: RecipeEngines: %v", err))
		}
		for _, row := range recipe.Producers {
			if seen[row.Engine] {
				continue
			}
			seen[row.Engine] = true
			engines = append(engines, row.Engine)
		}
	}

	sort.Strings(engines)
	return engines
}

// RecipeRow is one recipe row as a status reader needs it.
// Bouncer is true when the row's engine is the Bouncer.
type RecipeRow struct {
	Name    string
	Segment string
	OnStuck string
	Bouncer bool
}

// RowNames returns the row names of the named recipe in file order: shedrun.RecipeLoom is the loom recipe, shedrun.RecipeDarn the darn recipe, shedrun.RecipeBatten the batten recipe and shedrun.RecipeBeam the beam recipe.
// Any other recipe name is refused with an error naming it.
func RowNames(recipe string) ([]string, error) {
	rows, err := Rows(recipe)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names, nil
}

// Rows returns the rows of the named recipe in file order: shedrun.RecipeLoom is the loom recipe, shedrun.RecipeDarn the darn recipe, shedrun.RecipeBatten the batten recipe and shedrun.RecipeBeam the beam recipe.
// Any other recipe name is refused with an error naming it.
func Rows(recipe string) ([]RecipeRow, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	rows := make([]RecipeRow, 0, len(parsed.Producers))
	for _, row := range parsed.Producers {
		rows = append(rows, RecipeRow{Name: row.Name, Segment: row.Segment, OnStuck: row.OnStuck, Bouncer: row.Engine == bouncerEngine})
	}
	return rows, nil
}

// AmendmentTarget returns the optional `amendment_target` of the named recipe: the row a plan amendment routes to, empty when the recipe sets none.
// The recipe names are Rows': shedrun.RecipeLoom, shedrun.RecipeDarn, shedrun.RecipeBatten and shedrun.RecipeBeam, and any other name is refused with an error naming it.
func AmendmentTarget(recipe string) (string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", err
	}
	return parsed.AmendmentTarget, nil
}

// parseEmbeddedRecipe parses the embedded recipe the name selects, refusing a name that selects none.
func parseEmbeddedRecipe(recipe string) (shedbuild.Recipe, error) {
	var source []byte
	switch recipe {
	case shedrun.RecipeLoom:
		source = recipes.LoomRecipe
	case shedrun.RecipeDarn:
		source = recipes.DarnRecipe
	case shedrun.RecipeBatten:
		source = recipes.BattenRecipe
	case shedrun.RecipeBeam:
		source = recipes.BeamRecipe
	default:
		return shedbuild.Recipe{}, fmt.Errorf("loomrecipe: no embedded recipe named %q", recipe)
	}
	parsed, err := shedbuild.Parse(source)
	if err != nil {
		return shedbuild.Recipe{}, fmt.Errorf("loomrecipe: parse the %s recipe: %w", recipe, err)
	}
	return parsed, nil
}

// EntryNumbers numbers a recipe's entries 1, 2, … in row order, the numbers every status pane draws.
// A review segment, a non-empty segment holding a Bouncer row, is one entry: it takes one number, at the place of its first row, and every row of the segment maps to that number.
// Every other row, including a row of a segment with no Bouncer row, is an entry of its own.
// A row that is not in rows has no key in the result.
func EntryNumbers(rows []RecipeRow) map[string]int {
	reviewSegments := map[string]bool{}
	for _, row := range rows {
		if row.Bouncer && row.Segment != "" {
			reviewSegments[row.Segment] = true
		}
	}
	numbers := make(map[string]int, len(rows))
	segmentNumbers := map[string]int{}
	next := 0
	for _, row := range rows {
		if reviewSegments[row.Segment] {
			number, seen := segmentNumbers[row.Segment]
			if !seen {
				next++
				number = next
				segmentNumbers[row.Segment] = number
			}
			numbers[row.Name] = number
			continue
		}
		next++
		numbers[row.Name] = next
	}
	return numbers
}
