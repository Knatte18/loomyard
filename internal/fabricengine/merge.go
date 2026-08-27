// merge.go implements fabric's public merge surface: MergeOptions/MergeResult, MergeIn, and Merge.
// MergeIn merges a source branch into the current pair's own warp and weft checkouts and surfaces
// any conflicts for resolution in that same worktree.
// Merge merges a source branch into a target pair the caller opened a handle on — squash-capable,
// expected conflict-free — synchronizing that target to its own upstream first and self-aborting to
// *ErrMergeInRequired on any conflict, since conflict resolution belongs in the source pair's own
// worktree, not the target's.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// mergeNoConflicts is the empty-never-nil Conflicts value every MergeResult that carries no
// conflicts returns, so a caller's JSON never sees a "null" conflicts field.
var mergeNoConflicts = []string{}

// finalizeMergeResult stamps the two envelope-shaping fields a merge verb's result must carry on
// EVERY return: the accumulated mutation record, and a non-nil Conflicts slice.
// It runs from each verb's own deferred call on the named result, which is the only place that sees
// every return site at once. Setting the sentinel at the individual return sites instead is what
// left the property half-true: the success paths were spelled out one by one and the roughly two
// dozen `return MergeResult{}, err` guard refusals and mid-flight failures were not, so every error
// return of all four verbs handed back a nil slice that marshals as `"conflicts": null` — the exact
// null-versus-[] distinction MergeResult's own godoc promises a consumer never has to make.
func finalizeMergeResult(res *MergeResult, rec *Mutations) {
	res.Mutations = rec.Snapshot()
	if res.Conflicts == nil {
		res.Conflicts = mergeNoConflicts
	}
}

// recheckMergePreconditionsUnderLock re-verifies, immediately after the write lock is acquired, the
// three preconditions MergeIn/Merge could only observe racily before it: no fabric merge record, no
// foreign git-level merge state, and a clean pair.
// The record re-check is the only one of the three another FABRIC process can trip (every fabric
// merge writes a record before mutating); the foreign and dirty re-checks exist for the
// CONSTRAINTS-sanctioned human running plain git in the warp checkout, whose mid-wait `git merge`
// or tracked edit the pre-lock guard stage cannot see — the guard stage runs before the verb's two
// network fetches and before any lock wait, a window of real seconds.
// Acting on those stale answers was destructive in both arms: foreign conflicted state appearing
// mid-window made the failing MergeStart read as MergeConflicted, so fabric recorded — and, in
// Merge's conflict path, force-reset — a merge it never started; tracked dirt appearing mid-window
// made MergeStart fail genuinely and selfAbortMergeAttempt reset the dirt away under force: true.
// Foreign state is checked before dirtiness because a foreign conflicted index is ALSO
// tracked-dirty, and the foreign refusal is the one naming that state's actual remedy.
// The residual window between these re-checks and MergeStart itself stays open — no re-check closes
// a TOCTOU against an external actor — but the seconds-wide parts are covered.
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

	dirtyReasons, err := pairDirtyReason(f)
	if err != nil {
		return err
	}
	if len(dirtyReasons) > 0 {
		return newMergeGuardError(dirtyReasons)
	}
	return nil
}

// MergeOptions controls a merge verb's behavior: Squash selects a squash merge (batch 4's Merge
// only — MergeIn never squashes), and Message overrides the conclude commit's message.
type MergeOptions struct {
	Squash  bool
	Message string
}

