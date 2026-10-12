// gate.go is the parent-review gate: the two closures card 1's GateEntry.Gate and GateEntry.Final take, driving the round store.
// The closures never send anything; they return the prompt text for the wait loop to send at a turn boundary.
// The gate reviews every rewrite after a reject, one round each, until the store's rejected rounds reach GateConfig.Cap;
// at the cap it fails terminally rather than letting the rewrite through, and the run halts `blocked` until the parent approves.

package parentreview

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// maxGatePrompts is the per-round cap on gate-driven prompts; notify-driven prompts sit outside it.
	maxGatePrompts = 3
	// promptThrottle is the minimum gap between two gate-driven prompts.
	promptThrottle = time.Minute
	// maxKeepAlives is the per-round cap on keep-alive pings; after the last, the writer's prompt cache is left to expire.
	maxKeepAlives = 3
)

// Aligner is the alignment agent the gate spawns before any delivery to the reviewer.
type Aligner interface {
	// Spawn starts the agent for a round; a second call for the same round returns the same attempt.
	Spawn(round int) (Alignment, error)
	// Poll reports the agent's progress without blocking.
	Poll(a Alignment) (AlignmentPoll, error)
	// Close stops a live agent and closes its attempt.
	Close(a Alignment, reason string) error
}

// AlignmentPoll is one Aligner.Poll answer.
type AlignmentPoll struct {
	// Done reports that the agent has ended, with or without a verdict.
	Done bool
	// Verdict is VerdictApprove, VerdictReject, "escalate", or empty when no verdict applies.
	Verdict string
	// Report is the path of the agent's report.
	Report string
	// Reason says why Done came without a verdict, or what an escalation asks.
	Reason string
}

// GateConfig is the told input of NewGate.
type GateConfig struct {
	// Aligner is the alignment agent; nil means the gate delivers straight to the reviewer.
	Aligner Aligner
	// RenderAlignmentBrief renders the reviewer brief written after a fallback, naming the agent's report or why no verdict came; required when Aligner is set.
	RenderAlignmentBrief func(report, reason string) (string, error)
	Store                Store
	// Slug is the run slug, named in the notify way-forward.
	Slug string
	// Reviewer is the full agent name of the run's parent, which the wiring resolves; empty means no reviewer and the gate passes.
	Reviewer string
	// DecisionRecord and SupportLog are the discussion paths the request carries.
	DecisionRecord string
	SupportLog     string
	// WaitBound is how long an open request waits for a verdict, measured from its opened-at.
	WaitBound time.Duration
	// Cap is the number of rejected rounds that fails the entry terminally; it is recorded on each request the gate opens.
	// The count comes from the store, so it survives an attach, a driver restart and a resume.
	// Zero or less means no cap.
	Cap int
	// ReviewerLive is the told liveness seam: while it reports false, or errors, the gate holds its delivery prompt, so the prompt budget is not spent during an orch relaunch.
	// Nil means the reviewer is treated as live, which is how a legacy-name reviewer with no known worktree is wired.
	// The hold never skips the review short of WaitBound.
	ReviewerLive func() (bool, error)
	// RenderDelivery renders the one-line delivery prompt for a brief path.
	RenderDelivery func(briefPath string) (string, error)
	// RenderBrief renders the reviewer brief written beside the request.
	RenderBrief func() (string, error)
	// KeepAlive is the gap between keep-alive pings to the idle writer while a delivered request waits, kept inside the writer's prompt-cache lifetime; zero turns pings off.
	KeepAlive time.Duration
	// RenderKeepAlive renders the one-line keep-alive prompt for ping number ping; required when KeepAlive is positive.
	RenderKeepAlive func(ping int) (string, error)
}

// NewGate returns the entry's per-arrival closure and its final-verdict closure over cfg.
func NewGate(cfg GateConfig) (gate, final shuttleengine.Gate) {
	g := &closures{cfg: cfg}
	return g.gate, g.final
}

type closures struct {
	cfg        GateConfig
	loggedNone bool
	// heldRound, warnedRound and warnedErr remember what holding last logged; round numbers start at 1, so zero means none.
	heldRound   int
	warnedRound int
	warnedErr   string
}

func (c *closures) now() time.Time {
	if c.cfg.Store.Now != nil {
		return c.cfg.Store.Now()
	}
	return time.Now()
}

