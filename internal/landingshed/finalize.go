// finalize.go implements the Finalize producer.
// Under the parent lock it merges the parent branch into the task worktree and judges the merge, then merges the task branch into the parent pair's own worktree and pushes the parent branch.
// It reports only what the shedengine.ShedProducer seam can carry: a verdict, an output pointer whose Reason the engine persists, and an error.
//
// This producer's merge critical section is the parent-side merge call (step 4 below),
// and a future regeneration step folds into that same critical section rather than becoming a separate row.
// No hook, no injectable interface, and no scaffolded span with an empty body is built for that today,
// because an interface with a permanently-nil implementation is exactly the hypothetical-requirement design this codebase avoids.
//
// A merged pull request does not make this producer redundant:
// the remote service only ever sees one of the two sides, so the other side's branch was never in the pull request at all, and after a remote-side merge the visible side reports already-up-to-date while the other genuinely merges.
//
// A landing the pull-request resolver says goes through a pull request merges the run's open pull request on GitHub, under the parent lock and before the parent-side merge; see finalize_pr.go.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// finalizeName is the producer name Finalize's log lines and error text carry.
const finalizeName = "Finalize"

// parentMerger is the narrow seam Finalize holds the opened parent pair's handle behind, mirroring
// the resolver seam's own reasoning: production has exactly one way to obtain it --
// deps.OpenParentFabric, adapted by NewFinalize's parentOpener closure -- and the seam exists so
// this package's own in-package tests can substitute a fake without a second public construction
// path anyone outside could reach for by mistake.
type parentMerger interface {
	// Merge merges source into the parent pair's own worktree. See fabricengine.Fabric.Merge.
	Merge(source string, opts fabricengine.MergeOptions) (fabricengine.MergeResult, error)
	// PushBranch pushes the parent pair's own branch to its upstream. See
	// fabricengine.Fabric.PushBranch.
	PushBranch(opts fabricengine.SyncOptions) (fabricengine.PushResult, error)
	// HeadSHA reads the parent pair's own current head commit. See fabricengine.Fabric.HeadSHA.
	HeadSHA() (string, error)
	// MidMerge reads the merge the parent pair has parked, if any. See fabricengine.Fabric.MidMerge.
	MidMerge() (fabricengine.MidMergeState, error)
	// FastForwardPair moves the parent pair to its upstream as a fast-forward only. See fabricengine.Fabric.FastForwardPair.
	FastForwardPair(opts fabricengine.SyncOptions) (fabricengine.FastForwardResult, error)
	// CodeDirtyPaths reads the parent pair's code checkout and its tracked uncommitted paths. See fabricengine.Fabric.CodeDirtyPaths.
	CodeDirtyPaths() (checkout string, paths []string, err error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentMerger.
var _ parentMerger = (*fabricengine.Fabric)(nil)

// Finalize is the ShedProducer that merges the parent branch into the task worktree under the parent lock, then merges the task branch into the parent pair's own worktree.
type Finalize struct {
	deps     Deps
	resolver resolver
	// parentOpener adapts deps.OpenParentFabric's concrete *fabricengine.Fabric return type onto
	// the parentMerger seam. It is called once per Call, never at construction: the pair
	// constructor it wraps stat-checks a layout that may not be wired yet, so opening eagerly
	// would fail before the run's own preflight has confirmed anything is wired.
	parentOpener func() (parentMerger, error)
	// gate is the post-merge verify gate the merged tree passes through.
	// A zero value carries no command, so struct-literal tests run without a gate.
	gate verifyGate
	// premerge clears the merge-in a previous attempt left parked.
	// A zero value runs no probe, so struct-literal tests run without it.
	premerge preMerge
	// guard holds the parent lock and tests whether the parent moved.
	// A zero value takes no lock and reads no tips, so struct-literal tests run without it.
	guard parentGuard
	// lockFreed reports that the current locked pass freed the parent lock for an agent session, so Call runs the pass again.
	lockFreed bool
	// parentHeadMerged is the parent pair's head read right before the parent branch was last merged into the task worktree, the base of the landed diff.
	// It is empty when this process read none, as in a Finalize resumed past the merge.
	parentHeadMerged string
}

var _ shedengine.ShedProducer = (*Finalize)(nil)

// NewFinalize constructs a Finalize from deps, rejecting a nil OpenFabric or OpenParentFabric closure, a nil Lock.Store, a nil Lock.Owner and a nil ReadParentTips up front with a distinct error each -- rather than nil-panicking at call time.
// A nil parent opener is a construction error rather than a silent no-op: this producer never creates a
// worktree to merge into, so without a working opener it could never reach the parent pair at all.
//
// It builds its resolver at construction time from the told values and stores it behind the
// package's own resolver seam, exactly as the sibling producer's constructor does and for the same
// reason -- the resolver's constructor performs no I/O, so a mis-wired resolver is a construction
// error rather than a first-call surprise.
func NewFinalize(deps Deps) (*Finalize, error) {
	if deps.OpenFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenFabric must not be nil")
	}
	if deps.OpenParentFabric == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.OpenParentFabric must not be nil")
	}
	if deps.DescriptionPath == "" {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.DescriptionPath must not be empty")
	}
	if deps.Lock.Store == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.Lock.Store must not be nil")
	}
	if deps.Lock.Owner == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.Lock.Owner must not be nil")
	}
	if deps.ReadParentTips == nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: Deps.ReadParentTips must not be nil")
	}

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: open pair: %w", err)
	}

	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:         fabricHandle,
		Shuttle:        deps.Shuttle,
		WorktreeRoot:   deps.WorktreeRoot,
		ScratchDir:     deps.ScratchDir,
		StencilsDir:    deps.StencilsDir,
		ParentName:     deps.ParentName,
		LanguageSkills: deps.LanguageSkills,
		ConflictSpec:   deps.Config.Conflict,
		Registry:       deps.Registry,
		Timeout:        time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: build resolver: %w", err)
	}

	gate := newVerifyGate(deps)
	gate.recorder = &failureRecorder{failingTests: deps.FailingTests, traceFailure: deps.TraceFailure, head: deps.TaskHead}
	return &Finalize{
		deps:     deps,
		resolver: res,
		gate:     gate,
		premerge: newPreMerge(deps),
		guard:    newParentGuard(deps),
		parentOpener: func() (parentMerger, error) {
			h, err := deps.OpenParentFabric()
			if err != nil {
				return nil, err
			}
			return h, nil
		},
	}, nil
}

