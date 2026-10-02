// gate.go is the parent-review gate: the two closures card 1's GateEntry.Gate and GateEntry.Final take, driving the round store.
// The closures never send anything; they return the prompt text for the wait loop to send at a turn boundary.

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
	// Reviewer is the reviewer's full name from the seed; empty means no reviewer and the gate passes.
	Reviewer string
	// DecisionRecord and SupportLog are the discussion paths the request carries.
	DecisionRecord string
	SupportLog     string
	// WaitBound is how long an open request waits for a verdict, measured from its opened-at.
	WaitBound time.Duration
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
		switch {
		case r.Verdict.Kind == VerdictApprove, r.Verdict.Consumed:
			return passed, nil
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
	})
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	return c.carry(r, false)
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
		return c.pending(""), nil
	}
	if r.Verdict.Kind == VerdictReject && !r.Verdict.Consumed {
		logger.Warn("parent review rejected the discussion and the run ended before the writer addressed it", "slug", c.cfg.Slug, "review", r.ReviewPath())
	}
	return passed, nil
}