func (c *closures) noReviewer() bool {
	if c.cfg.Reviewer != "" {
		return false
	}
	if !c.loggedNone {
		c.loggedNone = true
		logger.Info("parent review skipped: the run has no reviewer", "slug", c.cfg.Slug)
	}
	return true
}

func (c *closures) pending(send string) shuttleengine.GateResult {
	return shuttleengine.GateResult{
		Pending:              true,
		Send:                 send,
		SendFailedWayForward: fmt.Sprintf("run `lyx loom review notify %s` to re-send the notice", c.cfg.Slug),
	}
}

func (c *closures) prompt(r Round) (string, error) {
	return c.cfg.RenderDelivery(r.BriefPath())
}

// atCap reports whether the store's rejected rounds reach the cap, and how many there are.
func (c *closures) atCap() (rejected int, at bool, err error) {
	rejected, err = c.cfg.Store.RejectedRounds()
	if err != nil {
		return 0, false, err
	}
	return rejected, c.cfg.Cap > 0 && rejected >= c.cfg.Cap, nil
}

// terminal fails the entry at once; its findings become the run's blocked reason.
func (c *closures) terminal(r Round, rejected int, atCap bool, afterStart string) shuttleengine.GateResult {
	if atCap {
		return shuttleengine.GateResult{Terminal: true, Findings: fmt.Sprintf(
			"The parent reviewer rejected the discussion of %s in %d rounds; the latest review is %s. To continue, run `lyx loom review approve %s` from the parent, then `lyx loom start` in the task worktree.",
			c.cfg.Slug, rejected, r.ReviewPath(), c.cfg.Slug)}
	}
	return shuttleengine.GateResult{Terminal: true, Findings: fmt.Sprintf(
		"The parent reviewer rejected the discussion of %s; the review is %s, and the rewrite after it was never reviewed. `lyx loom start` in the task worktree %s.",
		c.cfg.Slug, r.ReviewPath(), afterStart)}
}

// gate reads the latest round before opening anything.
// No round or request opens a request; an approve passes; a reject at the cap fails terminally, opening and consuming nothing;
// an unconsumed reject below the cap is consumed and fails with its findings, and a consumed one opens the next round.
// An expired request passes, and an open request with no verdict waits, notifies and prompts;
// while ReviewerLive reports the reviewer not live it only waits, carrying no prompt and leaving the prompt count and waiting notifies untouched.
// Once delivery is recorded, a due keep-alive ping is carried after the prompt cases, at most maxKeepAlives per round.
// With an Aligner set, the agent is spawned for a round before anything is delivered and polled on every evaluation, and the delivery cases above run only once it has fallen back or never existed;
// a verdict recorded by the parent, or the wait bound, stops a running agent first.
func (c *closures) gate() (shuttleengine.GateResult, error) {
	passed := shuttleengine.GateResult{Passed: true}
	if c.cfg.Aligner == nil && c.noReviewer() {
		return passed, nil
	}
	s := c.cfg.Store
	r, ok, err := s.Latest()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if !ok || r.Request == nil {
		return c.open()
	}
	if r.Request.State != StateOpen {
		return passed, nil
	}
	if r.Verdict != nil {
		if r.aligning() {
			if err := c.closeAlignment(r, AlignmentStopped, "the parent recorded a verdict first"); err != nil {
				return shuttleengine.GateResult{}, err
			}
		}
		if r.Verdict.Kind == VerdictApprove {
			return passed, nil
		}
		rejected, at, err := c.atCap()
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if at {
			return c.terminal(r, rejected, true, ""), nil
		}
		if r.Verdict.Consumed {
			if _, err := s.BeginRound(); err != nil {
				return shuttleengine.GateResult{}, err
			}
			return c.open()
		}
		if err := s.MarkConsumed(); err != nil {
			return shuttleengine.GateResult{}, err
		}
		return shuttleengine.GateResult{Findings: fmt.Sprintf(
			"The parent reviewer rejected the discussion; their findings are in %s. Address every finding, by fixing the discussion or answering it, as your prompt's parent-review section describes.",
			r.ReviewPath())}, nil
	}
	now := c.now()
	if now.Sub(r.Request.OpenedAt) >= c.cfg.WaitBound {
		if r.aligning() {
			if err := c.closeAlignment(r, AlignmentStopped, "the review reached its wait bound"); err != nil {
				return shuttleengine.GateResult{}, err
			}
		}
		if err := s.MarkExpired(); errors.Is(err, ErrVerdictRecorded) {
			// A verdict landed since the read above: evaluate again, so it is read rather than expired.
			return c.gate()
		} else if err != nil {
			return shuttleengine.GateResult{}, err
		}
		logger.Warn("parent review timed out; letting the discussion through", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "request", r.RequestPath())
		return passed, nil
	}
	if c.cfg.Aligner != nil {
		if r.Alignment == nil && r.Delivery.Prompts == 0 && r.Delivery.LastPromptAt.IsZero() && r.Delivery.DeliveredAt.IsZero() {
			return c.spawnAlignment(r)
		}
		res, handled, err := c.pollAlignment(r)
		if err != nil || handled {
			return res, err
		}
	}
	return c.deliver(r)
}