// Call runs one Finalize iteration.
//
// Finalize takes the parent lock first, before anything that reads or moves the parent, and releases it on every way out:
// a paused lock wait or an unreadable lock record is a halting Stuck, and a landing that reaches Done, a Stuck and an error all free the lock.
// Under that lock it runs the owner checks: a run origin ended or stopped is Stuck, and a run or parent another machine owns, an unreachable origin and a failed owner read halt.
// A parent already holding the task's work skips the owner checks and the unreachable halt, and an unreachable origin then proceeds.
// It then opens the parent pair and syncs it: a pair that is missing, dirty or mid-merge, or a parent that cannot be fast-forwarded to its upstream, is Stuck with a way forward before any merge.
// It then merges the parent branch into the task worktree through its resolver, which resolves a conflict with the conflict session under the lock, and gates the merged tree, whether the merge conflicted or was large.
// The `landing` gate judges the merged tree, and a failing gate runs the merge fixer in finalize mode; a fixer change over the caps is settled in place by the fix review and the fixer, and Finalize never sends the run back to an earlier row.
// The lock is freed before the fixer or a fix review starts, and once their change is let through Finalize takes it again at the front of the queue and runs every step under the lock again, from the pull-request step on.
// A Finalize resumed with such a change already let through starts at the front too.
// After a merge-in-required error from the parent-side merge it re-runs the merge and its gate once under the same lock, so a code-side parent commit that arrived in between is merged, gated and landed.
//
// Immediately before each landing attempt it re-reads the parent lock record, so a lock something released lands nothing and is a halting Stuck.
// The re-read and the landing are not atomic: a release between them is caught only by the parent push's rejection.
// The landing commit carries the change description and, after a blank line, the credit block, when the config yields one.
//
// Right after the lock it follows the pull-request resolver, asked with GitHub's rules for the parent branch: a no closes a recorded open pull request, drops the record and lands locally;
// a rules read that fails for a reason other than a transient or token one halts when a pull request is recorded, and closes nothing;
// a yes needs the recorded pull request and halts without one, and halts on one closed without the operator's ending, except under a parent that takes its pull request at once, which reopens a closed one only after the owner checks, just before the merge;
// a closed pull request whose record carries the operator's ending is reopened and merged on GitHub like an open one, and a merged one lands through the already-landed path.
// An open pull request is merged on GitHub just before the parent-side merge, with the pushed task head as its expected head, and the parent pair is fast-forwarded to the merge, so the parent-side merge then finds the parent holding the task's work.
// A parent whose beam.yaml pull_request is merge needs no record: Finalize opens the pull request itself at that point and squash-merges it with no review.
// A no marked TrueMerge never squashes the parent-side merge, so the parent's history keeps the task branch's merge commits.
// A parent push the remote refuses as a protected branch is Stuck naming that setting, unless the landing already went through it.
//
// Finalize is idempotent over an already-landed parent: when the parent already contains the task's
// changes -- an operator squash-merged the task on the remote, or an earlier Finalize landed and
// failed later -- the empty squash is classified as already-up-to-date, so Fabric.Merge commits
// nothing and returns no error. Finalize then proceeds exactly as for a fresh landing: the board
// task is marked done, the push is a no-op, and the verdict is Done.
func (fz *Finalize) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, finalizeName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// Step 1a: parse the final-summary artifact before any commit, any catch-up merge-in, and any
	// parent-side mutation, so a missing or malformed artifact returns an error and the run never
	// half-lands. This is never Stuck and never a silent fallback to an unset MergeOptions.Message --
	// the composed message below is load-bearing for step 4's conclude commit.
	summary, err := summaryparser.Parse(fz.deps.DescriptionPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: parse change description: %w", finalizeName, err)
	}

	// Step 1a2: read the engines the task ran under, for the same reason: the credit block is part of step 4's message.
	credit, err := fz.deps.creditBlock()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", finalizeName, err)
	}
	// The regression check is read here for the same reason, and follows the body in both messages.
	verdict, err := fz.deps.driftVerdict()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", finalizeName, err)
	}
	summary.Body = withSection(summary.Body, verdict)
	landingMessage := summary.LandingMessage(credit)

	// Step 1b: commit the product's own status file, so the pair carries no tracked modification
	// when the merge guard below runs. This must happen inside Call rather than once at bootstrap:
	// Shed rewrites that file on every transition, including the persist a resumed run makes
	// immediately before calling this producer, so any earlier commit is already stale by now.
	//
	// A commit failure maps to a returned error, never to Stuck: a git fault is infrastructure
	// rather than a merge precondition a human resolves by editing the branch, and it is the same
	// disposition loomshed's own commit decorators already give a failing commit seam.
	if fz.deps.CommitStatus != nil {
		if err := fz.deps.CommitStatus(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: commit status file: %w", finalizeName, err)
		}
	}

	// Steps 1b2 to 7 run under the parent lock: once, and again each time a fixer or fix review ran with the lock freed, queued then at the front, as a run resumed after its fixer is.
	front, err := fz.returningFromFixer()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	for ; ; front = true {
		outcome, out, err, again := fz.landUnderLock(ctx, front, summary.Title, bodyWithCredit(summary.Body, credit), landingMessage)
		if !again {
			return outcome, out, err
		}
	}
}

// returningFromFixer reports whether the row's attempt record shows a fixer's change already let through, which is a run coming back from its fixer and queues at the front.
// A record that does not decode reads as no, and the gate then halts naming it.
func (fz *Finalize) returningFromFixer() (bool, error) {
	rec, reason, err := fz.readAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName))
	if err != nil {
		return false, err
	}
	return reason == "" && rec.Judged != "", nil
}

