// publish.go implements the Publish producer: when the task's parent branch requires a pull
// request, it merges the parent's own catch-up into the task worktree, pushes the task branch, and
// opens or refreshes the pull request against that parent -- reporting only what the
// shedengine.ShedProducer seam can carry: a bare verdict, an output pointer nobody persists, and an
// error.
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
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// publishName is the producer name Publish's log lines, error text, and stuck-reason filename
// carry.
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

	return &Publish{deps: deps, resolver: res}, nil
}

// Call runs one Publish iteration.
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

	// Step 3b: commit the product's own status file before the merge below, for the reason
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

	// Step 5: push the task branch. Mandatory and load-bearing: agents commit per fix and never
	// push, so without this the task branch exists only locally, the create call fails, and the
	// resume query could never match either.
	if err := p.deps.PushBranch(); err != nil {
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

	queryCtx, cancel := context.WithTimeout(ctx, publishGitHubTimeout)
	defer cancel()

	prs, _, err := client.PullRequests.List(queryCtx, owner, repo, &github.PullRequestListOptions{
		State:     "all",
		Head:      fmt.Sprintf("%s:%s", owner, p.deps.TaskBranch),
		Base:      p.deps.ParentBranch,
		Sort:      "created",
		Direction: "desc",
	})
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", publishName, "action", "query existing pull request", "owner", owner, "repo", repo, "cause", err)
		return p.stuckOrCancelled(ctx, publishGitHubErrorReason("query existing pull request", err))
	}

	// Step 8: branch on what the query found.
	if len(prs) == 0 {
		summary, err := websterengine.ParseSummary(websterengine.SummaryPath(p.deps.WebsterDir))
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: parse summary artifact: %w", publishName, err)
		}

		createCtx, createCancel := context.WithTimeout(ctx, publishGitHubTimeout)
		defer createCancel()

		if _, _, err := client.PullRequests.Create(createCtx, owner, repo, &github.NewPullRequest{
			Title: &summary.Title,
			Body:  &summary.Body,
			Head:  &p.deps.TaskBranch,
			Base:  &p.deps.ParentBranch,
		}); err != nil {
			logger.Warn("landingshed: github call failed", "producer", publishName, "action", "create pull request", "owner", owner, "repo", repo, "cause", err)
			return p.stuckOrCancelled(ctx, publishGitHubErrorReason("create pull request", err))
		}

		// Stuck rather than done: a done verdict would let the run advance to the next row and
		// merge to the parent seconds after opening the pull request, defeating it entirely.
		return p.stuckOrCancelled(ctx, "pull request created; awaiting review")
	}

	pr := prs[0]
	switch {
	case pr.GetState() == "open":
		// No second pull request created and no second merge-in. The push at step 5 still ran, so
		// a resumed call refreshes the pull request with any commits added since.
		return p.stuckOrCancelled(ctx, fmt.Sprintf("an open pull request already exists against parent branch %q", p.deps.ParentBranch))
	case pr.GetMerged():
		return shedengine.Done, shedengine.OutputPointer{}, nil
	default:
		// Closed and not merged: a human decision to stop, which must never read as proceed.
		return p.stuckOrCancelled(ctx, "the pull request was closed without being merged")
	}
}

// stuckOrCancelled consults cancelErr first -- the point-9 obligation every non-success exit
// discharges -- and otherwise records reason via reportStuck and returns a bare Stuck verdict.
func (p *Publish) stuckOrCancelled(ctx context.Context, reason string, fields ...any) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, publishName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(publishName, reason, p.deps.ScratchDir, fields...)
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
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
