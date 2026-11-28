// merge.go implements the merge verbs MergeIn, MergeInAt and Merge, and their MergeOptions and MergeResult.
// Every verb merges the warp checkout alone; the weft is not a merge participant.
// MergeIn merges a source branch into the current pair's own warp checkout;
// it leaves any conflicts there for resolution.
// Merge merges a source branch into a target pair the caller opened a handle on.
// It can squash and expects no conflicts.
// A warp-side conflict self-aborts and returns *ErrMergeInRequired.
// Conflicts resolve in the source pair's own worktree, not the target's.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// lockWaitHook is a test seam, nil in production.
// When set, it is called with the weft write lock's path just before a merge verb waits to acquire that lock.
// An atomic pointer lets parallel tests read it without a race.
var lockWaitHook atomic.Pointer[func(lockPath string)]

// acquireWeftWriteLock takes the weft write lock in lockDir for a merge verb.
// When lockWaitHook is installed, it is called with the lock's path before the wait starts.
func acquireWeftWriteLock(lockDir string) (*lock.FileLock, error) {
	lockPath := filepath.Join(lockDir, weftWriteLockFile)
	if hook := lockWaitHook.Load(); hook != nil {
		(*hook)(lockPath)
	}
	return lock.AcquireWriteLock(lockPath)
}

// mergeNoConflicts is the empty, non-nil Conflicts value of a MergeResult with no conflicts.
// It keeps a caller's JSON from ever seeing a "null" conflicts field.
var mergeNoConflicts = []string{}

// finalizeMergeResult stamps the two envelope fields every merge verb result carries on each return:
// the accumulated mutation record, and a non-nil Conflicts slice.
// Each verb calls it from a deferred call on its named result, so it runs on every return, error returns included.
// Without it, error returns would marshal a nil slice as "conflicts": null.
func finalizeMergeResult(res *MergeResult, rec *Mutations) {
	res.Mutations = rec.Snapshot()
	if res.Conflicts == nil {
		res.Conflicts = mergeNoConflicts
	}
}

// recheckMergePreconditionsUnderLock re-verifies, once the write lock is held, three preconditions:
// no fabric merge record, no foreign git-level merge state, and a clean pair.
// MergeIn and Merge can only observe these racily before the lock.
// The record check guards against another fabric process, which writes a record before it mutates.
// The fabric boundary allows a human to run plain git in the warp checkout.
// The foreign and dirty checks catch state a human changes in the warp checkout during the lock wait.
// The pre-lock guard stage cannot see that change, since it runs before the fetches and the lock wait.
// Foreign state is checked before dirtiness because a foreign conflicted index is also tracked-dirty,
// and the foreign refusal names that state's actual remedy.
// The re-checks cover the seconds-wide parts of the window before MergeStart.
// The rest of that window stays open, since no re-check closes a race against an external actor.
func (f *Fabric) recheckMergePreconditionsUnderLock() error {
	recordNow, err := f.mergeRecordExists()
	if err != nil {
		return err
	}
	if recordNow {
		return newMergeGuardError([]string{mergeReasonAlreadyInProgress})
	}

	foreign, err := f.foreignMergeStatePresent()
	if err != nil {
		return err
	}
	if foreign {
		return &ErrForeignMergeState{}
	}

	dirtyReasons, dirtyPaths, err := pairDirtyReason(f)
	if err != nil {
		return err
	}
	if len(dirtyReasons) > 0 {
		return newMergeGuardError(dirtyReasons).withDirt(f.warpPath, dirtyPaths)
	}
	return nil
}

// MergeOptions controls Merge.
// Squash selects a squash merge, and Message overrides the conclude commit's message.
type MergeOptions struct {
	Squash  bool
	Message string
}

// MergeResult reports what a merge verb did to the pair.
// AlreadyUpToDate reports a no-op: both sides' resolved source SHA was already an ancestor of that side's HEAD.
// Conflicts lists the unified, worktree-relative paths a merge attempt left conflicted.
// It is empty, never nil, on every return, error returns included.
// Committed reports whether the pair now carries this merge's conclude commit.
// A merge that fast-forwarded both sides creates no commit and reports Committed false.
// A call that finds the work already done after taking the lock reports AlreadyUpToDate true.
// It reports the same answer a strictly sequential run of the same calls gives.
// MutationRecord is embedded and carries the mutations accumulated over the call.
type MergeResult struct {
	MutationRecord
	AlreadyUpToDate bool     `json:"already_up_to_date"`
	Conflicts       []string `json:"conflicts"`
	Committed       bool     `json:"committed"`
}

