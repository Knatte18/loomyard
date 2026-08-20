// mergestage.go implements MergeStageResolved, the narrow verb that stages agent-resolved conflict
// paths so MergeContinue's index guard — an index probe, ConflictedFiles(), that no amount of
// content editing can clear — can pass.

package fabricengine

import (
	"fmt"
	"path/filepath"
)

// StageResult is the mutating result type MergeStageResolved returns, embedding MutationRecord per
// the Mutation Record Invariant.
type StageResult struct {
	MutationRecord
}

// MergeStageResolved takes unified, worktree-relative paths and stages each on whichever side's
// index actually lists it unmerged.
// A path listed by neither side is an error rather than a silent skip — the caller passed something
// that is not conflicted, and silently staging it would hide a caller bug.
// Deletions stage rather than erroring, because a delete/modify conflict is legitimately resolved by
// the file being gone.
// An empty or nil paths is a no-op, returning StageResult{} with an empty mutation record and a nil
// error.
// MergeContinue's own conflict guard is deliberately left untouched by this verb — the two are
// independent gates, not one relocated one.
//
// This is the one mutating verb in the merge surface that takes NO weft write lock and asserts no
// merge-record precondition, and that is a decision rather than an omission. With no merge in
// progress both sides' ConflictedFiles() are empty, so every path fails the partition above and the
// call errors before staging anything — there is no state it can reach where it writes outside a
// merge. While a merge IS in progress the four guarded sibling verbs (Commit, Pull, Topology's
// Checkout and Remove) already refuse on the record, so no other fabric writer can be in either
// index concurrently. A lock here would serialize this verb against nothing.
// The contrast with Merge's pre-merge sync — which does need the lock, because it mutates before
// the record exists — is what makes the difference load-bearing rather than stylistic.
func (f *Fabric) MergeStageResolved(paths []string) (res StageResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if len(paths) == 0 {
		return StageResult{}, nil
	}

	warpConflicts, err := f.warp.ConflictedFiles()
	if err != nil {
		return StageResult{}, fmt.Errorf("fabricengine: read conflicted files: %w", err)
	}
	weftConflicts, err := f.weft.ConflictedFiles()
	if err != nil {
		return StageResult{}, fmt.Errorf("fabricengine: read conflicted files: %w", err)
	}

	warpSet := make(map[string]bool, len(warpConflicts))
	for _, p := range warpConflicts {
		warpSet[p] = true
	}
	weftSet := make(map[string]bool, len(weftConflicts))
	for _, p := range weftConflicts {
		weftSet[p] = true
	}

	// Partition every path before staging anything, so a bad path fails the whole call rather than
	// leaving one side half-staged.
	var warpBatch, weftBatch []string
	for _, p := range paths {
		switch {
		case warpSet[p]:
			warpBatch = append(warpBatch, p)
		case weftSet[p]:
			weftBatch = append(weftBatch, p)
		default:
			// A path listed by both sides cannot occur here: unifyConflictPaths already treats that
			// collision as unmappable and self-aborts the merge before any caller could ever see the
			// path, so the only remaining case reaching this branch is "neither side lists it".
			return StageResult{}, fmt.Errorf("fabricengine: %s is not conflicted on either side", p)
		}
	}

	// Warp first, then weft, matching concludeMergeSides' fixed side ordering.
	if len(warpBatch) > 0 {
		if err := f.warp.StageResolved(warpBatch); err != nil {
			return StageResult{}, fmt.Errorf("fabricengine: stage resolved paths: %w", err)
		}
		rec.Append(KindMergeResolvedStaged, f.warpPath, "")
	}
	if len(weftBatch) > 0 {
		if err := f.weft.StageResolved(weftBatch); err != nil {
			return StageResult{}, fmt.Errorf("fabricengine: stage resolved paths: %w", err)
		}
		rec.Append(KindMergeResolvedStaged, f.weftPath, "")
	}

	return StageResult{}, nil
}