// landUnderLock runs one pass of Call's steps from the parent lock to the landing: it takes the lock, at the front of the queue when front is set, and releases it on every way out.
// title, body and landingMessage are the GitHub merge's title and message and the landing commit's message.
// again is true when the pass freed the lock for a fixer or fix review whose change was let through, so Call runs the pass again;
// otherwise the other values are what Call returns.
func (fz *Finalize) landUnderLock(ctx context.Context, front bool, title, body, landingMessage string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	fz.lockFreed = false
	end := func(outcome shedengine.Outcome, out shedengine.OutputPointer, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return outcome, out, err, false
	}
	requeue := func() (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return "", shedengine.OutputPointer{}, nil, true
	}

	// Step 1b2: take the parent lock first, before anything reads or moves the parent, and release it on every way out of this pass.
	// A lock a dead Finalize left is taken over by the acquire, since the lock store grants a record whose owner process no longer lives.
	lockReason, err := fz.guard.ensure(ctx, finalizeName, front)
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return end("", shedengine.OutputPointer{}, cerr)
		}
		return end("", shedengine.OutputPointer{}, err)
	}
	if lockReason != "" {
		return end(stuckVerdict(ctx, finalizeName, lockReason, true))
	}
	defer fz.guard.release(finalizeName)

	// Step 1b2b: under the lock, decide how the landing relates to the run's pull request, following the resolver PR-Review follows.
	landing, outcome, out, err, stop := fz.pullRequestLanding(ctx)
	if stop {
		return end(outcome, out, err)
	}

	// Step 1b3: judge the run against origin under the lock, and halt unless this machine owns the run and its parent.
	// A run whose Finalize already merged into the local parent here is never read as ended or owned elsewhere, so a Finalize re-entered after its done mark proceeds.
	if outcome, out, err, stop := fz.ownerChecks(ctx); stop {
		return end(outcome, out, err)
	}

	// Step 1b3b: under the same lock, halt a beam's landing while a member or a forked pair remains under it.
	if outcome, out, err, stop := fz.landCheck(ctx); stop {
		return end(outcome, out, err)
	}

	// Step 1b4: open the parent pair, which this producer never creates, and sync it before any merge.
	parentHandle, err := fz.parentOpener()
	if err != nil {
		return end(fz.stuckOrCancelled(ctx, fmt.Sprintf("no live pair for parent branch %q: %v; way forward: re-create the parent pair, then run \"lym loom resume\"", fz.deps.ParentBranch, err), "error", err))
	}
	syncReason, err := fz.syncParent(ctx, parentHandle)
	if outcome, out, err, stop := fz.gateStop(ctx, syncReason, err); stop {
		return end(outcome, out, err)
	}

	// Step 1c: read the task's per-worktree config changes before any parent-side mutation,
	// so the diff is taken against the parent as the task forked from it;
	// the notice it yields is queued only at Done.
	notice := fz.configChangeNotice()

	// landed ends the pass once the parent-side merge is done: it pushes the parent and, for a beam whose recorded pull request is spent, drops that record after the push answered Done, since the landing has happened by then.
	landed := func() (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		outcome, out, err := fz.pushParent(ctx, parentHandle, notice, landing.atOnce || landing.trueMerge)
		if err == nil && outcome == shedengine.Done && landing.dropBeamRecord {
			fz.dropBeamRecord()
		}
		return end(outcome, out, err)
	}

	// Steps 2 and 3: merge the parent branch into the task worktree and judge the merge, then gate the merged tree.
	if outcome, out, err, done := fz.mergeAndGate(ctx, parentHandle); done {
		return end(outcome, out, err)
	}
	if fz.lockFreed {
		return requeue()
	}

	// Step 2b: commit the parent pair's own run records,
	// so a sibling run's uncommitted status record does not trip the parent-side merge guard.
	if outcome, out, err, done := fz.commitParentRecords(ctx); done {
		return end(outcome, out, err)
	}

	// Step 5: re-check the lock immediately before the landing, so a release since the acquire lands nothing.
	if outcome, out, err, stop := fz.recheckLock(ctx); stop {
		return end(outcome, out, err)
	}

	// Step 5b: a landing through an open pull request merges it on GitHub first, once, so the parent-side merge below finds the parent holding the task's work.
	// The retry after a merge-in-required error never comes back here, since the GitHub merge is done.
	if landing.merge {
		if outcome, out, err, stop := fz.mergeOnGitHub(ctx, parentHandle, landing, title, body); stop {
			return end(outcome, out, err)
		}
	}

	// Step 6: the parent-side merge, this producer's own merge critical section. Message is set
	// whether or not Config.Squash is true -- it is the conclude-commit message for both merge
	// shapes, so gating it on Squash would leave the non-squash landing commit with an unset message.
	// A parent that takes its pull request at once always squashes on GitHub, so the parent-side merge squashes too, whatever landing.yaml says.
	// A true merge never squashes, since a squash would drop the merged commit from the parent's history.
	mergeOpts := fabricengine.MergeOptions{Squash: (fz.deps.Config.Squash || landing.atOnce) && !landing.trueMerge, Message: landingMessage}
	_, mergeErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
	if mergeErr == nil {
		fz.markTaskDone()
		fz.recordLanding(parentHandle)
		return landed()
	}

	// Retry: on the merge-in-required error, re-run steps 2 to 5 under the same lock and retry the parent-side merge exactly once.
	// Step 2 merges the parent again, so a code-side commit that arrived between the check and the parent-side merge is merged, gated and landed.
	var mergeInRequired *fabricengine.ErrMergeInRequired
	if errors.As(mergeErr, &mergeInRequired) {
		if outcome, out, err, done := fz.mergeAndGate(ctx, parentHandle); done {
			return end(outcome, out, err)
		}
		if fz.lockFreed {
			return requeue()
		}
		if outcome, out, err, done := fz.commitParentRecords(ctx); done {
			return end(outcome, out, err)
		}
		if outcome, out, err, stop := fz.recheckLock(ctx); stop {
			return end(outcome, out, err)
		}
		_, retryErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
		if retryErr == nil {
			fz.markTaskDone()
			fz.recordLanding(parentHandle)
			return landed()
		}
		mergeErr = retryErr
	}

	// Steps 6-7: a guard error carrying the dirty-worktree reason is recognized through the
	// accessor rather than a matched string, and surfaced verbatim -- never stash, never reset,
	// never retry. Any other merge error is stuck with the error surfaced.
	var guardErr *fabricengine.MergeGuardError
	if errors.As(mergeErr, &guardErr) && guardErr.WorktreeDirty() {
		return end(fz.stuckOrCancelled(ctx, fz.dirtyParentReason(parentHandle, guardErr)))
	}
	return end(fz.stuckOrCancelled(ctx, fmt.Sprintf("parent-side merge failed: %v", mergeErr), "error", mergeErr))
}

// freeLock frees the parent lock before an agent session that works on the task worktree alone, so other runs land on the parent meanwhile.
// Call then takes the lock again at the front of the queue and runs its locked pass again, merging the parent anew.
// A Finalize with no lock store frees nothing and goes on in the same pass.
func (fz *Finalize) freeLock() {
	if fz.lockFreed || fz.guard.lock.Store == nil {
		return
	}
	fz.guard.release(finalizeName)
	fz.lockFreed = true
	logger.Info("landingshed: freed the parent lock for an agent session; the run takes it again after", "producer", finalizeName, "branch", fz.deps.ParentBranch)
}

// syncParent probes the opened parent pair and, with a push, brings its local branch level with its upstream; it returns a Stuck reason, or "" when Finalize may go on.
// A pair mid-merge or holding tracked dirt in its code checkout is Stuck before anything else, whether the parent is behind, equal, without an upstream or under the push skip.
// Under the told push skip it then neither fetches nor fast-forwards.
// Otherwise a parent branch ahead of or diverged from its upstream is Stuck, since Finalize never rewrites the parent.
// A parent behind its upstream is fast-forwarded in its own pair, which moves it to the upstream tip and touches no local commit;
// an upstream that is no longer a fast-forward of the pair is Stuck.
// A parent already holding the task's work is past its parent-side merge and is left as it is.
func (fz *Finalize) syncParent(ctx context.Context, parent parentMerger) (string, error) {
	branch := fz.deps.ParentBranch
	const resume = `then run "lym loom resume"`

	if fz.deps.ParentHoldsTaskWork != nil {
		held, err := fz.deps.ParentHoldsTaskWork()
		if err != nil {
			return "", fmt.Errorf("landingshed: %s: read whether the parent branch already holds the task's work: %w", finalizeName, err)
		}
		if held {
			return "", nil
		}
	}

	state, err := parent.MidMerge()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the merge state of the parent pair of %q: %w", finalizeName, branch, err)
	}
	if state.Kind != fabricengine.MidMergeNone {
		return fmt.Sprintf("a merge is in progress in the parent pair of %q; way forward: finish or abort that merge in the parent pair, %s", branch, resume), nil
	}
	checkout, dirty, err := parent.CodeDirtyPaths()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the dirty paths of the parent pair of %q: %w", finalizeName, branch, err)
	}
	if len(dirty) > 0 {
		return DirtyParentRefusal(checkout, dirty), nil
	}
	if fz.deps.PushSkipped {
		return "", nil
	}

	tips, err := fz.deps.ReadParentTips(true)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", finalizeName, branch, err)
	}
	switch tips.Relation {
	case fabricengine.ParentNoUpstream, fabricengine.ParentEqual:
		return "", nil
	case fabricengine.ParentAhead:
		return fmt.Sprintf("the local parent branch %q is ahead of its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabricengine.ParentDiverged:
		return fmt.Sprintf("the local parent branch %q has diverged from its upstream; way forward: push or reconcile %q in its own pair, %s", branch, branch, resume), nil
	case fabricengine.ParentBehind:
	default:
		return "", fmt.Errorf("landingshed: %s: unknown relation %q of parent branch %q to its upstream", finalizeName, tips.Relation, branch)
	}

	result, err := parent.FastForwardPair(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped})
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: fast-forward the parent pair of %q: %w", finalizeName, branch, err)
	}
	switch result.Refusal {
	case "":
		return "", nil
	case fabricengine.FastForwardDirty:
		return fmt.Sprintf("the parent pair of %q has uncommitted changes; way forward: commit or stash them in the parent pair, %s", branch, resume), nil
	case fabricengine.FastForwardMergeInProgress:
		return fmt.Sprintf("a merge is in progress in the parent pair of %q; way forward: finish or abort that merge in the parent pair, %s", branch, resume), nil
	case fabricengine.FastForwardNotFastForward:
		return fmt.Sprintf("the upstream of %q is no longer a fast-forward of the parent pair; way forward: reconcile %q with its upstream in its own pair, %s", branch, branch, resume), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown fast-forward refusal %q for parent branch %q", finalizeName, result.Refusal, branch)
	}
}