// MergeIn merges source into f's current pair's warp checkout; the weft side is not a merge participant.
// Conflicts are a result state, not an error.
// A call that produces conflicts returns (MergeResult{Conflicts: […]}, nil) and leaves the pair mid-merge.
// The caller resolves that merge with MergeContinue or MergeAbort. MergeIn never squashes.
func (f *Fabric) MergeIn(source string) (res MergeResult, err error) {
	return f.mergeIn(source, "")
}

// MergeInAt merges the commit warpSHA into f's current pair's warp checkout.
// The code side takes warpSHA, not the pick the freshness rule makes for source.
// source still names the merge: the merge state records it as the merged branch.
// So MergeContinue and MergeAbort treat the call as a merge-in of source.
// A warpSHA that names no commit in the pair's code repository fails the source-not-found guard.
// Nothing is merged in that case.
func (f *Fabric) MergeInAt(source, warpSHA string) (res MergeResult, err error) {
	return f.mergeIn(source, warpSHA)
}

// mergeIn is the body of MergeIn and MergeInAt.
// An empty warpSHA resolves the code side from source through the freshness rule.
// A set warpSHA replaces that pick after checking it resolves to a commit.
// The records side and every other guard still resolve from source.
func (f *Fabric) mergeIn(source, warpSHA string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	l, err := lymcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve layout for %s: %w", f.warpPath, err)
	}
	anchorRel, wiredNames, err := resolveMergeGeometry(l)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve merge geometry for %s: %w", f.warpPath, err)
	}

	// Foreign-state refusal: git-level merge state that fabric did not itself start refuses the whole call.
	// The call leaves that state untouched.
	// The refusal applies only when no fabric record covers that state;
	// a recorded merge takes the in-progress guard path below.
	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return MergeResult{}, err
	}
	if !recordExists {
		foreign, err := f.foreignMergeStatePresent()
		if err != nil {
			return MergeResult{}, err
		}
		if foreign {
			return MergeResult{}, &ErrForeignMergeState{}
		}
	}

	// Aggregate every guard: each member is evaluated regardless of an earlier failure,
	// so the reported reason set never discloses evaluation order.
	var reasons []string
	inProgressReasons, err := mergeInProgressReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, inProgressReasons...)

	dirtyReasons, dirtyPaths, err := pairDirtyReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, dirtyReasons...)

	detachedReasons, err := detachedHeadReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, detachedReasons...)

	sources, sourceReasons := resolveMergeSources(f, source)
	if warpSHA != "" {
		sourceReasons = nil
		if _, err := f.code.CommitTree(warpSHA); err != nil {
			sourceReasons = []string{mergeReasonSourceNotFound}
		} else {
			sources.warpSHA = warpSHA
		}
	}
	reasons = append(reasons, sourceReasons...)

	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons).withDirt(f.warpPath, dirtyPaths)
	}

	// Pre-lock already-up-to-date probe: no lock taken, no record written, empty mutation record.
	// This is the degenerate no-op.
	// The HEAD read here serves only this probe.
	// The record's starts are re-read under the lock, where no concurrent writer can stale them.
	warpStart, err := f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	warpUpToDate, err := f.code.IsAncestor(sources.warpSHA, warpStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify merge source: %w", err)
	}
	if warpUpToDate {
		return MergeResult{AlreadyUpToDate: true, Conflicts: mergeNoConflicts}, nil
	}

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fileLock, err := acquireWeftWriteLock(lockDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fileLock.Release() }()

	// Re-verify under the lock what the pre-lock guard could only observe racily
	// (recheckMergePreconditionsUnderLock).
	// A record another process wrote during the wait would be silently overwritten by saveMergeState below,
	// so the other merge's content would conclude under this record's name.
	// A plain-git merge or tracked edit a human makes during the wait would be adopted or force-reset by MergeStart.
	if err := f.recheckMergePreconditionsUnderLock(); err != nil {
		return MergeResult{}, err
	}

	// Re-read warpStart under the lock and discard the pre-lock read.
	// A concurrent writer, most plausibly a Commit landing new tips, may hold the lock while this call waits.
	// That makes the pre-lock SHA stale,
	// and a stale start would make MergeAbort reset through that writer's landed commits.
	// weftStart has no pre-lock read; the weft is not a merge participant, so this is its only read.
	warpStart, err = f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	weftStart, err := f.records.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}

	// WeftOutcome is set in the literal, before the record's first write, not in a second write after it.
	// Writing it twice would leave an empty-WeftOutcome record on disk for one extra file write,
	// and an empty outcome is what mergeAttemptIncompleteReason refuses a MergeContinue resume on.
	// A crash in that window would leave an unresumable record for a state this code never intends to occupy.
	st := &mergeState{
		Verb:        MergeVerbMergeIn,
		Source:      source,
		Squash:      false,
		Message:     "",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpSource:  sources.warpSHA,
		WeftSource:  sources.weftSHA,
		WeftOutcome: mergeOutcomeAlreadyUpToDate,
		StartedAt:   time.Now(),
	}
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}

	warpOutcome, err := f.code.MergeStart(sources.warpSHA, false)
	if err != nil {
		return MergeResult{}, f.selfAbortMergeAttempt(rec, st, "warp", err)
	}
	st.WarpOutcome = mergeOutcomeString(warpOutcome)
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	if warpOutcome != gitrepo.MergeAlreadyUpToDate {
		rec.Append(KindMergeStaged, f.warpPath, sources.warpSHA)
	}

	if warpOutcome == gitrepo.MergeConflicted {
		var warpConflicts []string
		warpConflicts, err = f.code.ConflictedFiles()
		if err != nil {
			return MergeResult{}, err
		}

		unified, unmappable := unifyConflictPaths(warpConflicts, nil, anchorRel, wiredNames)
		if unmappable {
			logger.Warn("fabricengine: MergeIn produced unmappable conflict paths; self-aborting",
				"warp_conflicts", warpConflicts)
			if err := f.resetMergeSides(rec, st.WarpStart); err != nil {
				return MergeResult{}, err
			}
			if err := f.deleteMergeState(); err != nil {
				return MergeResult{}, err
			}
			return MergeResult{}, &ErrUnmergeableState{}
		}

		return MergeResult{Conflicts: unified}, nil
	}

	// With both sides clean, conclude the merge and record the pair's post-merge correspondence.
	// Record it even when one side never moved, then clear the record.
	if err := concludeMergeSides(f, rec, st, ""); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
	}
	newWeftHEAD, err := f.records.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
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

