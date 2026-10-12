// pralign.go implements the PR-Align producer: the row before PR-Gate that spawns the alignment agent for the open pull request and applies its verdict through the records PR-Gate reads.
// Every outcome it does not apply, and every failure, is left to the run's parent through PR-Gate, with an alignment note naming why.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/google/go-github/v75/github"
)

// prAlignName is the producer name PRAlign's log lines and error text carry, and the row name the recipe pins.
const prAlignName = "PR-Align"

// alignmentTickFloor is the poll interval a zero AlignmentDeps.Tick takes.
const alignmentTickFloor = time.Second

// The reasons a note carries when the agent's approve or reject is not applied.
const (
	decisionRecordedReason = "a decision was recorded for this task while the alignment agent ran"
	approveNotGivenReason  = "the alignment agent approves; alignment_pr_approve is false, so the approval is the run's parent's to give"
	secondRejectReason     = "the alignment agent rejected this run once already; a second agent reject goes to the run's parent"
	pausedReason           = "the run was paused while the alignment agent ran"
)

// AlignmentConfig is the alignment settings PR-Align reads on each call.
type AlignmentConfig struct {
	// Off is true when alignment is switched off, so PR-Align spawns nothing.
	Off bool
	// PRApprove is true when an agent approve is applied as the approval; false leaves the approval to the run's parent.
	PRApprove bool
	// Settings is the agent's resolved model settings and timeout.
	Settings alignment.Settings
}

// AlignmentSession is the alignment agent's session as PR-Align drives it; *alignment.Session satisfies it.
type AlignmentSession interface {
	// Begin spawns the agent for in.Key, or returns the attempt that already holds the key.
	Begin(in alignment.SpawnInput) (alignment.Attempt, error)
	// Poll reports where the attempt stands without blocking.
	Poll(a alignment.Attempt) (alignment.Outcome, error)
	// Close stops the attempt's agent if it is live and closes the attempt.
	Close(a alignment.Attempt, reason string) error
	// MarkApplied records the verdict Go applied and whether the parent was notified of it.
	MarkApplied(a alignment.Attempt, applied string, notified bool) error
	// Attempts lists every attempt of the session, sorted by number.
	Attempts() ([]alignment.Attempt, error)
}

var _ AlignmentSession = (*alignment.Session)(nil)

// AlignmentDeps carries the told seams PR-Align runs on.
type AlignmentDeps struct {
	// Config is read on each call.
	Config func() (AlignmentConfig, error)
	// Session is the agent's session at the PR-Gate point.
	Session AlignmentSession
	// Snapshot reads the decision snapshot: the decision record and the notes naming the task.
	Snapshot func() (string, error)
	// Prompt holds the run's static prompt fields; PR-Align fills the point, the parent branch, the pull request number and head.
	Prompt alignment.PromptInput
	// NotePath is the alignment note record.
	// Empty means no note, which is how a run with no PR-Align row is built.
	NotePath string
	// Tick is the poll interval; zero takes one second.
	Tick time.Duration
}

// PRAlign is the ShedProducer that has the alignment agent settle the open pull request's review, or leaves it to the run's parent.
//
// Bound: an agent approve writes the approval record, so PR-Gate lands the run and batten's Land row deploys with no orch turn.
// That is bounded by alignment_pr_approve, the decision snapshot and head checks, and one agent reject per run.
// PRAlign takes no parent lock.
type PRAlign struct {
	deps Deps
}

var _ shedengine.ShedProducer = (*PRAlign)(nil)

// NewPRAlign constructs a PRAlign from deps, rejecting an empty ApprovalPath, an empty RejectionPath, a nil TaskHead, a nil Alignment.Config, Alignment.Session or Alignment.Snapshot, and an empty Alignment.NotePath with a distinct error each.
func NewPRAlign(deps Deps) (*PRAlign, error) {
	switch {
	case deps.ApprovalPath == "":
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.ApprovalPath must not be empty")
	case deps.RejectionPath == "":
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.RejectionPath must not be empty")
	case deps.TaskHead == nil:
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.TaskHead must not be nil")
	case deps.Alignment.Config == nil:
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.Alignment.Config must not be nil")
	case deps.Alignment.Session == nil:
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.Alignment.Session must not be nil")
	case deps.Alignment.Snapshot == nil:
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.Alignment.Snapshot must not be nil")
	case deps.Alignment.NotePath == "":
		return nil, fmt.Errorf("landingshed: NewPRAlign: Deps.Alignment.NotePath must not be empty")
	}
	return &PRAlign{deps: deps}, nil
}

