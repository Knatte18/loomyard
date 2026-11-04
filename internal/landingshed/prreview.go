// prreview.go implements the PR-Review row: it decides whether the landing goes through a pull request, and when it does pushes the task branch, opens or finds the pull request and records it, then waits on the pull request until the review ends.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

const (
	// prWaitFloor is the shortest time between two wakes of the wait, and the one a moved task head returns it to.
	prWaitFloor = time.Second
	// prWaitCeiling is the longest time between two wakes while nothing changes.
	prWaitCeiling = time.Minute
	// prPollInterval is how long the wait leaves between two reads of the pull request's status.
	prPollInterval = time.Minute
)

// PRReview is the ShedProducer that lands a run through a GitHub pull request when ResolvePullRequest says so.
// A no closes any pull request the run recorded and passes, except that a failed read of GitHub's rules with a pull request recorded is Stuck and closes nothing; a yes pushes the task branch, finds or opens the pull request, sets its title from the change description and its body from the description's body, the regression check and the credit block, records it, keeps one interactive session for the operator to discuss it in, and waits on it.
// The wait gates and pushes the agent's commits, sets the title and body again when the text yielded from the change description, the regression check and the credit block changes, and ends when the pull request is merged or approved, or closed or continued by the operator.
// A reopen request, read on entry and at every wake, ends the row first: it sends the run back through the row's reroute edge, the pull request kept open as a draft or closed.
// It takes no parent lock.
type PRReview struct {
	name string
	deps Deps
	gate verifyGate
}

var _ shedengine.ShedProducer = (*PRReview)(nil)

// NewPRReview constructs a PRReview that reports under name and reads its told values from deps.
func NewPRReview(name string, deps Deps) *PRReview {
	gate := newVerifyGate(deps)
	if deps.PullRequest.DirtyPaths != nil {
		gate.dirty = func(string) ([]string, error) { return deps.PullRequest.DirtyPaths() }
	}
	return &PRReview{name: name, deps: deps, gate: gate}
}

// Call runs the row once and is safe to re-enter: a record, an open pull request on the task branch or a recorded ending of the review each keep a re-entry from opening a second pull request.
// A yes starts or resumes the run's one interactive session and ends in the wait on the pull request, which has no timeout.
// The session is stopped on every verdict, Stuck or error of the row, and left running when the context is cancelled.
// A cancelled context is an error, as for every landing row; a transient push, GitHub or session-start failure is an error the driver re-steps, and any other failure is Stuck with Halt.
func (p *PRReview) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkWiring(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	outcome, pointer, err := p.decide(ctx)
	var halt *sessionStartHalt
	if errors.As(err, &halt) {
		outcome, pointer, err = stuckVerdict(ctx, p.name, halt.haltReason(), true)
	}
	p.endSession(ctx, err)
	return outcome, pointer, err
}

// decide answers a standing reopen request first; otherwise it resolves whether the landing goes through a pull request and runs the row's side of the answer.
func (p *PRReview) decide(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	pr := p.deps.PullRequest
	if outcome, pointer, err, answered := p.answerReopen(ctx); answered {
		return outcome, pointer, err
	}

	mode, worktree, err := pr.ParentMode()
	if err != nil {
		return stuckVerdict(ctx, p.name, beamModeUnreadableReason(worktree, err), true)
	}
	field, err := pr.EntryField()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the landing entry's pull_request: %w", p.name, err)
	}
	origin, err := pr.Origin()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the origin: %w", p.name, err)
	}
	hasCommits, err := pr.TaskHasCommits()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read whether the task branch has commits beyond its parent: %w", p.name, err)
	}

	_, recordFound, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)
	}
	decision, err := resolveWithParentRule(ctx, p.name, p.deps, mode, field, origin, hasCommits, recordFound)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if decision.Halt != "" {
		return stuckVerdict(ctx, p.name, decision.Halt, true)
	}
	if !decision.Open {
		return p.closeRecordedPullRequest(ctx, origin, decision.Reason)
	}
	return p.openPullRequest(ctx, origin)
}

