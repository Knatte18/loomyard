// prreview.go implements the PR-Review row: it decides whether the landing goes through a pull request.
// When it does, it pushes the task branch, opens or finds the pull request, and records it.
// Then it waits on the pull request until the review ends.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedtransient"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

const (
	// prWaitFloor is the shortest time between two wakes of the wait, and the wait returns to it when the task head moves.
	prWaitFloor = time.Second
	// prWaitCeiling is the longest time between two wakes while nothing changes.
	prWaitCeiling = time.Minute
	// prPollInterval is how long the wait leaves between two reads of the pull request's status.
	prPollInterval = time.Minute
)

// PRReview is the ShedProducer that lands a run through a GitHub pull request when the landing goes through one.
// That is decided from beam.yaml's pull_request for the landing and GitHub's rules for the parent.
// When the landing does not go through one, the row closes any recorded pull request GitHub reports open, and passes.
// With a pull request recorded, a failed read of GitHub's rules is Stuck and closes nothing.
// A transient failure is an error the driver re-steps; a token failure falls back to beam.yaml.
// When the landing merges its pull request at once, the row passes at once,
// and the landing's merge step opens and merges it with no review.
// Otherwise the row pushes the task branch and finds or opens the pull request.
// The pull request's title comes from the change description.
// Its body comes from the description's body, the regression check and the credit block.
// The row records it, keeps one interactive session for the operator to discuss it in, and waits on it.
// With the parent review setting on, it asks the parent orch to review the open pull request before it waits.
// The wait gates and pushes the agent's commits.
// The text is built from the change description, the regression check and the credit block.
// When it changes, the wait sets the title and body again.
// The wait ends when the pull request is merged or approved, or closed or continued by the operator.
// It also ends when the agent has sent findings back: the row reroutes to Code-Bouncer with the findings file as output.
// It takes no parent lock.
type PRReview struct {
	name string
	deps Deps
	gate verifyGate
}

var _ shedengine.ShedProducer = (*PRReview)(nil)

// NewPRReview constructs a PRReview that reports under name and reads the values it needs from deps.
func NewPRReview(name string, deps Deps) *PRReview {
	gate := newVerifyGate(deps)
	if deps.PullRequest.DirtyPaths != nil {
		gate.dirty = func(string) ([]string, error) { return deps.PullRequest.DirtyPaths() }
	}
	return &PRReview{name: name, deps: deps, gate: gate}
}

// Call runs the row once and is safe to re-enter.
// A record, an open pull request on the task branch or a recorded review ending keeps a re-entry from opening another.
// When the landing goes through a pull request, the row starts or resumes the run's one interactive session.
// It then waits on the pull request, which has no timeout.
// The session is stopped on every verdict, Stuck or error of the row, and left running when the context is cancelled.
// A cancelled context returns an error.
// A transient push, GitHub or session-start failure is an error the driver re-steps.
// Any other such failure is Stuck with Halt.
// A send-back mark in the record is settled first, before any session starts or resumes.
func (p *PRReview) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkWiring(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	end := p.settleSendBack(ctx)
	if end == nil {
		end = p.decide(ctx)
		var startHalt *sessionStartHalt
		if errors.As(end.err, &startHalt) {
			end = halt(ctx, p.name, startHalt.haltReason())
		}
	}
	p.endSession(ctx, end.err)
	return end.result()
}

