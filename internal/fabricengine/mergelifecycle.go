// mergelifecycle.go implements the merge lifecycle verbs MergeContinue, MergeAbort and MergeInProgress.
// It also holds concludeMergeSides, the conclude phase that MergeIn and MergeContinue both run.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// concludeMergeSides lands the conclude-commit on both sides of an in-progress merge, under the
// caller's already-held write lock: warp first, then weft.
// The weft arm covers records an older binary wrote, where WeftOutcome can be "staged".
// Records this binary writes carry mergeOutcomeAlreadyUpToDate on the weft side, so the arm's guard skips it.
// A side whose recorded outcome is fast_forwarded or up_to_date is skipped, so no commit is fabricated on it.
// A side whose committed SHA is already recorded is skipped too, which keeps a resumed MergeContinue idempotent.
// A crash or I/O failure between a side's `git commit` and the record re-save leaves the commit landed unrecorded.
// A re-run `git commit` then fails on the clean tree.
// The conclude check adopts such a commit into the record when git parentage shows it is this merge's conclude.
// MergeContinue then finishes the states that MergeAbort must not destroy.
// Every caller must have established that both recorded outcomes are non-empty first.
// An empty outcome means the merge never started on that side.
// Concluding it would commit one side of a merge whose other side does not exist.
// MergeContinue enforces this before the call.
// The effective message is the explicit msg, then the record's Message, then empty.
// An empty message makes git use the message it prepared for the merge.
// After each side lands, its SHA is saved to the record and KindMergeCommitted is recorded.
// Its target is the checkout path and its detail is the SHA.
// If a conclude fails on either side, nothing is rolled back.
// The record is kept, the git failure is logged, and *ErrMergeIncomplete is returned.
func concludeMergeSides(f *Fabric, rec *Mutations, st *mergeState, msg string) error {
	effectiveMsg := msg
	if effectiveMsg == "" {
		effectiveMsg = st.Message
	}

	if st.WarpOutcome != mergeOutcomeFastForwarded && st.WarpOutcome != mergeOutcomeAlreadyUpToDate && st.WarpCommitted == "" {
		sha, landed, err := sideConcludeAlreadyLanded(f.code, st.WarpStart, st.WarpSource, st.Squash)
		if err != nil {
			return err
		}
		if !landed {
			if err := f.code.MergeConclude(effectiveMsg); err != nil {
				logger.Warn("fabricengine: merge conclude failed", "side", "warp", "error", err)
				return &ErrMergeIncomplete{}
			}
			sha, err = f.code.CurrentSHA()
			if err != nil {
				logger.Warn("fabricengine: resolve warp HEAD after conclude failed", "error", err)
				return &ErrMergeIncomplete{}
			}
		}
		st.WarpCommitted = sha
		if err := f.saveMergeState(st); err != nil {
			return err
		}
		rec.Append(KindMergeCommitted, f.warpPath, sha)
	}

	if st.WeftOutcome != mergeOutcomeFastForwarded && st.WeftOutcome != mergeOutcomeAlreadyUpToDate && st.WeftCommitted == "" {
		sha, landed, err := sideConcludeAlreadyLanded(f.records, st.WeftStart, st.WeftSource, st.Squash)
		if err != nil {
			return err
		}
		if !landed {
			if err := f.records.MergeConclude(effectiveMsg); err != nil {
				logger.Warn("fabricengine: merge conclude failed", "side", "weft", "error", err)
				return &ErrMergeIncomplete{}
			}
			sha, err = f.records.CurrentSHA()
			if err != nil {
				logger.Warn("fabricengine: resolve weft HEAD after conclude failed", "error", err)
				return &ErrMergeIncomplete{}
			}
		}
		st.WeftCommitted = sha
		if err := f.saveMergeState(st); err != nil {
			return err
		}
		rec.Append(KindMergeCommitted, f.weftPath, sha)
	}

	return nil
}