// maxListedDirtyPaths caps how many dirty paths a refusal lists before it counts the rest.
const maxListedDirtyPaths = 20

// gotoResumeWayForward is the closing step every dirty-parent way forward ends in.
const gotoResumeWayForward = `"lym loom goto --to Finalize" and "lym loom resume" in the task worktree`

// dirtyPathList renders paths Go-quoted and joined by ", ", the first maxListedDirtyPaths of them, ending in ", and N more" when there are more.
// Quoting keeps a path holding a space or a comma one item, and the form holds no line break.
func dirtyPathList(paths []string) string {
	shown := paths
	if len(shown) > maxListedDirtyPaths {
		shown = shown[:maxListedDirtyPaths]
	}
	quoted := make([]string, len(shown))
	for i, p := range shown {
		quoted[i] = strconv.Quote(p)
	}
	list := strings.Join(quoted, ", ")
	if more := len(paths) - len(shown); more > 0 {
		list += fmt.Sprintf(", and %d more", more)
	}
	return list
}

// dirtyParentSubject names the parent's code checkout and the tracked paths dirty in it.
func dirtyParentSubject(checkout string, paths []string) string {
	return fmt.Sprintf("the parent's code checkout %s has uncommitted tracked paths: %s", checkout, dirtyPathList(paths))
}

// DirtyParentRefusal words a refusal that the parent's code checkout holds uncommitted tracked paths:
// the checkout, the paths (capped, quoted) and the way forward when the dirt is the only obstacle.
func DirtyParentRefusal(checkout string, paths []string) string {
	return fmt.Sprintf("%s; way forward: commit or discard them in %s with plain git, then %s",
		dirtyParentSubject(checkout, paths), checkout, gotoResumeWayForward)
}

// dirtyParentReason words the Stuck reason for a parent-side merge the dirty guard refused, in one of four forms by the other reasons the guard error carries:
// a merge already in progress, a detached HEAD, a branch not synced to its upstream, or dirt alone.
func (fz *Finalize) dirtyParentReason(parentHandle parentMerger, guardErr *fabricengine.MergeGuardError) string {
	checkout := guardErr.Checkout
	text := guardErr.Error()
	switch {
	case guardErr.AlreadyInProgress():
		reason := fmt.Sprintf("%s; the parent's code checkout is %s", text, checkout)
		wayForward := gotoResumeWayForward
		state, err := parentHandle.MidMerge()
		switch {
		case err != nil:
			logger.Warn("landingshed: read the parent pair's parked merge failed", "producer", finalizeName, "cause", err)
			wayForward = fz.parkedMergeWayForward(checkout)
		case state.Kind == fabricengine.MidMergeParked:
			reason += fmt.Sprintf("; the parked fabric merge is a %s of %s in the parent pair", state.Verb, state.Source)
			wayForward = fz.parkedMergeWayForward(checkout)
		}
		return reason + "; way forward: " + wayForward
	case guardErr.DetachedHead():
		return fmt.Sprintf("%s: %s; way forward: check out the parent branch %q in %s with plain git, and when the checkout refuses to carry the changes, set them aside first with plain git and restore them on the branch after it; then commit them on the parent branch or discard them in %s with plain git; then %s",
			text, dirtyParentSubject(checkout, guardErr.DirtyPaths), fz.deps.ParentBranch, checkout, checkout, gotoResumeWayForward)
	case guardErr.NotSynced():
		return fmt.Sprintf("%s: %s; way forward: commit them on the parent branch or discard them in %s with plain git; then push or reconcile the parent branch %q in its own pair; then %s",
			text, dirtyParentSubject(checkout, guardErr.DirtyPaths), checkout, fz.deps.ParentBranch, gotoResumeWayForward)
	default:
		return text + ": " + DirtyParentRefusal(checkout, guardErr.DirtyPaths)
	}
}

// parkedMergeWayForward words the way forward out of a fabric merge parked in the parent pair.
// "--abort" hard-resets the parent's code checkout, and a plain git commit or stash there would take the staged merge with it, so the operator saves a copy of their own change outside the checkout first.
func (fz *Finalize) parkedMergeWayForward(checkout string) string {
	return fmt.Sprintf(`the party that started that merge, or the orch or the operator when that party is gone, concludes it with "lym fabric merge --continue" or discards it with "lym fabric merge --abort" in the parent pair, then %s; "--abort" hard-resets the parent's code checkout to the merge's start, so the operator first saves a copy of any change of their own in %s outside it, since a plain git commit or stash there would take the staged merge with it; a session lym refuses the verb from reports status: FAILED and the orch runs it`,
		gotoResumeWayForward, checkout)
}