// settleSendBack handles the record's send-back mark at the start of a call.
// With no mark it does nothing.
// With a mark it reads the row's newest own history entry.
// When that entry is the reroute of the mark's file, shed persisted the reroute:
// one write clears the mark, the fixed-in-place count with its counted sets, the parent-review ask and the session.
// The record is committed, and the row runs as on any entry.
// With any other newest entry it keeps all of them and sends the findings back again.
// That covers a re-step, a crash before the persist and a goto onto the row.
// It also covers the resume after a dirty-tree halt or a reroute-cap halt.
// The file must match, not only the outcome.
// Otherwise a crash after a later send-back takes an earlier send-back's reroute for its own.
// It returns nil when the row runs as on any entry, and otherwise how the call ends.
func (p *PRReview) settleSendBack(ctx context.Context) *ending {
	pr := p.deps.PullRequest
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)}
	}
	if !found || rec.SendBack == "" {
		return nil
	}
	status, err := pr.ShedStatus()
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the run's status: %w", p.name, err)}
	}
	if !p.reroutePersisted(status.History, rec.SendBack) {
		return p.sendBack(ctx, rec)
	}
	err = p.updateRecord(func(r *pullrequest.Record) {
		r.SendBack, r.FixHereCount, r.CountedSets, r.ParentReviewAsked = "", 0, nil, false
		r.StrandGUID, r.StrandName, r.SessionID, r.Engine = "", "", "", ""
	})
	if err != nil {
		return &ending{err: err}
	}
	return nil
}

// reroutePersisted reports whether the row's newest own entry in history is a reroute whose output is file.
func (p *PRReview) reroutePersisted(history []shedengine.HistoryEntry, file string) bool {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Producer == p.name {
			return history[i].Outcome == shedengine.OutcomeReroute && history[i].Output == file
		}
	}
	return false
}

// sendBack ends the call with the send-back the record's mark names.
// It stops the live session, then reads the tree again, since the agent may have changed it after the verb answered.
// On a clean tree it returns Stuck with Reroute set and the mark's file as the output, and clears nothing:
// shed persists the reroute entry with that file, and the row's next call clears the mark.
// On a dirty tree it halts for the parent, and the resumed row meets the mark on a clean tree and reroutes.
func (p *PRReview) sendBack(ctx context.Context, rec pullrequest.Record) *ending {
	if _, err := p.stopLiveSession(rec); err != nil {
		return &ending{err: err}
	}
	_, dirty, err := p.readTree()
	if err != nil {
		return &ending{err: err}
	}
	if len(dirty) > 0 {
		return halt(ctx, p.name, fmt.Sprintf("pull request #%d has findings sent back to Code-Review, but the task worktree has uncommitted changes: %s; way forward: commit or discard them in the task worktree, then run lym loom resume", rec.Number, listShown(dirty)))
	}
	end := stuck(ctx, p.name, fmt.Sprintf("pull request #%d has findings sent back to Code-Review; they are in %s", rec.Number, rec.SendBack))
	if end.err == nil {
		end.out.Reroute, end.out.Path = true, rec.SendBack
	}
	return end
}

// decide resolves whether the landing goes through a pull request and runs the row's side of the answer.
func (p *PRReview) decide(ctx context.Context) *ending {
	pr := p.deps.PullRequest

	mode, err := pr.Mode()
	if err != nil {
		return halt(ctx, p.name, beamModeUnreadableReason(err))
	}
	origin, err := pr.Origin()
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the origin: %w", p.name, err)}
	}
	hasCommits, err := pr.TaskHasCommits()
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read whether the task branch has commits beyond its parent: %w", p.name, err)}
	}

	_, recordFound, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)}
	}
	decision, err := resolveWithParentRule(ctx, p.name, p.deps, mode, NotABeam, origin, hasCommits, recordFound)
	if err != nil {
		return &ending{err: err}
	}
	if decision.Halt != "" {
		return halt(ctx, p.name, decision.Halt)
	}
	if !decision.Open {
		return p.closeRecordedPullRequest(ctx, origin, decision.Reason)
	}
	if decision.MergeAtOnce {
		logger.Info("landingshed: this landing merges its pull request at once, so Finalize opens and merges it with no review", "row", p.name, "reason", decision.Reason)
		return done()
	}
	return p.openPullRequest(ctx, origin)
}

// optionalPullRequestSeams names the PullRequestDeps fields the row runs without.
var optionalPullRequestSeams = []string{"ParentReviewWanted", "AskParent", "BeamRecord", "DropBeamRecord"}

