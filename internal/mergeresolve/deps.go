// deps.go declares this package's told-value surface: the two narrow seams it drives
// (MergeSurface over the Fabric merge verbs, Shuttle over one shuttle run), the Deps struct every
// value in which is told by the caller and derived by nobody here, the two-value Outcome and its
// carrying Result, and the constructed Resolver type with its validating New.

package mergeresolve

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// MergeSurface is the narrow five-method seam this package drives over Fabric's own merge verbs.
// Every verb named here is vocabulary-free by construction — none of the five names either
// fabric-internal side — which is why this seam can exist in a package outside the Fabric
// Vocabulary Invariant's owner set at all.
type MergeSurface interface {
	// MergeIn merges source into the current pair, returning any conflicts as a result state rather
	// than an error.
	MergeIn(source string) (fabricengine.MergeResult, error)
	// MergeStageResolved stages the given, already-resolved conflict paths so MergeContinue's own
	// index guard can pass.
	MergeStageResolved(paths []string) (fabricengine.StageResult, error)
	// MergeContinue concludes an in-progress merge once every conflict has been resolved.
	MergeContinue(msg string) (fabricengine.MergeResult, error)
	// MergeAbort discards an in-progress merge, restoring the pair to its pre-merge state.
	MergeAbort() (fabricengine.MergeResult, error)
	// MergeInProgress reports whether a merge is currently in progress on the pair.
	MergeInProgress() (bool, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies MergeSurface.
var _ MergeSurface = (*fabricengine.Fabric)(nil)

// Shuttle is the one-method seam this package drives to spawn a fresh LLM session over a conflict,
// the same shape shedadapters.Shuttle already uses over shuttleengine.Runner.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// The compile-time assertion that *shuttleengine.Runner satisfies Shuttle.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// Deps carries every value Resolve needs, all of it told by the caller and derived by nobody in
// this package.
type Deps struct {
	// Fabric is the merge seam this package drives. Told by the caller.
	Fabric MergeSurface
	// Shuttle is the seam this package spawns a fresh conflict-resolution session through. Told by
	// the caller.
	Shuttle Shuttle
	// WorktreeRoot is the absolute root the conflicted paths returned by the merge seam are relative
	// to, needed for the marker scan's own file reads. Told by the caller.
	WorktreeRoot string
	// ScratchDir is the absolute scratch directory the conflict-resolution report is written into.
	// Told by the caller; a caller writing into it creates it first, on every write path.
	ScratchDir string
	// StencilsDir is the absolute directory the conflict-resolution stencil is read from. Told by
	// the caller.
	StencilsDir string
	// ConflictSpec is the model-spec string (see internal/modelspec) selecting the model the
	// conflict-resolution session runs under. Told by the caller.
	ConflictSpec string
	// Registry resolves ConflictSpec's alias, when it is one, to a concrete model. Told by the
	// caller.
	Registry modelspec.Registry
	// Timeout bounds the conflict-resolution session's wall-clock deadline. Told by the caller.
	Timeout time.Duration
}

// Outcome is Resolve's two-value verdict.
type Outcome string

const (
	// OutcomeResolved reports that the merge landed cleanly, with no conflicts left unresolved.
	OutcomeResolved Outcome = "resolved"
	// OutcomeStuck reports that the merge could not be brought to a resolved state and was aborted.
	OutcomeStuck Outcome = "stuck"
)

// Result is Resolve's return value.
type Result struct {
	// Outcome is the two-value verdict.
	Outcome Outcome
	// Reason is the human-facing explanation of a stuck verdict. It is a returned string, never a
	// log line or a file — writing it to disk and logging it is the calling producer's job, since
	// the producer is what knows its own name and its own scratch path conventions.
	Reason string
	// AlreadyUpToDate reports the degenerate case where the merge was a no-op because the pair was
	// already ahead of source.
	AlreadyUpToDate bool
}

// Resolver is the constructed type Resolve is a method of.
type Resolver struct {
	deps Deps
}

// New constructs a Resolver from deps, rejecting a nil Fabric, a nil Shuttle, and an empty
// WorktreeRoot, ScratchDir, or StencilsDir up front with a distinct error each — rather than
// nil-panicking or writing to the wrong place at call time.
func New(deps Deps) (*Resolver, error) {
	if deps.Fabric == nil {
		return nil, fmt.Errorf("mergeresolve: New: Deps.Fabric must not be nil")
	}
	if deps.Shuttle == nil {
		return nil, fmt.Errorf("mergeresolve: New: Deps.Shuttle must not be nil")
	}
	if deps.WorktreeRoot == "" {
		return nil, fmt.Errorf("mergeresolve: New: Deps.WorktreeRoot must not be empty")
	}
	if deps.ScratchDir == "" {
		return nil, fmt.Errorf("mergeresolve: New: Deps.ScratchDir must not be empty")
	}
	if deps.StencilsDir == "" {
		return nil, fmt.Errorf("mergeresolve: New: Deps.StencilsDir must not be empty")
	}
	return &Resolver{deps: deps}, nil
}
