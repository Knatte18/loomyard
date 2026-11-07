// names.go derives facts from the embedded recipes.
// It gives the engine set, the row names and rows in file order, and the retired row names.
// It also gives the amendment target and the entry number of each row.

package loomrecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// RecipeEngines returns the sorted engine set of the tapestry, darn, pawl, sampler, motif and raddle recipes.
// It parses each recipe, expands its blocks, and collects each row's engine name, without duplicates.
// The recipes are the loom module's, so the set is their union.
// It holds the Block engine of the tapestry recipe's Motif block row.
// Deriving the set from the recipes, rather than writing it down, keeps every union of consumer engine sets honest.
// It needs no second hand-maintained table alongside the recipes.
//
// RecipeEngines never returns a nil slice alongside an error.
// A parse failure panics, naming this package.
// A recipe that fails to parse is a build-time defect in an embedded file, not a runtime condition.
func RecipeEngines() []string {
	seen := map[string]bool{}
	var engines []string
	for _, name := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn, shedrun.RecipePawl, shedrun.RecipeSampler, shedrun.RecipeMotif, shedrun.RecipeRaddle} {
		recipe, err := parseEmbeddedRecipe(name)
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
// OnDone is the row its Done outcome routes to, empty for a row that ends the run.
// Bouncer reports whether the row's engine is the Bouncer.
// IsBlock reports whether the row is a block row.
// An expanded recipe runs a block row before the rows of the block's sub-recipe.
// SkipTo is the row a skipped block row routes to.
// BlockPath is the names of the block rows enclosing the row, outermost first and joined by "/".
// It is empty for a row outside every block.
type RecipeRow struct {
	Name      string
	Segment   string
	OnStuck   string
	OnDone    string
	Bouncer   bool
	BlockPath string
	SkipTo    string
	IsBlock   bool
}

// RowNames returns the row names of the named recipe in expanded order.
// The recipe names are the shedrun.Recipe* constants; any other name is refused with an error naming it.
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

// Rows returns the rows of the named recipe in expanded order.
// Each block row is followed by the rows of its sub-recipe.
// The recipe names are the shedrun.Recipe* constants; any other name is refused with an error naming it.
func Rows(recipe string) ([]RecipeRow, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	rows := make([]RecipeRow, 0, len(parsed.Producers))
	for _, row := range parsed.Producers {
		rows = append(rows, RecipeRow{
			Name:      row.Name,
			Segment:   row.Segment,
			OnStuck:   row.OnStuck,
			OnDone:    row.OnDone,
			Bouncer:   row.Engine == bouncerEngine,
			BlockPath: row.BlockPath,
			SkipTo:    row.SkipTo,
			IsBlock:   row.Engine == shedrecipe.EngineBlock,
		})
	}
	return rows, nil
}

// RetiredRowNames returns the sorted row names the named recipe's `retired` map declares:
// the rows a run of an earlier version of the recipe may still stand at.
// It reads only the recipe's `retired` map, not the arm-time renames.
// It is empty for a recipe that retires none.
// The recipe names are the shedrun.Recipe* constants; any other name is refused with an error naming it.
func RetiredRowNames(recipe string) ([]string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parsed.Retired))
	for name := range parsed.Retired {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// AmendmentTarget returns the optional `amendment_target` of the named recipe:
// the row a plan amendment routes to, empty when the recipe sets none.
// The recipe names are the shedrun.Recipe* constants; any other name is refused with an error naming it.
func AmendmentTarget(recipe string) (string, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return "", err
	}
	return parsed.AmendmentTarget, nil
}

// embeddedRecipes is the one table from recipe name to embedded recipe bytes.
var embeddedRecipes = map[string][]byte{
	shedrun.RecipeTapestry: recipes.TapestryRecipe,
	shedrun.RecipeDarn:     recipes.DarnRecipe,
	shedrun.RecipeBatten:   recipes.BattenRecipe,
	shedrun.RecipePawl:     recipes.PawlRecipe,
	shedrun.RecipeSampler:  recipes.SamplerRecipe,
	shedrun.RecipeMotif:    recipes.MotifRecipe,
	shedrun.RecipeRaddle:   recipes.RaddleRecipe,
}

// embeddedRecipeSource returns the source of the embedded recipe the name selects, refusing a name that selects none.
// An empty name reads as the tapestry recipe, as an unseeded run does.
// It is the resolver handed to every expansion of an embedded recipe.
func embeddedRecipeSource(recipe string) ([]byte, error) {
	if recipe == "" {
		recipe = shedrun.RecipeTapestry
	}
	if source, ok := embeddedRecipes[recipe]; ok {
		return source, nil
	}

	return nil, fmt.Errorf("loomrecipe: no embedded recipe named %q", recipe)
}

// parseEmbeddedRecipe parses the embedded recipe the name selects and expands its blocks.
// It refuses a name that selects none.
// An empty name reads as the tapestry recipe, as an unseeded run does.
// A recipe holding no block row comes back as parsed.
func parseEmbeddedRecipe(recipe string) (shedbuild.Recipe, error) {
	source, err := embeddedRecipeSource(recipe)
	if err != nil {
		return shedbuild.Recipe{}, err
	}
	parsed, err := shedbuild.Parse(source)
	if err != nil {
		return shedbuild.Recipe{}, fmt.Errorf("loomrecipe: parse the %s recipe: %w", recipe, err)
	}
	expanded, err := shedbuild.Expand(parsed, embeddedRecipeSource)
	if err != nil {
		return shedbuild.Recipe{}, fmt.Errorf("loomrecipe: expand the %s recipe: %w", recipe, err)
	}
	return expanded, nil
}

// EntryNumbers numbers a recipe's entries 1, 2, … in row order, the numbers every status pane draws.
// A review segment, a non-empty segment holding a Bouncer row, is one entry:
// it takes one number, at the place of its first row,
// and every row of the segment maps to that number.
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