// checkWiring returns an error naming the first told value the row needs and Deps leaves unset.
func (p *PRReview) checkWiring() error {
	pr := p.deps.PullRequest
	var missing string
	switch {
	case pr.RecordPath == "":
		missing = "PullRequest.RecordPath"
	case pr.RecordLockPath == "":
		missing = "PullRequest.RecordLockPath"
	case pr.ParentMode == nil:
		missing = "PullRequest.ParentMode"
	case pr.EntryField == nil:
		missing = "PullRequest.EntryField"
	case pr.Origin == nil:
		missing = "PullRequest.Origin"
	case pr.API == nil:
		missing = "PullRequest.API"
	case pr.PushTask == nil:
		missing = "PullRequest.PushTask"
	case pr.CommitsMissing == nil:
		missing = "PullRequest.CommitsMissing"
	case pr.TaskHasCommits == nil:
		missing = "PullRequest.TaskHasCommits"
	case pr.DirtyPaths == nil:
		missing = "PullRequest.DirtyPaths"
	case pr.MailAgent == nil:
		missing = "PullRequest.MailAgent"
	case pr.PauseCovered == nil:
		missing = "PullRequest.PauseCovered"
	case pr.Now == nil:
		missing = "PullRequest.Now"
	case pr.Sleep == nil:
		missing = "PullRequest.Sleep"
	case pr.RecordChanges == nil:
		missing = "PullRequest.RecordChanges"
	case pr.Sessions == nil:
		missing = "PullRequest.Sessions"
	case pr.SessionLive == nil:
		missing = "PullRequest.SessionLive"
	case pr.RegisterSession == nil:
		missing = "PullRequest.RegisterSession"
	case p.deps.TaskHead == nil:
		missing = "TaskHead"
	}
	if missing != "" {
		return fmt.Errorf("landingshed: %s: Deps.%s is not set", p.name, missing)
	}
	return nil
}

// closeRecordedPullRequest ends the run's pull request when the landing no longer goes through one.
// It logs reason, closes a recorded pull request GitHub reports open, stops the recorded session, drops the record, and returns Done.
func (p *PRReview) closeRecordedPullRequest(ctx context.Context, origin Origin, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	logger.Info("landingshed: no pull request for this landing", "row", p.name, "reason", reason)
	pr := p.deps.PullRequest

	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)
	}
	if !found {
		return shedengine.Done, shedengine.OutputPointer{}, nil
	}

	api, err := pr.API(origin.URL)
	if err != nil {
		return p.githubFailure(ctx, "open the GitHub client", err)
	}
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return p.githubFailure(ctx, "read the pull request's status", err)
	}
	if status.State == pullrequest.StateOpen {
		if err := api.Close(ctx, rec.Number, "lym closed this pull request: "+reason); err != nil {
			return p.githubFailure(ctx, "close the pull request", err)
		}
	}
	// The session is found through the record, so it is stopped before the record goes, and a failed stop keeps the record for the re-step to retry.
	if _, err := p.stopLiveSession(rec); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// openPullRequest pushes the task branch, then finds or opens the run's pull request, sets its title from the change description and its body from the description's body and the credit block, records it and waits on it.
// A pause that covers the run holds the call before the push.
func (p *PRReview) openPullRequest(ctx context.Context, origin Origin) (shedengine.Outcome, shedengine.OutputPointer, error) {
	pr := p.deps.PullRequest
	if err := p.holdWhileCovered(ctx); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if err := pr.PushTask(); err != nil {
		if terr := transientFailure(ctx, p.name, "push the task branch", err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
		return p.pushHalt(ctx, err)
	}

	head, err := p.deps.TaskHead()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the task head: %w", p.name, err)
	}
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)
	}
	api, err := pr.API(origin.URL)
	if err != nil {
		return p.githubFailure(ctx, "open the GitHub client", err)
	}
	if found && rec.ReviewEnded {
		// The operator already ended this review.
		// A head past the one gated then means commits were added since that need review, so the review starts afresh.
		if head == rec.GatedHead {
			return p.wait(ctx, api, rec, nil, time.Time{}, prText{})
		}
		if err := pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
		}
		found = false
	}

	text, err := p.pullRequestText()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", p.name, err)
	}

	// A recorded pull request GitHub no longer reports open is left as it is, for the wait to judge.
	var ref pullrequest.Ref
	var entryStatus *pullrequest.Status
	var entryReadAt time.Time
	editTitleAndBody := true
	if found {
		entryReadAt = pr.Now()
		status, err := api.Status(ctx, rec.Number)
		if err != nil {
			return p.githubFailure(ctx, "read the pull request's status", err)
		}
		entryStatus = &status
		// A reopen left the pull request a draft, which GitHub refuses to merge, so the review starts by marking it ready.
		if status.State == pullrequest.StateOpen && status.Draft {
			if err := api.SetDraft(ctx, rec.Number, false); err != nil {
				return p.githubFailure(ctx, "mark the pull request ready for review", err)
			}
		}
		if status.State != pullrequest.StateOpen {
			// A merged pull request has nothing left to discuss, so only a closed one gets a session, for the agent to clean the tree under a continue flag.
			if status.State != pullrequest.StateMerged {
				if rec, err = p.ensureSession(ctx, rec); err != nil {
					return "", shedengine.OutputPointer{}, err
				}
			}
			return p.wait(ctx, api, rec, entryStatus, entryReadAt, prText{})
		}
		ref = pullrequest.Ref{Number: rec.Number, URL: rec.URL}
	} else {
		existing, open, err := api.FindOpen(ctx, p.deps.TaskBranch, p.deps.ParentBranch)
		if err != nil {
			return p.githubFailure(ctx, "look for an open pull request", err)
		}
		ref = existing
		if !open {
			ref, err = api.Create(ctx, p.deps.TaskBranch, p.deps.ParentBranch, text.title, text.body)
			if err != nil {
				return p.githubFailure(ctx, "open the pull request", err)
			}
			editTitleAndBody = false
		}
	}
	if editTitleAndBody {
		if err := api.Edit(ctx, ref.Number, text.title, text.body); err != nil {
			return p.githubFailure(ctx, "set the pull request's title and body", err)
		}
	}
	if err := p.recordPullRequest(ref, head, found); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	rec, found, err = pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil || !found {
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read back the pull request record (found %v): %w", p.name, found, err)
	}
	if rec, err = p.ensureSession(ctx, rec); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return p.wait(ctx, api, rec, entryStatus, entryReadAt, text)
}