// configChangeNotice composes the one-line notice about the task's per-worktree config changes, key by key, or "" when there is nothing to report or no seam is wired.
// Per changed file and its module it names each key whose tip value differs from both the fork point and the parent, with the three values;
// a file that changes no key, such as a comments-only edit, and a key the parent already holds are not named.
// Finalize never carries those files to the parent,
// so the notice tells the orchestrator to re-apply each change meant for the parent.
// A read or parse failure is logged and reported in the notice instead of stopping the landing.
func (fz *Finalize) configChangeNotice() string {
	if fz.deps.ConfigChanges == nil {
		return ""
	}
	changes, err := fz.deps.ConfigChanges()
	if err != nil {
		logger.Warn("landingshed: read config changes failed", "producer", finalizeName, "cause", err)
		line := fmt.Sprintf("loom: the config changes of task branch %q could not be read: %v; diff its config files against the parent branch %q by hand",
			fz.deps.TaskBranch, err, fz.deps.ParentBranch)
		return strings.Join(strings.Fields(line), " ")
	}

	var keyed, unread []string
	for _, file := range changes.Files {
		contents := changes.Contents[file]
		module := strings.TrimSuffix(path.Base(file), path.Ext(file))
		keys, err := configengine.ChangedKeys(contents.Base, contents.Tip, contents.Parent)
		if err != nil {
			logger.Warn("landingshed: compare config keys failed", "producer", finalizeName, "file", file, "cause", err)
			unread = append(unread, fmt.Sprintf("%s (module %s): %v", file, module, err))
			continue
		}
		var named []string
		for _, key := range keys {
			named = append(named, fmt.Sprintf("%s.%s: base %s, tip %s, parent %s", module, key.Key, key.Base, key.Tip, key.Parent))
		}
		if len(named) > 0 {
			keyed = append(keyed, fmt.Sprintf("%s (module %s): %s", file, module, strings.Join(named, "; ")))
		}
	}

	var parts []string
	if len(keyed) > 0 {
		parts = append(parts, fmt.Sprintf("task branch %q changed per-worktree config keys since it forked from %q: %s; landing does not carry them to the parent branch, so re-apply each change meant for the parent on the parent's copy of the same file",
			fz.deps.TaskBranch, fz.deps.ParentBranch, strings.Join(keyed, "; ")))
	}
	if len(unread) > 0 {
		parts = append(parts, fmt.Sprintf("the config files of task branch %q changed since it forked from %q could not be compared key by key: %s; diff them against the parent branch by hand",
			fz.deps.TaskBranch, fz.deps.ParentBranch, strings.Join(unread, "; ")))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(strings.Fields("loom: "+strings.Join(parts, "; also, ")), " ")
}

// markTaskDone marks the task's board entry done right after a parent-side merge succeeds, ahead of
// the push, so a skipped or failed push still leaves the task done: the change is on the parent once
// the merge lands. A failure is a logged warning and never changes the verdict.
func (fz *Finalize) markTaskDone() {
	if fz.deps.MarkTaskDone == nil {
		return
	}
	if err := fz.deps.MarkTaskDone(); err != nil {
		logger.Warn("landingshed: mark board task done failed", "producer", finalizeName, "cause", err)
	}
}

// recordLanding hands the parent pair's head, the commit the landing put on the parent branch, to Deps.RecordLanding, and then the landed diff's size to Deps.RecordLandedSize.
// A failed head read or record is logged and never changes the verdict.
func (fz *Finalize) recordLanding(parentHandle parentMerger) {
	if fz.deps.RecordLanding == nil && fz.deps.RecordLandedSize == nil {
		return
	}
	commit, err := parentHandle.HeadSHA()
	if err != nil {
		logger.Warn("landingshed: read the landing commit failed", "producer", finalizeName, "cause", err)
		return
	}
	if fz.deps.RecordLanding != nil {
		if err := fz.deps.RecordLanding(commit); err != nil {
			logger.Warn("landingshed: record the landing commit failed", "producer", finalizeName, "cause", err)
		}
	}
	fz.recordLandedSize(commit)
}

// recordLandedSize hands Deps.RecordLandedSize the parent head merged into the task worktree and the landing commit.
// A Finalize that holds no such head records nothing, which is logged at Info; a failed record is logged at Warn and never changes the verdict.
func (fz *Finalize) recordLandedSize(landing string) {
	if fz.deps.RecordLandedSize == nil {
		return
	}
	if fz.parentHeadMerged == "" {
		logger.Info("landingshed: no merged parent head was read in this process, so the landed size is not recorded", "producer", finalizeName)
		return
	}
	if err := fz.deps.RecordLandedSize(fz.parentHeadMerged, landing); err != nil {
		logger.Warn("landingshed: record the landed size failed", "producer", finalizeName, "cause", err)
	}
}

// keepParentHead reads the parent pair's head, which the parent branch's merge into the task worktree is about to bring in, and keeps it as the base of the landed diff.
// A failed read is logged at Warn and keeps no head, so the landed size goes unrecorded.
func (fz *Finalize) keepParentHead(parentHandle parentMerger) {
	head, err := parentHandle.HeadSHA()
	if err != nil {
		logger.Warn("landingshed: read the parent head before the merge-in failed", "producer", finalizeName, "cause", err)
		head = ""
	}
	fz.parentHeadMerged = head
}

// pushParent publishes the parent branch the merge just landed on to its upstream, so a landing
// reaches the remote rather than only this hub's parent worktree, and reports Done.
// A transient push failure (shedtransient.Class non-empty) is returned as an error, so the driver re-steps once;
// the re-step's parent-side Merge reports AlreadyUpToDate and goes straight to the push.
// Any other failed push is Stuck: the merge has already landed locally, so a human pushes (or reconciles a diverged remote) by hand,
// and a failure recorded as a failed push carries the way forward of its class in the reason instead.
// When origin's parent already holds the task's work, the reason also carries the repair of an adopter-first landing.
// Deps.PushSkipped suppresses the push.
//
// A push the remote refuses as a protected branch carries protectedParentWayForward alone, in place of any class's way forward and the repair note.
// parentMerges is true when setting a parent to take its pull request at once would change nothing, because the landing went through such a parent or is a true merge, so that way forward leaves out the advice to set that mode.
// On its Done return only, a non-empty notice is queued through Deps.Notify, and a failure there is a logged warning that never changes the verdict.
// The push is the parent branch's own, whether the landing went through a pull request merged on GitHub or not.
func (fz *Finalize) pushParent(ctx context.Context, parentHandle parentMerger, notice string, parentMerges bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if _, err := parentHandle.PushBranch(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped}); err != nil {
		if terr := transientFailure(ctx, finalizeName, "push parent branch", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		if protectedBranchRefusal(err) {
			cause := err
			var failed *fabricengine.PushFailedError
			if errors.As(err, &failed) {
				cause = failed.Err
			}
			reason := fmt.Sprintf("parent branch %q was merged locally but the remote refused its push as a protected branch: %v; %s",
				fz.deps.ParentBranch, cause, protectedParentWayForward(fz.deps.TaskBranch, fz.deps.ParentBranch, parentMerges))
			return fz.stuckOrCancelled(ctx, reason, "error", err)
		}
		reason := fmt.Sprintf("parent branch %q was merged locally but its push failed: %v", fz.deps.ParentBranch, err)
		// A classified failure's text already ends in the way forward of its class, and a plain push is what a hold-back refuses.
		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) {
			reason += "; push it by hand"
		}
		reason += fz.adopterFirstRepair(parentHandle)
		return fz.stuckOrCancelled(ctx, reason, "error", err)
	}
	// The landing is done, so a failed removal of the attempt record changes no verdict.
	if err := RemoveFixerAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName)); err != nil {
		logger.Warn("landingshed: remove the fixer attempt record failed", "producer", finalizeName, "cause", err)
	}
	if notice != "" && fz.deps.Notify != nil {
		if err := fz.deps.Notify(notice); err != nil {
			logger.Warn("landingshed: queue config-change notice failed", "producer", finalizeName, "cause", err)
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// protectedBranchMarkers are the lower-case texts of GitHub's push rejection for a branch that takes changes only through a pull request: classic branch protection and repository rules.
var protectedBranchMarkers = []string{"protected branch", "repository rule violations"}

// protectedBranchRefusal reports whether err is a push the remote refused because the branch is protected.
func protectedBranchRefusal(err error) bool {
	text := strings.ToLower(err.Error())
	for _, marker := range protectedBranchMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// protectedParentWayForward is the way forward of a parent push refused as a protected branch, for task landing on parent.
// It names beam.yaml's pull_request=merge, at the parent's worktree or the hub's default layer, unless parentMerges says that setting would change nothing, and the hand landing of the work the local parent alone holds.
func protectedParentWayForward(task, parent string, parentMerges bool) string {
	handLanding := fmt.Sprintf(`this run's work is on the local parent only, so merge task branch %q into %q through a pull request on GitHub by hand, then run "git fetch origin" and "git reset --keep origin/%s" in the parent's worktree, then "lym loom resume" in the task worktree, which finds the work on the parent and ends the landing`, task, parent, parent)
	if parentMerges {
		return "way forward: " + handLanding
	}
	return `way forward: mark the parent to land through a pull request lym squash-merges on GitHub at once, with "lym config beam --set pull_request=merge" in the parent's worktree, or for every branch of the hub with "lym config beam --layer default --set pull_request=merge" from the prime; ` + handLanding
}

// recheckLock re-reads the parent lock record immediately before a landing attempt.
// stop is false when the record still names this run, so Call lands.
// Otherwise the other values are what Call returns at once: a halting Stuck for a release or an unreadable record, an error for an infrastructure fault.
func (fz *Finalize) recheckLock(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	reason, err := fz.guard.stillHeld(finalizeName)
	if err != nil {
		return fz.errorStop(ctx, err)
	}
	if reason != "" {
		return fz.halt(ctx, reason)
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// mergeAndGate merges the parent branch into the task worktree, judges the merge and gates the merged tree.
// done is false when the merged tree passed and Call proceeds to the parent-side merge; otherwise the other values are what Call returns at once.
// Every pass, the first and the retry after the parent moved again, is committed and verified before the parent-side merge.
// A halt, a gate failure or a dirty tree returns before the parent-side merge, the board update and the push.
func (fz *Finalize) mergeAndGate(ctx context.Context, parentHandle parentMerger) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if outcome, out, err, done := fz.mergeParent(ctx, parentHandle); done {
		return outcome, out, err, true
	}
	return fz.gateAndFix(ctx)
}

// gateAndFix runs the `landing` gate over the merged tree and, on a failure, the merge fixer in finalize mode, which is Finalize's step 4.
// The gate skips a tree it already verified, so a run that went right holds the lock for one merge, a skipped gate and the landing.
// A passing gate removes the landing failure record and a failing one writes it.
// The fixer and the fix review run with the parent lock freed.
//
// A failing gate spawns the fixer on the attempts landing.yaml's finalize_fix_attempts leaves the entry, read from the attempt record under the row's entry.
// A gate whose every failed run crashed its test runner judged no code, so it halts with the gate's LandingStopPrefix reason, spawning no fixer and spending no attempt.
// A pass after the lock was taken again spends from the same attempts, so a second failed verify never grants a fresh cap.
// The fixer's whole change is judged against the size and kind caps from the entry's Baseline, the HEAD before its first turn, so turns that each stay under the cap but sum past it are over it.
// An entry has no attempts left when its Spent reaches the cap, whatever the record's Halt notes, and such an entry spawns no fixer: over a failing gate it ends as a spent cap, and over a passing one its change since Baseline is judged.
// A passing gate with a fix review's findings pending, and a turn left, spawns the fixer on those findings.
// A spent cap keeps the fixer's commits and halts with LandingStopPrefix, noting `spent` in the attempt record's Halt.
// A resume after a halt at the fix review round cap lands the change when the operator accepted it at the task HEAD the resume finds, and otherwise runs the fix reviews afresh over the change as it then stands.
// An escalate report halts with EscalateStopPrefix on a tapestry, darn or sampler run (Deps.EscalateJudgedFirst) and with LandingStopPrefix on a pawl run;
// an unusable turn and an attempt record that does not decode each halt with LandingStopPrefix, and a failed spawn is an error that spends no attempt.
// done is false when the tree passed and its change is settled, and Call proceeds; otherwise the other values are what Call returns at once.
func (fz *Finalize) gateAndFix(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	attemptsPath := FixerAttemptsPath(fz.deps.ScratchDir, finalizeName)
	rec, reason, err := fz.readAttempts(attemptsPath)
	if err != nil {
		return fz.errorStop(ctx, err)
	}
	if reason != "" {
		return fz.halt(ctx, reason)
	}
	if rec.Halt == fixerHaltReviewCapped {
		rec.Halt = ""
		if rec.AcceptedHead == "" {
			rec.ReviewRounds, rec.FixReview, rec.Findings = 0, "", nil
		}
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fz.errorStop(ctx, err)
		}
	}
	if rec, err = fz.rebaseAfterParentMerge(rec); err != nil {
		return fz.errorStop(ctx, err)
	}

	gateReason, crashed, err := fz.gate.check(ctx, finalizeName, fz.deps.ParentBranch)
	if outcome, out, err, stop := fz.gateStop(ctx, "", err); stop {
		return outcome, out, err, true
	}
	if crashed {
		return fz.halt(ctx, gateReason)
	}
	passed := gateReason == ""
	if passed {
		fz.gate.clearFailure()
		reason, err = fz.gate.clean(finalizeName, "after the verify")
		if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
			return outcome, out, err, true
		}
	}

	spent := rec.Spent >= fz.deps.Config.FinalizeFixAttempts
	switch {
	case passed && (rec.FixReview == "" || spent):
		return fz.judgeFixerChange(ctx, rec)
	case spent:
		return fz.endSpent(ctx, rec, fz.failedGateLog(gateReason))
	}
	return fz.runFixer(ctx, rec)
}

// rebaseAfterParentMerge moves the entry's Baseline to the task head when this pass's merge brought parent commits onto the head the fixer's change was last let through at,
// so the parent's commits never count as the fixer's change and a fixer change after the merge is judged from it.
// Any other record comes back as it is.
func (fz *Finalize) rebaseAfterParentMerge(rec FixerAttempts) (FixerAttempts, error) {
	if rec.Judged == "" {
		return rec, nil
	}
	merge, found, err := ReadMergeRecord(fz.deps.MergeRecordPath)
	if err != nil {
		return rec, fmt.Errorf("landingshed: %s: read the merge record: %w", finalizeName, err)
	}
	if !found || merge.UpToDate || merge.TaskHead != rec.Judged {
		return rec, nil
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return rec, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err)
	}
	rec.Baseline, rec.Judged = head, head
	return rec, WriteFixerAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName), rec)
}