// MergeResult reports what a merge verb did on the pair. AlreadyUpToDate reports the degenerate
// no-op where both sides' resolved source SHA was already an ancestor of that side's HEAD.
// Conflicts lists the unified, worktree-relative paths a merge attempt left conflicted — empty,
// never nil, when there are none, on every return of every verb including the error ones, which
// finalizeMergeResult is what guarantees. Committed reports whether the pair now carries this
// merge's conclude-commit.
// Both flags are derived from the merge-state record's own fields (mergeState.landedConcludeCommit
// and mergeState.bothSidesAlreadyUpToDate), never hardcoded per return site: a merge that
// fast-forwarded both sides fabricates no commit and reports Committed false, and a call that finds
// the work already done after taking the lock reports AlreadyUpToDate true — the same answer a
// strictly sequential run of the same calls gives.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type MergeResult struct {
	MutationRecord
	AlreadyUpToDate bool     `json:"already_up_to_date"`
	Conflicts       []string `json:"conflicts"`
	Committed       bool     `json:"committed"`
}

// MergeIn merges source into f's current pair's warp checkout; the weft side is not a merge
// participant. Conflicts are a result state, not an error: a MergeIn call that produces conflicts
// returns (MergeResult{Conflicts: […]}, nil), leaving the pair mid-merge for resolution via
// MergeContinue or MergeAbort. MergeIn never squashes.
func (f *Fabric) MergeIn(source string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	l, err := lyxcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve layout for %s: %w", f.warpPath, err)
	}
	anchorRel, wiredNames, err := resolveMergeGeometry(l)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve merge geometry for %s: %w", f.warpPath, err)
	}

	// Foreign-state refusal: git-level merge state that fabric did not itself start refuses the
	// whole call, leaving the foreign state untouched, but only when no fabric record already
	// covers it — a recorded merge takes the ordinary in-progress guard path below instead.
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

	// Aggregate every guard, evaluating each member regardless of an earlier failure, so the
	// reported reason set never discloses evaluation order.
	var reasons []string
	inProgressReasons, err := mergeInProgressReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, inProgressReasons...)

	dirtyReasons, err := pairDirtyReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, dirtyReasons...)

	detachedReasons, err := detachedHeadReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, detachedReasons...)

	sources, sourceReasons := resolveMergeSources(f, l, source)
	reasons = append(reasons, sourceReasons...)

	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons)
	}

	// Pre-lock already-up-to-date probe: no lock taken, no record written, empty mutation record —
	// the degenerate no-op, mirroring Commit's own precedent. The two HEAD reads here serve this
	// probe only; the record's starts are re-read under the lock below, where no concurrent writer
	// can stale them.
	warpStart, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	weftStart, err := f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	warpUpToDate, err := f.warp.IsAncestor(sources.warpSHA, warpStart)
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
	fileLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fileLock.Release() }()

	// Re-verify under the lock what the pre-lock guard could only observe racily
	// (recheckMergePreconditionsUnderLock): a record written by another process mid-wait would
	// otherwise be silently overwritten by saveMergeState below — its conclude landing the OTHER
	// merge's content under this record's name — and a human's mid-wait plain-git merge or tracked
	// edit would otherwise be adopted or force-reset by the MergeStart calls below.
	if err := f.recheckMergePreconditionsUnderLock(); err != nil {
		return MergeResult{}, err
	}

	// Re-read both starts under the lock, discarding the pre-lock reads: a concurrent writer that
	// held this lock while this call waited (a Commit landing new tips, most plausibly) makes the
	// pre-lock SHAs stale, and recording a stale start means MergeAbort would reset THROUGH that
	// writer's landed commits.
	warpStart, err = f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	weftStart, err = f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}

	st := &mergeState{
		Verb:       "merge-in",
		Source:     source,
		Squash:     false,
		Message:    "",
		WarpStart:  warpStart,
		WeftStart:  weftStart,
		WarpSource: sources.warpSHA,
		WeftSource: sources.weftSHA,
		StartedAt:  time.Now(),
	}
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	st.WeftOutcome = mergeOutcomeAlreadyUpToDate
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}

	warpOutcome, err := f.warp.MergeStart(sources.warpSHA, false)
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
		warpConflicts, err = f.warp.ConflictedFiles()
		if err != nil {
			return MergeResult{}, err
		}

		unified, unmappable := unifyConflictPaths(warpConflicts, nil, anchorRel, wiredNames)
		if unmappable {
			logger.Warn("fabricengine: MergeIn produced unmappable conflict paths; self-aborting",
				"warp_conflicts", warpConflicts)
			if err := f.resetMergeSides(rec, st.WarpStart, st.WeftStart); err != nil {
				return MergeResult{}, err
			}
			if err := f.deleteMergeState(); err != nil {
				return MergeResult{}, err
			}
			return MergeResult{}, &ErrUnmergeableState{}
		}

		return MergeResult{Conflicts: unified}, nil
	}

	// Both sides clean: conclude, record the pair's post-merge correspondence — even when one side
	// never moved — and clear the record.
	if err := concludeMergeSides(f, rec, st, ""); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
	}
	newWeftHEAD, err := f.weft.CurrentSHA()
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

