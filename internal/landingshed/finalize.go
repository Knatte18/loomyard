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

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
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

// taskMerger is the narrow seam over the task pair's merge-in of the parent branch and its abort, which `*fabricengine.Fabric` satisfies.
// A loom Finalize merges through it, so a conflict is a result state it aborts, never a session it runs.
type taskMerger interface {
	// MergeIn merges source into the task pair, returning any conflicts as a result state rather than an error. See fabricengine.Fabric.MergeIn.
	MergeIn(source string) (fabricengine.MergeResult, error)
	// MergeAbort discards the merge the task pair has parked. See fabricengine.Fabric.MergeAbort.
	MergeAbort() (fabricengine.MergeResult, error)
}

// The compile-time assertion that *fabricengine.Fabric satisfies taskMerger.
var _ taskMerger = (*fabricengine.Fabric)(nil)

// conflictSessions wraps the conflict session runner and counts the sessions it starts, so a darn Finalize can tell a merge that needed one.
type conflictSessions struct {
	inner   mergeresolve.Shuttle
	started atomic.Int64
}

// Run counts the session and starts it on the wrapped runner.
func (c *conflictSessions) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	c.started.Add(1)
	return c.inner.Run(spec)
}

// count returns the sessions started so far; a nil counter has started none.
func (c *conflictSessions) count() int64 {
	if c == nil {
		return 0
	}
	return c.started.Load()
}