// prText is a pull request's title and body.
// A title is never empty, so the zero prText stands for a text not known.
type prText struct {
	title string
	body  string
}

// pullRequestText returns the title and body the pull request carries for the change description as it reads now: the description's title, and its body followed by the regression check and the credit block.
func (p *PRReview) pullRequestText() (prText, error) {
	summary, err := summaryparser.Parse(p.deps.DescriptionPath)
	if err != nil {
		return prText{}, fmt.Errorf("read the change description: %w", err)
	}
	credit, err := p.deps.creditBlock()
	if err != nil {
		return prText{}, err
	}
	verdict, err := p.deps.driftVerdict()
	if err != nil {
		return prText{}, err
	}
	return prText{title: summary.Title, body: bodyWithCredit(withSection(summary.Body, verdict), credit)}, nil
}

// syncText sets the pull request's title and body again when the change description no longer yields the text they were last set to, so an edit of the description reaches the pull request at the next poll.
// A wait that knows no last-set text takes the description's present text as it, and edits nothing.
// A text that cannot be built is logged once per episode and left for a later poll, since the agent may be mid-edit; a failed edit is judged as pollFailure judges one, with a transient failure's Warn logged once per episode of failed edits.
// It reports whether it ended the call.
func (p *PRReview) syncText(ctx context.Context, api pullrequest.API, state *prWaitState, number int) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	text, err := p.pullRequestText()
	if err != nil {
		if !state.textWarned {
			logger.Warn("landingshed: no pull request text could be built, so the pull request keeps its title and body", "row", p.name, "cause", err)
			state.textWarned = true
		}
		return "", shedengine.OutputPointer{}, false, nil
	}
	state.textWarned = false
	switch {
	case state.synced == prText{}:
		state.synced = text
		return "", shedengine.OutputPointer{}, false, nil
	case text == state.synced:
		state.editWarned = false
		return "", shedengine.OutputPointer{}, false, nil
	}
	if err := api.Edit(ctx, number, text.title, text.body); err != nil {
		return p.pollFailure(ctx, &state.editWarned, "set the pull request's title and body", err)
	}
	state.synced = text
	state.editWarned = false
	return "", shedengine.OutputPointer{}, false, nil
}

// pushHalt is the Stuck verdict of a push that failed for a reason other than a transient one.
func (p *PRReview) pushHalt(ctx context.Context, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return stuckVerdict(ctx, p.name, pushHaltReason(p.deps.WorktreeRoot, err), true)
}

// recordPullRequest writes ref into the run's record.
// A new record is written whole with head as its gated head; an existing one gets only the pull request's fields, plus the gated head when it carries none, so a continue flag written meanwhile survives.
func (p *PRReview) recordPullRequest(ref pullrequest.Ref, head string, hadRecord bool) error {
	pr := p.deps.PullRequest
	var err error
	if hadRecord {
		err = pullrequest.UpdateRecord(pr.RecordPath, pr.RecordLockPath, func(rec *pullrequest.Record) error {
			rec.Number, rec.URL = ref.Number, ref.URL
			rec.Base, rec.Head = p.deps.ParentBranch, p.deps.TaskBranch
			if rec.GatedHead == "" {
				rec.GatedHead = head
			}
			return nil
		})
	} else {
		err = pullrequest.WriteRecord(pr.RecordPath, pr.RecordLockPath, pullrequest.Record{
			Number:    ref.Number,
			URL:       ref.URL,
			Base:      p.deps.ParentBranch,
			Head:      p.deps.TaskBranch,
			GatedHead: head,
		})
	}
	if err != nil {
		return fmt.Errorf("landingshed: %s: record the pull request: %w", p.name, err)
	}
	return nil
}

