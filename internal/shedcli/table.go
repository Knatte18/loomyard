// table.go declares the named-recipe arming table: the recipes map literal and its entry type.
// Each recipe is declared once, in recipes, and reached only through the lookup and names accessors.
// There is no init() self-registration and no runtime Register function.

package shedcli

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// entry is one recipe's arming contract.
// It holds the Location-taking function that resolves and wires the recipe's whole engine stack,
// and the set of generic shedverbs verbs the recipe supports.
type entry struct {
	// Arm is the recipe's exported Location-taking resolution entry point, loomcli.ArmAt or battencli.ArmAt.
	// It takes the already-resolved *lymcwd.Location, the invoked verb name and the run-id,
	// and it performs no lymcwd.Resolve of its own.
	// resolvePersistentPreRun resolves cwd once, ahead of the seed read that picks the recipe.
	// That same Location is passed on to Arm.
	Arm func(location *lymcwd.Location, verb string, runID string) (shedverbs.Spec, error)
	// Verbs names the shedverbs verbs this recipe supports.
	// This table is the sole authority on which verb and recipe pairs exist.
	// The generic verbs are registered once for every recipe, but a recipe need not support every one of them.
	Verbs []string
	// BootstrapVerb is non-empty when a run seeded for this recipe may be driven by an LLM.
	// It is taken from the owning module's exported constant, never a literal, so it cannot drift from that module.
	// The "lym shed seed --driver llm" validator tests it for emptiness, never the recipe name.
	BootstrapVerb string
	// RefuseSeedAt is the recipe's own answer to "may a seed for me be written in this worktree".
	// "lym shed seed" consults it after fabric's drivable-worktree check; nil allows any drivable worktree.
	// It is taken from the owning module's exported guard, never re-derived here,
	// so a seed is written only where that recipe's verbs would drive it.
	RefuseSeedAt func(location *lymcwd.Location) error
}

// recipes declares every named recipe, mapping each recipe name to the entry that arms it.
//
// The Verbs field gates dispatch: a recipe that excludes a verb must not reach the generic verb body.
// Without the gate, the body would run with StepBusyKind unset and PreStep nil, and emit kind: "".
// That is a value outside the six the Shed Verb-Set Invariant and the driver stencil pin closed.
// It would also skip the recipe's own PreRun pre-flight.
var recipes = map[string]entry{
	"tapestry": {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
	"motif": {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
	"batten": {
		Arm:           battencli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: battencli.BootstrapVerb,
		RefuseSeedAt:  battencli.RefuseUnlessPrime,
	},
	"darn": {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
	"pawl": {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
	"sampler": {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
	shedrun.RecipeRaddle: {
		Arm:           loomcli.ArmAt,
		Verbs:         []string{"run", "step", "status", "pause", "goto"},
		BootstrapVerb: loomcli.BootstrapVerb,
	},
}

// lookup returns the entry for the recipe name, or an error for an unknown name.
//
// The error lists the available recipes from names(), so the message cannot drift from recipes.
func lookup(name string) (entry, error) {
	e, ok := recipes[name]
	if !ok {
		return entry{}, fmt.Errorf("shedcli: unknown recipe %q; available recipes: %v", name, names())
	}
	return e, nil
}

// names returns every recipe name in recipes, sorted.
// Each call returns a fresh slice, so a caller may modify it without affecting later calls or recipes.
func names() []string {
	result := make([]string, 0, len(recipes))
	for name := range recipes {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
