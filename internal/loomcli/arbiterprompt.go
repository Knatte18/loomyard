// arbiterprompt.go composes the recipe description the arbiter stencil's recipe marker carries for a loom-engine run.
// The prompt, spec and verdict gate themselves are the shed-layer arbiter's.

package loomcli

import "github.com/Knatte18/loomyard/internal/shedrun"

// arbiterAbsentPath fills the report and brief markers when the stop has no such file.
const arbiterAbsentPath = "none"

// arbiterRepairVerbs names the verbs a loom-engine run's arbiter may run, as the stencil's recipe marker carries them.
const arbiterRepairVerbs = "Its repair verbs, run with `<slug>` the task named above:" +
	" at a review escalation exactly one of `lyx loom circling accept <slug>` or `lyx loom circling continue <slug>`;" +
	" `lyx loom decision add <slug> --by arbiter --title … --decision … --rationale … [--amends-plan]` to record a design call." +
	" A decision that adds, changes or removes a PATTERN entry also passes `--pattern-entry`, `--pattern-action` and, for `adds` or `changes`, `--pattern-text`, as `lyx loom decision add --help` says."

// arbiterRecipeDescription returns the description of recipe the arbiter stencil's recipe marker carries: what the run is and the repair verbs the arbiter may run.
func arbiterRecipeDescription(recipe string) string {
	switch recipe {
	case shedrun.RecipeDarn:
		return "This is a darn run, a task's code change driven through the loom engine. " + arbiterRepairVerbs
	case shedrun.RecipePawl:
		return "This is a pawl run, a beam's life driven through the loom engine. " + arbiterRepairVerbs
	}
	return "This is a loom run, a task driven from discussion to landing. " + arbiterRepairVerbs
}