// Merge merges source into f's target pair's warp checkout — the pair whose worktree the caller
// opened f on, via lyxcwd.ResolveWorktree + fabricengine.Open, never a pair Fabric resolves topology
// for itself; the weft side is not a merge participant.
// It is squash-capable (opts.Squash) and expects a conflict-free merge: a warp-side conflict
// self-aborts the warp side back to its pre-merge SHA and returns *ErrMergeInRequired, since conflict
// resolution belongs in the source pair's own worktree, not the target's — the caller runs MergeIn
// there instead, then retries Merge.
// Before merging, Merge synchronizes the target's warp side to its own upstream (fetch, then a
// fast-forward-only advance if it has one) — see syncSideBeforeMerge — so a target merely behind its
// upstream merges cleanly rather than guard-refusing.
// That sync step is also the SECOND half of the not-synced precondition, not merely a convenience:
// the guard-stage half (syncedToUpstreamReason) runs before anything in the call has fetched, so it
// cannot see a divergence this checkout has not learned about yet, and the sync step re-decides the
// same predicate on post-fetch knowledge and refuses with the same mergeReasonNotSynced. The warp side
// is the only half of that precondition the sync step now re-decides.
// That sync runs INSIDE the weft write lock, not ahead of it: it mutates the warp checkout at a point
// where no merge record exists yet, so the sibling verbs' record guard cannot serialize it and the
// lock is the only thing that can.
func (f *Fabric) Merge(source string, opts MergeOptions) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	l, err := lyxcwd.ResolveWorktree(f.warpPath)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve layout for %s: %w", f.warpPath, err)
	}

	// Foreign-state refusal: git-level merge state that fabric did not itself start refuses the
	// whole call, leaving the foreign state untouched, but only when no fabric record already
	// covers it — a recorded merge takes the ordinary in-progress guard path below instead.
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

	// Aggregate every guard, evaluating each member regardless of an earlier failure, so the
	// reported reason set never discloses evaluation order. The guard stage mutates no worktree, no
	// index, no branch tip and no fabric record — the sync step, which does, runs only after every
	// guard passed. It is not literally read-only, and saying so would be a claim a reader could
	// check and find false: resolveMergeSources runs a best-effort Fetch() on both sides, which
	// updates remote-tracking refs and FETCH_HEAD. Nothing a caller can observe as a change to the
	// pair moves here.
	var reasons []string
	inProgressReasons, err := mergeInProgressReason(f)
	if err != nil {
		return MergeResult{}, err
	}
	reasons = append(reasons, inProgressReasons...)

	dirtyReasons, err := pairDirtyReason(f)
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

	sources, sourceReasons := resolveMergeSources(f, l, source)
	reasons = append(reasons, sourceReasons...)

	if len(reasons) > 0 {
		return MergeResult{}, newMergeGuardError(reasons)
	}

	// The write lock is taken HERE, ahead of the sync step, because the sync step mutates both
	// checkouts and is therefore the first thing in this call that must be serialized against the
	// sibling weft-mutating verbs. It cannot rely on the merge record for that: the record does not
	// exist yet (it is written below), so mergeBlocksMutation reports false and Commit/Pull/Checkout/
	// Remove do not refuse. Running `git merge --ff-only` in the weft checkout while a concurrent
	// Commit holds this same lock and writes that same worktree is two uncoordinated writers on one
	// index. Only MergeIn can defer its acquisition, because MergeIn has no sync step and mutates
	// nothing before the record.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fileLock, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = fileLock.Release() }()

	// Re-verify under the lock what the pre-lock guard could only observe racily
	// (recheckMergePreconditionsUnderLock). The sync step below mutates both checkouts, so
	// proceeding over a record that appeared mid-wait would write into a pair some other merge is
	// mid-flight on — and proceeding over mid-wait foreign state or dirt would run `merge --ff-only`
	// and MergeStart into a checkout whose state the guard stage never saw.
	if err := f.recheckMergePreconditionsUnderLock(); err != nil {
		return MergeResult{}, err
	}

	// Pre-merge sync step: a recorded mutation, not a guard, and the first thing that touches either
	// checkout — every guard above has already passed.
	// It can still produce ONE guard refusal, mergeReasonNotSynced, because it is the first point in
	// the call that has fetched and can therefore see a divergence the pre-lock guard stage was too
	// early to know about (see syncSideBeforeMerge). That refusal is returned as-is rather than
	// wrapped: a caller matching *MergeGuardError must see the same error shape it would have seen had
	// the pre-lock guard caught the same divergence, and prefixing it with a sync-step message would
	// describe the step that DETECTED the problem instead of the precondition that failed.
	if err := f.syncSideBeforeMerge(rec, f.warp, f.warpPath, "warp"); err != nil {
		return MergeResult{}, wrapMergeSyncError(err)
	}

	// Post-sync already-up-to-date probe: no record written, empty mutation record beyond whatever
	// the sync step itself just recorded — the sync's own advance is real upstream catch-up the merge
	// did not cause, so it stays in the record even on this early-return path.
	warpStart, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	weftStart, err := f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD: %w", err)
	}
	warpUpToDate, err := f.warp.IsAncestor(sources.warpSHA, warpStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify merge source: %w", err)
	}
	if warpUpToDate {
		return MergeResult{AlreadyUpToDate: true, Conflicts: mergeNoConflicts}, nil
	}

	// Pre-merge SHAs are captured after the sync step, so MergeAbort returns the pair to its synced
	// state, never undoing a legitimate upstream advance.
	st := &mergeState{
		Verb:       "merge",
		Source:     source,
		Squash:     opts.Squash,
		Message:    opts.Message,
		WarpStart:  warpStart,
		WeftStart:  weftStart,
		WarpSource: sources.warpSHA,
		WeftSource: sources.weftSHA,
		StartedAt:  time.Now(),
	}
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	st.WeftOutcome = mergeOutcomeAlreadyUpToDate
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}

	warpOutcome, err := f.warp.MergeStart(sources.warpSHA, opts.Squash)
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

	// Any conflict on the warp side self-aborts: the target pair is restored exactly, no conflicted
	// state is ever left behind, and the conflicting side is not disclosed — a fixed message, with the
	// source traveling in the error's own field.
	if warpOutcome == gitrepo.MergeConflicted {
		if err := f.resetMergeSides(rec, st.WarpStart, st.WeftStart); err != nil {
			return MergeResult{}, err
		}
		if err := f.deleteMergeState(); err != nil {
			return MergeResult{}, err
		}
		return MergeResult{}, &ErrMergeInRequired{Source: source}
	}

	// Both sides clean: conclude (message precedence opts.Message, already carried in st.Message,
	// then git's own prepared MERGE_MSG/SQUASH_MSG), record the pair's post-merge correspondence —
	// even when one side never moved — and clear the record.
	if err := concludeMergeSides(f, rec, st, ""); err != nil {
		return MergeResult{}, err
	}

	newWarpHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, err
	}
	newWeftHEAD, err := f.weft.CurrentSHA()
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

