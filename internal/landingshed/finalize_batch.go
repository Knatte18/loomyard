// finalize_batch.go implements Finalize's lead of a landing batch over the queued runs.
// The leader builds their chain of landing commits and gates it in its own task worktree.
// It lands the chain with one guarded move.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/landingqueue"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// batchMember is one run of a landing batch: its queue entry and its own seams.
type batchMember struct {
	entry landingqueue.Entry
	deps  Deps
	// leftOut marks a run the batch no longer lands, which the next build of the chain drops.
	leftOut bool
}

// stands reports whether the run's owner process lives, no pause covers it and its task branch is where its entry says.
// A failed read answers false.
func (m batchMember) stands(queue *landingqueue.Store) bool {
	if !queue.Live(m.entry) {
		return false
	}
	if covered := m.deps.PullRequest.PauseCovered; covered != nil {
		paused, err := covered()
		if err != nil {
			logger.Warn("landingshed: read whether a pause covers a queued run failed, so the batch leaves it out", "producer", finalizeName, "slug", m.entry.Slug, "cause", err)
			return false
		}
		if paused {
			return false
		}
	}
	return m.atEntryHead()
}

// atEntryHead reports whether the run's task branch is at the head its entry names; a failed read answers false.
func (m batchMember) atEntryHead() bool {
	head, err := m.deps.TaskHead()
	if err != nil {
		logger.Warn("landingshed: read the task head of a queued run failed, so the batch leaves it out", "producer", finalizeName, "slug", m.entry.Slug, "cause", err)
		return false
	}
	return head == m.entry.TaskHead
}

// takeBatch returns the runs one landing batch takes, the caller's own first.
// The others are the queued runs with no outcome that still stand, in join order, up to landing.batch_max runs in all.
// A run whose seams cannot be built is skipped.
// It returns none for a run with no queue entry.
func (fz *Finalize) takeBatch(ctx context.Context) ([]batchMember, error) {
	own, found, err := fz.deps.Queue.Read(fz.deps.Slug)
	if err != nil {
		return nil, fmt.Errorf("landingshed: %s: read the queue entry: %w", finalizeName, err)
	}
	if !found {
		return nil, nil
	}
	entries, err := fz.deps.Queue.Entries()
	if err != nil {
		return nil, fmt.Errorf("landingshed: %s: read the landing queue: %w", finalizeName, err)
	}
	members := []batchMember{{entry: own, deps: fz.deps}}
	size := fz.deps.Config.BatchSize()
	for _, entry := range entries {
		if err := cancelErr(ctx, finalizeName); err != nil {
			return nil, err
		}
		if len(members) >= size {
			break
		}
		if entry.Slug == own.Slug || entry.Outcome != "" || !fz.deps.Queue.Live(entry) {
			continue
		}
		deps, err := fz.deps.MemberDeps(entry.Pair)
		if err != nil {
			logger.Warn("landingshed: a queued run's seams cannot be built, so the batch skips it", "producer", finalizeName, "slug", entry.Slug, "cause", err)
			continue
		}
		member := batchMember{entry: entry, deps: deps}
		if member.stands(fz.deps.Queue) {
			members = append(members, member)
		}
	}
	return members, nil
}