// checkWiring returns an error naming the first value the row needs that Deps leaves unset:
// a PullRequestDeps field that optionalPullRequestSeams does not name, or TaskHead.
func (p *PRReview) checkWiring() error {
	seams := reflect.ValueOf(p.deps.PullRequest)
	for i := range seams.NumField() {
		name := seams.Type().Field(i).Name
		if seams.Field(i).IsZero() && !slices.Contains(optionalPullRequestSeams, name) {
			return fmt.Errorf("landingshed: %s: Deps.PullRequest.%s is not set", p.name, name)
		}
	}
	if p.deps.TaskHead == nil {
		return fmt.Errorf("landingshed: %s: Deps.TaskHead is not set", p.name)
	}
	return nil
}

// closeRecordedPullRequest ends the run's pull request when the landing does not go through one.
// It logs reason and closes a recorded pull request GitHub reports open.
// It then stops the recorded session, drops the record, and returns Done.
func (p *PRReview) closeRecordedPullRequest(ctx context.Context, origin Origin, reason string) *ending {
	logger.Info("landingshed: no pull request for this landing", "row", p.name, "reason", reason)
	pr := p.deps.PullRequest

	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)}
	}
	if !found {
		return done()
	}

	if action, err := closeIfOpen(ctx, pr, origin.URL, rec.Number, reason); err != nil {
		return p.githubFailure(ctx, action, err)
	}
	// The session is found through the record, so it is stopped before the record goes,
	// and a failed stop keeps the record for the re-step to retry.
	if _, err := p.stopLiveSession(rec); err != nil {
		return &ending{err: err}
	}
	if err := removePRRecord(pr, p.name); err != nil {
		return &ending{err: err}
	}
	return done()
}

// openPullRequest pushes the task branch, then finds or opens the run's pull request.
// It sets the title from the change description.
// It sets the body from the description's body, the regression check and the credit block.
// It records the pull request and waits on it.
// A pause that covers the run holds the call before the push.
func (p *PRReview) openPullRequest(ctx context.Context, origin Origin) *ending {
	pr := p.deps.PullRequest
	if err := p.holdWhileCovered(ctx); err != nil {
		return &ending{err: err}
	}

	if end := pushTask(ctx, p.deps, p.name, ""); end != nil {
		return end
	}

	head, err := p.deps.TaskHead()
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the task head: %w", p.name, err)}
	}
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record: %w", p.name, err)}
	}
	api, err := pr.API(origin.URL)
	if err != nil {
		return p.githubFailure(ctx, "open the GitHub client", err)
	}
	if found && rec.ReviewEnded {
		// The operator already ended this review.
		// A head other than the gated head means commits were added that need review, so the review starts afresh.
		if head == rec.GatedHead {
			return p.wait(ctx, api, rec, nil, time.Time{}, prText{})
		}
		if err := removePRRecord(pr, p.name); err != nil {
			return &ending{err: err}
		}
		found = false
	}

	text, err := p.pullRequestText()
	if err != nil {
		return &ending{err: fmt.Errorf("landingshed: %s: %w", p.name, err)}
	}

	// A recorded pull request that GitHub reports as not open is left as it is, for the wait to judge.
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
		// GitHub refuses to merge a draft pull request, so the review starts by marking a found draft ready.
		if status.State == pullrequest.StateOpen && status.Draft {
			if err := api.SetDraft(ctx, rec.Number, false); err != nil {
				return p.githubFailure(ctx, "mark the pull request ready for review", err)
			}
		}
		if status.State != pullrequest.StateOpen {
			// A merged pull request has nothing left to discuss, so only a closed one gets a session.
			// The session is for the agent to clean the tree under a continue flag.
			if status.State != pullrequest.StateMerged {
				if rec, err = p.ensureSession(ctx, rec); err != nil {
					return &ending{err: err}
				}
			}
			return p.wait(ctx, api, rec, entryStatus, entryReadAt, prText{})
		}
		ref = pullrequest.Ref{Number: rec.Number, URL: rec.URL}
	} else {
		var action string
		if ref, editTitleAndBody, action, err = findOrOpenPullRequest(ctx, api, p.deps.TaskBranch, p.deps.ParentBranch, text); err != nil {
			return p.githubFailure(ctx, action, err)
		}
	}
	if editTitleAndBody {
		if err := api.Edit(ctx, ref.Number, text.title, text.body); err != nil {
			return p.githubFailure(ctx, "set the pull request's title and body", err)
		}
	}
	if err := p.recordPullRequest(ref, head, found); err != nil {
		return &ending{err: err}
	}
	rec, found, err = pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil || !found {
		return &ending{err: fmt.Errorf("landingshed: %s: read back the pull request record (found %v): %w", p.name, found, err)}
	}
	if rec, err = p.ensureSession(ctx, rec); err != nil {
		return &ending{err: err}
	}
	p.askParentReview(rec)
	return p.wait(ctx, api, rec, entryStatus, entryReadAt, text)
}