// syncSideBeforeMerge implements Merge's pre-merge sync step for one side: dir/repo with no upstream
// is a vacuous no-op, mirroring Fabric.Pull's own no-upstream rule.
// A side with an upstream runs a best-effort Fetch() (failure tolerated and logged via logger.Warn,
// never fatal), re-resolves the upstream SHA, and classifies the side against it EXHAUSTIVELY —
// equal, behind, ahead, or diverged — rather than testing "behind" alone and treating everything
// else as nothing to do.
// Behind advances via MergeFFOnly, never ResetHard: MergeFFOnly fails loudly on a raced divergence
// rather than silently discarding history the way a hard reset would. On an observed advance it
// records KindRepoAdvanced (Target = checkout path, Detail = the new SHA), the Fabric.Pull precedent
// (recordWarpAdvance) applied per-side here. Equal and ahead are both genuine no-ops.
//
// Diverged returns the mergeReasonNotSynced guard refusal, and that arm is what makes the guard set's
// not-synced promise true rather than merely usually-true. syncedToUpstreamReason runs in Merge's
// guard stage, BEFORE anything in the call has fetched — resolveMergeSources' own best-effort Fetch
// and this function's are both later — so it decides from whatever remote-tracking state the checkout
// happened to already carry. An operator who has not fetched since someone else pushed has an @{u}
// that is still an ancestor of their own HEAD, so the guard sees "ahead", passes, and the merge lands
// on a target genuinely diverged from its upstream: reproduced live end-to-end, ok:true with
// committed:true and `rev-list --left-right --count HEAD...@{u}` reporting 3 1 afterwards.
// The information was not missing, only late — THIS function fetches and then re-resolves @{u}, so by
// the time it runs the divergence is knowable. Collapsing ahead and diverged into one `return nil`
// discarded it. Refusing here re-decides the same predicate on post-fetch knowledge, which leaves the
// pre-lock guard as a cheap fast path rather than the only line of defence.
// The refusal carries the same fixed, side-free reason whichever side produced it, so returning on the
// first diverged side discloses no more than the aggregated guard stage does.
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