// leadBatch lands members, the caller's own run first, under the parent lock the caller holds.
// It builds a chain of one landing commit per run on the parent's head, gates the chain commit and lands it.
// A run whose work does not merge onto the chain gets the conflict outcome, and one whose work it holds is left out.
// A run left out later, or a parent that moved, rebuilds and gates again, at most landing.batch_max times.
// A failed gate is bisected to the run that fails it.
// It reports whether it settled the pass.
// A false answer leaves the caller to land alone.
// A true one with a nil ending is a pass that freed the lock.
func (fz *Finalize) leadBatch(ctx context.Context, parent parentMerger, members []batchMember, notice string) (bool, *ending) {
	defer fz.removeDetachMarker()
	failed := func(what string, err error) (bool, *ending) {
		return true, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	leader := members[0].entry.Slug
	for builds := 0; builds <= fz.deps.Config.BatchSize(); builds++ {
		members = slices.DeleteFunc(members, func(m batchMember) bool { return m.leftOut })
		if len(members) < 2 || members[0].entry.Slug != leader {
			return false, nil
		}
		base, err := parent.HeadSHA()
		if err != nil {
			return failed("read the parent head", err)
		}
		links := make([]fabricengine.ChainLink, len(members))
		for i, m := range members {
			links[i] = fabricengine.ChainLink{Slug: m.entry.Slug, TaskHead: m.entry.TaskHead, Message: m.entry.Message, TrueMerge: m.entry.TrueMerge}
		}
		chain, err := fz.deps.BuildChain(base, links)
		if err != nil {
			return failed("build the landing chain", err)
		}
		// The caller's own link leaves it with no chain to lead, and sets no outcome on its entry.
		if slices.Contains(chain.Conflicted, leader) {
			fz.freeLock()
			return true, nil
		}
		if slices.Contains(chain.Held, leader) {
			return true, fz.landHeld(ctx, notice)
		}
		for i := range members {
			slug := members[i].entry.Slug
			if slices.Contains(chain.Conflicted, slug) {
				if err := fz.deps.Queue.SetOutcome(slug, landingqueue.OutcomeConflict, ""); err != nil {
					return failed("set the conflict outcome of "+slug, err)
				}
				members[i].leftOut = true
			}
			if slices.Contains(chain.Held, slug) {
				members[i].leftOut = true
			}
		}
		if len(chain.Commits) < 2 {
			return false, nil
		}

		result, reason, end := fz.gateChainCommit(ctx, chain.Commits[len(chain.Commits)-1].Commit)
		if end != nil {
			return true, end
		}
		if reason != "" {
			// No test judged the code, so there is no run to blame.
			if result.Unjudged() {
				return true, halt(ctx, finalizeName, reason)
			}
			// A bisection that ends nothing and freed no lock moved nothing, so the chain is built again.
			if end := fz.bisectBatch(ctx, parent, base, chain.Commits, members, result, notice); end != nil || fz.lockFreed {
				return true, end
			}
			continue
		}
		landed, end := fz.landPrefix(ctx, parent, base, chain.Commits, members, notice)
		if landed || end != nil {
			return true, end
		}
	}
	logger.Info("landingshed: the batch was rebuilt as often as it may be, so the run lands alone", "producer", finalizeName)
	return false, nil
}

// bisectBatch finds the run that fails the batch gate by gating prefixes of chain, whose whole failed with failed.
// The run whose commit first fails gets the failed outcome with that commit, and the runs after it get none.
// The runs before it are landed.
// A failing run that is the caller's own lands nothing, and goes on to its fixer.
// A nil ending with no lock freed is a prefix that was not landed, whose chain must be built again.
// The runs from the failing one on are marked left out.
func (fz *Finalize) bisectBatch(ctx context.Context, parent parentMerger, base string, chain []fabricengine.ChainCommit, members []batchMember, failed verifytree.Result, notice string) *ending {
	// The empty prefix counts as passing and the whole chain as failing.
	passing, failing := 0, len(chain)
	for failing-passing > 1 {
		middle := (passing + failing) / 2
		result, reason, end := fz.gateChainCommit(ctx, chain[middle-1].Commit)
		switch {
		case end != nil:
			return end
		case reason == "":
			passing = middle
		case result.Unjudged():
			return halt(ctx, finalizeName, reason)
		default:
			failing, failed = middle, result
		}
	}

	culprit := chain[failing-1]
	if err := fz.deps.Queue.SetOutcome(culprit.Slug, landingqueue.OutcomeFailed, culprit.Commit); err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: set the failed outcome of %s: %w", finalizeName, culprit.Slug, err))
	}
	logger.Warn("landingshed: the batch gate fails on a run's work", "producer", finalizeName, "slug", culprit.Slug, "commit", culprit.Commit, "log", failed.Log)
	for _, link := range chain[passing:] {
		for i := range members {
			if members[i].entry.Slug == link.Slug {
				members[i].leftOut = true
			}
		}
	}
	if passing == 0 {
		fz.removeDetachMarker()
		return fz.failAsCulprit(ctx, base, failed)
	}
	_, end := fz.landPrefix(ctx, parent, base, chain[:passing], members, notice)
	return end
}