// deliver runs the delivery cases for an open request with no verdict: the liveness hold, then the notify, prompt, cap-warn and keep-alive cases.
// With an Aligner set, a run with no reviewer passes here, since the agent has already had its turn.
func (c *closures) deliver(r Round) (shuttleengine.GateResult, error) {
	if c.noReviewer() {
		return shuttleengine.GateResult{Passed: true}, nil
	}
	s := c.cfg.Store
	now := c.now()
	if c.holding(r) {
		return c.pending(""), nil
	}
	d := r.Delivery
	switch {
	case d.WaitingNotifys > 0:
		return c.carry(r, true)
	case d.DeliveredAt.IsZero() && d.Prompts < maxGatePrompts && (d.LastPromptAt.IsZero() || now.Sub(d.LastPromptAt) >= promptThrottle):
		return c.carry(r, false)
	case d.DeliveredAt.IsZero() && d.Prompts >= maxGatePrompts && !d.CapWarned:
		if err := s.MarkCapWarned(); err != nil {
			return shuttleengine.GateResult{}, err
		}
		logger.Warn("parent review notice still not delivered; gate re-sending stopped", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "request", r.RequestPath())
	case c.pingDue(r, now):
		return c.ping(r)
	}
	return c.pending(""), nil
}

// NewExpiryProbe returns the entry's read-only expiry probe over cfg.
// It answers true once the latest round's request is open, carries no verdict and is at least WaitBound old on the store's clock, and false otherwise.
// It sends, counts and writes nothing, and it marks no request expired: only the gate and final closures do, when they next read the round.
func NewExpiryProbe(cfg GateConfig) func() (bool, error) {
	g := &closures{cfg: cfg}
	return func() (bool, error) {
		if cfg.Reviewer == "" {
			return false, nil
		}
		r, ok, err := cfg.Store.peekLatest()
		if err != nil || !ok {
			return false, err
		}
		if r.Request == nil || r.Request.State != StateOpen || r.Verdict != nil {
			return false, nil
		}
		return g.now().Sub(r.Request.OpenedAt) >= cfg.WaitBound, nil
	}
}

// pingDue reports whether a keep-alive is due: pings are on, fewer than maxKeepAlives are counted, and KeepAlive has passed since the latest of the base, last prompt and last ping.
// The base is delivery, which must be recorded, or the request's opening while the alignment agent runs.
func (c *closures) pingDue(r Round, now time.Time) bool {
	d := r.Delivery
	if c.cfg.KeepAlive <= 0 || d.Pings >= maxKeepAlives {
		return false
	}
	last := d.DeliveredAt
	if r.aligning() {
		last = r.Request.OpenedAt
	} else if last.IsZero() {
		return false
	}
	if d.LastPromptAt.After(last) {
		last = d.LastPromptAt
	}
	if d.LastPingAt.After(last) {
		last = d.LastPingAt
	}
	return now.Sub(last) >= c.cfg.KeepAlive
}

// ping records one keep-alive and returns pending carrying its text.
// The ping counts toward the cap whether or not its send lands.
// Its send-failure way forward asks for nothing, since a re-sent notice would reach a reviewer who already has it.
func (c *closures) ping(r Round) (shuttleengine.GateResult, error) {
	text, err := c.cfg.RenderKeepAlive(r.Delivery.Pings + 1)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if err := c.cfg.Store.RecordPing(); err != nil {
		return shuttleengine.GateResult{}, err
	}
	return shuttleengine.GateResult{
		Pending:              true,
		Send:                 text,
		SendFailedWayForward: "nothing needs doing: the review is still pending, and the next keep-alive retries while the round's cap allows",
	}, nil
}

