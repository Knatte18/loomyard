// names.go declares the lifecycle recipe's row-name constants, the authority the recipe file's
// row names are pinned against, and RecipeEngines, the derived engine set the cross-consumer
// coverage guard in internal/shedrecipe trusts.

package lifecyclerecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// The three lifecycle recipe row names. These are durable on-disk identities that resume depends
// on: a rename here without a matching rename in contracts/recipes/lifecycle-recipe.yaml breaks
// resume for any in-flight run, and this package's own coverage guard pins the two against each
// other.
const (
	// NameWorktreeCreate is the row that creates the task worktree.
	NameWorktreeCreate = "Worktree-Create"
	// NameLoomRun is the row that runs the inner shed run inside the task worktree. The constant's
	// name may follow a neutralization of the producer it identifies, but its value is durable:
	// shedengine persists CurrentProducer -- this row name -- into the status file, so a rename of
	// the value breaks resume for an in-flight run. recipe_test.go pins the value against that
	// symmetry-minded rename.
	NameLoomRun = "Loom-Run"
	// NameWorktreeTeardown is the row that tears the task worktree down.
	NameWorktreeTeardown = "Worktree-Teardown"
)

// RecipeEngines parses recipes.LifecycleRecipe, collects each row's engine name, de-duplicates,
// and returns the result sorted. It is the input the cross-consumer coverage guard in
// internal/shedrecipe trusts, unioned with loomrecipe.RecipeEngines to check
// shedrecipe.Names() for closed coverage.
//
// RecipeEngines derives its result from the recipe rather than a hand-maintained literal, and
// never returns a nil slice alongside an error: a parse failure panics, naming this package, since
// a recipe that fails to parse is a build-time defect in an embedded file rather than a runtime
// condition.
func RecipeEngines() []string {
	recipe, err := shedbuild.Parse(recipes.LifecycleRecipe)
	if err != nil {
		panic(fmt.Sprintf("lifecyclerecipe: RecipeEngines: %v", err))
	}

	seen := make(map[string]bool, len(recipe.Producers))
	engines := make([]string, 0, len(recipe.Producers))
	for _, row := range recipe.Producers {
		if seen[row.Engine] {
			continue
		}
		seen[row.Engine] = true
		engines = append(engines, row.Engine)
	}

	sort.Strings(engines)
	return engines
}
