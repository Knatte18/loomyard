// mergelifecycle.go implements the merge lifecycle quartet — MergeContinue, MergeAbort,
// MergeInProgress — and the shared conclude phase concludeMergeSides that MergeIn (merge.go) and
// MergeContinue both land through.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// concludeMergeSides lands the conclude-commit on both sides of an in-progress merge, under the
// caller's already-held write lock: warp first, then weft, in that fixed order.
// The weft arm is retained deliberately, for cross-binary record compatibility, not because a weft
// conclude can still be produced: MergeIn and Merge now write WeftOutcome as mergeOutcomeAlreadyUpToDate
// from their first checkpoint onward, so this arm's own st.WeftOutcome != mergeOutcomeAlreadyUpToDate
// guard skips it on every record this binary produces, and it is reachable only for a fabric-merge.json
// record a binary predating this change left behind, where WeftOutcome can legitimately be "staged".
// A side whose recorded outcome is fast_forwarded or up_to_date is skipped — no commit is fabricated
// on a no-op side — and a side whose committed SHA is already recorded is skipped too, for
// idempotency across a resumed MergeContinue.
// Idempotency also covers a conclude the record never learned about: a crash (or I/O failure)
// between a side's `git commit` and the record re-save leaves that side's commit landed with its
// recorded SHA still empty, and re-running `git commit` there fails forever on a clean tree.
// sideConcludeAlreadyLanded detects that shape — by git parentage, not by HEAD movement alone — and
// the landed commit is adopted into the record instead of re-committed, so MergeContinue finishes
// the states MergeAbort's concludeLandedReason refuses to destroy.
// The two are deliberately not exact mirrors: concludeLandedReason over-refuses on an ambiguous
// read, while adoption refuses on one, so a squash record and a HEAD that moved for some other
// reason stay honestly stuck rather than being claimed as a successful conclude.
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
		sha, landed, err := sideConcludeAlreadyLanded(f.warp, st.WarpStart, st.WarpSource, st.Squash)
		if err != nil {
			return err
		}
		if !landed {
			if err := f.warp.MergeConclude(effectiveMsg); err != nil {
				logger.Warn("fabricengine: merge conclude failed", "side", "warp", "error", err)
				return &ErrMergeIncomplete{}
			}
			sha, err = f.warp.CurrentSHA()
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
		sha, landed, err := sideConcludeAlreadyLanded(f.weft, st.WeftStart, st.WeftSource, st.Squash)
		if err != nil {
			return err
		}
		if !landed {
			if err := f.weft.MergeConclude(effectiveMsg); err != nil {
				logger.Warn("fabricengine: merge conclude failed", "side", "weft", "error", err)
				return &ErrMergeIncomplete{}
			}
			sha, err = f.weft.CurrentSHA()
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

// sideConcludeAlreadyLanded reports whether one side already carries THIS merge's conclude-commit
// under a record that never learned about it, returning the landed SHA for adoption when it does.
// The shape it detects is the crash between a side's `git commit` and the record re-save: the
// caller's arm has already established the side's outcome is staged/conflicted with no recorded
// conclude SHA, so a commit really landed there and only the bookkeeping is missing.
//
// Adoption demands positive, discriminating evidence — it is a CLAIM, not a refusal, and the two
// resolve an ambiguity in opposite directions. concludeLandedReason's sideConcludeMayHaveLanded
// reads "HEAD moved off the recorded pre-merge start" to REFUSE MergeAbort, which is safe when the
// read is ambiguous because the cost of over-refusing is a stuck merge the operator can inspect.
// Reading the same ambiguous signal to claim a successful adopt is not safe, and was a real defect:
// the signal is satisfied by ANY commit landed on the checkout while the record was live, so an
// operator's plain `git merge --abort` followed by one unrelated commit made MergeContinue adopt
// that unrelated commit, report committed:true naming it, and delete the record — leaving the merge
// source un-merged with nothing left to inspect. A silent false success.
//
// The evidence is git's own parentage, and it is exact rather than heuristic. fabric starts a
// non-squash merge with `git merge --ff --no-commit <sourceSHA>`, so git writes exactly that SHA
// into MERGE_HEAD and the resulting conclude-commit has exactly two parents: the pre-merge start
// first, the merge source second. All five of these must hold before a commit is adopted:
//   - HEAD moved off the recorded pre-merge start,
//   - no live MERGE_HEAD (the merge is finished, not still concludable),
//   - the record is not a squash (see below),
//   - the record carries a resolved source SHA for this side,
//   - HEAD is a merge commit with EXACTLY two parents: the recorded start first, the recorded
//     source SHA second.
//
// The parent test is exact equality on both position and arity, deliberately not "at least two
// parents, with the source somewhere among them". That looser reading admitted a commit fabric can
// never produce: an operator who discards the staged merge and then merges the recorded source
// TOGETHER with an unrelated branch — `git merge <source> <other>` — lands a genuine octopus whose
// first parent is the recorded start and whose second is the recorded source, and the loose test
// adopted it. fabric then reported committed:true naming that commit, recorded correspondence, and
// deleted the record, while the checkout carried <other>'s content that no side of this merge ever
// brought in and that no merge_staged entry accounts for.
// Requiring exactly two parents rejects nothing fabric or its documented recovery route creates:
// `git commit --no-edit` over a live MERGE_HEAD, and `reset --hard <start>` followed by
// `git merge <source>`, both yield exactly two parents in exactly that order.
//
// A squash conclude is deliberately never adopted. `git merge --squash` writes no MERGE_HEAD and
// its conclude is an ORDINARY one-parent commit, so it carries no evidence distinguishing it from
// any other commit an operator might have landed — the non-squash predicate cannot be silently
// inherited there. Refusing keeps the honest, pre-fix behaviour: the re-run `git commit` fails on
// the clean tree and *ErrMergeIncomplete comes back with the record retained.
// An empty recorded source SHA — a record written by a binary older than this evidence — refuses
// for the same reason: no evidence is not satisfied evidence.
//
// Adoption is recorded as KindMergeCommitted by the caller even though this call ran no `git
// commit`: the commit is this merge's own conclude, and the resumed call is the first one whose
// record can honestly account for it.
// Probe failures propagate raw rather than as *ErrMergeIncomplete — nothing has been mutated yet,
// matching MergeContinue's own pre-lock probes.
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

// unifiedRemainingConflicts maps the paths still unmerged on each side onto the same unified,
// worktree-relative list MergeIn's own conflict result reports, so MergeContinue's unresolved-conflicts
// refusal can name them instead of only asserting that some exist.
// Without it the refusal was un-actionable in the one flow that most needs it: an operator who resolved
// and staged some of the reported paths and not all of them gets "unresolved conflicts remain" naming
// nothing, cannot re-run merge-in to reprint the list (a merge is already in progress), and — for a
// weft-side conflict — cannot see the remaining path from the visible worktree at all, since plain
// `git status` there does not reach through the junction. The only route back to the list was raw git
// inside the weft checkout, the one place the Fabric illusion says an operator never has to look.
//
// It is best-effort by design and never turns a precondition failure into a different error: a geometry
// read that fails is logged and yields no paths, because the caller is already returning the refusal
// that matters and replacing it with an unrelated I/O error would hide the actual precondition.
// An unmappable path yields NO list at all rather than a partial one. A list that silently omitted a
// path the operator must still resolve would mislead them about what is left — the same judgment
// unifyConflictPaths' own unmappable arm makes when it refuses to report a misleading path.
func (f *Fabric) unifiedRemainingConflicts(warpConflicts, weftConflicts []string) []string {
	l, err := lyxcwd.ResolveWorktree(f.warpPath)
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
// mergeReasonUnresolvedConflicts, a record whose attempt never reached both sides refuses with
// mergeReasonAttemptIncomplete, and a side about to be concluded whose live git merge is no longer the
// one the record describes refuses with mergeReasonRecordedMergeGone (recordedMergeGoneReason); all
// three are aggregated into one error, so which precondition failed never discloses evaluation order.
// The combined write lock is acquired BEFORE the record is read and the guards run — never after —
// so no concurrent lifecycle verb can retire or advance the record between what this call checked
// and what it acts on (see the in-body comment for the raced-MergeContinue shape that ordering
// closes). It then runs concludeMergeSides with msg as the optional
// message override, records correspondence, deletes the merge-state record, and returns a
// MergeResult whose Committed and AlreadyUpToDate are BOTH read off the record's own fields, never
// hardcoded per return site — Committed true when the pair carries this merge's conclude-commit and
// false when both sides only fast-forwarded or never moved, AlreadyUpToDate true when the attempt
// had recorded both sides already carrying the resolved source.
// AlreadyUpToDate is reachable here and not merely defensive: MergeIn/Merge's own pre-lock
// up-to-date probe is deliberately unlocked, so a call that loses that race records up_to_date on
// both sides, and a crash before its conclude leaves exactly that record for MergeContinue to
// resume. Hardcoding false there made the resumed call disagree with what a strictly sequential run
// of the same calls reports.
func (f *Fabric) MergeContinue(msg string) (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	// The lock comes FIRST, before the record is read and before any guard is evaluated. Evaluating
	// them ahead of the lock read a state another lock holder could change before this call could
	// act on it: a concurrent MergeContinue that concluded and deleted the record between this
	// call's guard reads and its lock acquisition left it concluding from a stale record — adopting
	// (and thereby resurrecting) a record the winner had already retired, and answering
	// committed:true where a strictly sequential run of the same two calls answers
	// *ErrNoMergeInProgress. Everything below the acquisition sees a state no other fabric writer
	// can be mutating.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
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

	warpConflicts, err := f.warp.ConflictedFiles()
	if err != nil {
		return MergeResult{}, err
	}
	weftConflicts, err := f.weft.ConflictedFiles()
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

	newWarpHEAD, err := f.warp.CurrentSHA()
	if err != nil {
		return MergeResult{}, fmt.Errorf("fabricengine: resolve checkout HEAD after merge: %w", err)
	}
	newWeftHEAD, err := f.weft.CurrentSHA()
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

// MergeAbort discards an in-progress merge, restoring the warp side to its pre-merge SHA — not both
// sides, since the weft is no longer a merge participant; see resetMergeSides for why it still takes
// and resets a weft SHA argument: with no record and no foreign git merge state it returns
// *ErrNoMergeInProgress; with no record but foreign state present it returns *ErrForeignMergeState,
// the same rule as MergeContinue.
// An attempt whose conclude may already have landed on the warp side refuses with a *MergeGuardError
// carrying mergeReasonConcludeLanded (concludeLandedReason), before anything is reset: restoring
// from the recorded pre-merge SHA would discard a commit that really landed, and in the
// MergeIn-with-conflicts flow that commit carries the operator's own resolutions. MergeContinue is
// the recovery for that shape, and it is idempotent across the resumed run.
// The combined write lock is acquired BEFORE the record is read and that guard runs — never after —
// so a conclude landing while this call waits for the lock is seen by the guard instead of being
// destroyed by the reset (see the in-body comment for the raced shape that ordering closes).
// It then resets the warp side via resetMergeSides — including a
// fast-forwarded side and a side that never moved — then deletes the merge-state record and returns
// MergeResult{} (Committed false).
func (f *Fabric) MergeAbort() (res MergeResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { finalizeMergeResult(&res, rec) }()

	// The lock comes FIRST, before the record is read and before the conclude-landed guard is
	// evaluated. This ordering is what makes the guard able to see a conclude that lands while this
	// call waits: with the guard ahead of the lock, a concurrent MergeContinue that concluded and
	// deleted the record between guard-eval and lock-acquire left this call to resetMergeSides
	// (force: true) the freshly landed conclude commits — precisely the destruction
	// concludeLandedReason exists to prevent — with deleteMergeState tolerating the record's absence
	// so nothing ever noticed. Everything below the acquisition sees a state no other fabric writer
	// can be mutating.
	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return MergeResult{}, err
	}
	fl, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
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

	if err := f.resetMergeSides(rec, st.WarpStart, st.WeftStart); err != nil {
		return MergeResult{}, err
	}
	if err := f.deleteMergeState(); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{Conflicts: mergeNoConflicts}, nil
}

// MergeInProgress reports whether fabric has a merge in progress on this pair: mergeRecordExists()'s
// bare boolean, no MutationRecord (a read-only probe stays off the embed table by design).
// It never consults foreignMergeStatePresent and never errors on foreign state: it answers "does
// fabric have a merge in progress", which foreign plain-git state does not make true.
func (f *Fabric) MergeInProgress() (bool, error) {
	return f.mergeRecordExists()
}