// readAttempts reads the fixer attempt record of the row's current entry.
// A record that does not decode comes back as a halting reason naming the file and how to remove it.
// A Finalize built with no row-entry read reads entry 0.
func (fz *Finalize) readAttempts(path string) (FixerAttempts, string, error) {
	entry := 0
	if fz.deps.RowEntry != nil {
		var err error
		if entry, err = fz.deps.RowEntry(finalizeName); err != nil {
			return FixerAttempts{}, "", fmt.Errorf("landingshed: %s: read the row entry: %w", finalizeName, err)
		}
	}
	rec, err := ReadFixerAttempts(path, entry)
	if err != nil {
		logger.Warn("landingshed: fixer attempt record unreadable", "producer", finalizeName, "cause", err)
		return FixerAttempts{}, fmt.Sprintf("%sthe fixer attempt record of %s does not decode: %s; way forward: remove %s, which grants the entry a fresh cap, then `lym loom resume`", LandingStopPrefix, finalizeName, path, path), nil
	}
	return rec, "", nil
}

// runFixer runs the merge fixer in finalize mode on the attempts the entry has left and maps how the run ended onto Call's return.
// The first spawn writes the HEAD before its first turn as the record's Baseline, which every resume of the entry keeps.
// A fixer spawned on a fix review's findings reads that review's report, which the record names.
// A fixed end clears the pending findings and is judged against the caps, a spent end ends as a spent cap, and every other end halts with its reason.
func (fz *Finalize) runFixer(ctx context.Context, rec FixerAttempts) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	fz.freeLock()
	mergeRecord, _, err := ReadMergeRecord(fz.deps.MergeRecordPath)
	if err != nil {
		return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the merge record the fixer reads: %w", finalizeName, err))
	}
	if rec.Baseline == "" {
		head, err := fz.deps.TaskHead()
		if err != nil {
			return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
		}
		rec.Baseline = head
	}
	outcome, updated, err := newFixerRun(fz.deps, finalizeName, fz.gate).run(ctx, FixerModeFinalize, rec, fz.deps.Config.FinalizeFixAttempts, [2]string{"", mergeRecord.MergedTip})
	if err != nil {
		return fz.errorStop(ctx, err)
	}
	switch outcome.end {
	case fixerEndFixed:
		fz.gate.clearFailure()
		if updated.FixReview != "" {
			updated.FixReview = ""
			if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName), updated); err != nil {
				return fz.errorStop(ctx, err)
			}
		}
		return fz.judgeFixerChange(ctx, updated)
	case fixerEndSpent:
		return fz.endSpent(ctx, updated, outcome.failureLog)
	}
	return fz.halt(ctx, outcome.haltReason(finalizeName, fz.deps.EscalateJudgedFirst))
}

