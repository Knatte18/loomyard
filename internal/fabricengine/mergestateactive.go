// mergestateactive.go holds MergeStateActive, the weft-only git-level mid-merge probe a
// path-scoped commit must consult before landing — distinct from both Fabric.MergeInProgress and
// the two-sided foreignMergeStatePresent.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// MergeStateActive reports whether l's weft sibling worktree is mid-merge AT THE GIT LEVEL — a live
// MERGE_HEAD or a non-empty conflicted index — rather than whether fabric itself has a merge in
// progress.
//
// It is weft-only by design, deliberately not the two-sided form foreignMergeStatePresent uses:
// warp and weft are independent clones with independent .git directories, and the status commit
// this probe exists for runs in the weft worktree alone, so warp-side git state cannot block it.
// Inheriting the two-sided form would freeze every status commit for the whole duration of a live
// warp conflict-resolution session — the one moment a resuming machine most needs to know the run
// is Stuck and since when.
//
// Fabric.MergeInProgress cannot serve as this probe: it is mergeRecordExists()'s bare boolean, never
// consults foreignMergeStatePresent, and is therefore false in precisely the foreign-state case this
// probe exists to catch — and it needs an open *Fabric, which a caller closure driving this probe
// does not hold. MergeStateActive takes an l *lyxcwd.Location instead, matching CommitAnchoredPaths
// and PushAnchored.
//
// A non-nil error is surfaced rather than swallowed: the decision that an unreadable probe means
// skip belongs to the caller, not to this function.
//
// This is a read-only probe, so it returns no result type and embeds no MutationRecord, per the
// Mutation Record Invariant's "a read-only one must not" clause.
func MergeStateActive(l *lyxcwd.Location) (bool, error) {
	repo := gitrepo.New(WeftWorktree(l))

	mergeHeadPresent, err := repo.MergeHeadPresent()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	conflicted, err := repo.ConflictedFiles()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}

	return mergeHeadPresent || len(conflicted) > 0, nil
}