// Finalize is the ShedProducer that merges the parent branch into the task worktree under the parent lock, then merges the task branch into the parent pair's own worktree.
type Finalize struct {
	deps     Deps
	resolver resolver
	// taskMerger merges the parent branch into the task pair for a loom Finalize, whose conflict it aborts.
	// A struct-literal Finalize leaves it nil and merges through its resolver, as a darn one does.
	taskMerger taskMerger
	// sessions counts the conflict sessions resolver starts; nil counts none.
	sessions *conflictSessions
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

	// A nil told shuttle stays nil, so the resolver's constructor refuses it as it refuses any other.
	sessions := &conflictSessions{inner: deps.Shuttle}
	var shuttle mergeresolve.Shuttle
	if deps.Shuttle != nil {
		shuttle = sessions
	}
	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:         fabricHandle,
		Shuttle:        shuttle,
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

	return &Finalize{
		deps:       deps,
		resolver:   res,
		taskMerger: fabricHandle,
		sessions:   sessions,
		gate:       newVerifyGate(deps),
		premerge:   newPreMerge(deps),
		guard:      newParentGuard(deps),
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
// a paused lock wait or an unreadable lock record is a halting Stuck, and a landing that reaches Done, a reroute, a Stuck and an error all free the lock.
// Under that lock it runs the owner checks: a run origin ended or stopped is Stuck, and a run or parent another machine owns, an unreachable origin and a failed owner read halt.
// A parent already holding the task's work skips the owner checks and the unreachable halt, and an unreachable origin then proceeds.
// It then opens the parent pair and syncs it: a pair that is missing, dirty or mid-merge, or a parent that cannot be fast-forwarded to its upstream, is Stuck with a way forward before any merge.
// It then merges the parent branch into the task worktree and judges the merge: a conflict a loom Finalize aborts, a conflict a darn Finalize resolves, and a merge large by its conflict or by files both sides changed reroute, with no gate run.
// After a merge-in-required error from the parent-side merge it re-runs the merge and its gate once under the same lock.
//
// Finalize is idempotent over an already-landed parent: when the parent already contains the task's
// changes -- an operator squash-merged the pull request on GitHub, or an earlier Finalize landed and
// failed later -- the empty squash is classified as already-up-to-date, so Fabric.Merge commits
// nothing and returns no error. Finalize then proceeds exactly as for a fresh landing: the board
// task is marked done, the push is a no-op, the pull request is closed naming the parent's current
// head, and the verdict is Done.
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

	// Step 1a2: read the engines the task ran under, for the same reason: the trailer block is part of step 4's message.
	var enginesRun []string
	if fz.deps.EnginesRun != nil {
		if enginesRun, err = fz.deps.EnginesRun(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the engines the task ran under: %w", finalizeName, err)
		}
	}
	landingMessage := summary.LandingMessage(fz.deps.Config.TrailerBlock(enginesRun))

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

	// Step 1b2: take the parent lock first, before anything reads or moves the parent, and release it on every way out of this Call.
	// A lock a dead Finalize left is taken over by the acquire, since the lock store grants a record whose owner process no longer lives.
	lockReason, err := fz.guard.ensure(ctx, finalizeName)
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}
	if lockReason != "" {
		return stuckVerdict(ctx, finalizeName, lockReason, true)
	}
	defer fz.guard.release(finalizeName)

	// Step 1b3: judge the run against origin under the lock, and halt unless this machine owns the run and its parent.
	// A run whose Finalize already merged into the local parent here is never read as ended or owned elsewhere, so a Finalize re-entered after its done mark proceeds.
	if outcome, out, err, stop := fz.ownerChecks(ctx); stop {
		return outcome, out, err
	}

	// Step 1b4: open the parent pair, which this producer never creates, and sync it before any merge.
	parentHandle, err := fz.parentOpener()
	if err != nil {
		return fz.stuckOrCancelled(ctx, fmt.Sprintf("no live pair for parent branch %q: %v; way forward: re-create the parent pair, then run \"lyx loom resume\"", fz.deps.ParentBranch, err), "error", err)
	}
	syncReason, err := fz.syncParent(ctx, parentHandle)
	if outcome, out, err, stop := fz.gateStop(ctx, syncReason, err); stop {
		return outcome, out, err
	}

	// Step 1c: read the task's per-worktree config changes before any parent-side mutation,
	// so the diff is taken against the parent as the task forked from it;
	// the notice it yields is queued only at Done.
	notice := fz.configChangeNotice()

	// Steps 2 and 3: merge the parent branch into the task worktree and judge the merge, then gate the merged tree.
	if outcome, out, err, done := fz.mergeAndGate(ctx); done {
		return outcome, out, err
	}

	// Step 2b: commit the parent pair's own run records,
	// so a sibling run's uncommitted status record does not trip the parent-side merge guard.
	if outcome, out, err, done := fz.commitParentRecords(ctx); done {
		return outcome, out, err
	}

	// Step 4: the parent-side merge, this producer's own merge critical section. Message is set
	// whether or not Config.Squash is true -- it is the conclude-commit message for both merge
	// shapes, so gating it on Squash would leave the non-squash landing commit with an unset message.
	mergeOpts := fabricengine.MergeOptions{Squash: fz.deps.Config.Squash, Message: landingMessage}
	_, mergeErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
	if mergeErr == nil {
		fz.markTaskDone()
		return fz.pushParent(ctx, parentHandle, notice)
	}

	// Step 5: on the merge-in-required error, test the parent again with the same two-ref read, then re-run steps 2 to 4 under the same lock and retry
	// the parent-side merge exactly once.
	// The retry runs only when the parent has not moved, which covers a parent moved by run records alone;
	// otherwise a code-side commit that arrived between the check and the parent-side merge, local or pushed, reroutes and never lands.
	var mergeInRequired *fabricengine.ErrMergeInRequired
	if errors.As(mergeErr, &mergeInRequired) {
		if outcome, out, err, stop := rerouteIfMoved(ctx, fz.guard, finalizeName); stop {
			return outcome, out, err
		}
		if outcome, out, err, done := fz.mergeAndGate(ctx); done {
			return outcome, out, err
		}
		if outcome, out, err, done := fz.commitParentRecords(ctx); done {
			return outcome, out, err
		}
		_, retryErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
		if retryErr == nil {
			fz.markTaskDone()
			return fz.pushParent(ctx, parentHandle, notice)
		}
		mergeErr = retryErr
	}

	// Steps 6-7: a guard error carrying the dirty-worktree reason is recognized through the
	// accessor rather than a matched string, and surfaced verbatim -- never stash, never reset,
	// never retry. Any other merge error is stuck with the error surfaced.
	var guardErr *fabricengine.MergeGuardError
	if errors.As(mergeErr, &guardErr) && guardErr.WorktreeDirty() {
		return fz.stuckOrCancelled(ctx, fz.dirtyParentReason(parentHandle, guardErr))
	}
	return fz.stuckOrCancelled(ctx, fmt.Sprintf("parent-side merge failed: %v", mergeErr), "error", mergeErr)
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
	const resume = `then run "lyx loom resume"`

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
const gotoResumeWayForward = `"lyx loom goto --to Finalize" and "lyx loom resume" in the task worktree`

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
	return fmt.Sprintf(`the party that started that merge, or the orch or the operator when that party is gone, concludes it with "lyx fabric merge --continue" or discards it with "lyx fabric merge --abort" in the parent pair, then %s; "--abort" hard-resets the parent's code checkout to the merge's start, so the operator first saves a copy of any change of their own in %s outside it, since a plain git commit or stash there would take the staged merge with it; a session lyx refuses the verb from reports status: FAILED and the orch runs it`,
		gotoResumeWayForward, checkout)
}