// nextPRWait is the wait after current: a second once the task head moved, otherwise double current up to a minute and never under a second.
func nextPRWait(current time.Duration, moved bool) time.Duration {
	if moved {
		return prWaitFloor
	}
	return min(max(current*2, prWaitFloor), prWaitCeiling)
}

// holdWhileCovered returns once no pause covers the run, sleeping with a growing interval while one does.
// The first covered read stops a live session, and the first uncovered read after that resumes it.
// A PauseCovered error is logged on every read and read as uncovered, and a cancelled context is an error.
func (p *PRReview) holdWhileCovered(ctx context.Context) error {
	pr := p.deps.PullRequest
	wait := prWaitFloor
	paused, stopped := false, false
	for {
		covered, err := pr.PauseCovered()
		if err != nil {
			logger.Warn("landingshed: the pause record is unreadable, reading the run as uncovered", "row", p.name, "cause", err)
			covered = false
		}
		if !covered {
			if stopped {
				return p.resumeSession(ctx)
			}
			return nil
		}
		if !paused {
			paused = true
			if stopped, err = p.stopRecordedSession(); err != nil {
				return err
			}
		}
		if err := pr.Sleep(ctx, wait); err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return cerr
			}
			return fmt.Errorf("landingshed: %s: sleep while a pause covers the run: %w", p.name, err)
		}
		wait = nextPRWait(wait, false)
	}
}

// prWaitState is what a wait carries from one wake to the next.
type prWaitState struct {
	// interval is the time slept after the last wake.
	interval time.Duration
	// wokeAt is when the last wake began.
	wokeAt time.Time
	// head is the task head the last wake read, empty before the first.
	head string
	// continueSeen is the continue flag the last wake read, or the starting record's before the first.
	continueSeen bool
	// pending is the status Call read on entry, which the first wake takes as its poll while it is fresh.
	pending *pullrequest.Status
	// lastPoll is when the status was last read, the entry read's time before the first poll.
	lastPoll time.Time
	// transientWarned is set after a transient poll failure logged its Warn and cleared by the next good poll.
	transientWarned bool
	// synced is the title and body the pull request was last set to, zero until the wait knows them.
	synced prText
	// textWarned is set after a failure to build the pull request text logged its Warn and cleared by the next build that succeeds.
	textWarned bool
	// editWarned is set after a transient failure of the title and body edit logged its Warn and cleared by the next edit that succeeds.
	editWarned bool
}

// wait holds the call on the pull request until the review ends.
// Each wake reads the pull request's status at most once a minute, gates and pushes the agent's new commits unless the pull request is merged, and judges the status: merged, closed, approved or continued by the operator is Done or Stuck, anything else keeps waiting.
// A wake with a head to gate reads the status whatever the minute says.
// A change to the record ends the sleep between two wakes early, so a continue flag the agent writes is read within a second.
// A poll that finds the pull request open and the review going sets its title and body again when the text yielded from the change description, the regression check and the credit block changed since they were last set.
// rec is the record the wait starts from, entryStatus and entryReadAt are the status Call read and its time, nil and zero when Call read none, and synced is the title and body Call set, zero when it set none.
// A cancelled context is an error.
func (p *PRReview) wait(ctx context.Context, api pullrequest.API, rec pullrequest.Record, entryStatus *pullrequest.Status, entryReadAt time.Time, synced prText) (shedengine.Outcome, shedengine.OutputPointer, error) {
	label := fmt.Sprintf("PR #%d", rec.Number)
	p.gate.markWait(label)
	defer p.gate.markWait("")
	changes, stopWatch := p.deps.PullRequest.RecordChanges()
	defer stopWatch()

	state := prWaitState{continueSeen: rec.Continue, pending: entryStatus, lastPoll: entryReadAt, synced: synced}
	for {
		outcome, pointer, ended, err := p.wake(ctx, api, &state, label)
		if err != nil || ended {
			return outcome, pointer, err
		}
		if err := p.pause(ctx, state.interval, state.wokeAt, changes); err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: sleep between wakes: %w", p.name, err)
		}
	}
}