// open writes the request for the latest round and carries its first prompt.
func (c *closures) open() (shuttleengine.GateResult, error) {
	brief, err := c.cfg.RenderBrief()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	r, err := c.cfg.Store.OpenRequest(OpenSpec{
		Slug:           c.cfg.Slug,
		Reviewer:       c.cfg.Reviewer,
		DecisionRecord: c.cfg.DecisionRecord,
		SupportLog:     c.cfg.SupportLog,
		Brief:          brief,
		Cap:            c.cfg.Cap,
	})
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if c.cfg.Aligner != nil {
		return c.spawnAlignment(r)
	}
	if c.holding(r) {
		return c.pending(""), nil
	}
	return c.carry(r, false)
}

func (r Round) aligning() bool {
	return r.Alignment != nil && r.Alignment.State == AlignmentRunning
}

// spawnAlignment starts the round's alignment agent and records it running, carrying no prompt.
// A spawn error is logged and recorded as a fallback, and the round goes straight to delivery.
func (c *closures) spawnAlignment(r Round) (shuttleengine.GateResult, error) {
	a, err := c.cfg.Aligner.Spawn(r.Number)
	if err != nil {
		logger.Warn("parent review alignment agent did not start; delivering to the reviewer", "slug", c.cfg.Slug, "round", r.Number, "error", err.Error())
		fallback := Alignment{State: AlignmentFallback, Reason: err.Error()}
		if err := c.cfg.Store.RecordAlignment(fallback); err != nil {
			return shuttleengine.GateResult{}, err
		}
		if err := c.writeAlignmentBrief("", fallback.Reason); err != nil {
			return shuttleengine.GateResult{}, err
		}
		return c.deliver(r)
	}
	a.State = AlignmentRunning
	if a.SpawnedAt.IsZero() {
		a.SpawnedAt = c.now()
	}
	if err := c.cfg.Store.RecordAlignment(a); err != nil {
		return shuttleengine.GateResult{}, err
	}
	return c.pending(""), nil
}

// pollAlignment polls a running agent.
// It reports handled when its result is the evaluation's, and not handled when the round has fallen back (or has no running agent) and the delivery cases are next.
// An approve or reject is recorded as the round's verdict; a parent verdict that landed first wins.
func (c *closures) pollAlignment(r Round) (shuttleengine.GateResult, bool, error) {
	if !r.aligning() {
		return shuttleengine.GateResult{}, false, nil
	}
	a := *r.Alignment
	p, err := c.cfg.Aligner.Poll(a)
	if err != nil {
		logger.Warn("parent review alignment poll failed; delivering to the reviewer", "slug", c.cfg.Slug, "round", r.Number, "error", err.Error())
		p = AlignmentPoll{Done: true, Reason: "polling the alignment agent failed: " + err.Error()}
	}
	if !p.Done {
		if c.pingDue(r, c.now()) {
			res, err := c.ping(r)
			return res, true, err
		}
		return c.pending(""), true, nil
	}
	if p.Verdict == VerdictApprove || p.Verdict == VerdictReject {
		err := c.cfg.Store.RecordVerdict(p.Verdict, p.Report)
		switch {
		case err == nil:
			r.Alignment.Report = p.Report
			if err := c.closeAlignment(r, AlignmentApplied, ""); err != nil {
				return shuttleengine.GateResult{}, true, err
			}
			res, err := c.gate()
			return res, true, err
		case errors.Is(err, ErrVerdictRecorded), errors.Is(err, ErrExpired), errors.Is(err, ErrNoOpenRequest):
			if err := c.closeAlignment(r, AlignmentStopped, "the round was settled before the agent's verdict was recorded"); err != nil {
				return shuttleengine.GateResult{}, true, err
			}
			res, err := c.gate()
			return res, true, err
		case !errors.Is(err, ErrEmptyReviewFile):
			return shuttleengine.GateResult{}, true, err
		}
		p.Reason = "the alignment agent's " + p.Verdict + " came without a readable report"
	}
	reason := p.Reason
	if reason == "" {
		reason = "the alignment agent ended without a verdict the gate could apply"
	}
	r.Alignment.Report = p.Report
	if err := c.closeAlignment(r, AlignmentFallback, reason); err != nil {
		return shuttleengine.GateResult{}, true, err
	}
	if err := c.writeAlignmentBrief(p.Report, reason); err != nil {
		return shuttleengine.GateResult{}, true, err
	}
	return shuttleengine.GateResult{}, false, nil
}

