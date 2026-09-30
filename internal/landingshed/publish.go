// publish.go implements the Publish producer: when the task's parent branch requires a pull
// request, it merges the parent's own catch-up into the task worktree, pushes the task branch, and
// opens or refreshes the pull request (title and body included) against that parent, then returns Done
// because the PR-Gate producer that follows owns the review wait -- reporting only what the
// shedengine.ShedProducer seam can carry: a verdict, an output pointer whose Reason the engine
// persists, and an error.
//
// Only the externally visible branch is pushed: the pull request is an artifact of the repository
// the remote service can see. The other side's remote state belongs to the merge step and the
// engine's own sync path.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// publishName is the producer name Publish's log lines and error text carry.
const publishName = "Publish"

// publishGitHubTimeout bounds each of Publish's calls into the GitHub API, so a stalled connection
// cannot hang an unattended run.
const publishGitHubTimeout = 30 * time.Second

// NewGitHubClient is the seam through which Publish obtains an authenticated *github.Client,
// swappable for testing -- mirrors internal/selfreportengine's identically-named seam.
var NewGitHubClient = githubclient.New

// Publish is the ShedProducer that opens or refreshes a pull request from the task branch against
// the configured parent branch, once the task branch itself has been pushed.
type Publish struct {
	deps     Deps
	resolver resolver
	gate     verifyGate
}

var _ shedengine.ShedProducer = (*Publish)(nil)

// NewPublish constructs a Publish from deps, rejecting a nil OpenFabric or PushBranch closure up
// front with a distinct error each -- rather than nil-panicking at call time.
//
// It builds the resolver at construction time, opening deps.OpenFabric's lazily-opened handle and
// calling mergeresolve.New with it alongside every other told value. Construction time is correct
// here and is not in tension with the laziness rule the pair opener carries: that rule exists
// because the pair constructor stat-checks a layout that may not be wired yet, whereas the
// resolver's own constructor performs no I/O at all and only rejects nil or empty told values.
// Building it eagerly therefore turns a mis-wired resolver into a construction error, which is
// exactly where it belongs.
func NewPublish(deps Deps) (*Publish, error) {
	if deps.OpenFabric == nil {
		return nil, fmt.Errorf("landingshed: NewPublish: Deps.OpenFabric must not be nil")
	}
	if deps.PushBranch == nil {
		return nil, fmt.Errorf("landingshed: NewPublish: Deps.PushBranch must not be nil")
	}
	if deps.DescriptionPath == "" {
		return nil, fmt.Errorf("landingshed: NewPublish: Deps.DescriptionPath must not be empty")
	}

	fabricHandle, err := deps.OpenFabric()
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewPublish: open pair: %w", err)
	}

	res, err := mergeresolve.New(mergeresolve.Deps{
		Fabric:       fabricHandle,
		Shuttle:      deps.Shuttle,
		WorktreeRoot: deps.WorktreeRoot,
		ScratchDir:   deps.ScratchDir,
		StencilsDir:  deps.StencilsDir,
		ConflictSpec: deps.Config.Conflict,
		Registry:     deps.Registry,
		Timeout:      time.Duration(deps.Config.ConflictTimeoutMin) * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("landingshed: NewPublish: build resolver: %w", err)
	}

	return &Publish{deps: deps, resolver: res, gate: newVerifyGate(deps)}, nil
}