// pause sleeps interval, or until changes delivers a change to the record.
// A change still waits out the rest of prWaitFloor since wokeAt, so no wake follows another by less than a second.
func (p *PRReview) pause(ctx context.Context, interval time.Duration, wokeAt time.Time, changes <-chan struct{}) error {
	pr := p.deps.PullRequest
	sleepCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	slept := make(chan error, 1)
	go func() { slept <- pr.Sleep(sleepCtx, interval) }()
	select {
	case err := <-slept:
		return err
	case <-changes:
		cancel()
		<-slept
		if rest := prWaitFloor - pr.Now().Sub(wokeAt); rest > 0 {
			return pr.Sleep(ctx, rest)
		}
		return nil
	}
}

// wake runs one wake of the wait and reports whether it ended the call with the returned verdict.
func (p *PRReview) wake(ctx context.Context, api pullrequest.API, state *prWaitState, label string) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	pr := p.deps.PullRequest
	end := func(outcome shedengine.Outcome, pointer shedengine.OutputPointer, err error) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
		return outcome, pointer, true, err
	}
	state.wokeAt = pr.Now()
	if err := cancelErr(ctx, p.name); err != nil {
		return end("", shedengine.OutputPointer{}, err)
	}
	if err := p.holdWhileCovered(ctx); err != nil {
		return end("", shedengine.OutputPointer{}, err)
	}
	if outcome, pointer, err, answered := p.answerReopen(ctx); answered {
		return end(outcome, pointer, err)
	}

	head, dirty, err := p.readTree()
	if err != nil {
		return end("", shedengine.OutputPointer{}, err)
	}
	moved := state.head != "" && head != state.head
	state.head = head
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil || !found {
		return end("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record (found %v): %w", p.name, found, err))
	}

	// The status is read before the gate, and forced when a gate is due, so that a pull request merged at a head the row never gated is not gated, pushed and recorded as if it had been.
	gateDue := len(dirty) == 0 && head != rec.GatedHead && head != rec.MailedHead
	status, polled, outcome, pointer, ended, err := p.pollStatus(ctx, api, state, rec, gateDue)
	if err != nil || ended {
		return outcome, pointer, ended, err
	}

	if gateDue && polled && status.State != pullrequest.StateMerged {
		outcome, pointer, ended, err := p.gateHead(ctx, head, rec, label)
		if err != nil || ended {
			return outcome, pointer, ended, err
		}
		moved = true
		if rec, found, err = pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath); err != nil || !found {
			return end("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record (found %v): %w", p.name, found, err))
		}
	}
	state.continueSeen = rec.Continue
	state.interval = nextPRWait(state.interval, moved)
	if !polled {
		return "", shedengine.OutputPointer{}, false, nil
	}
	if status.State == pullrequest.StateOpen && !rec.ReviewEnded && !rec.Continue {
		if outcome, pointer, ended, err := p.syncText(ctx, api, state, rec.Number); err != nil || ended {
			return outcome, pointer, ended, err
		}
	}

	// The session is stopped before the read that may end the review, so the agent cannot change the tree between the read and the verdict.
	stopped := false
	if sessionStopWanted(status, rec) {
		if stopped, err = p.stopLiveSession(rec); err != nil {
			return end("", shedengine.OutputPointer{}, err)
		}
	}
	head, dirty, err = p.readTree()
	if err != nil {
		return end("", shedengine.OutputPointer{}, err)
	}
	outcome, pointer, ended, err = p.judge(ctx, status, rec, head, dirty)
	if err == nil && !ended && stopped {
		if err = p.resumeSession(ctx); err != nil {
			return end("", shedengine.OutputPointer{}, err)
		}
	}
	return outcome, pointer, ended, err
}

// sessionStopWanted reports whether the read of status against rec may end the review, or act on the operator's ending, so the session must be stopped before the tree is read.
// That is a merged pull request, the continue flag on any state, and an approval of the head the row gated.
func sessionStopWanted(status pullrequest.Status, rec pullrequest.Record) bool {
	switch {
	case status.State == pullrequest.StateMerged, rec.Continue:
		return true
	case status.State == pullrequest.StateOpen:
		return status.Approved && !status.ChangesRequested && status.HeadSHA == rec.GatedHead
	}
	return false
}

// readTree reads the task head and the task worktree's uncommitted paths.
func (p *PRReview) readTree() (string, []string, error) {
	head, err := p.deps.TaskHead()
	if err != nil {
		return "", nil, fmt.Errorf("landingshed: %s: read the task head: %w", p.name, err)
	}
	dirty, err := p.deps.PullRequest.DirtyPaths()
	if err != nil {
		return "", nil, fmt.Errorf("landingshed: %s: read the task worktree's uncommitted paths: %w", p.name, err)
	}
	return head, dirty, nil
}