// Call runs one PR-Align iteration.
// It ends Done on every path except a cancellation or an error a producer returns today, and never Stuck.
// A parent that needs no pull request, alignment switched off, and an unusable, absent, closed or merged pull request are Done with nothing spawned.
// So is a pull request that already carries a matching approval or rejection, or whose head already has a closed attempt.
// Otherwise it spawns the agent, or reuses the open attempt of the pull request's head, waits for its verdict and applies it through RecordApproval or RecordRejection.
// Anything it does not apply is written as an alignment note and left to the run's parent.
// A transient GitHub failure is returned as an error.
func (p *PRAlign) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, prAlignName); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if !contains(p.deps.Config.RequirePRToBase, p.deps.ParentBranch) {
		return p.done(ctx)
	}
	cfg, err := p.deps.Alignment.Config()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the alignment config: %w", prAlignName, err)
	}
	if cfg.Off {
		return p.done(ctx)
	}

	pr, stop, err := p.findPullRequest(ctx)
	if err != nil || stop {
		return p.finish(ctx, err)
	}
	number, head := pr.GetNumber(), pr.GetHead().GetSHA()
	if p.decided(number, head) {
		return p.done(ctx)
	}

	key := fmt.Sprintf("pr-%d-%s", number, head)
	attempts, err := p.deps.Alignment.Session.Attempts()
	if err != nil {
		return p.noteAndDone(ctx, AlignmentNote{PRNumber: number, HeadSHA: head, Reason: fmt.Sprintf("listing the alignment attempts failed: %v", err)})
	}
	for _, earlier := range attempts {
		if earlier.Key == key && earlier.Closed {
			return p.done(ctx)
		}
	}

	snapshot, err := p.deps.Alignment.Snapshot()
	if err != nil {
		return p.noteAndDone(ctx, AlignmentNote{PRNumber: number, HeadSHA: head, Reason: fmt.Sprintf("reading the decision snapshot failed: %v", err)})
	}
	prompt := p.deps.Alignment.Prompt
	prompt.Point = alignment.PointPRGate
	prompt.ParentBranch = p.deps.ParentBranch
	prompt.PRNumber = number
	prompt.HeadSHA = head
	attempt, err := p.deps.Alignment.Session.Begin(alignment.SpawnInput{
		Key:      key,
		Prompt:   prompt,
		Settings: cfg.Settings,
		Snapshot: snapshot,
		PRNumber: number,
		HeadSHA:  head,
	})
	if err != nil {
		return p.noteAndDone(ctx, AlignmentNote{PRNumber: number, HeadSHA: head, Reason: fmt.Sprintf("starting the alignment agent failed: %v", err)})
	}

	outcome, ended, err := p.wait(ctx, attempt)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if !ended.Done {
		return p.noteAndDone(ctx, AlignmentNote{PRNumber: number, HeadSHA: head, Reason: ended.Detail})
	}
	return p.apply(ctx, cfg, attempt, outcome, attempts, applyAt{number: number, head: head})
}

// waitEnd says how a wait ended without a verdict: Done is false, and Detail names why, after the attempt was closed.
type waitEnd struct {
	Done   bool
	Detail string
}

// wait polls the attempt once per tick until it is done.
// A Poll error and a pause close the attempt and end the wait with a detail; a cancelled context closes it and is returned as the cancellation error.
func (p *PRAlign) wait(ctx context.Context, attempt alignment.Attempt) (alignment.Outcome, waitEnd, error) {
	tick := p.deps.Alignment.Tick
	if tick <= 0 {
		tick = alignmentTickFloor
	}
	for {
		outcome, err := p.deps.Alignment.Session.Poll(attempt)
		if err != nil {
			p.closeAttempt(attempt, "polling the alignment agent failed")
			return alignment.Outcome{}, waitEnd{Detail: fmt.Sprintf("polling the alignment agent failed: %v", err)}, nil
		}
		if outcome.Done {
			return outcome, waitEnd{Done: true}, nil
		}
		if p.deps.Lock.PauseRequested != nil {
			paused, err := p.deps.Lock.PauseRequested()
			if err != nil {
				logger.Warn("landingshed: reading the pause request failed", "producer", prAlignName, "cause", err)
			}
			if err == nil && paused {
				p.closeAttempt(attempt, pausedReason)
				return alignment.Outcome{}, waitEnd{Detail: pausedReason}, nil
			}
		}
		timer := time.NewTimer(tick)
		select {
		case <-ctx.Done():
			timer.Stop()
			p.closeAttempt(attempt, "the run was interrupted while the alignment agent ran")
			return alignment.Outcome{}, waitEnd{}, cancelErr(ctx, prAlignName)
		case <-timer.C:
		}
	}
}

