// mergelifecycle.go implements the merge lifecycle quartet — MergeContinue, MergeAbort,
// MergeInProgress — and the shared conclude phase concludeMergeSides that MergeIn (merge.go) and
// MergeContinue both land through.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// concludeMergeSides lands the conclude-commit on both sides of an in-progress merge, under the
// caller's already-held write lock: warp first, then weft, in that fixed order.
// A side whose recorded outcome is fast_forwarded or up_to_date is skipped — no commit is fabricated
// on a no-op side — and a side whose committed SHA is already recorded is skipped too, for
// idempotency across a resumed MergeContinue.
// Every caller must have established that both recorded outcomes are non-empty first: an empty
// outcome means MergeStart never ran on that side, and concluding it would commit one side of a
// merge whose other side does not exist. MergeIn and Merge satisfy that by construction;
// MergeContinue enforces it explicitly via mergeAttemptIncompleteReason.
// Message precedence is explicit msg, then st.Message, then empty — an empty effective message
// concludes via MergeConclude("")'s git-prepared MERGE_MSG/SQUASH_MSG.
// After each side lands, its new SHA is written into st's committed field and saved, and
// KindMergeCommitted is recorded (Target = checkout path, Detail = new SHA).
// If a conclude fails on either side (whichever landed first or second), nothing is rolled back:
// the record is retained, the git failure is logged internally, and *ErrMergeIncomplete is returned.
func concludeMergeSides(f *Fabric, rec *Mutations, st *mergeState, msg string) error {
	effectiveMsg := msg
	if effectiveMsg == "" {
		effectiveMsg = st.Message
	}

	if st.WarpOutcome != mergeOutcomeFastForwarded && st.WarpOutcome != mergeOutcomeAlreadyUpToDate && st.WarpCommitted == "" {
		if err := f.warp.MergeConclude(effectiveMsg); err != nil {
			logger.Warn("fabricengine: merge conclude failed", "side", "warp", "error", err)
			return &ErrMergeIncomplete{}
		}
		sha, err := f.warp.CurrentSHA()
		if err != nil {
			logger.Warn("fabricengine: resolve warp HEAD after conclude failed", "error", err)
			return &ErrMergeIncomplete{}
		}
		st.WarpCommitted = sha
		if err := f.saveMergeState(st); err != nil {
			return err
		}
		rec.Append(KindMergeCommitted, f.warpPath, sha)
	}

	if st.WeftOutcome != mergeOutcomeFastForwarded && st.WeftOutcome != mergeOutcomeAlreadyUpToDate && st.WeftCommitted == "" {
		if err := f.weft.MergeConclude(effectiveMsg); err != nil {
			logger.Warn("fabricengine: merge conclude failed", "side", "weft", "error", err)
			return &ErrMergeIncomplete{}
		}
		sha, err := f.weft.CurrentSHA()
		if err != nil {
			logger.Warn("fabricengine: resolve weft HEAD after conclude failed", "error", err)
			return &ErrMergeIncomplete{}
		}
		st.WeftCommitted = sha
		if err := f.saveMergeState(st); err != nil {
			return err
		}
		rec.Append(KindMergeCommitted, f.weftPath, sha)
	}

	return nil
}

// mergeStateOrForeignErr resolves the disposition shared by MergeContinue and MergeAbort when no
// fabric merge record exists: *ErrForeignMergeState when git-level merge state exists that fabric
// did not start, *ErrNoMergeInProgress otherwise.
func (f *Fabric) mergeStateOrForeignErr() error {
	foreign, err := f.foreignMergeStatePresent()
	if err != nil {
		return err
	}
	if foreign {
		return &ErrForeignMergeState{}
	}
	return &ErrNoMergeInProgress{}
}