// closeAlignment stops the round's agent and settles its record as state with the record's report and the reason.
// An Aligner.Close error is logged and never returned, so the record is settled and the agent is never polled again.
func (c *closures) closeAlignment(r Round, settled, reason string) error {
	if err := c.cfg.Aligner.Close(*r.Alignment, reason); err != nil {
		logger.Warn("parent review alignment agent did not close cleanly", "slug", c.cfg.Slug, "round", r.Number, "error", err.Error())
	}
	return c.cfg.Store.SettleAlignment(settled, r.Alignment.Report, reason)
}

// writeAlignmentBrief replaces the round's brief with the reviewer brief that follows a fallback.
func (c *closures) writeAlignmentBrief(report, reason string) error {
	brief, err := c.cfg.RenderAlignmentBrief(report, reason)
	if err != nil {
		return err
	}
	return c.cfg.Store.WriteBrief(brief)
}

// holding reports whether the reviewer has no live session, in which case the gate carries no prompt and spends neither a gate prompt nor a waiting notify.
// A ReviewerLive error counts as not live and is never returned.
// The Info line and the error Warn each fire once per round, the Warn once per distinct error text, since a pending entry is evaluated on every poll tick.
func (c *closures) holding(r Round) bool {
	if c.cfg.ReviewerLive == nil {
		return false
	}
	live, err := c.cfg.ReviewerLive()
	if err == nil && live {
		return false
	}
	if err != nil && (c.warnedRound != r.Number || c.warnedErr != err.Error()) {
		c.warnedRound, c.warnedErr = r.Number, err.Error()
		logger.Warn("parent review reviewer liveness check failed; holding the delivery prompt", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "round", r.Number, "error", err.Error())
	}
	if c.heldRound != r.Number {
		c.heldRound = r.Number
		logger.Info("parent review holding the delivery prompt: the reviewer has no live session", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "round", r.Number)
	}
	return true
}

// carry records one prompt and returns pending carrying its text.
func (c *closures) carry(r Round, fromNotify bool) (shuttleengine.GateResult, error) {
	text, err := c.prompt(r)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if err := c.cfg.Store.RecordPrompt(fromNotify); err != nil {
		return shuttleengine.GateResult{}, err
	}
	return c.pending(text), nil
}

// final never opens a request, carries no prompt and consumes nothing.
// It passes only on no reviewer, no round or request, an approve or an expired request;
// an open request with no verdict is marked expired and passes with a timeout Warn.
// Any other reject on the latest round fails terminally, because the rewrite after it was never reviewed: at the cap with the cap's line, below it with the line saying what `lyx loom start` then does.
// With an Aligner set, a running agent is stopped first, and no reviewer passes only a round with no verdict, so an agent's reject fails as a parent's does.
func (c *closures) final() (shuttleengine.GateResult, error) {
	passed := shuttleengine.GateResult{Passed: true}
	if c.cfg.Aligner == nil && c.noReviewer() {
		return passed, nil
	}
	r, ok, err := c.cfg.Store.Latest()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if !ok || r.Request == nil || r.Request.State != StateOpen {
		return passed, nil
	}
	if r.aligning() {
		if err := c.closeAlignment(r, AlignmentStopped, "the writer's session ended"); err != nil {
			return shuttleengine.GateResult{}, err
		}
	}
	if r.Verdict == nil {
		if c.cfg.Aligner != nil && c.noReviewer() {
			return passed, nil
		}
		if err := c.cfg.Store.MarkExpired(); errors.Is(err, ErrVerdictRecorded) {
			// A verdict landed since the read above: evaluate again, so it is read rather than expired.
			return c.final()
		} else if err != nil {
			return shuttleengine.GateResult{}, err
		}
		logger.Warn("parent review timed out; letting the discussion through", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "request", r.RequestPath())
		return passed, nil
	}
	if r.Verdict.Kind != VerdictReject {
		return passed, nil
	}
	rejected, at, err := c.atCap()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if r.Verdict.Consumed {
		return c.terminal(r, rejected, at, "re-spawns the writer, whose discussion then goes to the parent as the next round"), nil
	}
	return c.terminal(r, rejected, at, "re-spawns the writer and re-prompts it with that review"), nil
}