// Merge merges source into f's target pair's warp checkout.
// The caller opens f on that pair's worktree with lymcwd.ResolveWorktree and fabricengine.Open.
// Fabric never picks the target pair's topology itself.
// The weft side is not a merge participant.
// Merge can squash (opts.Squash) and expects a conflict-free merge.
// A warp-side conflict aborts the warp side back to its pre-merge SHA and returns *ErrMergeInRequired.
// Conflicts resolve in the source pair's worktree, not the target's.
// The caller runs MergeIn there, then retries Merge.
// Before merging, Merge syncs the target's warp side to its upstream by fetching, then fast-forwarding only.
// A target merely behind its upstream therefore merges cleanly instead of being refused.
// The guard stage checks the not-synced precondition before anything is fetched,
// so it cannot see a divergence the checkout has not learned about yet.
// The sync step checks it again after the fetch and refuses with the same not-synced reason.
// The sync runs inside the weft write lock, not ahead of it.
// It mutates the warp checkout while no merge record exists yet, so only the lock serializes it.
func (f *Fabric) Merge(source string, opts MergeOptions) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	// Foreign-state refusal: git-level merge state that fabric did not itself start refuses the whole call.
	// The call leaves that state untouched.
	// The refusal applies only when no fabric record covers that state;
	// a recorded merge takes the in-progress guard path below.
	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return MergeResult{}, err
	}
	if !recordExists {
		foreign, err := f.foreignMergeStatePresent()
		if err != nil {
			return MergeResult{}, err
		}
		if foreign {
			return MergeResult{}, &ErrForeignMergeState{}
		}
	}

	// Aggregate every guard: each member is evaluated regardless of an earlier failure,
	// so the reported reason set never discloses evaluation order.
	// The guard stage changes no worktree, index, branch tip or fabric record.
	// The sync step, which does change them, runs only after every guard passes.
	// The stage is not literally read-only:
	// resolveMergeSources runs a best-effort Fetch() on both sides, which updates remote-tracking refs and FETCH_HEAD.
	// Nothing a caller can observe as a change to the pair moves here.
	var reasons []string
	inProgressReasons, err := mergeInProgressReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, inProgressReasons...)

	dirtyReasons, dirtyPaths, err := pairDirtyReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, dirtyReasons...)

	detachedReasons, err := detachedHeadReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, detachedReasons...)

	syncReasons, err := syncedToUpstreamReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, syncReasons...)

	sources, sourceReasons := resolveMergeSources(f, source)
	reasons = append(reasons, sourceReasons...)

	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons).withDirt(f.warpPath, dirtyPaths)
	}

	// The write lock is taken here, ahead of the sync step, because the sync step mutates both checkouts.
	// That makes it the first step of this call that must be serialized against the sibling weft-mutating verbs.
	// The merge record cannot serialize it, since it is written later.
	// Until then mergeBlocksMutation reports false, so the sibling verbs do not refuse.
	// A concurrent Commit holds this lock and writes that same worktree.
	// Running `git merge --ff-only` in the weft checkout would then make two uncoordinated writers on one index.
	// Only MergeIn can defer its acquisition, because MergeIn has no sync step and mutates nothing before the record.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fileLock, err := acquireWeftWriteLock(lockDir)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fileLock.Release() }()

	// Re-verify under the lock what the pre-lock guard could only observe racily (recheckMergePreconditionsUnderLock).
	// The sync step below mutates both checkouts.
	// A record that appeared during the wait would mean writing into a pair another merge is in flight on.
	// Mid-wait foreign state or dirt would let `merge --ff-only` and MergeStart run on a checkout the guard never saw.
	if err := f.recheckMergePreconditionsUnderLock(); err != nil {
		return MergeResult{}, err
	}

	// Pre-merge sync step: a recorded mutation, not a guard, and the first thing that touches either checkout.
	// Every guard above has already passed.
	// The step can still refuse with `mergeReasonNotSynced`, because it is the first point in the call that has fetched.
	// It can therefore see a divergence the pre-lock guard stage could not yet know about.
	// The refusal is returned as-is, not wrapped.
	// A caller matching *MergeGuardError then sees the same error shape a pre-lock refusal would give.
	// A sync-step message would describe the step that detected the problem, not the precondition that failed.
	if err := f.syncSideBeforeMerge(rec, f.code, f.warpPath, "warp"); err != nil {
		return MergeResult{}, wrapMergeSyncError(err)
	}

	// Already-up-to-date probe after the sync step.
	// This early return writes no merge record.
	// The sync's own advance is real upstream catch-up that the merge did not cause, so it stays recorded.
	warpStart, err := f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	weftStart, err := f.records.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	warpUpToDate, err := f.code.IsAncestor(sources.warpSHA, warpStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify merge source: %w", err)
	}
	if warpUpToDate {
		return MergeResult{AlreadyUpToDate: true, Conflicts: mergeNoConflicts}, nil
	}

	// Pre-merge SHAs are captured after the sync step, so MergeAbort returns the pair to its synced state.
	// That restore never undoes a legitimate upstream advance.
	// WeftOutcome is set in the literal rather than by a second write.
	// That avoids saving an intermediate record with an empty outcome.
	st := &mergeState{
		Verb:        MergeVerbMerge,
		Source:      source,
		Squash:      opts.Squash,
		Message:     opts.Message,
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpSource:  sources.warpSHA,
		WeftSource:  sources.weftSHA,
		WeftOutcome: mergeOutcomeAlreadyUpToDate,
		StartedAt:   time.Now(),
	}
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}

	warpOutcome, err := f.code.MergeStart(sources.warpSHA, opts.Squash)
	if err != nil {
		return MergeResult{}, f.selfAbortMergeAttempt(rec, st, "warp", err)
	}
	st.WarpOutcome = mergeOutcomeString(warpOutcome)
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	if warpOutcome != gitrepo.MergeAlreadyUpToDate {
		rec.Append(KindMergeStaged, f.warpPath, sources.warpSHA)
	}

	// Any conflict on the warp side self-aborts.
	// The target pair is restored exactly, and no conflicted state is left behind.
	// The conflicting side is not disclosed: the error carries a fixed message,
	// and the source travels in its own field.
	if warpOutcome == gitrepo.MergeConflicted {
		if err := f.resetMergeSides(rec, st.WarpStart); err != nil {
			return MergeResult{}, err
		}
		if err := f.deleteMergeState(); err != nil {
			return MergeResult{}, err
		}
		return MergeResult{}, &ErrMergeInRequired{Source: source}
	}

	// Both sides clean: conclude the merge, record the pair's post-merge correspondence, and clear the record.
	// The correspondence is recorded even when one side never moved.
	// Message precedence: opts.Message, already carried in st.Message, then git's own prepared MERGE_MSG or SQUASH_MSG.
	if err := concludeMergeSides(f, rec, st, ""); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.code.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
	}
	newWeftHEAD, err := f.records.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
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