// configChangeNotice composes the one-line notice about the task's per-worktree config changes, or "" when there is nothing to report or no seam is wired.
// Finalize never carries those files to the parent,
// so the notice tells the orchestrator to re-apply each change meant for the parent.
// A read failure is logged and reported in the notice instead of stopping the landing.
func (fz *Finalize) configChangeNotice() string {
	if fz.deps.ConfigChanges == nil {
		return ""
	}
	changes, err := fz.deps.ConfigChanges()
	var line string
	switch {
	case err != nil:
		logger.Warn("landingshed: read config changes failed", "producer", finalizeName, "cause", err)
		line = fmt.Sprintf("loom: the config changes of task branch %q could not be read: %v; diff its config files against the parent branch %q by hand",
			fz.deps.TaskBranch, err, fz.deps.ParentBranch)
	case len(changes.Files) > 0:
		line = fmt.Sprintf("loom: task branch %q changed per-worktree config files since it forked from %q: %s (base %s, tip %s, parent tip %s); landing does not carry them to the parent branch, so re-apply each change meant for the parent on the parent's copy of the same file",
			fz.deps.TaskBranch, fz.deps.ParentBranch, strings.Join(changes.Files, ", "), changes.Base, changes.Tip, changes.ParentTip)
	default:
		return ""
	}
	return strings.Join(strings.Fields(line), " ")
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

// pushParent publishes the parent branch the merge just landed on to its upstream, so a landing
// reaches the remote rather than only this hub's parent worktree, and reports Done.
// A transient push failure (shedtransient.Class non-empty) is returned as an error, so the driver re-steps once;
// the re-step's parent-side Merge reports AlreadyUpToDate and goes straight to the push.
// Any other failed push is Stuck: the merge has already landed locally, so a human pushes (or reconciles a diverged remote) by hand,
// and a failure recorded as a failed push carries the way forward of its class in the reason instead.
// When origin's parent already holds the task's work, the reason also carries the repair of an adopter-first landing.
// Deps.PushSkipped suppresses the push, exactly as it does for the task branch.
//
// On its Done return only, a non-empty notice is queued through Deps.Notify, and a failure there is a logged warning that never changes the verdict.
//
// After a successful push into a parent that requires a pull request, the task's pull request is
// closed best-effort (closePullRequest): the landing has already happened and is irreversible.
func (fz *Finalize) pushParent(ctx context.Context, parentHandle parentMerger, notice string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if _, err := parentHandle.PushBranch(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped}); err != nil {
		if terr := transientFailure(ctx, finalizeName, "push parent branch", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
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
	if !fz.deps.PushSkipped && contains(fz.deps.Config.RequirePRToBase, fz.deps.ParentBranch) {
		fz.closePullRequest(ctx, parentHandle)
	}
	if notice != "" && fz.deps.Notify != nil {
		if err := fz.deps.Notify(notice); err != nil {
			logger.Warn("landingshed: queue config-change notice failed", "producer", finalizeName, "cause", err)
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// closePullRequest closes the task branch's still-open pull request against the parent branch,
// after commenting the landing commit's SHA on it, so the inspection-only pull request does not
// linger as open after its work landed. Every failure is a logged warning and nothing more: the
// landing is the irreversible part and has already happened. A pull request that is already closed
// or merged is left alone.
func (fz *Finalize) closePullRequest(ctx context.Context, parentHandle parentMerger) {
	warn := func(action string, err error) {
		logger.Warn("landingshed: close pull request failed", "producer", finalizeName, "action", action, "cause", err)
	}

	sha, err := parentHandle.HeadSHA()
	if err != nil {
		warn("read parent head", err)
		return
	}
	owner, repo, err := githubclient.ParseOwnerRepo(fz.deps.OriginURL)
	if err != nil {
		warn("resolve origin URL", err)
		return
	}
	client, err := NewGitHubClient()
	if err != nil {
		warn("new github client", err)
		return
	}
	pr, _, err := FindPullRequest(ctx, client, owner, repo, fz.deps.TaskBranch, fz.deps.ParentBranch, fz.deps.RunStart)
	if err != nil {
		warn("query pull request", err)
		return
	}
	if pr == nil || pr.GetState() != "open" {
		return
	}

	commentCtx, commentCancel := context.WithTimeout(ctx, publishGitHubTimeout)
	defer commentCancel()
	body := fmt.Sprintf("Landed on `%s` as %s.", fz.deps.ParentBranch, sha)
	if _, _, err := client.Issues.CreateComment(commentCtx, owner, repo, pr.GetNumber(), &github.IssueComment{Body: &body}); err != nil {
		warn("comment on pull request", err)
		return
	}

	closeCtx, closeCancel := context.WithTimeout(ctx, publishGitHubTimeout)
	defer closeCancel()
	closed := "closed"
	if _, _, err := client.PullRequests.Edit(closeCtx, owner, repo, pr.GetNumber(), &github.PullRequest{State: &closed}); err != nil {
		warn("close pull request", err)
		return
	}
	logger.Info("landingshed: pull request closed after landing", "owner", owner, "repo", repo, "number", pr.GetNumber())
}

// mergeAndGate merges the parent branch into the task worktree, judges the merge and gates the merged tree.
// done is false when the merged tree passed and Call proceeds to the parent-side merge; otherwise the other values are what Call returns at once.
// Every pass, the first and the retry after the parent moved again, is committed and verified before the parent-side merge.
// A reroute, a gate failure or a dirty tree returns before the parent-side merge, the board update and the push.
func (fz *Finalize) mergeAndGate(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if outcome, out, err, done := fz.mergeParent(ctx); done {
		return outcome, out, err, true
	}
	reason, err := fz.gate.check(ctx, finalizeName, fz.deps.ParentBranch)
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	reason, err = fz.gate.clean(finalizeName, "after the verify")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// parentMergeBefore is what Finalize reads before it merges the parent branch into the task worktree.
type parentMergeBefore struct {
	// taskBase is the parent tip last merged into the task, empty when no merge-base read is wired.
	taskBase string
	// taskHead is the task head before the merge.
	taskHead string
	// earlier is the merge record the last Merge-In or Finalize wrote, zero when none is readable.
	earlier MergeRecord
}

// parentMerge is how the merge of the parent branch into the task worktree ended.
type parentMerge struct {
	// upToDate reports that the task already held the parent's tip.
	upToDate bool
	// conflicted reports that the merge needed the conflict session, or, for a loom Finalize, conflicted and was aborted.
	conflicted bool
	// conflicts are the paths a loom Finalize's aborted merge conflicted in.
	conflicts []string
}

// mergeParent merges the parent branch into the task worktree and judges the merge, which are Finalize's steps 2 and 3 under the parent lock it already holds.
// It clears a parked merge-in, checks the tree clean and merges.
// A loom Finalize merges through taskMerger and, on a conflict, aborts the merge and reroutes, leaving the merge and its conflict session to Merge-In outside the lock.
// A darn Finalize merges through its resolver, which resolves a conflict with the conflict session under the lock, since darn has no other merging row.
//
// The merge is large when it needed the conflict session, or when the parent's changes since the merged tip the merge record names touch a file the task changed.
// A merge record that cannot be read is logged and read as absent, so the parent side is measured from the task's merge base; a parent already holding the task's work is never large.
// The merge record is written with the new merged tip and the aligned tip as it was.
// A large merge keeps the merge commit and reroutes, with no gate run and no fixer.
// done is false when the merge is within bounds and Call goes on to the gate; otherwise the other values are what Call returns at once.
func (fz *Finalize) mergeParent(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
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
	if len(merge.conflicts) > 0 {
		outcome, out, err := fz.reroute(ctx, fz.rerouteReason(fmt.Sprintf("merging parent branch %q into the task conflicts in %s, and the merge was aborted", fz.deps.ParentBranch, strings.Join(merge.conflicts, ", "))))
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

	switch {
	case merge.conflicted:
		outcome, out, err := fz.reroute(ctx, fz.rerouteReason(fmt.Sprintf("merging parent branch %q into the task needed the conflict session, so the merge is large", fz.deps.ParentBranch)))
		return outcome, out, err, true
	case len(overlap.Files) > 0:
		outcome, out, err := fz.reroute(ctx, fz.rerouteReason(fmt.Sprintf("the parent's changes since %s touch files the task changed, so the merge is large: %s", fz.mergedTipRead(before), strings.Join(overlap.Files, ", "))))
		return outcome, out, err, true
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
// A loom Finalize aborts a conflicted merge and reports its conflicts; a darn Finalize resolves it.
func (fz *Finalize) mergeIntoTask(ctx context.Context) (parentMerge, string, error) {
	parent := fz.deps.ParentBranch
	if fz.deps.ConflictReroutes {
		result, err := fz.taskMerger.MergeIn(parent)
		if err != nil {
			return parentMerge{}, fmt.Sprintf("merge-in of parent branch %q failed: %v", parent, err), nil
		}
		if len(result.Conflicts) > 0 {
			if _, err := fz.taskMerger.MergeAbort(); err != nil {
				return parentMerge{}, "", fmt.Errorf("landingshed: %s: abort the conflicted merge-in of parent branch %q: %w", finalizeName, parent, err)
			}
			return parentMerge{conflicted: true, conflicts: result.Conflicts}, "", nil
		}
		return parentMerge{upToDate: result.AlreadyUpToDate}, "", nil
	}

	startedBefore := fz.sessions.count()
	result, err := fz.resolver.Resolve(ctx, parent, "")
	if err != nil {
		return parentMerge{}, "", fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", finalizeName, parent, err)
	}
	if result.Outcome == mergeresolve.OutcomeStuck {
		return parentMerge{}, result.Reason, nil
	}
	return parentMerge{upToDate: result.AlreadyUpToDate, conflicted: fz.sessions.count() > startedBefore}, "", nil
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

// rerouteReason words a reroute's Stuck reason around cause, which names why the run goes back along Finalize's on_reroute edge.
func (fz *Finalize) rerouteReason(cause string) string {
	return fmt.Sprintf("Finalize rerouted the run along its on_reroute edge: %s", cause)
}

// reroute returns Stuck with Reroute set for reason, which spends no bounce budget; the deferred release frees the parent lock first.
// A cancellation is returned as its error.
func (fz *Finalize) reroute(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, finalizeName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(finalizeName, reason, "reroute", true)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason, Reroute: true}, nil
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
	reason := fmt.Sprintf("the parent pair's run records could not be committed: %v; way forward: run \"lyx fabric commit\" in the parent pair, then \"lyx loom resume\"", err)
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
