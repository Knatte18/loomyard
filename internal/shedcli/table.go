// table.go declares the named-recipe arming table: entry, the package-level recipes map literal,
// and the two accessors (lookup, names) that are the table's only reach points. There is no init()
// self-registration and no runtime Register function, mirroring internal/shedrecipe/registry.go's
// own shape: one declaration site, reached through accessors.

package shedcli

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// entry is one recipe's arming contract: the Location-taking function that resolves and wires that
// recipe's whole engine stack, and the set of the four generic shedverbs verbs that recipe actually
// supports.
type entry struct {
	// Arm is the recipe's exported Location-taking resolution entry point -- loomcli.ArmAt or
	// battencli.ArmAt -- called with the already-resolved *lyxcwd.Location, the invoked verb name,
	// and the already-resolved run-id. It performs no lyxcwd.Resolve of its own: resolvePersistentPreRun
	// resolves cwd exactly once, ahead of the seed read that decides which recipe this entry belongs
	// to, and hands the same Location on to Arm here.
	Arm func(location *lyxcwd.Location, verb string, runID string) (shedverbs.Spec, error)
	// Verbs names the shedverbs verbs this recipe supports. It is this table's sole authority on
	// which verb/recipe pairs exist: the four generic verbs are registered once for every recipe,
	// but the recipes do not all support all four.
	Verbs []string
}

// recipes is the single place every named recipe is declared, mapping each recipe name to the
// entry that arms it.
//
// The gate this table's Verbs field backs stays even though every recipe now supports every verb it
// once excluded, because a future recipe may still exclude one: without this table gating a
// recipe's dispatch, an excluded verb would reach the generic body with StepBusyKind unset and
// PreStep nil, emitting kind: "" -- a sixth value outside the five the Shed Verb-Set Invariant and
// ly-drive both pin closed -- and skipping that recipe's own PreRun pre-flight.
var recipes = map[string]entry{
	"loom": {
		Arm:   loomcli.ArmAt,
		Verbs: []string{"run", "step", "status", "pause"},
	},
	"batten": {
		Arm:   battencli.ArmAt,
		Verbs: []string{"run", "step", "status", "pause"},
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
