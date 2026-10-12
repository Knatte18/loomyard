// prgate.go implements the PR-Gate producer: the one row that owns every review decision after Publish has opened or refreshed the pull request.
// It reads the pull request and the operator's two decision records and lands the run, holds it awaiting a decision, or bounces the findings to the rework row.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prGateName is the producer name PRGate's log lines and error text carry, and the row name the recipe pins.
const prGateName = "PR-Gate"

// PRGate is the ShedProducer that decides, from the pull request and the decision records, whether the run lands, waits or goes to rework.
type PRGate struct {
	deps Deps
	// guard holds the parent lock and tests whether the parent moved.
	// A zero value takes no lock and reads no tips, so struct-literal tests run without it.
	guard parentGuard
}

var _ shedengine.ShedProducer = (*PRGate)(nil)

// NewPRGate constructs a PRGate from deps, rejecting an empty ApprovalPath, an empty RejectionPath, a nil TaskHead, a nil Lock.Store, a nil Lock.Owner and a nil ReadParentTips with a distinct error each.
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
	if deps.Lock.Store == nil {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.Lock.Store must not be nil")
	}
	if deps.Lock.Owner == nil {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.Lock.Owner must not be nil")
	}
	if deps.ReadParentTips == nil {
		return nil, fmt.Errorf("landingshed: NewPRGate: Deps.ReadParentTips must not be nil")
	}
	return &PRGate{deps: deps, guard: newParentGuard(deps)}, nil
}

// Call runs one PR-Gate iteration, evaluating these rows top to bottom, first match wins:
//
//  1. the parent branch needs no pull request: Done, with no GitHub call;
//  2. the run cannot take the parent lock (a paused wait or an unreadable record: a halting Stuck), or the parent moved since Merge-In merged it (a reroute Stuck), both before any GitHub call;
//  3. the pull request is merged: Done;
//  4. there is no pull request, or it is closed unmerged: Awaiting;
//  5. either decision record is unreadable or malformed: Awaiting;
//  6. an approval matches the pull request number, its head and the local task head: Done;
//  7. a rejection matches all three: Stuck after releasing the run's parent lock, the only Stuck exit that routes by on_stuck, which the recipe routes to rework;
//  8. a record is present but stale: Awaiting;
//  9. no record: Awaiting.
//
// A transient query failure is returned as an error, never marked on a verdict.
func (g *PRGate) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, prGateName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if !contains(g.deps.Config.RequirePRToBase, g.deps.ParentBranch) {
		return g.done(ctx)
	}

	// A re-call after a resume takes back a record handed to batten, since an acquire by the same run is granted.
	if outcome, out, err, stop := holdParent(ctx, g.guard, prGateName); stop {
		return outcome, out, err
	}

	owner, repo, err := githubclient.ParseOwnerRepo(g.deps.OriginURL)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("origin URL unusable: %v", err), "")
	}
	client, err := NewGitHubClient()
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prGateName, "action", "new github client", "cause", err)
		return g.awaiting(ctx, fmt.Sprintf("github client unavailable: %v", err), "")
	}
	pr, err := FindPullRequest(ctx, client, owner, repo, g.deps.TaskBranch, g.deps.ParentBranch)
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prGateName, "action", "query existing pull request", "owner", owner, "repo", repo, "cause", err)
		if terr := transientFailure(ctx, prGateName, "query existing pull request", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		return g.awaiting(ctx, publishGitHubErrorReason("query existing pull request", err), "")
	}

	switch {
	case pr != nil && !pr.GetMergedAt().IsZero():
		return g.done(ctx)
	case pr == nil:
		return g.awaiting(ctx, "no pull request exists for the task branch; reopen the pull request or abandon the task", "")
	case pr.GetState() != "open":
		return g.awaiting(ctx, withPRURL("the pull request was closed without being merged; reopen the pull request or abandon the task", pr.GetHTMLURL()), "")
	}

	approval, approved, err := ReadApproval(g.deps.ApprovalPath)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("the approval record %s is unreadable or malformed (%v); re-run `lyx loom approve` to overwrite it", g.deps.ApprovalPath, err), "")
	}
	rejection, rejected, err := ReadRejection(g.deps.RejectionPath)
	if err != nil {
		return g.awaiting(ctx, fmt.Sprintf("the rejection record %s is unreadable or malformed (%v); re-run `lyx loom reject` to overwrite it", g.deps.RejectionPath, err), "")
	}
	if !approved && !rejected {
		clause := g.alignmentClause(pr.GetNumber(), pr.GetHead().GetSHA())
		return g.awaiting(ctx, clause+withPRURL("the pull request awaits review; run `lyx loom approve` to land it or `lyx loom reject` to send findings to rework", pr.GetHTMLURL()), clause)
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
	clause := g.alignmentClause(pr.GetNumber(), head)
	return g.awaiting(ctx, clause+withPRURL(fmt.Sprintf("the %s of pull request #%d at %s no longer matches: %s; inspect the pull request and re-run `lyx loom approve` or `lyx loom reject` (the record was written by `lyx loom %s`)", kind, recNumber, recHead, mismatch, verb), pr.GetHTMLURL()), clause)
}

// alignmentClause returns the clause that opens an awaiting reason when the alignment note matches the pull request number and head: the agent's report path first, then why it did not settle the review.
// It is empty when no note path is told, no note exists or the note concerns another pull request or head; an unreadable note is logged and adds nothing.
func (g *PRGate) alignmentClause(number int, head string) string {
	if g.deps.Alignment.NotePath == "" {
		return ""
	}
	note, found, err := ReadAlignmentNote(g.deps.Alignment.NotePath)
	if err != nil {
		logger.Warn("landingshed: alignment note unreadable; naming no alignment report", "producer", prGateName, "cause", err)
		return ""
	}
	if !found || note.PRNumber != number || note.HeadSHA != head {
		return ""
	}
	if note.Report == "" {
		return fmt.Sprintf("alignment did not settle this pull request - %s; ", note.Reason)
	}
	return fmt.Sprintf("alignment report %s did not settle this pull request - %s; ", note.Report, note.Reason)
}

// done returns Done after consulting cancelErr first, so every exit of Call, Done included, reports an operator stop as the cancellation error.
func (g *PRGate) done(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// awaiting ends a call at a human hand-off: a cancelled context returns the cancellation error, and otherwise it logs reason and returns Awaiting with reason on the output pointer.
// A non-empty notice, without its trailing separator, rides on the pointer as the one-line notice for the run's parent.
func (g *PRGate) awaiting(ctx context.Context, reason, notice string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	logger.Info("landingshed: producer awaiting", "producer", prGateName, "reason", reason)
	return shedengine.Awaiting, shedengine.OutputPointer{Reason: reason, ParentNotice: strings.TrimSuffix(notice, "; ")}, nil
}

// stuck returns Stuck with reason after consulting cancelErr first, and after releasing the run's parent lock.
// The only Stuck exit is a matching rejection, which leaves the window between Merge-In and Finalize for rework; the reworked run takes the lock again at Merge-In.
func (g *PRGate) stuck(ctx context.Context, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prGateName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	g.guard.release(prGateName)
	reportStuck(prGateName, reason)
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}
