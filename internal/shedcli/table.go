// table.go declares the named-recipe arming table: entry, the package-level recipes map literal,
// and the two accessors (lookup, names) that are the table's only reach points. There is no init()
// self-registration and no runtime Register function, mirroring internal/shedrecipe/registry.go's
// own shape: one declaration site, reached through accessors.

package shedcli

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/lifecyclecli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// entry is one recipe's arming contract: the function that resolves and wires that recipe's whole
// engine stack, the positional-argument contract cobra validates before Arm ever runs, and the set
// of the four generic shedverbs verbs that recipe actually supports.
type entry struct {
	// Arm is the recipe's exported resolution entry point -- loomcli.Arm or lifecyclecli.Arm --
	// called with the resolved cwd, the invoked verb name, and the command's positional arguments.
	Arm func(cwd string, verb string, args []string) (shedverbs.Spec, error)
	// Args is the cobra.PositionalArgs contract this recipe's commands validate against, shared
	// with the same recipe's own <module>cli commands so both invocation paths refuse a wrong
	// argument count byte-identically.
	Args cobra.PositionalArgs
	// Verbs names the shedverbs verbs this recipe supports. It is this table's sole authority on
	// which verb/recipe pairs exist: the four generic verbs are registered once for every recipe,
	// but the recipes do not all support all four -- lifecycle has no "step" analogue.
	Verbs []string
}

// recipes is the single place every named recipe is declared, mapping each recipe name to the
// entry that arms it.
//
// lifecycle has no "step" analogue: without this table gating step's dispatch, "lyx shed step
// --recipe lifecycle" would reach the generic step body with StepBusyKind unset and PreStep nil,
// emitting kind: "" -- a sixth value outside the five the Shed Verb-Set Invariant and ly-drive both
// pin closed -- and skipping lifecycle's whole PreRun pre-flight.
var recipes = map[string]entry{
	"loom": {
		Arm:   loomcli.Arm,
		Args:  cobra.NoArgs,
		Verbs: []string{"run", "step", "status", "pause"},
	},
	"lifecycle": {
		Arm:   lifecyclecli.Arm,
		Args:  cobra.ExactArgs(1),
		Verbs: []string{"run", "status", "pause"},
	},
}

// lookup resolves name against recipes, returning its entry.
//
// An unknown name's error names the available recipes, read fresh from names() rather than baked
// in, so the message cannot drift from the table it describes.
func lookup(name string) (entry, error) {
	e, ok := recipes[name]
	if !ok {
		return entry{}, fmt.Errorf("shedcli: unknown recipe %q; available recipes: %v", name, names())
	}
	return e, nil
}

// names returns every recipe name registered in recipes, sorted, in a freshly allocated slice each
// call -- mutating the returned slice never affects a later call or the table itself.
func names() []string {
	result := make([]string, 0, len(recipes))
	for name := range recipes {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