// sideConcludeAlreadyLanded reports whether one side already carries this merge's conclude-commit
// under a record that never learned about it, returning the landed SHA for adoption when it does.
// The shape it detects is the crash between a side's `git commit` and the record re-save.
// The caller has already established that the side's outcome is staged or conflicted with no recorded
// conclude SHA, so a commit may have landed there with only the bookkeeping missing.
//
// Adoption claims success, so it must not act on an ambiguous signal.
// Refusing on HEAD moving off the recorded pre-merge start is safe.
// Over-refusing leaves a stuck merge the operator can inspect.
// Claiming success on that signal is not safe.
// Any commit landed on the checkout while the record was live satisfies it.
// A claim on that signal could adopt an unrelated commit and report the merge as committed.
//
// The evidence is git's own parentage, which is exact rather than heuristic.
// fabric starts a non-squash merge with `git merge --ff --no-commit <sourceSHA>`, so MERGE_HEAD holds
// exactly that SHA, and the conclude-commit has exactly two parents: the pre-merge start first, the
// merge source second.
// All five of these must hold before a commit is adopted:
//   - HEAD moved off the recorded pre-merge start,
//   - no live MERGE_HEAD (the merge is finished, not still concludable),
//   - the record is not a squash (see below),
//   - the record carries a resolved source SHA for this side,
//   - HEAD is a merge commit with exactly two parents: the recorded start first, the recorded
//     source SHA second.
//
// The parent test is exact on both position and count.
// A commit with more parents, or with the source in another position, is not this merge's conclude.
// That holds even when such a commit contains the source.
// Fabric's own conclude (`git commit --no-edit` over a live MERGE_HEAD) produces exactly two parents in that order.
// So does resetting to the start and merging the source, and the test rejects neither route.
//
// A squash conclude is never adopted.
// `git merge --squash` writes no MERGE_HEAD.
// Its conclude is an ordinary one-parent commit, with no evidence separating it from other commits.
// Refusing leaves the re-run `git commit` to fail on the clean tree.
// It returns *ErrMergeIncomplete with the record retained.
// An empty recorded source SHA, which a record without one carries, refuses too.
// No evidence is not satisfied evidence.
//
// The caller records an adoption as KindMergeCommitted, although this call ran no `git commit`.
// The commit is this merge's own conclude.
// Probe failures return raw rather than as *ErrMergeIncomplete, because nothing has been mutated yet.
func sideConcludeAlreadyLanded(repo *gitrepo.Repo, start, sourceSHA string, squash bool) (sha string, landed bool, err error) {
	head, err := repo.CurrentSHA()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: resolve HEAD to classify conclude state: %w", err)
	}
	if head == start {
		return "", false, nil
	}
	mergeHeadPresent, err := repo.MergeHeadPresent()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: check merge head to classify conclude state: %w", err)
	}
	if mergeHeadPresent {
		return "", false, nil
	}
	if squash || sourceSHA == "" {
		return "", false, nil
	}

	parents, err := repo.CommitParents(head)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: read commit parents to classify conclude state: %w", err)
	}
	if len(parents) != 2 || parents[0] != start || parents[1] != sourceSHA {
		return "", false, nil
	}
	return head, true, nil
}

