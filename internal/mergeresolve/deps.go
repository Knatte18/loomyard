// deps.go declares the seams this package drives, the Deps struct, and the Outcome and Result types.
// It also declares the Resolver type and its constructor New.
// Every field of Deps is supplied by the caller; this package derives none of them.

package mergeresolve

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// MergeSurface is the narrow seam this package drives over Fabric's merge verbs.
// None of these verbs names a Fabric-internal side, so the seam needs no Fabric vocabulary.
type MergeSurface interface {
	// MergeIn merges source into the current pair.
	// A conflict comes back as a result state, not an error.
	MergeIn(source string) (fabricengine.MergeResult, error)
	// MergeInAt merges the commit at tip from source into the current pair.
	// It merges tip rather than the freshness rule's pick, and conflicts come back as a result state, not an error.
	MergeInAt(source, tip string) (fabricengine.MergeResult, error)
	// MergeStageResolved stages the given conflict paths, which are already resolved.
	// It lets the merge's concluding step pass its index guard.
	MergeStageResolved(paths []string) (fabricengine.StageResult, error)
	// MergeStageTracked stages every tracked modification and deletion,
	// so edits the conflict session made outside the conflicted paths land in the merge commit.
	MergeStageTracked() (fabricengine.StageResult, error)
	// MergeUntrackedFiles lists the untracked, non-ignored worktree-relative paths, which a merge commit never carries.
	MergeUntrackedFiles() ([]string, error)
	// MergeContinue concludes an in-progress merge once every conflict has been resolved.
	MergeContinue(msg string) (fabricengine.MergeResult, error)
	// MergeAbort discards an in-progress merge, restoring the pair to its pre-merge state.
	MergeAbort() (fabricengine.MergeResult, error)
	// MergeInProgress reports whether a merge is currently in progress on the pair.
	MergeInProgress() (bool, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies MergeSurface.
var _ MergeSurface = (*fabricengine.Fabric)(nil)

// Shuttle is the one-method seam this package drives to spawn a fresh LLM session over a conflict.
type Shuttle interface {
	Run(shuttleengine.Spec) (shuttleengine.Result, error)
}

// The compile-time assertion that *shuttleengine.Runner satisfies Shuttle.
var _ Shuttle = (*shuttleengine.Runner)(nil)

// Deps carries every value a conflict resolution needs.
// The caller supplies all of them; this package derives none.
type Deps struct {
	// Fabric is the merge seam this package drives. The caller supplies it.
	Fabric MergeSurface
	// Shuttle is the seam this package spawns a fresh conflict-resolution session through.
	// The caller supplies it.
	Shuttle Shuttle
	// WorktreeRoot is the absolute root the conflicted paths from the merge seam are relative to.
	// The marker scan reads files under it, and the caller supplies it.
	WorktreeRoot string
	// ScratchDir is the absolute scratch directory the conflict-resolution report is written into.
	// The caller supplies it and creates it before every write into it.
	ScratchDir string
	// StencilsDir is the absolute directory the conflict-resolution stencil is read from.
	// The caller supplies it.
	StencilsDir string
	// ParentName is the name of the session the conflict session escalates to, rendered into its parent directive.
	// The caller supplies it; empty renders the no-parent variant.
	ParentName string
	// ConflictSpec is the model-spec string that selects the model the conflict-resolution session runs under.
	// The format is defined in internal/modelspec, and the caller supplies it.
	ConflictSpec string
	// Registry resolves a model alias to a concrete model, when the model spec is one.
	// The caller supplies it.
	Registry modelspec.Registry
	// Timeout bounds the conflict-resolution session's wall-clock deadline. The caller supplies it.
	Timeout time.Duration
	// LanguageSkills are the hub's language skills the conflict session loads after its own; empty for none.
	// The caller supplies it.
	LanguageSkills []string
}

// Outcome is the two-value verdict of a conflict resolution.
type Outcome string

const (
	// OutcomeResolved reports that the merge landed cleanly, with no conflicts left unresolved.
	OutcomeResolved Outcome = "resolved"
	// OutcomeStuck reports that the merge could not be brought to a resolved state and was aborted.
	OutcomeStuck Outcome = "stuck"
)

// Result is the value a conflict resolution returns, carrying its verdict and explanation.
type Result struct {
	// Outcome is the two-value verdict.
	Outcome Outcome
	// Reason is the human-facing explanation of a stuck verdict.
	// It is a returned string, never a log line or a file: the calling producer writes and logs it.
	// This package does not write or log it, because the producer knows its own name and scratch path conventions.
	Reason string
	// AlreadyUpToDate reports whether the merge was a no-op.
	// That happens when the pair was already ahead of source.
	AlreadyUpToDate bool
}

// Resolver runs conflict resolutions over the Deps it was constructed with.
type Resolver struct {
	deps Deps
}

// New constructs a Resolver from deps.
// It rejects a nil Fabric or Shuttle, and an empty WorktreeRoot, ScratchDir, or StencilsDir, each with its own error.
// Checking up front keeps a bad Deps from panicking or writing to the wrong place at call time.
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
