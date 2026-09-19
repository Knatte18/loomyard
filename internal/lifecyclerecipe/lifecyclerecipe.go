// lifecyclerecipe.go implements ShedPaths and New: the five told Shed-only values and the entry
// point that parses contracts/recipes.LifecycleRecipe, builds it against a caller-supplied
// shedrecipe.Env, and returns the assembled *shedengine.Shed.

package lifecyclerecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// ShedPaths carries the five told values shedengine.Shed itself reads and no shedrecipe.Env
// registry entry reads: StatusPath, LockPath, StatusLockPath, MaxBounces, and CommitStatus.
type ShedPaths struct {
	// StatusPath is the durable status file; it is told and never derived. See
	// shedengine.Shed.StatusPath's own field doc.
	StatusPath string
	// LockPath is the run lock, held non-blocking for the whole of one Run. See
	// shedengine.Shed.LockPath's own field doc.
	LockPath string
	// StatusLockPath is the lock internal/state itself takes; it must name a different file from
	// LockPath. See shedengine.Shed.StatusLockPath's own field doc.
	StatusLockPath string
	// MaxBounces is the default a ProducerDef.MaxBounces of 0 inherits. 0 means "use the internal
	// default", never "no bounces allowed" -- the budget it seeds is per-producer and
	// episode-scoped, not run-wide. See shedengine.Shed.MaxBounces's own field doc.
	MaxBounces int
	// CommitStatus is copied verbatim onto the constructed shedengine.Shed. See
	// shedengine.Shed.CommitStatus's own field doc.
	CommitStatus func(producer, state string) error
}

// New parses recipes.LifecycleRecipe, builds it against env, and returns a *shedengine.Shed
// carrying the built []shedengine.ProducerDef plus paths' five fields.
//
// New uses shedbuild.Parse on the embedded bytes, never shedbuild.Load -- there is no on-disk
// runtime location for this recipe. It surfaces both the parse error and the build error rather
// than swallowing either, wrapping each with a "lifecyclerecipe: " prefix and nothing more.
//
// New never calls shedbuild.Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go
// both state that Check is authoring-time only, because a resumed run legitimately starts
// mid-graph and reachability-from-entry is the wrong production question.
//
// New performs no nil-guard or absolute-path check of its own on any Env field: each registry
// entry validates exactly the fields it reads.
//
// Unlike loomrecipe.New's two, New performs no coherence check across its two arguments: no
// lifecycle registry entry reads Env.StatusPath or Env.StatusLockPath, so there is no duplicated
// copy for a check to guard, and a check added for symmetry would guard nothing.
func New(env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error) {
	recipe, err := shedbuild.Parse(recipes.LifecycleRecipe)
	if err != nil {
		return nil, fmt.Errorf("lifecyclerecipe: %w", err)
	}

	producers, err := shedbuild.Build(recipe, env)
	if err != nil {
		return nil, fmt.Errorf("lifecyclerecipe: %w", err)
	}

	return &shedengine.Shed{
		Producers:      producers,
		StatusPath:     paths.StatusPath,
		LockPath:       paths.LockPath,
		StatusLockPath: paths.StatusLockPath,
		MaxBounces:     paths.MaxBounces,
		CommitStatus:   paths.CommitStatus,
	}, nil
}