// failAsCulprit takes a run whose own work fails the batch gate on base to its fixer, without gating again.
// It merges base into the task worktree under the lock, which gives the tree that failed.
// It then writes the landing failure record from result and runs the fixer as after a failing gate.
// It returns nil when the fixer's change was let through.
func (fz *Finalize) failAsCulprit(ctx context.Context, base string, result verifytree.Result) *ending {
	if end := fz.mergeParent(ctx, base); end != nil {
		return end
	}
	fz.gate.recordFailure(result)
	rec, reason, err := fz.readAttempts(FixerAttemptsPath(fz.deps.RecordsDir, finalizeName))
	if err != nil {
		return fail(ctx, finalizeName, err)
	}
	if reason != "" {
		return halt(ctx, finalizeName, reason)
	}
	if rec, err = fz.rebaseAfterParentMerge(rec); err != nil {
		return fail(ctx, finalizeName, err)
	}
	if rec.Spent >= fz.deps.Config.FinalizeFixAttempts {
		return fz.endSpent(ctx, rec, fz.failedGateLog(result.Log))
	}
	return fz.runFixer(ctx, rec)
}

// gateChainCommit runs the batch gate over commit, checked out with a detached HEAD in the caller's task worktree.
// It puts the worktree back on the task branch on every way out.
// The detach marker is written before the checkout and left standing.
// It returns the gate's result and the reason it failed, empty for a pass.
// A non-nil ending ends the call.
// A worktree that cannot be put back is a halting Stuck naming its dirty paths.
func (fz *Finalize) gateChainCommit(ctx context.Context, commit string) (result verifytree.Result, reason string, end *ending) {
	failed := func(what string, err error) *ending {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	if err := fz.deps.Detach.WriteMarker(); err != nil {
		return result, "", failed("write the detach marker", err)
	}
	left, err := fz.deps.TaskHead()
	if err != nil {
		return result, "", failed("read the task head", err)
	}
	if err := fz.keepMergeInBase(left); err != nil {
		return result, "", &ending{err: err}
	}

	defer func() {
		// The base file is only an aid to the cleanup, so a failed write of it never stops the restore.
		keepErr := fz.keepMergeInBase(commit)
		if err := fz.deps.Detach.Restore(); err != nil {
			dirty, dirtyErr := fz.gate.dirtyPaths(finalizeName, "after the batch gate")
			if dirtyErr != nil {
				logger.Warn("landingshed: read the dirty paths after a failed restore failed", "producer", finalizeName, "cause", dirtyErr)
			}
			end = halt(ctx, finalizeName, fmt.Sprintf("the task worktree could not be put back on its branch from the chain commit %s: %v; dirty paths: %s; way forward: clean them in the task worktree, then `lym loom resume`", commit, err, dirtyPathList(dirty)))
			return
		}
		if keepErr != nil {
			end = &ending{err: keepErr}
			return
		}
		if restored := fz.removeLeftovers(ctx); restored != nil {
			end = restored
			return
		}
		cleanReason, err := fz.gate.clean(finalizeName, "after the batch gate")
		if cleaned := gateEnding(ctx, finalizeName, cleanReason, err); cleaned != nil {
			end = cleaned
		}
	}()

	if err := fz.deps.Detach.Checkout(commit); err != nil {
		return result, "", failed("check the chain commit out", err)
	}
	if end := fz.removeLeftovers(ctx); end != nil {
		return result, "", end
	}
	cleanReason, err := fz.gate.clean(finalizeName, "at the chain commit")
	if end := gateEnding(ctx, finalizeName, cleanReason, err); end != nil {
		return result, "", end
	}
	gate := newBatchGate(fz.deps)
	gate.onFlakyPass = func(pass flakyPass) {
		for _, failure := range pass.failures {
			logger.Warn("landingshed: a failure of the batch gate did not repeat", "producer", finalizeName, "commit", commit, "failure", failure, "log", pass.log)
		}
	}
	reason, _, result, err = gate.checkResult(ctx, finalizeName, fz.deps.ParentBranch)
	if err != nil {
		return result, "", fail(ctx, finalizeName, err)
	}
	return result, reason, nil
}

// removeDetachMarker removes the detach marker once the task worktree is back on a branch.
// A worktree still detached keeps it, so the resume that follows can restore it.
// A failure is logged.
func (fz *Finalize) removeDetachMarker() {
	if fz.deps.Detach.RemoveMarker == nil {
		return
	}
	if fz.deps.DetachState != nil {
		state, err := fz.deps.DetachState()
		if err != nil {
			logger.Warn("landingshed: read whether the task worktree is detached failed, so the detach marker stays", "producer", finalizeName, "cause", err)
			return
		}
		if state.Detached {
			return
		}
	}
	if err := fz.deps.Detach.RemoveMarker(); err != nil {
		logger.Warn("landingshed: remove the detach marker failed", "producer", finalizeName, "cause", err)
	}
}

// judgeMember reports whether the caller may land member's run, from reads over that run's own seams alone.
// It answers false for a run that has ended, stopped or moved to another machine.
// It also answers false for one whose landing is blocked or goes through GitHub.
// So it does for one that would need an action before it lands, and for one with a fixer change in its current entry.
// A failed read is returned as the error, with false.
func (fz *Finalize) judgeMember(ctx context.Context, member batchMember) (bool, error) {
	deps := member.deps
	if changed, err := (&Finalize{deps: deps}).holdsFixerChange(); err != nil || changed {
		return false, err
	}
	if deps.Reconcile != nil {
		report, err := reconcileReport(ctx, finalizeName, deps.Reconcile)
		if err != nil || report.Unreachable {
			return false, err
		}
		for _, entry := range report.Entries {
			switch entry.State {
			case fabriclayout.EntryEnded, fabriclayout.EntryStopped, fabriclayout.EntryOwnedElsewhere:
				return false, nil
			}
		}
		// A task branch behind origin would be fast-forwarded by the run's own reconcile first.
		for _, branch := range report.Branches {
			if branch.Slug == member.entry.Slug && branch.Repo == fabriclayout.RepoCode && branch.State == fabriclayout.BranchFastForward {
				return false, nil
			}
		}
	}
	if deps.ParentOwner != nil {
		parent, err := deps.ParentOwner()
		if err != nil || !parent.Here {
			return false, err
		}
	}
	if deps.LandBlockers != nil {
		blockers, err := deps.LandBlockers()
		if err != nil || len(blockers) > 0 {
			return false, err
		}
	}
	if deps.OwnedElsewhere != nil && !deps.PushSkipped {
		_, _, elsewhere, err := deps.OwnedElsewhere()
		if err != nil || elsewhere {
			return false, err
		}
	}

	needsGitHub, err := (&Finalize{deps: deps}).needsGitHubOnDisk()
	if err != nil || needsGitHub {
		return false, err
	}
	pr := deps.PullRequest
	if pr.Mode == nil {
		return true, nil
	}
	mode, err := pr.Mode()
	if err != nil {
		return false, err
	}
	origin, err := pr.Origin()
	if err != nil {
		return false, err
	}
	hasCommits, err := pr.TaskHasCommits()
	if err != nil {
		return false, err
	}
	_, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return false, err
	}
	beam := NotABeam
	if pr.BeamRecord != nil {
		beam = BeamWithoutPullRequest
	}
	decision, err := resolveWithParentRule(ctx, finalizeName, deps, mode, beam, origin, hasCommits, found)
	if err != nil {
		return false, err
	}
	return !decision.Open && decision.Halt == "", nil
}

