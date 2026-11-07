// battenrecipe.go builds the batten Shed from the embedded batten recipe.
// It also projects that recipe's routing for callers that never build a Shed.

package battenrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses recipes.BattenRecipe, builds it against env, and returns a *shedengine.Shed
// carrying the built []shedengine.ProducerDef plus paths' five fields.
//
// New delegates its parse-and-build work to shedbuild.NewShed, wrapping a non-nil returned error
// with a "battenrecipe: " prefix and nothing more.
// shedbuild.NewShed already names the offending row's zero-based index and name in every error it raises after decode.
// The decoder keeps yaml line numbers, so no further position work is needed.
//
// New never calls shedbuild.Check, which is for authoring time only.
// A resumed run legitimately starts mid-graph, so reachability from the entry is the wrong question in production.
//
// New performs no nil-guard or absolute-path check of its own on any Env field:
// each registry entry validates exactly the fields it reads.
//
// New checks no coherence between env and paths.
// No batten registry entry reads Env.StatusPath or Env.StatusLockPath.
// A check would therefore guard no duplicated copy of either field.
func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	shed, err := shedbuild.NewShed(recipes.BattenRecipe, env, paths)
	if err != nil {
		return nil, fmt.Errorf("battenrecipe: %w", err)
	}

	return shed, nil
}

// Routing projects the embedded batten recipe's routing without building any engine,
// so a caller that never builds a Shed can still compute progress.
// A parse error is returned prefixed with "battenrecipe: ".
// Routing.MaxBounces is left 0.
func Routing() (shedengine.Routing, error) {
	routing, err := shedbuild.RoutingOf(recipes.BattenRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("battenrecipe: %w", err)
	}

	return routing, nil
}