// parentReviewRequestPrefix opens the request the row mails the parent orch, so the orch knows the line for one.
const parentReviewRequestPrefix = "[pr-review request]"

// askParentReview asks the parent orch to review the open pull request rec records, when the setting is on.
// It asks once per record until a send-back clears the ask, and the request names the PR-Review session the orch answers.
// An unreadable setting, a failed post, an orch no record names and a failed write of the ask into the record are logged,
// and the row's next entry asks again.
// The row asks and does not gate: the orch's answer reaches the session's agent as creel mail, which the row never reads.
func (p *PRReview) askParentReview(rec pullrequest.Record) {
	pr := p.deps.PullRequest
	if rec.ParentReviewAsked || pr.ParentReviewWanted == nil || pr.AskParent == nil {
		return
	}
	wanted, err := pr.ParentReviewWanted()
	if err != nil {
		logger.Warn("landingshed: the parent review setting is unreadable, so the parent orch is not asked", "row", p.name, "cause", err)
		return
	}
	if !wanted {
		return
	}
	posted, err := pr.AskParent(p.parentReviewRequest(rec))
	if err != nil {
		logger.Warn("landingshed: could not ask the parent orch for a review of the pull request", "row", p.name, "cause", err)
		return
	}
	if !posted {
		logger.Info("landingshed: no parent orch is recorded to ask for a review of the pull request", "row", p.name)
		return
	}
	// The request is posted, so a failed write is no reason for the driver to re-step and post it again.
	if err := p.updateRecord(func(r *pullrequest.Record) { r.ParentReviewAsked = true }); err != nil {
		logger.Warn("landingshed: could not record that the parent orch was asked for a review of the pull request", "row", p.name, "cause", err)
	}
}

// parentReviewRequest is the line that asks the parent orch to review the pull request rec records.
func (p *PRReview) parentReviewRequest(rec pullrequest.Record) string {
	return fmt.Sprintf("%s %s: pull request %s, from branch %s into %s, is open for your review; answer the PR-Review session with lym creel post %s \"<answer>\", saying you are satisfied or giving your findings, which its agent fixes before it asks you again",
		parentReviewRequestPrefix, p.deps.Slug, rec.URL, p.deps.TaskBranch, p.deps.ParentBranch, rec.StrandName)
}

// prText is a pull request's title and body.
// A title is never empty, so the zero prText stands for a text not known.
type prText struct {
	title string
	body  string
}

// pullRequestText returns the title and body the pull request carries for the change description read at the call.
// The title is the description's title,
// and the body is the description's body followed by the regression check and the credit block.
func (p *PRReview) pullRequestText() (prText, error) {
	summary, err := summaryparser.Parse(p.deps.DescriptionPath)
	if err != nil {
		return prText{}, fmt.Errorf("read the change description: %w", err)
	}
	credit, err := p.deps.completeSummary(summary)
	if err != nil {
		return prText{}, err
	}
	return prText{title: summary.Title, body: bodyWithCredit(summary.Body, credit)}, nil
}