// applyAt carries what Call read before the spawn.
type applyAt struct {
	number int
	head   string
}

// apply reads the pull request and the task head again and applies the finished attempt's outcome, or writes the note that says why it was not applied.
// Every path closes the attempt.
func (p *PRAlign) apply(ctx context.Context, cfg AlignmentConfig, attempt alignment.Attempt, outcome alignment.Outcome, attempts []alignment.Attempt, spawned applyAt) (shedengine.Outcome, shedengine.OutputPointer, error) {
	pr, stop, err := p.findPullRequest(ctx)
	if err != nil {
		// The verdict file survives, so the next step polls the open attempt again.
		return p.finish(ctx, err)
	}
	note := AlignmentNote{PRNumber: spawned.number, HeadSHA: spawned.head}
	if !stop {
		note.PRNumber, note.HeadSHA = pr.GetNumber(), pr.GetHead().GetSHA()
	}
	if _, statErr := os.Stat(attempt.ReportPath()); statErr == nil {
		note.Report = attempt.ReportPath()
	}
	leave := func(note AlignmentNote) (shedengine.Outcome, shedengine.OutputPointer, error) {
		p.closeAttempt(attempt, "the alignment verdict was handled")
		return p.noteAndDone(ctx, note)
	}

	if stop {
		note.Reason = "the pull request is no longer open for review"
		return leave(note)
	}
	taskHead, err := p.deps.TaskHead()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read task branch head: %w", prAlignName, err)
	}

	switch {
	case outcome.Cause != "":
		note.Reason = fmt.Sprintf("the alignment agent gave no verdict (%s): %s", outcome.Cause, outcome.Detail)
		return leave(note)
	case note.PRNumber != spawned.number || note.HeadSHA != spawned.head:
		note.Reason = fmt.Sprintf("the pull request moved to #%d at %s while the alignment agent ran", note.PRNumber, note.HeadSHA)
		return leave(note)
	case taskHead != note.HeadSHA:
		note.Reason = fmt.Sprintf("the local task head %s differs from the pull request head %s", taskHead, note.HeadSHA)
		return leave(note)
	case outcome.Verdict.Verdict == alignment.VerdictEscalate:
		note.Reason = outcome.Verdict.Question
		return leave(note)
	}

	if outcome.Verdict.Verdict == alignment.VerdictApprove {
		current, err := p.deps.Alignment.Snapshot()
		if err != nil {
			note.Reason = fmt.Sprintf("reading the decision snapshot failed: %v", err)
			return leave(note)
		}
		if current != attempt.Snapshot {
			note.Reason = decisionRecordedReason
			return leave(note)
		}
		if !cfg.PRApprove {
			note.Reason = approveNotGivenReason
			return leave(note)
		}
		return p.applyApprove(ctx, attempt, outcome.Verdict, note)
	}

	for _, earlier := range attempts {
		if earlier.Key != attempt.Key && earlier.Applied == alignment.VerdictReject {
			note.Reason = secondRejectReason
			return leave(note)
		}
	}
	return p.applyReject(ctx, attempt, note)
}

// applyApprove records the approval of the note's pull request and head, queues the supersede notice once, and marks the attempt applied.
func (p *PRAlign) applyApprove(ctx context.Context, attempt alignment.Attempt, verdict alignment.Verdict, note AlignmentNote) (shedengine.Outcome, shedengine.OutputPointer, error) {
	approval := Approval{PRNumber: note.PRNumber, HeadSHA: note.HeadSHA, ApprovedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := RecordApproval(p.deps.ApprovalPath, p.deps.RejectionPath, approval); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: record the approval: %w", prAlignName, err)
	}
	notified := attempt.Notified
	if len(verdict.Superseded) > 0 && !notified && p.deps.Notify != nil {
		line := fmt.Sprintf("alignment approved %s pull request #%d and supersedes board entries to amend: %s", p.deps.Alignment.Prompt.Slug, note.PRNumber, strings.Join(verdict.Superseded, ", "))
		if err := p.deps.Notify(line); err != nil {
			logger.Warn("landingshed: queueing the supersede notice failed", "producer", prAlignName, "cause", err)
		} else {
			notified = true
		}
	}
	if err := p.deps.Alignment.Session.MarkApplied(attempt, alignment.VerdictApprove, notified); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: mark the attempt applied: %w", prAlignName, err)
	}
	p.closeAttempt(attempt, "the alignment verdict was applied")
	return p.done(ctx)
}

