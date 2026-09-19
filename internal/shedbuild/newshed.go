// newshed.go declares ShedPaths, the five told Shed-only values, and NewShed, the assembler that
// parses a caller-supplied recipe, builds it against a caller-supplied shedrecipe.Env, and returns
// the assembled *shedengine.Shed.

package shedbuild

import (
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// ShedPaths carries the five told values shedengine.Shed itself reads and no shedrecipe.Env
// registry entry reads: StatusPath, LockPath, StatusLockPath, MaxBounces, and CommitStatus.
//
// These five cannot travel in shedrecipe.Env: Env holds roots and run-wide values the registry
// entries read, and no entry reads LockPath.
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

// NewShed parses recipe, builds it against env, and returns a *shedengine.Shed carrying the built
// []shedengine.ProducerDef plus paths' five fields.
//
// NewShed takes the recipe bytes as its first argument rather than reading an embedded var of its
// own, because contracts/recipes' //go:embed vars are named per-recipe and this package must stay
// recipe-agnostic.
//
// NewShed returns both the parse error and the build error unwrapped, adding no prefix of its own:
// shedbuild already names the offending row's zero-based index and name in every error it raises
// after decode, and the decoder keeps yaml line numbers, so the unwrapped error is self-locating.
// Each of NewShed's two callers wraps a non-nil returned error with its own single package prefix,
// so NewShed must not add a second one on top.
//
// NewShed never calls Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go both state
// that Check is authoring-time only, because a resumed run legitimately starts mid-graph and
// reachability-from-entry is the wrong production question.
//
// NewShed performs no nil-guard or absolute-path check of its own on any Env field: each registry
// entry validates exactly the fields it reads.
func NewShed(recipe []byte, env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error) {
	parsed, err := Parse(recipe)
	if err != nil {
		return nil, err
	}

	producers, err := Build(parsed, env)
	if err != nil {
		return nil, err
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