// syncText sets the pull request's title and body again when the change description yields other text than the last set,
// so an edit of the description reaches the pull request at the next poll.
// A wait that knows no last-set text records the description's text as the last set, and edits nothing.
// A text that cannot be built is logged once per episode and left for a later poll, since the agent may be mid-edit.
// A transient failure of the edit is logged once per episode of failed edits and left for a later poll;
// any other failed edit ends the call with a verdict.
// It returns nil when the wait goes on.
func (p *PRReview) syncText(ctx context.Context, api pullrequest.API, state *prWaitState, number int) *ending {
	text, err := p.pullRequestText()
	if err != nil {
		if !state.textWarned {
			logger.Warn("landingshed: no pull request text could be built, so the pull request keeps its title and body", "row", p.name, "cause", err)
			state.textWarned = true
		}
		return nil
	}
	state.textWarned = false
	switch {
	case state.synced == prText{}:
		state.synced = text
		return nil
	case text == state.synced:
		state.editWarned = false
		return nil
	}
	if err := api.Edit(ctx, number, text.title, text.body); err != nil {
		return p.pollFailure(ctx, &state.editWarned, "set the pull request's title and body", err)
	}
	state.synced = text
	state.editWarned = false
	return nil
}

// recordPullRequest writes ref into the run's record and commits the record.
// A new record is written whole with head as its gated head;
// an existing one gets only the pull request's fields, plus the gated head when it carries none,
// so a continue flag written meanwhile survives.
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
	return commitPRRecord(p.deps.PullRequest, p.name)
}

// nextPRWait returns the wait that follows current.
// It is a second once the task head moved; otherwise it doubles current, up to a minute and never under a second.
func nextPRWait(current time.Duration, moved bool) time.Duration {
	if moved {
		return prWaitFloor
	}
	return min(max(current*2, prWaitFloor), prWaitCeiling)
}

// holdWhileCovered returns once no pause covers the run, sleeping with a growing interval while one does.
// The first covered read stops a live session, and the first uncovered read after that resumes it.
// An unreadable pause record is logged on every read and read as uncovered, and a cancelled context is an error.
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
	// pending is the status read before the wait starts, which the first wake takes as its poll while it is fresh.
	pending *pullrequest.Status
	// lastPoll is when the status was last read, the entry read's time before the first poll.
	lastPoll time.Time
	// transientWarned is set after a transient poll failure logged its Warn and cleared by the next good poll.
	transientWarned bool
	// synced is the title and body the pull request was last set to, zero until the wait knows them.
	synced prText
	// textWarned is set after a failure to build the pull request text logged its Warn.
	// The next build that succeeds clears it.
	textWarned bool
	// editWarned is set after a transient failure of the title and body edit logged its Warn.
	// The next edit that succeeds clears it.
	editWarned bool
}

// wait holds the call on the pull request until the review ends.
// Each wake reads the pull request's status at most once a minute.
// It gates and pushes the agent's new commits unless the pull request is merged,
// and it judges the status: merged, closed, approved or continued by the operator is Done or Stuck;
// anything else keeps waiting.
// A wake with a head to gate reads the status whatever the minute says.
// A change to the record ends the sleep between two wakes early,
// so a continue flag the agent writes is read within a second.
// A poll that finds the pull request open and the review going re-sets its title and body when the text has changed.
// The text is yielded from the change description, the regression check and the credit block.
// It has changed when it differs from the text last set.
// rec is the record the wait starts from.
// entryStatus and entryReadAt are the status read before the wait and its time, nil and zero when none was read.
// synced is the title and body set before the wait, zero when none was set.
// A cancelled context is an error.
func (p *PRReview) wait(ctx context.Context, api pullrequest.API, rec pullrequest.Record, entryStatus *pullrequest.Status, entryReadAt time.Time, synced prText) *ending {
	label := fmt.Sprintf("PR #%d", rec.Number)
	p.gate.markWait(label)
	defer p.gate.markWait("")
	changes, stopWatch := p.deps.PullRequest.RecordChanges()
	defer stopWatch()

	state := prWaitState{continueSeen: rec.Continue, pending: entryStatus, lastPoll: entryReadAt, synced: synced}
	for {
		if end := p.wake(ctx, api, &state, label); end != nil {
			return end
		}
		if err := p.pause(ctx, state.interval, state.wokeAt, changes); err != nil {
			return fail(ctx, p.name, fmt.Errorf("landingshed: %s: sleep between wakes: %w", p.name, err))
		}
	}
}

