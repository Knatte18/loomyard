// prgate.go implements the PR-Gate producer: the one row that owns every review decision after
// Publish has opened or refreshed the pull request.
// It reads the pull request and the operator's two decision records and lands the run, holds it
// awaiting a decision, or bounces the findings to the rework row.

package landingshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prGateName is the producer name PRGate's log lines and error text carry, and the row name the recipe pins.
const prGateName = "PR-Gate"

// PRGate is the ShedProducer that decides, from the pull request and the decision records, whether the run lands, waits or goes to rework.
type PRGate struct {
	deps Deps
}

var _ shedengine.ShedProducer = (*PRGate)(nil)

// NewPRGate constructs a PRGate from deps, rejecting an empty ApprovalPath, an empty RejectionPath and a nil TaskHead with a distinct error each.
func NewPRGate(deps Deps) (*PRGate, error) {
	if deps.ApprovalPath == "" {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.ApprovalPath must not be empty")
	}
	if deps.RejectionPath == "" {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.RejectionPath must not be empty")
	}
	if deps.TaskHead == nil {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.TaskHead must not be nil")
	}
	return &PRGate{deps: deps}, nil
}

// Call runs one PR-Gate iteration, evaluating these rows top to bottom, first match wins:
//
//  1. the parent branch needs no pull request: Done, with no GitHub call;
//  2. the pull request is merged: Done;
//  3. there is no pull request, or it is closed unmerged: Awaiting;
//  4. either decision record is unreadable or malformed: Awaiting;
//  5. an approval matches the pull request number, its head and the local task head: Done;
//  6. a rejection matches all three: Stuck, the only Stuck exit, which the recipe routes to rework;
//  7. a record is present but stale: Awaiting;
//  8. no record: Awaiting.
//
// A transient query failure is returned as an error, never marked on a verdict.
func (g *PRGate) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, prGateName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if !contains(g.deps.Config.RequirePRToBase, g.deps.ParentBranch) {
		return g.done(ctx)
	}

	owner, repo, err := githubclient.ParseOwnerRepo(g.deps.OriginURL)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("origin URL unusable: %v", err))
	}
	client, err := NewGitHubClient()
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prGateName, "action", "new github client", "cause", err)
		return g.awaiting(ctx, fmt.Sprintf("github client unavailable: %v", err))
	}
	pr, err := FindPullRequest(ctx, client, owner, repo, g.deps.TaskBranch, g.deps.ParentBranch)
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prGateName, "action", "query existing pull request", "owner", owner, "repo", repo, "cause", err)
		if terr := transientFailure(ctx, prGateName, "query existing pull request", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		return g.awaiting(ctx, publishGitHubErrorReason("query existing pull request", err))
	}

	switch {
	case pr != nil && !pr.GetMergedAt().IsZero():
		return g.done(ctx)
	case pr == nil:
		return g.awaiting(ctx, "no pull request exists for the task branch; reopen the pull request or abandon the task")
	case pr.GetState() != "open":
		return g.awaiting(ctx, withPRURL("the pull request was closed without being merged; reopen the pull request or abandon the task", pr.GetHTMLURL()))
	}

	approval, approved, err := ReadApproval(g.deps.ApprovalPath)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("the approval record %s is unreadable or malformed (%v); re-run `lyx loom approve` to overwrite it", g.deps.ApprovalPath, err))
	}
	rejection, rejected, err := ReadRejection(g.deps.RejectionPath)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("the rejection record %s is unreadable or malformed (%v); re-run `lyx loom reject` to overwrite it", g.deps.RejectionPath, err))
	}
	if !approved && !rejected {
		return g.awaiting(ctx, withPRURL("the pull request awaits review; run `lyx loom approve` to land it or `lyx loom reject` to send findings to rework", pr.GetHTMLURL()))
	}

	head := pr.GetHead().GetSHA()
	taskHead, err := g.deps.TaskHead()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read task branch head: %w", prGateName, err)
	}

	if approved && pr.GetNumber() == approval.PRNumber && head == approval.HeadSHA && taskHead == approval.HeadSHA {
		return g.done(ctx)
	}
	if rejected && pr.GetNumber() == rejection.PRNumber && head == rejection.HeadSHA && taskHead == rejection.HeadSHA {
		return g.stuck(ctx, withPRURL(fmt.Sprintf("pull request #%d was rejected at %s; the findings go to rework", rejection.PRNumber, rejection.HeadSHA), pr.GetHTMLURL()))
	}

	kind, verb, recNumber, recHead := "approval", "approve", approval.PRNumber, approval.HeadSHA
	if !approved {
		kind, verb, recNumber, recHead = "rejection", "reject", rejection.PRNumber, rejection.HeadSHA
	}
	var mismatch string
	switch {
	case head != recHead:
		mismatch = fmt.Sprintf("the pull request head is now %s", head)
	case taskHead != recHead:
		mismatch = fmt.Sprintf("the local task head is now %s", taskHead)
	default:
		mismatch = fmt.Sprintf("the open pull request is now #%d", pr.GetNumber())
	}
	return g.awaiting(ctx, withPRURL(fmt.Sprintf("the %s of pull request #%d at %s no longer matches: %s; inspect the pull request and re-run `lyx loom approve` or `lyx loom reject` (the record was written by `lyx loom %s`)", kind, recNumber, recHead, mismatch, verb), pr.GetHTMLURL()))
}

// done returns Done after consulting cancelErr first, so every exit of Call, Done included, reports an operator stop as the cancellation error.
func (g *PRGate) done(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// awaiting ends a call at a human hand-off: a cancelled context returns the cancellation error, and otherwise it logs reason and returns Awaiting with reason on the output pointer.
func (g *PRGate) awaiting(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	logger.Info("landingshed: producer awaiting", "producer", prGateName, "reason", reason)
	return shedengine.Awaiting, shedengine.OutputPointer{Reason: reason}, nil
}

// stuck returns Stuck with reason after consulting cancelErr first.
func (g *PRGate) stuck(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	reportStuck(prGateName, reason)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}
