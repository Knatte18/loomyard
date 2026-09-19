// lifecyclerecipe.go implements New: the entry point that delegates to shedbuild.NewShed to parse
// contracts/recipes.LifecycleRecipe, build it against a caller-supplied shedrecipe.Env, and return
// the assembled *shedengine.Shed.

package lifecyclerecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses recipes.LifecycleRecipe, builds it against env, and returns a *shedengine.Shed
// carrying the built []shedengine.ProducerDef plus paths' five fields.
//
// New delegates its parse-and-build work to shedbuild.NewShed, wrapping a non-nil returned error
// with a "lifecyclerecipe: " prefix and nothing more: shedbuild.NewShed already names the
// offending row's zero-based index and name in every error it raises after decode, and the
// decoder keeps yaml line numbers, so no further position work is needed.
//
// New never calls shedbuild.Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go
// both state that Check is authoring-time only, because a resumed run legitimately starts
// mid-graph and reachability-from-entry is the wrong production question.
//
// New performs no nil-guard or absolute-path check of its own on any Env field: each registry
// entry validates exactly the fields it reads.
//
// Unlike loomrecipe.New, New performs no coherence check across its two arguments: no lifecycle
// registry entry reads Env.StatusPath or Env.StatusLockPath, so there is no duplicated copy for a
// check to guard, and a check added for symmetry would guard nothing.
func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	shed, err := shedbuild.NewShed(recipes.LifecycleRecipe, env, paths)
	if err != nil {
		return nil, fmt.Errorf("lifecyclerecipe: %w", err)
	}

	return shed, nil
}
