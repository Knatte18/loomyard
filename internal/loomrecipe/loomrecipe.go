// loomrecipe.go implements New: the coherence guard across env and paths, then the delegation to
// shedbuild.NewShed that parses contracts/recipes.LoomRecipe, builds it against a caller-supplied
// shedrecipe.Env, and returns the assembled *shedengine.Shed.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses recipes.LoomRecipe, builds it against env, and returns a *shedengine.Shed carrying
// the built []shedengine.ProducerDef plus paths' five fields.
//
// New delegates its parse-and-build work to shedbuild.NewShed, wrapping a non-nil returned error
// with a "loomrecipe: " prefix and nothing more: shedbuild.NewShed already names the offending
// row's zero-based index and name in every error it raises after decode, and the decoder keeps
// yaml line numbers, so no further position work is needed.
//
// New never calls shedbuild.Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go
// both state that Check is authoring-time only, because a resumed run legitimately starts
// mid-graph and reachability-from-entry is the wrong production question.
//
// New performs no nil-guard or absolute-path check of its own on any Env field: each registry
// entry validates exactly the fields it reads, and preflightEntry's
// requireAbsRoot("Preflight", "Cwd", …) is what now covers the guard loomshed.New's nil-Preflight
// check used to.
//
// New does make one check of its own, and it is a coherence check across its two arguments rather
// than a validation of either: it returns an error when env.StatusPath != paths.StatusPath, and
// another when env.StatusLockPath != paths.StatusLockPath. Both run as New's first act, ahead of
// the delegation to shedbuild.NewShed. The ordering is load-bearing, not stylistic:
// loomPreflightEntry calls requireAbsRoot("LoomPreflight", "StatusPath", …) during Build, so a
// divergence involving an empty or relative value would otherwise surface as that entry's
// absoluteness error and hide which of the two copies was wrong. Each error names both sides'
// values so the divergence is readable without a debugger.
//
// StatusPath and StatusLockPath are deliberately told twice -- once in env (for
// loomPreflightEntry) and once in paths (for Shed) -- and that duplication is inherent to the
// split between the two argument types New takes; it must not be collapsed. This check is the
// guard the duplication needs. loomshed.Deps carried one StatusPath field feeding both
// NewLoomPreflight and shedengine.Shed, so the two could not disagree; splitting it into an Env
// copy (read by loomPreflightEntry) and a shedbuild.ShedPaths copy (read by Shed) makes a
// divergent fill possible for the first time, and its consequence is silent: Shed would persist
// its status to one file while Loom-Preflight reads another, which surfaces as a broken resume
// rather than as any error. That is the same class of silent durable-identity hazard the
// row-name-authority-stays-with-the-go-constants Shared Decision exists to machine-check, and New
// is the only place in the tree where both copies are visible at once, so no other layer can make
// this check.
func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if env.StatusPath != paths.StatusPath {
		return nil, fmt.Errorf("loomrecipe: env.StatusPath %q != paths.StatusPath %q", env.StatusPath, paths.StatusPath)
	}
	if env.StatusLockPath != paths.StatusLockPath {
		return nil, fmt.Errorf("loomrecipe: env.StatusLockPath %q != paths.StatusLockPath %q", env.StatusLockPath, paths.StatusLockPath)
	}

	shed, err := shedbuild.NewShed(recipes.LoomRecipe, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	return shed, nil
}
