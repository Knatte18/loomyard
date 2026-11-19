// mergestateactive.go holds MergeStateActive, the weft-only probe for git-level mid-merge state.
// A path-scoped commit consults it before it lands.

package fabricengine

import (
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// MergeStateActive reports whether the weft worktree of l is in a git operation a records commit cannot pass.
// MergeStateOperation names that operation:
// a live MERGE_HEAD or a non-empty conflicted index, or a cherry-pick, revert or rebase.
// It does not report whether fabric itself has a merge in progress.
// In a one-repository worktree a merge counts only when its index holds a records path unlike HEAD's.
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
	operation, err := MergeStateOperation(l)
	if err != nil {
		return false, err
	}
	return operation != "", nil
}