// syncSideBeforeMerge brings one side up to its upstream before a merge, and classifies it against that upstream.
// A side with no upstream is a no-op.
// A side with an upstream runs a best-effort Fetch, logging a failure via logger.Warn and never failing.
// It re-resolves the upstream SHA and classifies the side as equal, behind, ahead, or diverged.
// Behind advances via MergeFFOnly, never ResetHard.
// MergeFFOnly fails loudly on a raced divergence rather than silently discarding history.
// An observed advance is recorded as KindRepoAdvanced, with the checkout path as Target and the new SHA as Detail.
// Equal and ahead are no-ops.
//
// Diverged refuses with mergeReasonNotSynced.
// The pre-merge guard stage decides from remote-tracking state before anything fetches,
// so it can read a checkout whose upstream has moved on as ahead, and pass it.
// This function fetches before it classifies, so it sees a divergence the guard stage could not.
// Collapsing ahead and diverged into one no-op would discard that information.
// The refusal carries the same fixed, side-free reason whichever side produced it.
func (f *Fabric) syncSideBeforeMerge(rec *Mutations, repo *gitrepo.Repo, dir, sideLabel string) error {
	_, hasUpstream, err := upstreamSHAAt(dir)
	if err != nil {
		return err
	}
	if !hasUpstream {
		return nil
	}

	if err := repo.Fetch(); err != nil {
		logger.Warn("fabricengine: best-effort fetch before merge sync failed", "side", sideLabel, "error", err)
	}

	upstreamSHA, hasUpstream, err := upstreamSHAAt(dir)
	if err != nil {
		return err
	}
	if !hasUpstream {
		return nil
	}

	head, err := repo.CurrentSHA()
	if err != nil {
		return fmt.Errorf("fabricengine: resolve HEAD in %s: %w", dir, err)
	}
	if head == upstreamSHA {
		return nil
	}

	behind, err := repo.IsAncestor(head, upstreamSHA)
	if err != nil {
		return fmt.Errorf("fabricengine: classify sync state in %s: %w", dir, err)
	}
	if behind {
		if err := repo.MergeFFOnly(upstreamSHA); err != nil {
			return err
		}
		rec.Append(KindRepoAdvanced, dir, upstreamSHA)
		return nil
	}

	ahead, err := repo.IsAncestor(upstreamSHA, head)
	if err != nil {
		return fmt.Errorf("fabricengine: classify sync state in %s: %w", dir, err)
	}
	if ahead {
		return nil
	}

	return newMergeGuardError([]string{mergeReasonNotSynced})
}