// gateHead runs the pr-review gate over head, a clean tree whose head the record has not gated.
// A pass pushes head and records it as gated, and a failure pushes nothing and mails its reason to the PR-Review session once per head.
// A runner crash judges no code, so it halts the row naming the crash and mails nothing; the resume gates the head again.
// A head moved or a tree dirtied during the gate discards the verdict.
// It reports whether it ended the call.
func (p *PRReview) gateHead(ctx context.Context, head string, rec pullrequest.Record, label string) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	pr := p.deps.PullRequest
	reason, crashed, err := p.gate.checkHead(ctx, p.name)
	p.gate.markWait(label)
	if err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	after, dirty, err := p.readTree()
	if err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	if after != head || len(dirty) > 0 {
		return "", shedengine.OutputPointer{}, false, nil
	}

	if crashed {
		outcome, pointer, serr := stuckVerdict(ctx, p.name, reason, true)
		return outcome, pointer, true, serr
	}
	if reason == "" {
		if err := pr.PushTask(); err != nil {
			if terr := transientFailure(ctx, p.name, "push the task branch", err); terr != nil {
				return "", shedengine.OutputPointer{}, true, terr
			}
			outcome, pointer, perr := p.pushHalt(ctx, err)
			return outcome, pointer, true, perr
		}
		if err := p.updateRecord(func(r *pullrequest.Record) { r.GatedHead = head }); err != nil {
			return "", shedengine.OutputPointer{}, true, err
		}
		return "", shedengine.OutputPointer{}, false, nil
	}

	if rec.MailedHead == head {
		return "", shedengine.OutputPointer{}, false, nil
	}
	p.mailFailure(rec.StrandName, reason)
	if err := p.updateRecord(func(r *pullrequest.Record) { r.MailedHead = head }); err != nil {
		return "", shedengine.OutputPointer{}, true, err
	}
	return "", shedengine.OutputPointer{}, false, nil
}

// mailFailure posts reason to the PR-Review session's strand, or logs it when no session is recorded.
// A failed post is logged and not retried.
func (p *PRReview) mailFailure(strandName, reason string) {
	if strandName == "" {
		logger.Warn("landingshed: the pr-review gate refused the task head and no PR-Review session is recorded to tell", "row", p.name, "reason", reason)
		return
	}
	if err := p.deps.PullRequest.MailAgent(strandName, reason); err != nil {
		logger.Warn("landingshed: could not tell the PR-Review session the pr-review gate refused the task head", "row", p.name, "strand", strandName, "cause", err)
	}
}

// updateRecord applies mutate to the run's pull request record under its lock, so a flag written meanwhile survives.
func (p *PRReview) updateRecord(mutate func(rec *pullrequest.Record)) error {
	pr := p.deps.PullRequest
	err := pullrequest.UpdateRecord(pr.RecordPath, pr.RecordLockPath, func(rec *pullrequest.Record) error {
		mutate(rec)
		return nil
	})
	if err != nil {
		return fmt.Errorf("landingshed: %s: update the pull request record: %w", p.name, err)
	}
	return nil
}

// pollStatus returns the pull request's status for this wake when one is due, and reports whether it did.
// The status Call read on entry is the first wake's poll while it is under a minute old; after that a poll is due once a minute, and at once on the wake whose record first carries the continue flag.
// A forced poll is also due whenever no fresh entry status is at hand.
// A failed poll is judged as a GitHub failure, except that a transient one logs one Warn per episode and leaves the wake without a status.
func (p *PRReview) pollStatus(ctx context.Context, api pullrequest.API, state *prWaitState, rec pullrequest.Record, force bool) (pullrequest.Status, bool, shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	now := p.deps.PullRequest.Now()
	due := state.lastPoll.IsZero() || now.Sub(state.lastPoll) >= prPollInterval
	pending := state.pending
	state.pending = nil
	switch {
	case pending != nil && !due:
		return *pending, true, "", shedengine.OutputPointer{}, false, nil
	case !due && !force && !(rec.Continue && !state.continueSeen):
		return pullrequest.Status{}, false, "", shedengine.OutputPointer{}, false, nil
	}

	state.lastPoll = now
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		outcome, pointer, ended, ferr := p.pollFailure(ctx, &state.transientWarned, "read the pull request's status", err)
		return pullrequest.Status{}, false, outcome, pointer, ended, ferr
	}
	state.transientWarned = false
	return status, true, "", shedengine.OutputPointer{}, false, nil
}