// wrapMergeSyncError shapes syncSideBeforeMerge's error for Merge's own return: a *MergeGuardError
// passes through untouched, so the caller sees the ordinary aggregated-precondition shape, and every
// other error is wrapped with the sync-step context that names where it happened.
func wrapMergeSyncError(err error) error {
	var guardErr *MergeGuardError
	if errors.As(err, &guardErr) {
		return guardErr
	}
	return fmt.Errorf("fabricengine: sync checkout before merge: %w", err)
}

// selfAbortMergeAttempt implements the a-genuine-MergeStart-error-mid-attempt-self-aborts-
// symmetrically Shared Decision: it resets both sides to their captured pre-merge SHAs and deletes
// the merge-state record, then returns a side-free wrapped error — identical shape whichever side
// failed, the only variation being the wrapped git cause itself.
// side and mergeErr are logged internally via logger.Warn only; neither reaches the returned error's
// message.
// If the reset itself fails, the record is deliberately retained (the pair is then in a state only
// MergeAbort can clear) and the reset error is returned instead.
func (f *Fabric) selfAbortMergeAttempt(rec *Mutations, st *mergeState, side string, mergeErr error) error {
	logger.Warn("fabricengine: merge attempt failed mid-way; self-aborting", "side", side, "error", mergeErr)

	if err := f.resetMergeSides(rec, st.WarpStart, st.WeftStart); err != nil {
		return err
	}
	if err := f.deleteMergeState(); err != nil {
		return err
	}
	return fmt.Errorf("fabricengine: merge attempt failed: %w", mergeErr)
}