// pause sleeps `interval`, or until `changes` delivers a change to the record.
// A change still waits out the rest of `prWaitFloor` since `wokeAt`,
// so no wake follows another by less than `prWaitFloor`.
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

// wake runs one wake of the review:
// it reads the task head and the pull request record, gates the head when one is due, and judges the review.
// It returns nil when the wait goes on, and otherwise how the call ends.
func (p *PRReview) wake(ctx context.Context, api pullrequest.API, state *prWaitState, label string) *ending {
	pr := p.deps.PullRequest
	state.wokeAt = pr.Now()
	if err := cancelErr(ctx, p.name); err != nil {
		return &ending{err: err}
	}
	if err := p.holdWhileCovered(ctx); err != nil {
		return &ending{err: err}
	}

	head, dirty, err := p.readTree()
	if err != nil {
		return &ending{err: err}
	}
	moved := state.head != "" && head != state.head
	state.head = head
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil || !found {
		return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record (found %v): %w", p.name, found, err)}
	}
	if rec.SendBack != "" {
		return p.sendBack(ctx, rec)
	}

	// The status is read before the gate, and forced when a gate is due,
	// so a pull request merged at a head the row never gated is not gated, pushed and recorded as if it had been.
	gateDue := len(dirty) == 0 && head != rec.GatedHead && head != rec.MailedHead
	status, polled, end := p.pollStatus(ctx, api, state, rec, gateDue)
	if end != nil {
		return end
	}

	if gateDue && polled && status.State != pullrequest.StateMerged {
		if end := p.gateHead(ctx, head, rec, label); end != nil {
			return end
		}
		moved = true
		if rec, found, err = pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath); err != nil || !found {
			return &ending{err: fmt.Errorf("landingshed: %s: read the pull request record (found %v): %w", p.name, found, err)}
		}
	}
	state.continueSeen = rec.Continue
	state.interval = nextPRWait(state.interval, moved)
	if !polled {
		return nil
	}
	if status.State == pullrequest.StateOpen && !rec.ReviewEnded && !rec.Continue {
		if end := p.syncText(ctx, api, state, rec.Number); end != nil {
			return end
		}
	}

	// The session is stopped before the read that may end the review,
	// so the agent cannot change the tree between the read and the verdict.
	// A wake whose read cannot end the review leaves the session at work and judges the tree it read.
	stopped := false
	if sessionStopWanted(status, rec, head, dirty) {
		if stopped, err = p.stopLiveSession(rec); err != nil {
			return &ending{err: err}
		}
		head, dirty, err = p.readTree()
		if err != nil {
			return &ending{err: err}
		}
	}
	end = p.judge(ctx, status, rec, head, dirty)
	if end == nil && stopped {
		if err = p.resumeSession(ctx); err != nil {
			return &ending{err: err}
		}
	}
	return end
}