// pollFailure judges a failed GitHub call of the wait named by action.
// A token failure and any other failure but a transient one end the call as githubFailure judges them; a transient failure logs one Warn per episode, tracked in warned, and lets the wait go on.
// It reports whether it ended the call.
func (p *PRReview) pollFailure(ctx context.Context, warned *bool, action string, err error) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, true, cerr
	}
	if !tokenFailure(err) && shedtransient.Class(err) != "" {
		if !*warned {
			logger.Warn("landingshed: a GitHub call failed transiently, the wait goes on", "row", p.name, "action", action, "cause", err)
			*warned = true
		}
		return "", shedengine.OutputPointer{}, false, nil
	}
	outcome, pointer, gerr := p.githubFailure(ctx, action, err)
	return outcome, pointer, true, gerr
}

// judge reads the pull request's status against the record and the task worktree, and reports whether it ends the call.
// A merged pull request takes the merged verdicts whatever the continue flag says, since nothing is left to merge.
// The operator's ending leaves the pull request as GitHub has it, for Finalize to merge on GitHub, so ending the review never lands the run by a direct push to the parent.
func (p *PRReview) judge(ctx context.Context, status pullrequest.Status, rec pullrequest.Record, head string, dirty []string) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	done := func() (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
		return shedengine.Done, shedengine.OutputPointer{}, true, nil
	}
	stuck := func(reason string) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
		outcome, pointer, err := stuckVerdict(ctx, p.name, reason, true)
		return outcome, pointer, true, err
	}
	clean := len(dirty) == 0
	gated := head == rec.GatedHead

	switch status.State {
	case pullrequest.StateMerged:
		missing, err := p.deps.PullRequest.CommitsMissing(status.HeadSHA, head)
		if err != nil {
			return "", shedengine.OutputPointer{}, true, fmt.Errorf("landingshed: %s: list the task commits the merged pull request lacks: %w", p.name, err)
		}
		switch {
		case len(missing) > 0:
			return stuck(fmt.Sprintf("pull request #%d was merged on GitHub without these task commits: %s; way forward: file them as a board note, then run lym batten cancel %s, then lym board set-status %s done, since the pull request's work is on the parent and the cancel leaves the entry cleared", rec.Number, listShown(missing), p.deps.Slug, p.deps.Slug))
		case status.HeadSHA != rec.GatedHead:
			return stuck(fmt.Sprintf("pull request #%d was merged on GitHub at %s, but the head this row gated is %s; way forward: run lym loom goto Finalize, which fast-forwards the local parent to the merge under the lock and lands through the already-landed path, marking the task done, then run the gate over the parent branch in its worktree and file a board note for any fix it needs", rec.Number, status.HeadSHA, rec.GatedHead))
		case !clean:
			return stuck(p.dirtyEndReason(rec.Number, dirty))
		}
		return done()

	case pullrequest.StateClosed:
		switch {
		case rec.ReviewEnded && clean:
			return done()
		case rec.ReviewEnded:
			return stuck(p.dirtyEndReason(rec.Number, dirty))
		case rec.Continue && clean && gated:
			return p.endReview(rec)
		case rec.Continue:
			return "", shedengine.OutputPointer{}, false, nil
		}
		return stuck(fmt.Sprintf("pull request #%d was closed on GitHub without being merged; way forward: reopen it on GitHub and run lym loom resume, or run lym batten cancel %s", rec.Number, p.deps.Slug))

	case pullrequest.StateOpen:
		if rec.ReviewEnded {
			if clean {
				return done()
			}
			return stuck(p.dirtyEndReason(rec.Number, dirty))
		}
		if rec.Continue {
			if !clean || !gated {
				return "", shedengine.OutputPointer{}, false, nil
			}
			return p.endReview(rec)
		}
		if status.Approved && !status.ChangesRequested && status.HeadSHA == rec.GatedHead && status.HeadSHA == head && clean {
			return done()
		}
	}
	return "", shedengine.OutputPointer{}, false, nil
}

