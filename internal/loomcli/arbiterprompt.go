// arbiterprompt.go composes the recipe description the arbiter stencil's recipe marker carries for a loom-engine run.
// The prompt, spec and verdict gate themselves are the shed-layer arbiter's.

package loomcli

import "github.com/Knatte18/loomyard/internal/shedrun"

// arbiterAbsentPath fills the report and brief markers when the stop has no such file.
const arbiterAbsentPath = "none"

// arbiterRepairVerbs names the verbs a loom-engine run's arbiter may run, as the stencil's recipe marker carries them.
const arbiterRepairVerbs = "Its repair verbs, run with `<slug>` the task named above:" +
	" at a review escalation exactly one of `lym loom circling accept <slug>` or `lym loom circling continue <slug>`;" +
	" `lym loom decision add <slug> --by arbiter --title … --decision … --rationale … [--amends-plan]` to record a design call." +
	" A decision that adds, changes or removes a PATTERN entry also passes `--pattern-entry`, `--pattern-action` and, for `adds` or `changes`, `--pattern-text`, as `lym loom decision add --help` says."

// arbiterRecipeDescription returns the description of recipe the arbiter stencil's recipe marker carries: what the run is and the repair verbs the arbiter may run.
// A loom recipe with a Sampler-Session row is described as a task's result made with the operator in one interactive session, one with a Plan-Write row as a task driven from discussion to landing, and any other as a task's code change driven from its decision record through one build session.
func arbiterRecipeDescription(recipe string) string {
	if recipe == shedrun.RecipePawl {
		return "This is a pawl run, a beam's life driven through the loom engine. " + arbiterRepairVerbs
	}
	if recipeHasEngine(recipe, samplerSessionEngine) {
		return "This is a sampler run, a task's result made with the operator in one interactive session, then its optional review and landing. " + arbiterRepairVerbs
	}
	if recipeHasEngine(recipe, planWriteEngine) {
		return "This is a tapestry run, a task driven from discussion to landing. " + arbiterRepairVerbs
	}
	return "This is a " + recipeLabel(recipe) + " run, a task's code change driven from its decision record through one build session, its review and landing. " + arbiterRepairVerbs
}

// recipeLabel returns the recipe name a description shows, the tapestry recipe's for an empty name.
func recipeLabel(recipe string) string {
	if recipe == "" {
		return shedrun.RecipeTapestry
	}

	return recipe
}
