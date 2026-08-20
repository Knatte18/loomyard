// merge.go implements fabric's public merge surface: MergeOptions/MergeResult, MergeIn, and Merge.
// MergeIn merges a source branch into the current pair's own warp and weft checkouts and surfaces
// any conflicts for resolution in that same worktree.
// Merge merges a source branch into a target pair the caller opened a handle on — squash-capable,
// expected conflict-free — synchronizing that target to its own upstream first and self-aborting to
// *ErrMergeInRequired on any conflict, since conflict resolution belongs in the source pair's own
// worktree, not the target's.

package fabricengine

import (
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

// MergeOptions controls a merge verb's behavior: Squash selects a squash merge (batch 4's Merge
// only — MergeIn never squashes), and Message overrides the conclude commit's message.
type MergeOptions struct {
	Squash  bool
	Message string
}

// MergeResult reports what a merge verb did on the pair. AlreadyUpToDate reports the degenerate
// no-op where both sides' resolved source SHA was already an ancestor of that side's HEAD.
// Conflicts lists the unified, worktree-relative paths a merge attempt left conflicted — empty,
// never nil, when there are none. Committed reports whether the pair now carries this merge's
// conclude-commit.
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

// MergeIn merges source into f's current pair: the warp side merges source itself, the weft side
// merges WeftBranchName(source), both resolved against the freshness rule (resolveMergeSources).
// Conflicts are a result state, not an error: a MergeIn call that produces conflicts returns
// (MergeResult{Conflicts: […]}, nil), leaving the pair mid-merge for resolution via MergeContinue or
// MergeAbort. MergeIn never squashes.
func (f *Fabric) MergeIn(source string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

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
	// the degenerate no-op, mirroring Commit's own precedent.
	warpStart, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve warp HEAD: %w", err)
	}
	weftStart, err := f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve weft HEAD: %w", err)
	}
	warpUpToDate, err := f.warp.IsAncestor(sources.warpSHA, warpStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify warp merge source: %w", err)
	}
	weftUpToDate, err := f.weft.IsAncestor(sources.weftSHA, weftStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify weft merge source: %w", err)
	}
	if warpUpToDate && weftUpToDate {
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

	weftOutcome, err := f.weft.MergeStart(sources.weftSHA, false)
	if err != nil {
		return MergeResult{}, f.selfAbortMergeAttempt(rec, st, "weft", err)
	}
	st.WeftOutcome = mergeOutcomeString(weftOutcome)
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	if weftOutcome != gitrepo.MergeAlreadyUpToDate {
		rec.Append(KindMergeStaged, f.weftPath, sources.weftSHA)
	}

	if warpOutcome == gitrepo.MergeConflicted || weftOutcome == gitrepo.MergeConflicted {
		var warpConflicts, weftConflicts []string
		if warpOutcome == gitrepo.MergeConflicted {
			warpConflicts, err = f.warp.ConflictedFiles()
			if err != nil {
				return MergeResult{}, err
			}
		}
		if weftOutcome == gitrepo.MergeConflicted {
			weftConflicts, err = f.weft.ConflictedFiles()
			if err != nil {
				return MergeResult{}, err
			}
		}

		unified, unmappable := unifyConflictPaths(warpConflicts, weftConflicts, anchorRel, wiredNames)
		if unmappable {
			logger.Warn("fabricengine: MergeIn produced unmappable conflict paths; self-aborting",
				"warp_conflicts", warpConflicts, "weft_conflicts", weftConflicts)
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

// Merge merges source into f's target pair — the pair whose worktree the caller opened f on, via
// lyxcwd.ResolveWorktree + fabricengine.Open, never a pair Fabric resolves topology for itself.
// It is squash-capable (opts.Squash, applied identically to both sides) and expects a conflict-free
// merge: any conflict on either side self-aborts both sides back to their pre-merge SHAs and returns
// *ErrMergeInRequired, since conflict resolution belongs in the source pair's own worktree, not the
// target's — the caller runs MergeIn there instead, then retries Merge.
// Before merging, Merge synchronizes the target to its own upstream (fetch, then a fast-forward-only
// advance per side that has one) — see syncSideBeforeMerge — so a target merely behind its upstream
// merges cleanly rather than guard-refusing.
// That sync runs INSIDE the weft write lock, not ahead of it: it mutates both checkouts at a point
// where no merge record exists yet, so the sibling verbs' record guard cannot serialize it and the
// lock is the only thing that can.
func (f *Fabric) Merge(source string, opts MergeOptions) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

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
	// reported reason set never discloses evaluation order. The guard stage is strictly read-only:
	// nothing mutates here, including the sync step, which runs only after every guard passed.
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

	// Pre-merge sync step: a recorded mutation, not a guard, and the first thing that touches either
	// checkout — every guard above has already passed.
	if err := f.syncSideBeforeMerge(rec, f.warp, f.warpPath, "warp"); err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: sync warp before merge: %w", err)
	}
	if err := f.syncSideBeforeMerge(rec, f.weft, f.weftPath, "weft"); err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: sync weft before merge: %w", err)
	}

	// Post-sync already-up-to-date probe: no record written, empty mutation record beyond whatever
	// the sync step itself just recorded — the sync's own advance is real upstream catch-up the merge
	// did not cause, so it stays in the record even on this early-return path.
	warpStart, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve warp HEAD: %w", err)
	}
	weftStart, err := f.weft.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve weft HEAD: %w", err)
	}
	warpUpToDate, err := f.warp.IsAncestor(sources.warpSHA, warpStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify warp merge source: %w", err)
	}
	weftUpToDate, err := f.weft.IsAncestor(sources.weftSHA, weftStart)
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: classify weft merge source: %w", err)
	}
	if warpUpToDate && weftUpToDate {
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

	weftOutcome, err := f.weft.MergeStart(sources.weftSHA, opts.Squash)
	if err != nil {
		return MergeResult{}, f.selfAbortMergeAttempt(rec, st, "weft", err)
	}
	st.WeftOutcome = mergeOutcomeString(weftOutcome)
	if err := f.saveMergeState(st); err != nil {
		return MergeResult{}, err
	}
	if weftOutcome != gitrepo.MergeAlreadyUpToDate {
		rec.Append(KindMergeStaged, f.weftPath, sources.weftSHA)
	}

	// Any conflict on either side (mappable or not — Merge, unlike MergeIn, never reports a
	// conflicted path, since the target pair is not where the caller resolves it) self-aborts: the
	// target pair is restored exactly, no conflicted state is ever left behind, and the conflicting
	// side is not disclosed — a fixed message, with the source traveling in the error's own field.
	if warpOutcome == gitrepo.MergeConflicted || weftOutcome == gitrepo.MergeConflicted {
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
// never fatal), re-resolves the upstream SHA, and — when HEAD is strictly behind it — advances via
// MergeFFOnly, never ResetHard: MergeFFOnly fails loudly on a raced divergence rather than silently
// discarding history the way a hard reset would.
// On an observed advance it records KindRepoAdvanced (Target = checkout path, Detail = the new SHA),
// the Fabric.Pull precedent (recordWarpAdvance) applied per-side here.
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
	if !behind {
		return nil
	}

	if err := repo.MergeFFOnly(upstreamSHA); err != nil {
		return err
	}
	rec.Append(KindRepoAdvanced, dir, upstreamSHA)
	return nil
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
