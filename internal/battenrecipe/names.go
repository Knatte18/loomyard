// names.go declares the batten recipe's row-name constants, the names the recipe file's rows must match.
// It also declares RowsBeforeRunShed and RecipeEngines:
// the derived engine set that the cross-consumer coverage guard in internal/shedrecipe trusts.

package battenrecipe

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// The batten recipe row names.
// These are durable on-disk identities that resume depends on:
// a rename here without a matching rename in contracts/recipes/batten-recipe.yaml breaks resume for any in-flight run;
// this package's coverage guard pins the two against each other.
const (
	// NameWorktreeCreate is the row that creates the task worktree.
	NameWorktreeCreate = "Worktree-Create"
	// NameSeedChild is the row that seeds the task worktree's inner shed run.
	// It derives the run's recipe from the Board task's own type and its driver from prime's own seed.
	// It runs between Worktree-Create and Run-Shed;
	// the inner shed run cannot start until its seed exists.
	NameSeedChild = "Seed-Child"
	// NameOpenWindow is the row that opens a terminal window on the task worktree when batten.yaml's open_window is on.
	NameOpenWindow = "Open-Window"
	// NameSeedIDE is the row that writes the task worktree's VS Code config when batten.yaml's seed_ide is on.
	NameSeedIDE = "Seed-IDE"
	// NameOpenIDE is the row that opens VS Code on the task worktree when batten.yaml's open_ide is on.
	NameOpenIDE = "Open-IDE"
	// NameRunShed is the row that runs the inner shed run inside the task worktree.
	// Its value is durable, not its name.
	// shedengine persists CurrentProducer, this row name, into the status file.
	// Changing the value breaks resume for any in-flight run.
	// The value must match the row's name in contracts/recipes/batten-recipe.yaml,
	// or this package's coverage guard fails.
	// The constant's own name may differ from the value, since only the value is persisted.
	// Renaming the constant alone leaves resume unaffected.
	// recipe_test.go pins the value against a rename, so a changed value fails that test.
	NameRunShed = "Run-Shed"
	// NameLand is the row that lands a finished child into the prime: pull, deploy, board prune and the ready notice.
	// It runs between Run-Shed and Worktree-Teardown and carries no on_stuck, so a failed step halts batten blocked there.
	NameLand = "Land"
	// NameWorktreeTeardown is the row that tears the task worktree down.
	NameWorktreeTeardown = "Worktree-Teardown"
)

// RowsBeforeRunShed returns the names of the batten recipe's rows listed before Run-Shed, in recipe order:
// the rows a task's run passes through before its inner run starts.
// A recipe that fails to parse or holds no Run-Shed row panics, naming this package.
func RowsBeforeRunShed() []string {
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		panic(fmt.Sprintf("battenrecipe: RowsBeforeRunShed: %v", err))
	}
	var rows []string
	for _, row := range recipe.Producers {
		if row.Name == NameRunShed {
			return rows
		}
		rows = append(rows, row.Name)
	}
	panic("battenrecipe: RowsBeforeRunShed: the batten recipe holds no " + NameRunShed + " row")
}

// RecipeEngines returns the distinct engine names of the batten recipe's rows, sorted.
// The cross-consumer coverage guard in internal/shedrecipe trusts this result.
//
// It derives the engines from the recipe rather than a hand-maintained list.
// A parse failure panics, naming this package: the embedded recipe is a build-time defect, not a runtime condition.
func RecipeEngines() []string {
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		panic(fmt.Sprintf("battenrecipe: RecipeEngines: %v", err))
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
