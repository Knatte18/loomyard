// mergestateactive.go holds MergeStateActive, the weft-only probe for git-level mid-merge state.
// A path-scoped commit consults it before it lands.

package fabricengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// MergeStateActive reports whether the weft worktree of l is mid-merge at the git level: a live
// MERGE_HEAD or a non-empty conflicted index, rather than whether fabric itself has a merge in
// progress.
//
// It is weft-only, because warp and weft are independent clones with independent .git directories.
// The status commit this probe guards runs in the weft worktree alone, so warp-side git state cannot block it.
// A two-sided check would freeze every status commit while a live warp conflict-resolution session runs.
// That is the moment a resuming machine most needs to know the run is Stuck and since when.
//
// The probe takes an l *lymcwd.Location rather than an open *Fabric.
// A check on fabric's own merge record misses git-level merge state that fabric did not start.
// That is the case this probe exists to catch, and callers of this probe do not hold an open Fabric.
//
// A non-nil error is surfaced rather than swallowed: the decision that an unreadable probe means
// skip belongs to the caller, not to this function.
//
// The probe is read-only, so it returns a plain bool and error, not a result type that embeds a mutation record.
func MergeStateActive(l *lymcwd.Location) (bool, error) {
	repo := gitrepo.New(RecordsWorktree(l))

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