// endReview records the operator's ending in the record, unless rec already carries it, and ends the call Done.
func (p *PRReview) endReview(rec pullrequest.Record) (shedengine.Outcome, shedengine.OutputPointer, bool, error) {
	if !rec.ReviewEnded {
		if err := p.updateRecord(func(r *pullrequest.Record) { r.ReviewEnded = true }); err != nil {
			return "", shedengine.OutputPointer{}, true, err
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, true, nil
}

// dirtyEndReason is the Stuck reason for a review that ended with uncommitted changes in the task worktree.
func (p *PRReview) dirtyEndReason(number int, dirty []string) string {
	return fmt.Sprintf("pull request #%d ended the review, but the task worktree has uncommitted changes: %s; way forward: discard them in the task worktree, after filing any worth keeping as a board note, then run lym loom resume", number, listShown(dirty))
}

const (
	// reopenDraftComment is the comment a reopen posts on the pull request it keeps open as a draft.
	reopenDraftComment = "lym reopened this landing for more work: the pull request stays open as a draft, and lym refreshes its description and marks it ready for review when the landing goes on."
	// reopenCloseComment is the comment a reopen posts on the pull request it closes.
	reopenCloseComment = "lym closed this pull request: the landing was reopened for more work, and its next pull request is a new one."
)

// answerReopen sends the run back through the row's reroute edge when the Reopen seam reads a request, and reports whether it answered one.
// It stops the session, then keeps an open recorded pull request open as a draft with a comment saying so, or closes it with a comment when the request asks.
// A record it keeps loses the continue flag, the operator's ending, the mailed head and the session, so the next entry starts a fresh review of the same pull request; a pull request it closes, or that GitHub reports closed or merged, loses its record.
// The verdict is Stuck with Reroute and a reason naming what became of the pull request.
// The request stays for the reroute's target to remove, so a run the reroute cap halts here answers it again when resumed.
// A failed read or record write is an error, and a GitHub failure is judged as githubFailure judges one.
func (p *PRReview) answerReopen(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := p.deps.PullRequest
	if pr.Reopen == nil {
		return "", shedengine.OutputPointer{}, nil, false
	}
	answered := func(outcome shedengine.Outcome, pointer shedengine.OutputPointer, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return outcome, pointer, err, true
	}
	req, found, err := pr.Reopen()
	if err != nil {
		return answered("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the reopen request: %w", p.name, err))
	}
	if !found {
		return "", shedengine.OutputPointer{}, nil, false
	}
	reroute := func(fate string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		reason := "the landing was reopened for more work; " + fate
		logger.Info("landingshed: a reopen request sends the run back", "row", p.name, "reason", reason)
		return answered(shedengine.Stuck, shedengine.OutputPointer{Reroute: true, Reason: reason}, nil)
	}

	rec, recorded, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return answered("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err))
	}
	if !recorded {
		return reroute("the run has no pull request")
	}
	if _, err := p.stopLiveSession(rec); err != nil {
		return answered("", shedengine.OutputPointer{}, err)
	}
	origin, err := pr.Origin()
	if err != nil {
		return answered("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: read the origin: %w", p.name, err))
	}
	api, err := pr.API(origin.URL)
	if err != nil {
		return answered(p.githubFailure(ctx, "open the GitHub client", err))
	}
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return answered(p.githubFailure(ctx, "read the pull request's status", err))
	}

	fate := fmt.Sprintf("pull request #%d is %s", rec.Number, status.State)
	switch {
	case status.State == pullrequest.StateOpen && req.Close:
		if err := api.Close(ctx, rec.Number, reopenCloseComment); err != nil {
			return answered(p.githubFailure(ctx, "close the pull request", err))
		}
		fate = fmt.Sprintf("pull request #%d is closed", rec.Number)
	case status.State == pullrequest.StateOpen:
		if !status.Draft {
			if err := api.SetDraft(ctx, rec.Number, true); err != nil {
				return answered(p.githubFailure(ctx, "turn the pull request into a draft", err))
			}
		}
		if err := api.Comment(ctx, rec.Number, reopenDraftComment); err != nil {
			return answered(p.githubFailure(ctx, "comment on the pull request", err))
		}
		fate = fmt.Sprintf("pull request #%d stays open as a draft", rec.Number)
	case status.State == pullrequest.StateMerged:
		fate = fmt.Sprintf("pull request #%d is already merged", rec.Number)
	}

	// A merged pull request already carries work to the parent, so keeping its record would let the next landing judge the reopened work as landed.
	if req.Close || status.State == pullrequest.StateClosed || status.State == pullrequest.StateMerged {
		if err := pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath); err != nil {
			return answered("", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %w", p.name, err))
		}
		return reroute(fate)
	}
	err = p.updateRecord(func(r *pullrequest.Record) {
		r.Continue, r.ReviewEnded, r.MailedHead = false, false, ""
		r.StrandGUID, r.StrandName, r.SessionID, r.Engine = "", "", "", ""
	})
	if err != nil {
		return answered("", shedengine.OutputPointer{}, err)
	}
	return reroute(fate)
}

// githubFailure judges a failed GitHub call named by action.
// A token failure is Stuck with Halt first, since the HTTP client wraps it in a type the transient classifier reads as transient; then a transient failure is an error the driver re-steps; any other failure is Stuck with Halt.
func (p *PRReview) githubFailure(ctx context.Context, action string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if !tokenFailure(err) {
		if terr := transientFailure(ctx, p.name, action, err); terr != nil {
			return "", shedengine.OutputPointer{}, terr
		}
	}
	return stuckVerdict(ctx, p.name, githubHaltReason(action, err), true)
}