// Call runs one Publish iteration.
//
// A failed task-branch push, pull-request query or pull-request create is split by shedtransient.Class:
// a transient failure is returned as an error, so the driver re-steps once (a re-step re-queries before creating, so nothing is duplicated),
// and anything else is a Stuck verdict for a human.
// Done after a created or open pull request is safe because the next row is the PR-Gate producer, which owns approval, rejection and the wait;
// an open pull request's title and body are refreshed from the change description first.
// Out of scope: Finalize's pull-request close calls only warn;
// batten's Worktree-Create row keeps a failed fabricengine.Add push as Stuck, because Add's rollback keeps the branch it made and an immediate re-step would stop again;
// the status-commit and Seed-Child pushes only warn;
// batten's Worktree-Teardown returns a failed remote branch deletion as an unmarked error, since fabricengine.RemoveResult reports it as text with no chain to classify.
func (p *Publish) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, publishName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	// Step 2: no pull request is required against this parent branch at all -- return done
	// immediately, with no merge-in, no push, and no GitHub call whatsoever.
	if !contains(p.deps.Config.RequirePRToBase, p.deps.ParentBranch) {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	// Step 3: a pull request is required, but the push layer was told to skip pushing. Relying on
	// the push layer's own skip gating would produce a pull request for an unpushed branch, which
	// is the exact failure this gate exists to prevent.
	if p.deps.PushSkipped {
		return p.stuckOrCancelled(ctx, fmt.Sprintf("push skipped; a pull request is required against parent branch %q", p.deps.ParentBranch))
	}

	// Step 3a: commit the product's own status file before the merge below, for the reason
	// Finalize's own identical step states: Shed rewrites that file on every transition and
	// commits it only at bootstrap, and fabricengine's merge guard refuses any tracked
	// modification on either side of the pair. It sits after step 2's early return deliberately --
	// a run that needs no pull request never merges here, so it has nothing to commit for.
	if p.deps.CommitStatus != nil {
		if err := p.deps.CommitStatus(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: commit status file: %w", publishName, err)
		}
	}

	// Step 4: catch the task worktree up with the parent branch first.
	mergeResult, err := p.resolver.Resolve(ctx, p.deps.ParentBranch)
	if err != nil {
		if cerr := cancelErr(ctx, publishName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: merge-in against parent branch %q: %w", publishName, p.deps.ParentBranch, err)
	}
	if mergeResult.Outcome == mergeresolve.OutcomeStuck {
		return p.stuckOrCancelled(ctx, mergeResult.Reason)
	}

	// Step 4a: verify the merged tree before anything leaves the worktree.
	// A merge can compile cleanly and still break tests,
	// and only a no-op merge leaves the tree the plan-level verify already passed.
	reason, err := p.gate.check(ctx, publishName, p.deps.ParentBranch, !mergeResult.AlreadyUpToDate)
	if err != nil {
		if cerr := cancelErr(ctx, publishName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", publishName, err)
	}
	if reason != "" {
		return p.stuckOrCancelled(ctx, reason)
	}

	// Step 5: push the task branch. Mandatory and load-bearing: agents commit per fix and never
	// push, so without this the task branch exists only locally, the create call fails, and the
	// resume query could never match either.
	if err := p.deps.PushBranch(); err != nil {
		if terr := transientFailure(ctx, publishName, "push task branch", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		if errors.Is(err, gitrepo.ErrPushRejected) {
			return p.stuckOrCancelled(ctx, "push rejected by the remote", "error", err)
		}
		return p.stuckOrCancelled(ctx, fmt.Sprintf("push failed: %v", err), "error", err)
	}

	// Step 6: resolve owner/repo from the told origin URL.
	owner, repo, err := githubclient.ParseOwnerRepo(p.deps.OriginURL)
	if err != nil {
		return p.stuckOrCancelled(ctx, fmt.Sprintf("origin URL unusable: %v", err), "error", err)
	}

	client, err := NewGitHubClient()
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", publishName, "action", "new github client", "cause", err)
		return p.stuckOrCancelled(ctx, fmt.Sprintf("github client unavailable: %v", err), "error", err)
	}

	pr, err := FindPullRequest(ctx, client, owner, repo, p.deps.TaskBranch, p.deps.ParentBranch)
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", publishName, "action", "query existing pull request", "owner", owner, "repo", repo, "cause", err)
		if terr := transientFailure(ctx, publishName, "query existing pull request", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		return p.stuckOrCancelled(ctx, publishGitHubErrorReason("query existing pull request", err))
	}

	// Step 8: branch on what the query found.
	if pr == nil {
		summary, err := summaryparser.Parse(p.deps.DescriptionPath)
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: parse change description: %w", publishName, err)
		}

		createCtx, createCancel := context.WithTimeout(ctx, publishGitHubTimeout)
		defer createCancel()

		created, _, err := client.PullRequests.Create(createCtx, owner, repo, &github.NewPullRequest{
			Title: &summary.Title,
			Body:  &summary.Body,
			Head:  &p.deps.TaskBranch,
			Base:  &p.deps.ParentBranch,
		})
		if err != nil {
			logger.Warn("landingshed: github call failed", "producer", publishName, "action", "create pull request", "owner", owner, "repo", repo, "cause", err)
			if terr := transientFailure(ctx, publishName, "create pull request", err); terr != nil {
				return "", shedengine.OutputPointer{}, terr
			}
			return p.stuckOrCancelled(ctx, publishGitHubErrorReason("create pull request", err))
		}
		logger.Info("landingshed: pull request created", "owner", owner, "repo", repo, "number", created.GetNumber())

		// Done is safe here: the next row is the PR-Gate, which owns the review wait.
		if cerr := cancelErr(ctx, publishName); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	switch {
	case pr.GetState() == "open":
		// No second pull request created and no second merge-in. The push at step 5 still ran, so
		// the pull request already carries any commits added since; its title and body are refreshed
		// from the change description. Done is safe: the next row is the PR-Gate.
		return p.refreshPullRequest(ctx, client, owner, repo, pr)
	case !pr.GetMergedAt().IsZero():
		// GitHub's List Pull Requests endpoint -- the query above -- never populates the "merged"
		// boolean field; that field is only ever set on the single-PR Get endpoint's response. Every
		// PullRequest this switch sees therefore carries a nil Merged, so pr.GetMerged() would be
		// false unconditionally regardless of the PR's real state -- confirmed live: a genuinely
		// merged PR queried through this same List call came back with "merged":null and a populated
		// "merged_at" (crucible round 3, F-R3-1). merged_at IS populated by List, and is GitHub's own
		// documented signal for "this PR is merged" on a list response, so it is what this branch
		// checks instead.
		return shedengine.Done, shedengine.OutputPointer{}, nil
	default:
		// Closed and not merged: a human decision to stop, which must never read as proceed.
		return p.stuckOrCancelled(ctx, withPRURL("the pull request was closed without being merged", pr.GetHTMLURL()))
	}
}

// refreshPullRequest brings an open pull request's title and body in line with the change
// description, editing only when either differs, and returns Done.
//
// An edit failure takes the same split as the create call: a transient failure is returned as an
// error so the driver re-steps once, and anything else is a Stuck verdict for a human.
func (p *Publish) refreshPullRequest(ctx context.Context, client *github.Client, owner, repo string, pr *github.PullRequest) (shedengine.Outcome, shedengine.OutputPointer, error) {
	summary, err := summaryparser.Parse(p.deps.DescriptionPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: parse change description: %w", publishName, err)
	}
	if pr.GetTitle() != summary.Title || pr.GetBody() != summary.Body {
		editCtx, editCancel := context.WithTimeout(ctx, publishGitHubTimeout)
		defer editCancel()

		_, _, err := client.PullRequests.Edit(editCtx, owner, repo, pr.GetNumber(), &github.PullRequest{
			Title: &summary.Title,
			Body:  &summary.Body,
		})
		if err != nil {
			logger.Warn("landingshed: github call failed", "producer", publishName, "action", "edit pull request", "owner", owner, "repo", repo, "cause", err)
			if terr := transientFailure(ctx, publishName, "edit pull request", err); terr != nil {
				return "", shedengine.OutputPointer{}, terr
			}
			return p.stuckOrCancelled(ctx, publishGitHubErrorReason("edit pull request", err))
		}
		logger.Info("landingshed: pull request refreshed", "owner", owner, "repo", repo, "number", pr.GetNumber())
	}
	if cerr := cancelErr(ctx, publishName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// stuckOrCancelled consults cancelErr first -- the point-9 obligation every non-success exit
// discharges -- and otherwise logs reason via reportStuck and returns Stuck with reason on the
// output pointer.
// Callers that can see a transient remote failure return it as an error first (transientFailure), so a verdict here is always one for a human.
func (p *Publish) stuckOrCancelled(ctx context.Context, reason string, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, publishName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(publishName, reason, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}

// withPRURL ends a pull-request-state reason with the pull request's URL, or returns the bare
// reason when the URL is empty.
func withPRURL(reason, url string) string {
	if url == "" {
		return reason
	}
	return reason + ": " + url
}

// contains reports whether target appears in list.
func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// publishGitHubErrorReason classifies a GitHub API call error into the same three classes
// internal/selfreportengine's CreateIssue does: an unresolvable token, an API rejection, and
// everything else -- so the reason text names which class the failure fell into.
func publishGitHubErrorReason(action string, err error) string {
	if errors.Is(err, githubclient.ErrTokenUnresolvable) {
		return fmt.Sprintf("%s: github token not resolvable: %v", action, err)
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		return fmt.Sprintf("%s: github API rejected the request: %s", action, strings.TrimSpace(ghErr.Message))
	}
	return fmt.Sprintf("%s: failed to reach GitHub: %v", action, err)
}
