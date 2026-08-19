// merge.go implements fabric's first public merge surface: MergeOptions/MergeResult and MergeIn,
// which merges a source branch into the current pair's warp and weft checkouts and surfaces any
// conflicts for resolution in the worktree.
// Merge itself — the target-pair verb — is batch 4; this file ships only what MergeIn needs.

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
// never nil, when there are none. Committed reports whether the merge concluded with a landed
// commit on the sides that needed one.
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
	anchorRel, wiredNames, err := resolveMergeGeometry(f.warpPath)
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
		Verb:      "merge-in",
		Source:    source,
		Squash:    false,
		Message:   "",
		WarpStart: warpStart,
		WeftStart: weftStart,
		StartedAt: time.Now(),
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

	return MergeResult{Committed: true, Conflicts: mergeNoConflicts}, nil
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