// sessionStopWanted reports whether the read of status against rec may end the review or act on the operator's ending,
// so the session must be stopped before the tree is read again.
// head and dirty are the task head and uncommitted paths the wake read.
// The cases are a merged pull request, a recorded ending, the continue flag on a clean tree at the gated head,
// and an approval of the head the row gated.
// The continue flag on a dirty tree or an ungated head only keeps the review waiting,
// so the session is left at work instead of being stopped and resumed at every poll.
func sessionStopWanted(status pullrequest.Status, rec pullrequest.Record, head string, dirty []string) bool {
	switch {
	case status.State == pullrequest.StateMerged, rec.ReviewEnded:
		return true
	case rec.Continue:
		return len(dirty) == 0 && head == rec.GatedHead
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

// gateRetryHint closes the mail of a gate failure, naming the agent's way to gate the same head again.
const gateRetryHint = "; when the failure lies outside the change, such as a file another process held open, and the operator has asked you to continue, run lym loom pr-continue, which gates this head again"

// gateHead runs the pr-review gate over head, a clean tree whose head the record has not gated.
// A pass pushes head and records it as gated,
// and a failure pushes nothing and mails its reason to the PR-Review session once per head.
// The record keeps the mailed head, which `lym loom pr-continue` clears so that the next wake gates it again.
// A runner crash judges no code, so it halts the row naming the crash and mails nothing;
// the resume gates the head again.
// A head moved or a tree dirtied during the gate discards the verdict.
// It returns nil when the wait goes on.
func (p *PRReview) gateHead(ctx context.Context, head string, rec pullrequest.Record, label string) *ending {
	reason, crashed, err := p.gate.checkHead(ctx, p.name)
	p.gate.markWait(label)
	if err != nil {
		return &ending{err: err}
	}
	after, dirty, err := p.readTree()
	if err != nil {
		return &ending{err: err}
	}
	if after != head || len(dirty) > 0 {
		return nil
	}

	if crashed {
		return halt(ctx, p.name, reason)
	}
	if reason == "" {
		if end := pushTask(ctx, p.deps, p.name, ""); end != nil {
			return end
		}
		if err := p.updateRecord(func(r *pullrequest.Record) { r.GatedHead = head }); err != nil {
			return &ending{err: err}
		}
		return nil
	}

	if rec.MailedHead == head {
		return nil
	}
	p.mailFailure(rec.StrandName, reason+gateRetryHint)
	if err := p.updateRecord(func(r *pullrequest.Record) { r.MailedHead = head }); err != nil {
		return &ending{err: err}
	}
	return nil
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
// It then commits the record.
func (p *PRReview) updateRecord(mutate func(rec *pullrequest.Record)) error {
	pr := p.deps.PullRequest
	err := pullrequest.UpdateRecord(pr.RecordPath, pr.RecordLockPath, func(rec *pullrequest.Record) error {
		mutate(rec)
		return nil
	})
	if err != nil {
		return fmt.Errorf("landingshed: %s: update the pull request record: %w", p.name, err)
	}
	return commitPRRecord(p.deps.PullRequest, p.name)
}

// pollStatus returns the pull request's status for this wake when one is due, and reports whether it did.
// The status read on entry is the first wake's poll while it is under a minute old;
// after that a poll is due once a minute, and at once on the wake whose record first carries the continue flag.
// A forced poll is also due whenever no fresh entry status is at hand.
// A failed poll is judged as a GitHub failure, unless it is transient:
// it logs one Warn per episode and leaves the wake without a status.
// A non-nil ending ends the call at once.
func (p *PRReview) pollStatus(ctx context.Context, api pullrequest.API, state *prWaitState, rec pullrequest.Record, force bool) (pullrequest.Status, bool, *ending) {
	now := p.deps.PullRequest.Now()
	due := state.lastPoll.IsZero() || now.Sub(state.lastPoll) >= prPollInterval
	pending := state.pending
	state.pending = nil
	switch {
	case pending != nil && !due:
		return *pending, true, nil
	case !due && !force && !(rec.Continue && !state.continueSeen):
		return pullrequest.Status{}, false, nil
	}

	state.lastPoll = now
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return pullrequest.Status{}, false, p.pollFailure(ctx, &state.transientWarned, "read the pull request's status", err)
	}
	state.transientWarned = false
	return status, true, nil
}

// pollFailure judges a failed GitHub call of the wait named by action.
// A transient failure that is not a token failure logs one Warn per episode, tracked in warned, and lets the wait go on.
// Any other failure, token failures included, ends the call with the outcome of judging it as a GitHub failure.
// It returns nil when the wait goes on.
func (p *PRReview) pollFailure(ctx context.Context, warned *bool, action string, err error) *ending {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return &ending{err: cerr}
	}
	if !tokenFailure(err) && shedtransient.Class(err) != "" {
		if !*warned {
			logger.Warn("landingshed: a GitHub call failed transiently, the wait goes on", "row", p.name, "action", action, "cause", err)
			*warned = true
		}
		return nil
	}
	return p.githubFailure(ctx, action, err)
}

// judge reads the pull request's status against the record and the task worktree.
// It returns nil while the review goes on, and otherwise how the call ends.
// A merged pull request takes the merged verdicts whatever the continue flag says, since nothing is left to merge.
// The operator's ending leaves the pull request as GitHub has it, for Finalize to merge on GitHub.
// Ending the review therefore never lands the run by a direct push to the parent.
func (p *PRReview) judge(ctx context.Context, status pullrequest.Status, rec pullrequest.Record, head string, dirty []string) *ending {
	clean := len(dirty) == 0
	gated := head == rec.GatedHead

	switch status.State {
	case pullrequest.StateMerged:
		missing, err := p.deps.PullRequest.CommitsMissing(status.HeadSHA, head)
		if err != nil {
			return &ending{err: fmt.Errorf("landingshed: %s: list the task commits the merged pull request lacks: %w", p.name, err)}
		}
		switch {
		case len(missing) > 0:
			return halt(ctx, p.name, fmt.Sprintf("pull request #%d was merged on GitHub without these task commits: %s; way forward: file them as a board note, then run lym batten cancel %s, then lym board set-status %s done, since the pull request's work is on the parent and the cancel leaves the entry cleared", rec.Number, listShown(missing), p.deps.Slug, p.deps.Slug))
		case status.HeadSHA != rec.GatedHead:
			return halt(ctx, p.name, fmt.Sprintf("pull request #%d was merged on GitHub at %s, but the head this row gated is %s; way forward: run lym loom goto Finalize, which fast-forwards the local parent to the merge under the lock and lands through the already-landed path, marking the task done, then run the gate over the parent branch in its worktree and file a board note for any fix it needs", rec.Number, status.HeadSHA, rec.GatedHead))
		case !clean:
			return halt(ctx, p.name, p.dirtyEndReason(rec.Number, dirty))
		}
		return done()

	case pullrequest.StateClosed:
		switch {
		case rec.ReviewEnded && clean:
			return done()
		case rec.ReviewEnded:
			return halt(ctx, p.name, p.dirtyEndReason(rec.Number, dirty))
		case rec.Continue && clean && gated:
			return p.endReview(rec)
		case rec.Continue:
			return nil
		}
		return halt(ctx, p.name, fmt.Sprintf("pull request #%d was closed on GitHub without being merged; way forward: reopen it on GitHub and run lym loom resume, or run lym batten cancel %s", rec.Number, p.deps.Slug))

	case pullrequest.StateOpen:
		if rec.ReviewEnded {
			if clean {
				return done()
			}
			return halt(ctx, p.name, p.dirtyEndReason(rec.Number, dirty))
		}
		if rec.Continue {
			if !clean || !gated {
				return nil
			}
			return p.endReview(rec)
		}
		if status.Approved && !status.ChangesRequested && status.HeadSHA == rec.GatedHead && status.HeadSHA == head && clean {
			return done()
		}
	}
	return nil
}

// endReview records the operator's ending in the record, unless rec already carries it, and ends the call Done.
func (p *PRReview) endReview(rec pullrequest.Record) *ending {
	if !rec.ReviewEnded {
		if err := p.updateRecord(func(r *pullrequest.Record) { r.ReviewEnded = true }); err != nil {
			return &ending{err: err}
		}
	}
	return done()
}

// dirtyEndReason is the Stuck reason for a review that ended with uncommitted changes in the task worktree.
func (p *PRReview) dirtyEndReason(number int, dirty []string) string {
	return fmt.Sprintf("pull request #%d ended the review, but the task worktree has uncommitted changes: %s; way forward: discard them in the task worktree, after filing any worth keeping as a board note, then run lym loom resume", number, listShown(dirty))
}

// githubFailure judges the row's failed GitHub call named by action.
// A token failure and any other failure that is not transient are Stuck with Halt,
// and a transient one is an error the driver re-steps.
func (p *PRReview) githubFailure(ctx context.Context, action string, err error) *ending {
	return githubFailure(ctx, p.name, "", action, err)
}