// landPrefix moves the parent from base onto the last commit of prefix and records the landing of each of its runs.
// The first commit of prefix is the caller's own, and members holds the runs of every commit.
// It first runs the caller's owner checks and judges the other runs; it marks one that no longer lands left out.
// It reports false with a nil ending when it moved nothing and the chain must be built again:
// for a run left out and for a parent that moved from base.
// A true answer, or a non-nil ending, ends the call with that ending.
func (fz *Finalize) landPrefix(ctx context.Context, parent parentMerger, base string, prefix []fabricengine.ChainCommit, members []batchMember, notice string) (bool, *ending) {
	failed := func(what string, err error) (bool, *ending) {
		return true, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}
	if end := fz.ownerChecks(ctx, false); end != nil {
		return true, end
	}
	if end := fz.recheckLock(ctx); end != nil {
		return true, end
	}

	index := make([]int, len(prefix))
	for i, link := range prefix {
		index[i] = slices.IndexFunc(members, func(m batchMember) bool { return m.entry.Slug == link.Slug })
	}
	for _, at := range index[1:] {
		ok, err := fz.judgeMember(ctx, members[at])
		if err != nil {
			if cerr := cancelErr(ctx, finalizeName); cerr != nil {
				return true, &ending{err: cerr}
			}
			logger.Warn("landingshed: a queued run could not be judged, so the batch leaves it out", "producer", finalizeName, "slug", members[at].entry.Slug, "cause", err)
		}
		if !ok {
			members[at].leftOut = true
		}
	}
	leftOut := false
	for i, at := range index {
		if i == 0 {
			if !members[at].atEntryHead() {
				members[at].leftOut = true
			}
		} else if !members[at].leftOut && !members[at].stands(fz.deps.Queue) {
			members[at].leftOut = true
		}
		leftOut = leftOut || members[at].leftOut
	}
	if leftOut {
		return false, nil
	}

	// A sibling run's uncommitted status record would trip the guard of the move below.
	if end := fz.commitParentRecords(ctx); end != nil {
		return true, end
	}
	head, err := parent.HeadSHA()
	if err != nil {
		return failed("read the parent head", err)
	}
	if head != base {
		return false, nil
	}

	mark := landingqueue.MoveMark{Base: base}
	for i, link := range prefix {
		member := members[index[i]]
		records, err := member.deps.RecordsHead()
		if err == nil {
			records, err = member.deps.RecordsToHold(records)
		}
		if err != nil {
			logger.Warn("landingshed: read the records commit to hold failed, so the landing holds the one its entry names", "producer", finalizeName, "slug", link.Slug, "cause", err)
			records = member.entry.RecordsHead
		}
		mark.Members = append(mark.Members, landingqueue.MarkMember{Slug: link.Slug, Commit: link.Commit, TaskHead: member.entry.TaskHead, Records: records})
	}
	if err := fz.deps.Queue.WriteMark(mark); err != nil {
		return failed("mark the move", err)
	}
	removeMark := func() {
		if err := fz.deps.Queue.RemoveMark(); err != nil {
			logger.Warn("landingshed: remove the move mark failed; the next holder of the lock finishes it", "producer", finalizeName, "cause", err)
		}
	}
	fz.removeDetachMarker()
	moved, err := parent.MoveCodeTo(base, prefix[len(prefix)-1].Commit)
	var guardErr *fabricengine.MergeGuardError
	switch {
	case errors.As(err, &guardErr):
		removeMark()
		return true, stuck(ctx, finalizeName, fz.dirtyParentReason(parent, guardErr))
	case err != nil:
		removeMark()
		return failed("move the parent onto the chain", err)
	case !moved:
		removeMark()
		return false, nil
	}

	// A hub with pushes skipped records no landing: each run's own call commits its records and marks its task done.
	for _, member := range mark.Members {
		if !fz.deps.PushSkipped {
			if err := fz.deps.WriteLedgerLanding(member.Slug, member.Commit, member.Records); err != nil {
				return failed("write the landing of "+member.Slug, err)
			}
		}
		if err := fz.deps.Queue.SetLanded(member.Slug, member.TaskHead, member.Commit); err != nil {
			return failed("set the landed outcome of "+member.Slug, err)
		}
	}
	removeMark()

	own := prefix[0].Commit
	diffBase, err := fz.deps.FirstParent(own)
	if err != nil {
		logger.Warn("landingshed: read the first parent of the landing commit failed", "producer", finalizeName, "cause", err)
		diffBase = ""
	}
	fz.recordLanding(own, diffBase, nil)
	return true, fz.pushParent(ctx, own, nil, notice)
}
