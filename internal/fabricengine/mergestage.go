// mergestage.go implements the verbs that stage a conflict session's edits into the merge index.
// MergeStageResolved stages agent-resolved conflict paths.
// Content edits alone never clear the index's conflict state, which the continue guard reads.

package fabricengine

import (
	"fmt"
	"path/filepath"
)

// StageResult is the result of the merge staging verbs; it embeds MutationRecord to report what was staged.
type StageResult struct {
	MutationRecord
}

// MergeStageResolved takes unified, worktree-relative paths and stages each on the side whose index lists it unmerged.
// A path listed by neither side is an error, not a silent skip:
// the caller passed something that is not conflicted, and silently staging it would hide a caller bug.
// Deletions stage rather than error, because a delete/modify conflict is resolved by the file being gone.
// Empty or nil paths are a no-op: the call returns StageResult{} with an empty mutation record and a nil error.
// It leaves the continue guard untouched: the two are independent gates.
//
// Foreign git-level merge state, a MERGE_HEAD or unmerged index entries fabric did not start, is refused:
// the call returns *ErrForeignMergeState and leaves that state untouched, as the other mutating merge verbs do.
// The probe runs only when no fabric record exists, the same record-then-probe order the other merge verbs use.
// During a real merge that costs one record read, and fabric's own conflicts are never taken for foreign ones.
// An operator's own conflicted git merge can leave no fabric record while its index lists conflicts.
// Staging into it would write to a merge that fabric has no record of, so the call refuses it.
//
// The verb takes no weft write lock and asserts no merge-record precondition.
// While a merge record exists, the guarded sibling verbs refuse, so no other fabric writer can touch either index.
// With no record and no foreign state, both sides list no conflicts,
// so every path fails the partition and nothing is staged.
// A lock would serialize the verb against nothing.
func (f *Fabric) MergeStageResolved(paths []string) (res StageResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if len(paths) == 0 {
		return StageResult{}, nil
	}

	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return StageResult{}, err
	}
	if !recordExists {
		foreign, err := f.foreignMergeStatePresent()
		if err != nil {
			return StageResult{}, err
		}
		if foreign {
			return StageResult{}, &ErrForeignMergeState{}
		}
	}

	warpConflicts, err := f.code.ConflictedFiles()
	if err != nil {
		return StageResult{}, fmt.Errorf("fabricengine: read conflicted files: %w", err)
	}
	weftConflicts, err := f.records.ConflictedFiles()
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
			// A path listed by both sides cannot reach this branch:
			// the unified list marks that collision unmappable and aborts the merge before any caller sees the path.
			// So the only case left is that neither side lists the path.
			// The message names neither side, so it does not disclose which lists were checked.
			// It also names the exact-spelling contract:
			// a `./`-prefixed or absolute spelling of a conflicted path lands here too.
			return StageResult{}, fmt.Errorf("fabricengine: %s is not a conflicted path in this merge; pass paths exactly as the conflicts list reported them", p)
		}
	}

	// Warp first, then weft, in the fixed side order the merge conclusion uses.
	if len(warpBatch) > 0 {
		if err := f.code.StageResolved(warpBatch); err != nil {
			return StageResult{}, fmt.Errorf("fabricengine: stage resolved paths: %w", err)
		}
		rec.Append(KindMergeResolvedStaged, f.warpPath, "")
	}
	if len(weftBatch) > 0 {
		if err := f.records.StageResolved(weftBatch); err != nil {
			return StageResult{}, fmt.Errorf("fabricengine: stage resolved paths: %w", err)
		}
		rec.Append(KindMergeResolvedStaged, f.weftPath, "")
	}

	return StageResult{}, nil
}

// MergeStageTracked stages every tracked modification and deletion in the warp checkout of an in-progress fabric merge,
// so edits a conflict session made to tracked files land in the merge commit.
// Untracked files are never staged;
// MergeUntrackedFiles lists them so a caller can halt on them.
// Only the warp side is staged, since weft content is never a merge participant for a caller's own edits.
//
// With no fabric merge record it refuses with *ErrForeignMergeState when foreign git-level merge state is present;
// otherwise it refuses with *ErrNoMergeInProgress, and it stages nothing either way.
// It takes no weft write lock: with a record present, the guarded sibling verbs already refuse,
// so no other fabric writer can be in the index concurrently.
func (f *Fabric) MergeStageTracked() (res StageResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return StageResult{}, err
	}
	if !recordExists {
		return StageResult{}, f.mergeStateOrForeignErr()
	}

	if err := f.code.StageTrackedChanges(); err != nil {
		return StageResult{}, fmt.Errorf("fabricengine: stage tracked changes: %w", err)
	}
	rec.Append(KindMergeTrackedStaged, f.warpPath, "")
	return StageResult{}, nil
}

// MergeUntrackedFiles returns the untracked, non-ignored, worktree-relative paths in the task worktree's warp checkout,
// so a caller never names a fabric side.
// The weft is not read, since it is no merge participant.
// An untracked weft file never enters the merge commit, so listing one would only halt a merge that was right.
// Junctioned `_lym` and `.lym` are excluded through `.git/info/exclude`, so they never appear.
// It returns an empty slice, never nil, when there are none.
func (f *Fabric) MergeUntrackedFiles() ([]string, error) {
	untracked, err := f.code.UntrackedFiles()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list untracked files: %w", err)
	}
	return untracked, nil
}
