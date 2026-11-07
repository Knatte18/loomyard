// arbiterprompt.go composes the description the arbiter stencil's `{{.recipe}}` marker carries for a loom-engine run.
// The arbiter's launch prompt, shuttle spec and verdict gate live in `internal/shedarbiter`.

package loomcli

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// arbiterAbsentPath is the literal the arbiter stencil shows in a marker whose path or target is absent.
const arbiterAbsentPath = "none"

// arbiterRepairVerbs names the verbs a loom-engine run's arbiter may run.
// The verbs end every recipe description, so the arbiter stencil's `{{.recipe}}` marker carries them.
const arbiterRepairVerbs = "Its repair verbs, run with `<slug>` the task named above:" +
	" at a review escalation exactly one of `lym loom circling accept <slug>` or `lym loom circling continue <slug>`;" +
	" `lym loom decision add <slug> --by arbiter --title … --decision … --rationale … [--amends-plan]` to record a design call." +
	" A decision that adds, changes or removes a PATTERN entry, or `withdraws` a declared change, also passes `--pattern-entry`, `--pattern-action` and, for `adds` or `changes`, `--pattern-text`, as `lym loom decision add --help` says."

// arbiterRecipeDescription returns the description of recipe that the arbiter stencil's `{{.recipe}}` marker carries:
// what the run is and the repair verbs the arbiter may run.
// The pawl recipe is described as a beam's life driven through the loom engine.
// A recipe with a `SamplerSession` row is described as a task's result made with the operator.
// The result is made in one interactive session.
// A recipe with a `RaddleFile` row is described as a run that splits a Sketch's stored decision record.
// The record's parts become board entries the operator approves, and the run lands no code.
// The `RaddleFile` case is checked before the no-Finalize case, which it would otherwise match.
// A recipe with a `PlanWrite` row is described as a task driven from discussion to landing, labelled with its name.
// A recipe with no `Finalize` row is described as a task's Discussion that lands no code.
// Any other recipe is described as a task's code change driven from its decision record through one build session.
func arbiterRecipeDescription(recipe string) string {
	if recipe == shedrun.RecipePawl {
		return "This is a pawl run, a beam's life driven through the loom engine. " + arbiterRepairVerbs
	}
	if recipeHasEngine(recipe, samplerSessionEngine) {
		return "This is a sampler run, a task's result made with the operator in one interactive session, then its optional review and landing. " + arbiterRepairVerbs
	}
	if recipeHasEngine(recipe, planWriteEngine) {
		label := recipeLabel(recipe)
		return "This is " + indefiniteArticle(label) + " " + label + " run, a task driven from discussion to landing. " + arbiterRepairVerbs
	}
	if recipeHasEngine(recipe, raddleFileEngine) {
		return "This is a " + recipeLabel(recipe) + " run that splits a Sketch's stored decision record into board entries the operator approves; the run lands no code. " + arbiterRepairVerbs
	}
	if !recipeHasEngine(recipe, shedrecipe.LoomFinalizeEngine) {
		return "This is a " + recipeLabel(recipe) + " run, a task's Discussion: its decision record is written, reviewed and stored on the board entry, and the run lands no code. " + arbiterRepairVerbs
	}
	return "This is a " + recipeLabel(recipe) + " run, a task's code change driven from its decision record through one build session, its review and landing. " + arbiterRepairVerbs
}

// indefiniteArticle returns "an" for a non-empty label opening with a lowercase vowel and "a" otherwise.
func indefiniteArticle(label string) string {
	if strings.IndexByte("aeiou", label[0]) >= 0 {
		return "an"
	}
	return "a"
}

// recipeLabel returns the name a description shows for recipe, and `shedrun.RecipeTapestry` when recipe is empty.
func recipeLabel(recipe string) string {
	if recipe == "" {
		return shedrun.RecipeTapestry
	}

	return recipe
}