// unifiedRemainingConflicts maps the paths still unmerged on each side onto one worktree-relative list,
// so the unresolved-conflicts refusal can name them instead of only asserting that some exist.
// Without the list, that refusal gives the operator nothing to act on.
// A merge is already in progress, so merge-in cannot reprint the paths.
// For a weft-side conflict, the remaining path sits behind the junction and plain `git status` does not reach it.
//
// The lookup is best-effort and never turns a precondition failure into a different error.
// A geometry read that fails is logged and yields no paths.
// The caller is already returning the refusal that matters.
// An I/O error from this read would hide the actual precondition.
// An unmappable path yields no list at all.
// A partial list would silently omit a path the operator must still resolve.
func (f *Fabric) unifiedRemainingConflicts(warpConflicts, weftConflicts []string) []string {
	l, err := lymcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		logger.Warn("fabricengine: resolve layout to list remaining conflicts failed", "path", f.warpPath, "error", err)
		return nil
	}
	anchorRel, wiredNames, err := resolveMergeGeometry(l)
	if err != nil {
		logger.Warn("fabricengine: resolve merge geometry to list remaining conflicts failed", "path", f.warpPath, "error", err)
		return nil
	}
	unified, unmappable := unifyConflictPaths(warpConflicts, weftConflicts, anchorRel, wiredNames)
	if unmappable {
		logger.Warn("fabricengine: remaining conflicts include an unmappable path; reporting none",
			"warp_conflicts", warpConflicts, "weft_conflicts", weftConflicts)
		return nil
	}
	return unified
}

// mergeStateOrForeignErr picks the refusal for a lifecycle verb that finds no fabric merge record:
// It returns *ErrForeignMergeState when git-level merge state exists that fabric did not start.
// Otherwise it returns *ErrNoMergeInProgress.
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
// for either side.
// A crash before or between the two MergeStart calls leaves that shape.
// The shape occurs because merge.go persists each side's outcome only after that side's MergeStart returns.
// An empty outcome means the attempt never reached that side, so there is nothing there to conclude.
// MergeContinue must refuse the whole call before it lands anything.
// MergeAbort is then the one correct recovery: it resets the warp side to its recorded pre-merge SHA.
// Both sides are checked unconditionally, and the one aggregated reason names neither.
func mergeAttemptIncompleteReason(st *mergeState) []string {
	if st.WarpOutcome == "" || st.WeftOutcome == "" {
		return []string{mergeReasonAttemptIncomplete}
	}
	return nil
}

// MergeContinue concludes an in-progress merge once every conflict has been resolved in the worktree.
// With no record and no foreign git merge state it returns *ErrNoMergeInProgress.
// With no record but foreign state present it returns *ErrForeignMergeState, leaving that state untouched.
// Unmerged entries remaining on either side refuse with a *MergeGuardError carrying mergeReasonUnresolvedConflicts.
// A record whose attempt never reached both sides refuses with mergeReasonAttemptIncomplete.
// A side whose live git merge differs from the one the record describes refuses with mergeReasonRecordedMergeGone.
// All three are aggregated into one error, so which precondition failed never discloses evaluation order.
// The combined write lock is acquired before the record is read and the guards run,
// so no concurrent lifecycle verb can retire or advance the record between the checks and the action.
// It runs concludeMergeSides with msg as the message override.
// It then records correspondence and deletes the merge-state record.
// It returns a MergeResult whose Committed and AlreadyUpToDate come from the record's own fields.
// Committed is true when the pair carries this merge's conclude-commit.
// It is false when both sides only fast-forwarded or never moved.
// AlreadyUpToDate is true when the attempt had recorded both sides already carrying the resolved source.
// AlreadyUpToDate is reachable here, not only defensively.
// The up-to-date probe that merge verbs run before the lock is unlocked.
// A call that loses that race records up_to_date on both sides.
// A crash before its conclude leaves exactly that record for this call to resume.
// So the field comes from the record, not a constant.
func (f *Fabric) MergeContinue(msg string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	// The lock is taken before the record is read and before any guard runs.
	// Guards evaluated ahead of the lock could read a state another lock holder changes before this call acts.
	// A concurrent MergeContinue could conclude and delete the record in between.
	// This call would then conclude from a stale record.
	// It would answer committed:true, where a sequential run answers *ErrNoMergeInProgress.
	// Everything below the acquisition sees a state no other fabric writer can be mutating.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := acquireWeftWriteLock(lockDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fl.Release() }()

	st, err := f.loadMergeState()
	if err != nil {
		return MergeResult{}, err
	}
	if st == nil {
		return MergeResult{}, f.mergeStateOrForeignErr()
	}

	warpConflicts, err := f.code.ConflictedFiles()
	if err != nil {
		return MergeResult{}, err
	}
	weftConflicts, err := f.records.ConflictedFiles()
	if err != nil {
		return MergeResult{}, err
	}
	var reasons []string
	var stillConflicted []string
	if len(warpConflicts) > 0 || len(weftConflicts) > 0 {
		reasons = append(reasons, mergeReasonUnresolvedConflicts)
		stillConflicted = f.unifiedRemainingConflicts(warpConflicts, weftConflicts)
	}
	reasons = append(reasons, mergeAttemptIncompleteReason(st)...)
	recordedMergeGone, err := recordedMergeGoneReason(f, st)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, recordedMergeGone...)
	if len(reasons) > 0 {
		return MergeResult{Conflicts: stillConflicted}, newMergeGuardError(reasons)
	}

	if err := concludeMergeSides(f, rec, st, msg); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD after merge: %w", err)
	}
	newWeftHEAD, err := f.records.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD after merge: %w", err)
	}
	if err := f.RecordCorrespondence(newWarpHEAD, newWeftHEAD); err != nil {
		return MergeResult{}, err
	}
	if err := f.deleteMergeState(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{
		AlreadyUpToDate: st.bothSidesAlreadyUpToDate(),
		Conflicts:       mergeNoConflicts,
		Committed:       st.landedConcludeCommit(),
	}, nil
}