// mergeAttemptIncompleteReason reports mergeReasonAttemptIncomplete when st records an empty outcome
// for either side — the shape a crash between the two MergeStart calls (or before the first one)
// leaves behind, since merge.go persists each side's outcome only once that side's MergeStart has
// returned.
// An empty outcome means the attempt never reached that side, so there is nothing there to conclude
// and no way to finish the merge by concluding: MergeContinue must refuse the whole call before it
// lands anything, leaving MergeAbort — which restores both sides from the recorded pre-merge SHAs
// regardless of how far the attempt got — as the one correct recovery.
// Both sides are inspected unconditionally, and the single aggregated reason names neither.
func mergeAttemptIncompleteReason(st *mergeState) []string {
	if st.WarpOutcome == "" || st.WeftOutcome == "" {
		return []string{mergeReasonAttemptIncomplete}
	}
	return nil
}

// MergeContinue concludes an in-progress merge once every conflict has been resolved in the
// worktree: with no record and no foreign git merge state it returns *ErrNoMergeInProgress; with no
// record but foreign state present it returns *ErrForeignMergeState, leaving that state untouched.
// Unmerged entries remaining on either side refuse with a *MergeGuardError carrying
// mergeReasonUnresolvedConflicts, and a record whose attempt never reached both sides refuses with
// mergeReasonAttemptIncomplete; both are aggregated into one error, so which precondition failed
// never discloses evaluation order.
// Otherwise it acquires the combined write lock, runs concludeMergeSides with msg as the optional
// message override, records correspondence, deletes the merge-state record, and returns a
// MergeResult whose Committed is read off the record's own conclude-SHA fields — true when the pair
// carries this merge's conclude-commit, false when both sides only fast-forwarded or never moved
// and no commit was ever fabricated.
func (f *Fabric) MergeContinue(msg string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	st, err := f.loadMergeState()
	if err != nil {
		return MergeResult{}, err
	}
	if st == nil {
		return MergeResult{}, f.mergeStateOrForeignErr()
	}

	warpConflicts, err := f.warp.ConflictedFiles()
	if err != nil {
		return MergeResult{}, err
	}
	weftConflicts, err := f.weft.ConflictedFiles()
	if err != nil {
		return MergeResult{}, err
	}
	var reasons []string
	if len(warpConflicts) > 0 || len(weftConflicts) > 0 {
		reasons = append(reasons, mergeReasonUnresolvedConflicts)
	}
	reasons = append(reasons, mergeAttemptIncompleteReason(st)...)
	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons)
	}

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fl.Release() }()

	if err := concludeMergeSides(f, rec, st, msg); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve warp HEAD after merge: %w", err)
	}
	newWeftHEAD, err := f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve weft HEAD after merge: %w", err)
	}
	if err := f.RecordCorrespondence(newWarpHEAD, newWeftHEAD); err != nil {
		return MergeResult{}, err
	}
	if err := f.deleteMergeState(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{Committed: st.landedConcludeCommit()}, nil
}

// MergeAbort discards an in-progress merge, restoring both sides to their pre-merge SHAs: with no
// record and no foreign git merge state it returns *ErrNoMergeInProgress; with no record but foreign
// state present it returns *ErrForeignMergeState, the same rule as MergeContinue.
// Otherwise it acquires the combined write lock, resets both sides unconditionally via
// resetMergeSides — including a fast-forwarded side and a side that never moved — then deletes the
// merge-state record and returns MergeResult{} (Committed false).
func (f *Fabric) MergeAbort() (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	st, err := f.loadMergeState()
	if err != nil {
		return MergeResult{}, err
	}
	if st == nil {
		return MergeResult{}, f.mergeStateOrForeignErr()
	}

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fl.Release() }()

	if err := f.resetMergeSides(rec, st.WarpStart, st.WeftStart); err != nil {
		return MergeResult{}, err
	}
	if err := f.deleteMergeState(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{}, nil
}

// MergeInProgress reports whether fabric has a merge in progress on this pair: mergeRecordExists()'s
// bare boolean, no MutationRecord (a read-only probe stays off the embed table by design).
// It never consults foreignMergeStatePresent and never errors on foreign state: it answers "does
// fabric have a merge in progress", which foreign plain-git state does not make true.
func (f *Fabric) MergeInProgress() (bool, error) {
	return f.mergeRecordExists()
}