// failedGateLog names the log of the landing gate's failure: the failure record's log when one was written, and the gate's own reason otherwise.
func (fz *Finalize) failedGateLog(gateReason string) string {
	if failure, found, err := verifytree.ReadLandingFailure(fz.gate.paths); err == nil && found {
		return failure.LogPath
	}
	return singleLine(gateReason)
}

// judgeFixerChange judges the entry's fixer change, from its Baseline to HEAD, against the size and kind caps, and settles a change over them in place.
// An entry whose fixer never ran has no Baseline and no change.
// An entry whose Baseline is HEAD has no change.
// That entry, a change within the caps, one a fix review approved at this HEAD, and one the operator accepted at this HEAD let Finalize go on, and the record keeps HEAD as Judged.
// A change over them goes to the fix review, which reads only the fixer's commits, and each review spends one of landing.yaml's finalize_fix_review_rounds:
// an approval lets Finalize go on and is kept in the attempt record as the HEAD it approved, so a later pass at that HEAD lands without a second review;
// a rejection hands its findings to the fixer, on a fresh turn cap, and the fixer's fixed change is judged again;
// a review that gives no verdict runs again;
// a review that changed the task branch halts with LandingStopPrefix;
// and a review that does not approve on the last round halts for the operator with its open findings.
func (fz *Finalize) judgeFixerChange(ctx context.Context, rec FixerAttempts) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	proceed := func() (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return "", shedengine.OutputPointer{}, nil, false
	}
	if rec.Baseline == "" {
		return proceed()
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	attemptsPath := FixerAttemptsPath(fz.deps.ScratchDir, finalizeName)
	letThrough := func(rec FixerAttempts) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		rec.Judged = head
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fz.errorStop(ctx, err)
		}
		return proceed()
	}
	if rec.Baseline == head {
		return letThrough(rec)
	}
	change, err := fz.deps.LineChange(rec.Baseline, head)
	if err != nil {
		return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the fixer's change: %w", finalizeName, err))
	}
	capReason := FixCap{MaxLines: fz.deps.Config.FinalizeFixMaxLines, IsTestFile: fz.deps.IsTestFile}.Exceeded(change)
	if capReason == "" || rec.Reviewed == head || rec.AcceptedHead == head {
		return letThrough(rec)
	}
	// An acceptance covers only the HEAD it was given at, so a change past it starts the fix reviews afresh.
	if rec.AcceptedHead != "" {
		rec.AcceptedHead, rec.ReviewRounds, rec.Findings = "", 0, nil
	}

	fz.freeLock()
	for {
		review, err := runFixReview(fz.deps, fz.gate, finalizeName, rec.Baseline, head, capReason)
		if err != nil {
			return fz.errorStop(ctx, err)
		}
		rec.ReviewRounds++
		rec.FixReview = ""
		switch review.end {
		case fixReviewEndApproved:
			rec.Reviewed = head
			return letThrough(rec)
		case fixReviewEndChanged:
			if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
				return fz.errorStop(ctx, err)
			}
			return fz.halt(ctx, fmt.Sprintf("%sthe fix review session of %s changed the task branch: %s; session log: %s; way forward: restore the task branch to the fixer's commits %s..%s with a clean tree, then `lym loom resume`", LandingStopPrefix, finalizeName, review.cause, review.sessionLog, rec.Baseline, head))
		}
		if rec.ReviewRounds >= fz.deps.Config.FinalizeFixReviewRounds {
			return fz.haltReviewCapped(ctx, rec, head, capReason, review)
		}
		if review.end == fixReviewEndRejected {
			rec.FixReview, rec.Spent = review.reportPath, 0
			if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
				return fz.errorStop(ctx, err)
			}
			return fz.runFixer(ctx, rec)
		}
		if err := WriteFixerAttempts(attemptsPath, rec); err != nil {
			return fz.errorStop(ctx, err)
		}
	}
}

// haltReviewCapped halts the entry for the operator once its fix reviews ran out of rounds without approving the fixer's change.
// It keeps the fixer's commits, and the attempt record notes the halt and the findings the last review left open, which an acceptance writes into the change description.
func (fz *Finalize) haltReviewCapped(ctx context.Context, rec FixerAttempts, head, capReason string, review fixReviewOutcome) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	rec.Halt, rec.Findings, rec.AcceptedHead = fixerHaltReviewCapped, review.openFindings(), ""
	if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName), rec); err != nil {
		return fz.errorStop(ctx, err)
	}
	return fz.halt(ctx, fmt.Sprintf("%sthe fix review of %s did not approve the fixer's change over the caps (%s) in %d rounds: %s; fixer commits: %s..%s; way forward: run `lym loom circling accept %s` to land the change as it stands with the open findings in the change description, or fix the findings on the task branch; then `lym loom resume`",
		LandingStopPrefix, finalizeName, capReason, rec.ReviewRounds, review.verdictSummary(), rec.Baseline, head, fz.deps.Slug))
}

// endSpent ends an entry whose attempts are spent over a gate that still fails, naming the failed gate's log.
// It keeps the fixer's commits, notes `spent` in the attempt record's Halt and halts with LandingStopPrefix.
func (fz *Finalize) endSpent(ctx context.Context, rec FixerAttempts, log string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	commits := "none"
	if rec.Baseline != "" {
		commits = rec.Baseline + ".." + head
	}
	rec.Halt = fixerHaltSpent
	if err := WriteFixerAttempts(FixerAttemptsPath(fz.deps.ScratchDir, finalizeName), rec); err != nil {
		return fz.errorStop(ctx, err)
	}
	return fz.halt(ctx, fmt.Sprintf("%sthe fixer spent every attempt of %s and the landing gate still fails; log: %s; fixer commits: %s; way forward: fix the failure on the task branch, then `lym loom resume`", LandingStopPrefix, finalizeName, log, commits))
}

// halt returns a halting Stuck with reason from Call.
func (fz *Finalize) halt(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	outcome, out, err := stuckVerdict(ctx, finalizeName, reason, true)
	return outcome, out, err, true
}

// errorStop returns err from Call, or the cancellation error when ctx is cancelled.
func (fz *Finalize) errorStop(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if cerr := cancelErr(ctx, finalizeName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr, true
	}
	return "", shedengine.OutputPointer{}, err, true
}

// parentMergeBefore is what Finalize reads before it merges the parent branch into the task worktree.
type parentMergeBefore struct {
	// taskBase is the parent tip last merged into the task, empty when no merge-base read is wired.
	taskBase string
	// taskHead is the task head before the merge.
	taskHead string
	// earlier is the merge record the last Parent-Align or Finalize wrote, zero when none is readable.
	earlier MergeRecord
}

// parentMerge is how the merge of the parent branch into the task worktree ended.
type parentMerge struct {
	// upToDate reports that the task already held the parent's tip.
	upToDate bool
}