// MergeAbort discards an in-progress merge, restoring the warp side alone to its pre-merge SHA.
// The weft is not a merge participant, so this call leaves it alone.
// With no record and no foreign git merge state it returns *ErrNoMergeInProgress.
// With no record but foreign state present it returns *ErrForeignMergeState.
// An attempt whose conclude may already have landed on the warp side refuses with a *MergeGuardError
// carrying mergeReasonConcludeLanded (concludeLandedReason), before anything is reset.
// Restoring from the recorded pre-merge SHA would discard a commit that landed.
// In the merge-in-with-conflicts flow that commit carries the operator's own resolutions.
// MergeContinue recovers that shape, and it is idempotent across a resumed run.
// The combined write lock is acquired before the record is read and that guard runs,
// so the guard sees a conclude that lands while this call waits, rather than the reset destroying it.
// It then resets the warp side via resetMergeSides.
// That reset covers a side that fast-forwarded or never moved.
// It deletes the merge-state record and returns MergeResult{} with Committed false.
func (f *Fabric) MergeAbort() (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	// The lock is taken before the record is read and before the conclude-landed guard runs.
	// Taking the lock first lets the guard see a conclude that lands while this call waits.
	// With the guard ahead of the lock, a concurrent MergeContinue could conclude and delete the record in between.
	// This call would then force-reset the freshly landed conclude commits, the destruction the guard exists to prevent.
	// deleteMergeState tolerates the record's absence, so nothing reports the loss.
	// Everything below the acquisition sees a state no other fabric writer can be mutating.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := acquireWeftWriteLock(lockDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fl.Release() }()

	st, err := f.loadMergeState()
	if err != nil {
		return MergeResult{}, err
	}
	if st == nil {
		return MergeResult{}, f.mergeStateOrForeignErr()
	}

	reasons, err := concludeLandedReason(f, st)
	if err != nil {
		return MergeResult{}, err
	}
	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons)
	}

	if err := f.resetMergeSides(rec, st.WarpStart); err != nil {
		return MergeResult{}, err
	}
	if err := f.deleteMergeState(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{Conflicts: mergeNoConflicts}, nil
}

// MergeInProgress reports whether fabric has a merge in progress on this pair.
// It is a read-only probe: it returns no MutationRecord,
// and it answers from fabric's own merge record alone.
// It does not probe foreign plain-git merge state, which does not make a fabric merge in progress.
func (f *Fabric) MergeInProgress() (bool, error) {
	return f.mergeRecordExists()
}
