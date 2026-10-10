// finalize.go implements the Finalize producer: it catches the task worktree up with the parent
// branch, then merges the task branch into the parent pair's own worktree and pushes the parent
// branch -- reporting only what
// the shedengine.ShedProducer seam can carry: a verdict, an output pointer whose Reason the engine
// persists, and an error.
//
// This producer's merge critical section is the parent-side merge call (step 4 below), and a future
// regeneration step folds into that same critical section rather than becoming a separate row -- no
// hook, no injectable interface, and no scaffolded span with an empty body is built for that today,
// because an interface with a permanently-nil implementation is exactly the hypothetical-requirement
// design this codebase avoids.
//
// A merged pull request does not make this producer redundant: the remote service only ever sees
// one of the two sides, so the other side's branch was never in the pull request at all, and after a
// remote-side merge the visible side reports already-up-to-date while the other genuinely merges.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
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
}

// The compile-time assertion that *fabricengine.Fabric satisfies parentMerger.
var _ parentMerger = (*fabricengine.Fabric)(nil)

// Finalize is the ShedProducer that merges the task branch into the parent pair's own worktree,
// after first catching the task worktree up with the parent branch.
type Finalize struct {
	deps     Deps
	resolver resolver
	// parentOpener adapts deps.OpenParentFabric's concrete *fabricengine.Fabric return type onto
	// the parentMerger seam. It is called once per Call, never at construction: the pair
	// constructor it wraps stat-checks a layout that may not be wired yet, so opening eagerly
	// would fail before the run's own preflight has confirmed anything is wired.
	parentOpener func() (parentMerger, error)
	// gate is the post-merge verify gate every catch-up merge-in is followed by.
	// A zero value carries no command, so struct-literal tests run without a gate.
	gate verifyGate
}

var _ shedengine.ShedProducer = (*Finalize)(nil)

// NewFinalize constructs a Finalize from deps, rejecting a nil OpenFabric or OpenParentFabric
// closure up front with a distinct error each -- rather than nil-panicking at call time. A nil
// parent opener is a construction error rather than a silent no-op: this producer never creates a
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

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: open pair: %w", err)
	}

	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:       fabricHandle,
		Shuttle:      deps.Shuttle,
		WorktreeRoot: deps.WorktreeRoot,
		ScratchDir:   deps.ScratchDir,
		StencilsDir:  deps.StencilsDir,
		ParentName:   deps.ParentName,
		ConflictSpec: deps.Config.Conflict,
		Registry:     deps.Registry,
		Timeout:      time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewFinalize: build resolver: %w", err)
	}

	return &Finalize{
		deps:     deps,
		resolver: res,
		gate:     newVerifyGate(deps),
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

	// Step 1c: read the task's per-worktree config changes before any parent-side mutation,
	// so the diff is taken against the parent as the task forked from it;
	// the notice it yields is queued only at Done.
	notice := fz.configChangeNotice()

	// Step 2: catch the task worktree up with the parent branch.
	if outcome, out, err, done := fz.mergeInStep(ctx); done {
		return outcome, out, err
	}

	// Step 3: obtain the parent pair's handle. This producer never creates a worktree to merge
	// into; materializing a pair is a separate command's job and a human's decision.
	parentHandle, err := fz.parentOpener()
	if err != nil {
		return fz.stuckOrCancelled(ctx, fmt.Sprintf("no live pair for parent branch %q: %v", fz.deps.ParentBranch, err), "error", err)
	}

	// Step 4: the parent-side merge, this producer's own merge critical section. Message is set
	// whether or not Config.Squash is true -- it is the conclude-commit message for both merge
	// shapes, so gating it on Squash would leave the non-squash landing commit with an unset message.
	mergeOpts := fabricengine.MergeOptions{Squash: fz.deps.Config.Squash, Message: summary.LandingMessage(fz.deps.Config.CoAuthoredBy)}
	_, mergeErr := parentHandle.Merge(fz.deps.TaskBranch, mergeOpts)
	if mergeErr == nil {
		fz.markTaskDone()
		return fz.pushParent(ctx, parentHandle, notice)
	}

	// Step 5: on the merge-in-required error, re-run the resolver in the task worktree and retry
	// the parent-side merge exactly once -- no lock spans the window between this producer's own
	// catch-up merge-in and this parent-side merge, so a competing task can genuinely land in the
	// parent in between.
	var mergeInRequired *fabricengine.ErrMergeInRequired
	if errors.As(mergeErr, &mergeInRequired) {
		if outcome, out, err, done := fz.mergeInStep(ctx); done {
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
		return fz.stuckOrCancelled(ctx, guardErr.Error())
	}
	return fz.stuckOrCancelled(ctx, fmt.Sprintf("parent-side merge failed: %v", mergeErr), "error", mergeErr)
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
// Any other failed push is Stuck: the merge has already landed locally, so a human pushes (or reconciles a diverged remote) by hand.
// Deps.PushSkipped suppresses the push, exactly as it does for the task branch.
//
// On its Done return only, a non-empty notice is queued through Deps.Notify;
// a failure there is a logged warning and never changes the verdict.
//
// After a successful push into a parent that requires a pull request, the task's pull request is
// closed best-effort (closePullRequest): the landing has already happened and is irreversible.
func (fz *Finalize) pushParent(ctx context.Context, parentHandle parentMerger, notice string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if _, err := parentHandle.PushBranch(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped}); err != nil {
		if terr := transientFailure(ctx, finalizeName, "push parent branch", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		reason := fmt.Sprintf("parent branch %q was merged locally but its push failed: %v; push it by hand", fz.deps.ParentBranch, err)
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
	pr, err := FindPullRequest(ctx, client, owner, repo, fz.deps.TaskBranch, fz.deps.ParentBranch)
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

// mergeInStep runs the resolver's merge-in against the parent branch from the task worktree. When
// the merge-in produced no stuck verdict and no error, it reports done=false so Call proceeds to the
// parent-side merge; otherwise it reports done=true along with the outcome/output/error Call should
// return immediately.
//
// It checks the tree is clean before the merge-in and after it, then runs the post-merge verify gate and checks the tree is clean once more,
// so every merge-in, the first and the retry after the parent moved again, is committed and verified before the parent-side merge.
// A gate failure or a dirty tree returns before the parent opener, the parent-side merge, the board update, the push and the pull-request close.
func (fz *Finalize) mergeInStep(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	reason, err := fz.gate.clean(finalizeName, "before the merge-in")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	result, err := fz.resolver.Resolve(ctx, fz.deps.ParentBranch)
	if err != nil {
		if cerr := cancelErr(ctx, finalizeName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr, true
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", finalizeName, fz.deps.ParentBranch, err), true
	}
	if result.Outcome == mergeresolve.OutcomeStuck {
		outcome, out, err := fz.stuckOrCancelled(ctx, result.Reason)
		return outcome, out, err, true
	}
	reason, err = fz.gate.clean(finalizeName, "after the merge-in")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	reason, err = fz.gate.check(ctx, finalizeName, fz.deps.ParentBranch)
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	reason, err = fz.gate.clean(finalizeName, "after the verify")
	if outcome, out, err, stop := fz.gateStop(ctx, reason, err); stop {
		return outcome, out, err, true
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// gateStop maps one gate call's result onto mergeInStep's return.
// stop is false when the gate passed.
// Otherwise the other values are what mergeInStep returns at once:
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
