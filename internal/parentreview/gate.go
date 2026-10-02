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
)

// GateConfig is the told input of NewGate.
type GateConfig struct {
	Store Store
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
func (c *closures) gate() (shuttleengine.GateResult, error) {
	passed := shuttleengine.GateResult{Passed: true}
	if c.noReviewer() {
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
		if err := s.MarkExpired(); errors.Is(err, ErrVerdictRecorded) {
			// A verdict landed since the read above: evaluate again, so it is read rather than expired.
			return c.gate()
		} else if err != nil {
			return shuttleengine.GateResult{}, err
		}
		logger.Warn("parent review timed out; letting the discussion through", "slug", c.cfg.Slug, "reviewer", c.cfg.Reviewer, "request", r.RequestPath())
		return passed, nil
	}
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
	}
	return c.pending(""), nil
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
	if c.holding(r) {
		return c.pending(""), nil
	}
	return c.carry(r, false)
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
// It passes only on no reviewer, no round or request, an approve, or an expired request; an open request with no verdict is marked expired and passes with a timeout Warn.
// Any reject on the latest round fails terminally, because the rewrite after it was never reviewed: at the cap with the cap's line, below it with the line saying what `lyx loom start` then does.
func (c *closures) final() (shuttleengine.GateResult, error) {
	passed := shuttleengine.GateResult{Passed: true}
	if c.noReviewer() {
		return passed, nil
	}
	r, ok, err := c.cfg.Store.Latest()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if !ok || r.Request == nil || r.Request.State != StateOpen {
		return passed, nil
	}
	if r.Verdict == nil {
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
