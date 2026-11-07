// newshed.go declares ShedPaths, the five told values the Shed reads itself, and NewShed, the assembler.
// NewShed parses a caller-supplied recipe and builds it against a caller-supplied shedrecipe.Env.
// It returns the assembled *shedengine.Shed.

package shedbuild

import (
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedtransient"
)

// ShedPaths carries the five told values that the engine reads itself:
// StatusPath, LockPath, StatusLockPath, MaxBounces and CommitStatus.
//
// No registry entry reads any of these five, so they cannot travel in shedrecipe.Env.
type ShedPaths struct {
	// StatusPath is the durable status file.
	// It is told and never derived.
	StatusPath string
	// LockPath is the run lock.
	// It is held non-blocking for the whole of one Run.
	LockPath string
	// StatusLockPath is the lock internal/state itself takes;
	// it must name a file other than the run lock.
	StatusLockPath string
	// MaxBounces is the default a producer inherits when its own MaxBounces is 0.
	// 0 means "use the internal default", never "no bounces allowed";
	// the budget it seeds is per-producer and episode-scoped, not run-wide.
	MaxBounces int
	// CommitStatus is copied verbatim onto the built shed.
	// It is not wrapped here.
	CommitStatus func(producer, state string) error
	// RunID is copied onto the built shed.
	// It is not checked here.
	RunID string
	// MissingStatusWayForward is copied onto the built shed.
	// It is not checked here.
	MissingStatusWayForward string
	// PauseWayForward is copied onto the built shed.
	// It is not checked here.
	PauseWayForward string
	// RerouteCapWayForward is copied onto the built shed.
	// It is not checked here.
	RerouteCapWayForward string
	// BudgetWayForward is copied onto the built shed.
	// It is not checked here.
	BudgetWayForward string
	// RetiredMove is copied onto the built shed.
	// It is not checked here.
	RetiredMove func(from, to string, resumed bool) error
	// RetiredWayForward is copied onto the built shed.
	// It is not checked here.
	RetiredWayForward string
}

// NewShed parses recipe and builds it against env.
// It returns a *shedengine.Shed carrying the built []shedengine.ProducerDef and paths' five fields.
//
// NewShed takes the recipe bytes as its first argument rather than reading an embedded var of its own.
// The //go:embed vars in contracts/recipes are named per recipe, and this package must stay recipe-agnostic.
//
// NewShed returns both the parse error and the build error unwrapped, adding no prefix of its own.
// shedbuild already names the offending row's zero-based index and name in every error it raises after decode,
// and the decoder keeps yaml line numbers, so the unwrapped error is self-locating.
// Callers wrap a non-nil returned error with their own package prefix, so NewShed adds none.
//
// NewShed builds the recipe without expanding it, so a recipe holding a block row is refused:
// it goes through Expand and then NewShedFrom.
//
// NewShed never calls Check, which is for authoring time only:
// a resumed run legitimately starts mid-graph, so reachability from the entry is the wrong production question.
//
// NewShed performs no nil-guard or absolute-path check of its own on any Env field:
// each registry entry validates exactly the fields it reads.
func NewShed(recipe []byte, env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error) {
	parsed, err := Parse(recipe)
	if err != nil {
		return nil, err
	}

	return NewShedFrom(parsed, env, paths)
}

// NewShedFrom builds an already-parsed recipe against env.
// It returns a *shedengine.Shed carrying the built []shedengine.ProducerDef and paths' five fields.
// It lets a caller change the parsed recipe before it is built.
// It returns the build error unwrapped and never calls Check, which is for authoring time only.
func NewShedFrom(recipe Recipe, env shedrecipe.Env, paths ShedPaths) (*shedengine.Shed, error) {
	producers, err := Build(recipe, env)
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
		Transient:      shedtransient.Class,

		RunID:                   paths.RunID,
		MissingStatusWayForward: paths.MissingStatusWayForward,
		PauseWayForward:         paths.PauseWayForward,
		RerouteCapWayForward:    paths.RerouteCapWayForward,
		BudgetWayForward:        paths.BudgetWayForward,

		Retired:           recipe.Retired,
		RetiredMove:       paths.RetiredMove,
		RetiredWayForward: paths.RetiredWayForward,
	}, nil
}