// applyReject marks the attempt applied as a reject and then records the rejection with the agent's report as its findings.
// The mark comes first, so a restart between the two never leaves a written rejection uncounted.
func (p *PRAlign) applyReject(ctx context.Context, attempt alignment.Attempt, note AlignmentNote) (shedengine.Outcome, shedengine.OutputPointer, error) {
	findings, err := os.ReadFile(attempt.ReportPath())
	if err != nil || strings.TrimSpace(string(findings)) == "" {
		note.Reason = "the alignment agent rejected, but its report cannot be read as findings"
		p.closeAttempt(attempt, "the alignment verdict was handled")
		return p.noteAndDone(ctx, note)
	}
	if err := p.deps.Alignment.Session.MarkApplied(attempt, alignment.VerdictReject, false); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: mark the attempt applied: %w", prAlignName, err)
	}
	rejection := Rejection{PRNumber: note.PRNumber, HeadSHA: note.HeadSHA, RejectedAt: time.Now().UTC().Format(time.RFC3339), Findings: string(findings)}
	if err := RecordRejection(p.deps.ApprovalPath, p.deps.RejectionPath, rejection); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: record the rejection: %w", prAlignName, err)
	}
	p.closeAttempt(attempt, "the alignment verdict was applied")
	return p.done(ctx)
}

// findPullRequest reads the open pull request for the task branch.
// stop is true, with a nil pull request and a nil error, when the row has nothing to align: the origin or client is unusable, or the pull request is absent, closed or merged, each logged and left to PR-Gate to report.
// A transient GitHub failure comes back as the error.
func (p *PRAlign) findPullRequest(ctx context.Context) (pr *github.PullRequest, stop bool, err error) {
	owner, repo, err := githubclient.ParseOwnerRepo(p.deps.OriginURL)
	if err != nil {
		logger.Warn("landingshed: origin URL unusable; leaving the review to PR-Gate", "producer", prAlignName, "cause", err)
		return nil, true, nil
	}
	client, err := NewGitHubClient()
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prAlignName, "action", "new github client", "cause", err)
		return nil, true, nil
	}
	found, err := FindPullRequest(ctx, client, owner, repo, p.deps.TaskBranch, p.deps.ParentBranch)
	if err != nil {
		logger.Warn("landingshed: github call failed", "producer", prAlignName, "action", "query existing pull request", "owner", owner, "repo", repo, "cause", err)
		if terr := transientFailure(ctx, prAlignName, "query existing pull request", err); terr != nil {
			return nil, false, terr
		}
		return nil, true, nil
	}
	if found == nil || found.GetState() != "open" || !found.GetMergedAt().IsZero() {
		logger.Info("landingshed: no open pull request to align; leaving the review to PR-Gate", "producer", prAlignName)
		return nil, true, nil
	}
	return found, false, nil
}

// decided reports whether an approval or a rejection record already matches the pull request number and head.
// An unreadable record is not a match, so the row proceeds and PR-Gate reports the record.
func (p *PRAlign) decided(number int, head string) bool {
	if approval, found, err := ReadApproval(p.deps.ApprovalPath); err == nil && found && approval.PRNumber == number && approval.HeadSHA == head {
		return true
	}
	if rejection, found, err := ReadRejection(p.deps.RejectionPath); err == nil && found && rejection.PRNumber == number && rejection.HeadSHA == head {
		return true
	}
	return false
}

// noteAndDone writes n as the alignment note and ends Done.
func (p *PRAlign) noteAndDone(ctx context.Context, n AlignmentNote) (shedengine.Outcome, shedengine.OutputPointer, error) {
	logger.Info("landingshed: alignment left the review to the run's parent", "producer", prAlignName, "pull_request", n.PRNumber, "reason", n.Reason)
	if err := WriteAlignmentNote(p.deps.Alignment.NotePath, n); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", prAlignName, err)
	}
	return p.done(ctx)
}

// closeAttempt closes the attempt with reason; a failure is logged and never stops the row.
func (p *PRAlign) closeAttempt(attempt alignment.Attempt, reason string) {
	if err := p.deps.Alignment.Session.Close(attempt, reason); err != nil {
		logger.Warn("landingshed: closing the alignment attempt failed", "producer", prAlignName, "attempt", attempt.Number, "cause", err)
	}
}

// finish ends a call whose pull request read stopped it: the error when there is one, and Done otherwise.
func (p *PRAlign) finish(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return p.done(ctx)
}

// done returns Done after consulting cancelErr first, so an operator stop is reported as the cancellation error.
func (p *PRAlign) done(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, prAlignName); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