// wrapMergeSyncError shapes an error from the pre-merge sync step for Merge's return.
// A *MergeGuardError passes through untouched, so the caller sees the ordinary aggregated-precondition shape.
// Every other error is wrapped with the sync-step context that names where it happened.
func wrapMergeSyncError(err error) error {
	var guardErr *MergeGuardError
	if errors.As(err, &guardErr) {
		return guardErr
	}
	return fmt.Errorf("fabricengine: sync checkout before merge: %w", err)
}

// selfAbortMergeAttempt resets both sides to their captured pre-merge SHAs and deletes the merge-state record.
// It runs after a failed merge attempt and returns a side-free wrapped error.
// The error has the same shape whichever side failed; only the wrapped git cause varies.
// side and mergeErr are logged via logger.Warn only, and neither reaches the returned error's message.
// If the reset itself fails, the record is kept: the pair is then in a state only MergeAbort can clear.
// The reset error is returned instead.
func (f *Fabric) selfAbortMergeAttempt(rec *Mutations, st *mergeState, side string, mergeErr error) error {
	logger.Warn("fabricengine: merge attempt failed mid-way; self-aborting", "side", side, "error", mergeErr)

	if err := f.resetMergeSides(rec, st.WarpStart); err != nil {
		return err
	}
	if err := f.deleteMergeState(); err != nil {
		return err
	}
	return fmt.Errorf("fabricengine: merge attempt failed: %w", mergeErr)
}