// mergeParent merges the parent branch into the task worktree and judges the merge, which are Finalize's steps 2 and 3 under the parent lock it already holds.
// It clears a parked merge-in, checks the tree clean and merges.
// The merge goes through the resolver, which resolves a conflict with the conflict session under the lock.
// Right before the merge it keeps the parent pair's head, the base of the landed diff.
//
// The merge is large when the parent's changes since the merged tip the merge record names touch a file the task changed.
// A merge record that cannot be read is logged and read as absent, so the parent side is measured from the task's merge base; a parent already holding the task's work is never large.
// The merge record is written with the new merged tip and the aligned tip as it was.
// A large merge writes the merge record, with the overlap, and goes on to the gate like any other merge.
// done is false when the merge settled and Call goes on to the gate; otherwise the other values are what Call returns at once.
func (fz *Finalize) mergeParent(ctx context.Context, parentHandle parentMerger) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	reason, err := fz.premerge.clear(finalizeName, fz.deps.ParentBranch)
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	reason, err = fz.gate.clean(finalizeName, "before the merge-in")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}

	before, err := fz.readParentMergeBefore()
	if err != nil {
		return "", shedengine.OutputPointer{}, err, true
	}
	fz.keepParentHead(parentHandle)
	merge, reason, err := fz.mergeIntoTask(ctx)
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, err, true
	}
	if reason != "" {
		outcome, out, err := fz.stuckOrCancelled(ctx, reason)
		return outcome, out, err, true
	}
	reason, err = fz.gate.clean(finalizeName, "after the merge-in")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}

	newTip, overlap, err := fz.judgeParentMerge(before, merge)
	if err != nil {
		return "", shedengine.OutputPointer{}, err, true
	}
	mergeCommit := ""
	if !merge.upToDate {
		if mergeCommit, err = fz.deps.TaskHead(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the merge commit: %w", finalizeName, err), true
		}
	}
	if fz.gate.recorder != nil {
		fz.gate.recorder.mergeCommit = mergeCommit
	}
	record := MergeRecord{
		ParentBranch: fz.deps.ParentBranch,
		MergedTip:    newTip,
		AlignedTip:   before.earlier.AlignedTip,
		TaskBase:     before.taskBase,
		TaskHead:     before.taskHead,
		MergeCommit:  mergeCommit,
		UpToDate:     merge.upToDate,
		Overlap:      overlap,
	}
	if err := WriteMergeRecord(fz.deps.MergeRecordPath, record); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", finalizeName, err), true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// readParentMergeBefore reads what Finalize needs from before the merge: the task's merge base and head, and the earlier merge record.
// An unreadable earlier record is logged and read as absent.
// A Finalize built with no merge-base read reads an empty task base.
func (fz *Finalize) readParentMergeBefore() (parentMergeBefore, error) {
	var before parentMergeBefore
	head, err := fz.deps.TaskHead()
	if err != nil {
		return before, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err)
	}
	before.taskHead = head
	if fz.deps.MergeBase != nil {
		if before.taskBase, err = fz.deps.MergeBase(fz.deps.ParentBranch); err != nil {
			return before, fmt.Errorf("landingshed: %s: find the parent tip last merged into the task: %w", finalizeName, err)
		}
	}
	if earlier, found, err := ReadMergeRecord(fz.deps.MergeRecordPath); err != nil {
		logger.Warn("landingshed: earlier merge record unreadable, measuring the parent side from the task's merge base", "producer", finalizeName, "cause", err)
	} else if found {
		before.earlier = earlier
	}
	return before, nil
}

// mergedTipRead names the parent tip the parent side of the merge is measured from: the earlier merge record's merged tip, or the task's merge base when there is none.
func (fz *Finalize) mergedTipRead(before parentMergeBefore) string {
	if before.earlier.MergedTip != "" {
		return before.earlier.MergedTip
	}
	return before.taskBase
}

// mergeIntoTask merges the parent branch into the task worktree and reports how the merge ended, or a Stuck reason.
// The resolver resolves a conflict with the conflict session.
func (fz *Finalize) mergeIntoTask(ctx context.Context) (parentMerge, string, error) {
	parent := fz.deps.ParentBranch
	result, err := fz.resolver.Resolve(ctx, parent, "")
	if err != nil {
		return parentMerge{}, "", fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", finalizeName, parent, err)
	}
	if result.Outcome == mergeresolve.OutcomeStuck {
		return parentMerge{}, result.Reason, nil
	}
	return parentMerge{upToDate: result.AlreadyUpToDate}, "", nil
}

// judgeParentMerge reads the parent's tip after the merge and the files both sides changed since the merged tip the merge record names.
// An up-to-date merge, a parent already holding the task's work, an unchanged parent tip and a Finalize built with no path or merge-base read all report no overlap.
func (fz *Finalize) judgeParentMerge(before parentMergeBefore, merge parentMerge) (string, Overlap, error) {
	tips, err := fz.deps.ReadParentTips(false)
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: read the tips of parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err)
	}
	newTip := tips.Local
	parentFrom := fz.mergedTipRead(before)
	if merge.upToDate || fz.deps.ChangedPaths == nil || before.taskBase == "" || parentFrom == "" || parentFrom == newTip {
		return newTip, Overlap{}, nil
	}
	if fz.deps.ParentHoldsTaskWork != nil {
		held, err := fz.deps.ParentHoldsTaskWork()
		if err != nil {
			return "", Overlap{}, fmt.Errorf("landingshed: %s: read whether the parent branch already holds the task's work: %w", finalizeName, err)
		}
		if held {
			return newTip, Overlap{}, nil
		}
	}
	parentPaths, err := fz.deps.ChangedPaths(parentFrom, newTip)
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: list the paths the parent changed: %w", finalizeName, err)
	}
	taskPaths, err := fz.deps.ChangedPaths(before.taskBase, before.taskHead)
	if err != nil {
		return "", Overlap{}, fmt.Errorf("landingshed: %s: list the paths the task changed: %w", finalizeName, err)
	}
	return newTip, computeOverlap(parentPaths, taskPaths), nil
}

// commitParentRecords commits the parent pair's run records through Deps.CommitParentRecords, and is a no-op when no seam is wired.
// done is true when the step ends the Call: a Stuck verdict naming the parent pair and its run records, or a cancellation.
func (fz *Finalize) commitParentRecords(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if fz.deps.CommitParentRecords == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	err := fz.deps.CommitParentRecords()
	if err == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	reason := fmt.Sprintf("the parent pair's run records could not be committed: %v; way forward: run \"lym fabric commit\" in the parent pair, then \"lym loom resume\"", err)
	outcome, out, stuckErr := fz.stuckOrCancelled(ctx, reason, "error", err)
	return outcome, out, stuckErr, true
}

// gateStop maps one gate call's result onto mergeAndGate's return.
// stop is false when the gate passed.
// Otherwise the other values are what mergeAndGate returns at once:
// a cancellation or an infrastructure fault as an error, a Stuck reason as a Stuck verdict.
func (fz *Finalize) gateStop(ctx context.Context, reason string, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, err, true
	}
	if reason != "" {
		outcome, out, err := fz.stuckOrCancelled(ctx, reason)
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// stuckOrCancelled consults cancelErr first -- the obligation every non-success exit discharges --
// and otherwise logs reason via reportStuck and returns Stuck with reason on the output pointer.
func (fz *Finalize) stuckOrCancelled(ctx context.Context, reason string, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, finalizeName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(finalizeName, reason, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}
