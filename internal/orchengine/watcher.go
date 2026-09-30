// watcher.go — the watcher's decision core: one poll of the idle check and the persisted
// four-phase cycle (idle, handoff-requested, clearing, resuming).
//
// Every provider and reed interaction goes through the Session seam, so the whole state machine
// runs against a fake in untagged unit tests.
// The watcher saves State before every side effect, and a restarted watcher resumes from it:
// a non-idle phase's injection is treated as unconfirmed until a turn end proves it landed
// or a passing idle probe shows it did not, and then it is sent again.

package orchengine

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Session is the seam between the cycle machine and the live orchestrator session.
// Production satisfies it with an adapter over shuttleengine.Runner and reed.
type Session interface {
	// StrandAlive reports whether the strand exists and its pane is alive.
	StrandAlive(guid string) (bool, error)
	// ReadEvents returns the events past offset and the offset read through.
	ReadEvents(guid string, offset int64) ([]shuttleengine.Event, int64, error)
	// ContextTokens returns the context usage as of turnEnd; known false means unreadable.
	ContextTokens(turnEnd shuttleengine.Event) (int, bool, error)
	// SessionIdle reports whether the session shows an empty input box with no turn in progress.
	SessionIdle(guid string) (bool, error)
	// Send types text into the session as a new turn.
	Send(guid, text string) error
	// ClearSession types the provider's clear command into the session.
	ClearSession(guid string) error
}

// Clock supplies the current time, settable in tests.
type Clock interface {
	Now() time.Time
}

// Watcher runs the orch cycle, one Tick per poll.
type Watcher struct {
	session     Session
	cfg         Config
	paths       Paths
	stencilsDir string
	clock       Clock

	started bool   // Whether the cursor has been initialised from state.
	strand  string // Strand the cursor belongs to.
	cursor  int64  // Events-file position read through.

	newest     *shuttleengine.Event // Newest event read since the last return to idle.
	newestRead time.Time            // When newest was first read.

	seen phaseEvents // What the current non-idle phase has read so far.
}

// phaseEvents records the events read in the current non-idle phase,
// which the cursor has moved past and a later tick must still know.
type phaseEvents struct {
	turnEnd      bool
	firstTurnEnd shuttleengine.Event
	ask          bool
}

// NewWatcher builds a watcher over session.
func NewWatcher(session Session, cfg Config, paths Paths, stencilsDir string, clock Clock) *Watcher {
	return &Watcher{session: session, cfg: cfg, paths: paths, stencilsDir: stencilsDir, clock: clock}
}

// isTurnEnd reports whether ev ends a turn.
func isTurnEnd(ev shuttleengine.Event) bool {
	return ev.Kind == shuttleengine.EventStop || ev.Kind == shuttleengine.EventWaiting
}

// Tick runs one poll.
// done is true when the watcher should exit because the strand is gone.
// An error leaves the persisted phase as it was for the next tick or watcher.
func (w *Watcher) Tick() (done bool, err error) {
	st, err := LoadState(w.paths)
	if err != nil {
		return false, err
	}
	if st.Phase == "" {
		st.Phase = PhaseIdle
	}

	alive, err := w.session.StrandAlive(st.Strand)
	if err != nil {
		return false, err
	}
	if !alive {
		st.WatcherExit = "strand gone"
		return true, SaveState(w.paths, st)
	}

	if !w.started || w.strand != st.Strand {
		if st, err = w.initCursor(st); err != nil {
			return false, err
		}
	}

	events, next, err := w.session.ReadEvents(st.Strand, w.cursor)
	if err != nil {
		return false, err
	}
	now := w.clock.Now()

	readingChanged := false
	for _, ev := range events {
		if st.Phase == PhaseIdle && isTurnEnd(ev) {
			tokens, known, err := w.session.ContextTokens(ev)
			if err != nil {
				return false, err
			}
			st.LastContextTokens, st.LastContextKnown = tokens, known
			if !known {
				st.LastContextTokens = 0
			}
			readingChanged = true
		}
	}
	// No error point remains before the events are committed to memory.
	w.cursor = next
	for i := range events {
		ev := events[i]
		w.newest, w.newestRead = &ev, now
		if st.Phase == PhaseIdle {
			continue
		}
		if ev.Kind == shuttleengine.EventAsk {
			w.seen.ask = true
		}
		if isTurnEnd(ev) && !w.seen.turnEnd {
			w.seen.turnEnd, w.seen.firstTurnEnd = true, ev
		}
	}
	if readingChanged {
		if err := SaveState(w.paths, st); err != nil {
			return false, err
		}
	}

	switch st.Phase {
	case PhaseIdle:
		return false, w.tickIdle(st, now)
	case PhaseHandoffRequested:
		return false, w.tickHandoff(st, now)
	case PhaseClearing:
		return false, w.tickClearing(st, now)
	case PhaseResuming:
		return false, w.tickResuming(st, now)
	}
	return false, fmt.Errorf("orch: unknown phase %q", st.Phase)
}

// initCursor sets the read cursor from st on the watcher's first tick for a strand.
// A phase belonging to another strand is reset to idle, and a same-strand phase's injection is
// marked unconfirmed so the landed check runs again.
func (w *Watcher) initCursor(st State) (State, error) {
	w.started, w.strand = true, st.Strand
	w.newest, w.seen = nil, phaseEvents{}
	switch {
	case st.Phase == PhaseIdle:
		w.cursor = st.LastInjectionOffset
		return st, nil
	case st.PhaseStrand != st.Strand:
		st = ResetForFreshLaunch(st, st.Strand)
		w.cursor = 0
	default:
		st.PhaseInjected = false
		w.cursor = st.PhaseEventsOffset
	}
	return st, SaveState(w.paths, st)
}

// toIdle returns st to idle, persisting the cursor read through and forgetting the newest event,
// so only a turn end read after the return can qualify for the next injection.
func (w *Watcher) toIdle(st State, abortReason string) error {
	st.Phase = PhaseIdle
	st.PhaseInjected = false
	st.PendingHandoff, st.PendingResume = "", ""
	st.LastInjectionOffset = w.cursor
	if abortReason != "" {
		st.LastAbortReason = abortReason
	}
	w.newest, w.seen = nil, phaseEvents{}
	return SaveState(w.paths, st)
}

// enter persists a new non-idle phase, unconfirmed, before its side effect.
func (w *Watcher) enter(st State, phase Phase, now time.Time) (State, error) {
	st.Phase = phase
	st.PhaseStrand = st.Strand
	st.PhaseEnteredAt = now
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	w.seen = phaseEvents{}
	return st, SaveState(w.paths, st)
}

// confirm persists that the phase's injection landed.
func (w *Watcher) confirm(st State) error {
	st.PhaseInjected = true
	return SaveState(w.paths, st)
}

func (w *Watcher) tickIdle(st State, now time.Time) error {
	requested, err := CycleRequested(w.paths)
	if err != nil {
		return err
	}
	triggered := st.LastContextKnown && st.LastContextTokens >= w.cfg.Threshold()
	if !triggered && !requested {
		return nil
	}
	if w.newest == nil || !isTurnEnd(*w.newest) || now.Sub(w.newestRead) < w.cfg.IdleGrace() {
		return nil
	}
	idle, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return err
	}
	if !idle {
		return nil
	}

	path := NewHandoffPath(w.paths, now)
	text, err := RenderHandoffInstruction(w.stencilsDir, path)
	if err != nil {
		return err
	}
	st.PendingHandoff = path
	if st, err = w.enter(st, PhaseHandoffRequested, now); err != nil {
		return err
	}
	w.newest = nil
	if err := ClearCycleRequest(w.paths); err != nil {
		return err
	}
	if err := w.session.Send(st.Strand, text); err != nil {
		return err
	}
	return w.confirm(st)
}

func (w *Watcher) tickHandoff(st State, now time.Time) error {
	if w.seen.ask {
		return w.toIdle(st, "session asked a question during the handoff")
	}
	written, err := handoffWritten(st.PendingHandoff)
	if err != nil {
		return err
	}
	if written && w.seen.turnEnd {
		return w.startClearing(st, now)
	}
	if now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		return w.toIdle(st, "handoff timed out")
	}
	if st.PhaseInjected {
		return nil
	}
	if w.seen.turnEnd {
		return w.confirm(st)
	}
	idle, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return err
	}
	if !idle {
		return nil
	}
	text, err := RenderHandoffInstruction(w.stencilsDir, st.PendingHandoff)
	if err != nil {
		return err
	}
	if err := w.session.Send(st.Strand, text); err != nil {
		return err
	}
	return w.confirm(st)
}

// handoffWritten reports whether the handoff file exists and is non-empty.
func handoffWritten(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("orch: stat handoff: %w", err)
	}
	return info.Size() > 0, nil
}

// startClearing renders the resume prompt first, so the text the cleared session needs is known
// good before /clear runs, then persists clearing and types /clear.
func (w *Watcher) startClearing(st State, now time.Time) error {
	resume, err := RenderResumePrompt(w.stencilsDir, st.PendingHandoff)
	if err != nil {
		return w.toIdle(st, fmt.Sprintf("resume stencil %s failed to render: %v", resumeStencilName, err))
	}
	st.LastHandoff = st.PendingHandoff
	st.PendingResume = resume
	if st, err = w.enter(st, PhaseClearing, now); err != nil {
		return err
	}
	if err := w.session.ClearSession(st.Strand); err != nil {
		return err
	}
	return w.confirm(st)
}

func (w *Watcher) tickClearing(st State, now time.Time) error {
	timedOut := now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout()
	if !timedOut {
		if !st.PhaseInjected {
			// A clear that errored never reached the pane, and a pre-clear pane passes the idle probe too.
			if err := w.session.ClearSession(st.Strand); err != nil {
				return err
			}
			return w.confirm(st)
		}
		idle, err := w.session.SessionIdle(st.Strand)
		if err != nil {
			return err
		}
		if !idle {
			return nil
		}
	}

	if st.PhaseInjected {
		st.CycleCount++
	}
	st, err := w.enter(st, PhaseResuming, now)
	if err != nil {
		return err
	}
	if err := w.session.Send(st.Strand, st.PendingResume); err != nil {
		return err
	}
	return w.confirm(st)
}

func (w *Watcher) tickResuming(st State, now time.Time) error {
	if w.seen.turnEnd {
		tokens, known, err := w.session.ContextTokens(w.seen.firstTurnEnd)
		if err != nil {
			return err
		}
		st.LastContextTokens, st.LastContextKnown = tokens, known
		if !known {
			st.LastContextTokens = 0
		}
		return w.toIdle(st, "")
	}
	if now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		// The persisted reading predates /clear and says nothing about the cleared session.
		st.LastContextTokens, st.LastContextKnown = 0, false
		return w.toIdle(st, "resume timed out")
	}
	if st.PhaseInjected {
		return nil
	}
	idle, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return err
	}
	if !idle {
		return nil
	}
	if err := w.session.Send(st.Strand, st.PendingResume); err != nil {
		return err
	}
	return w.confirm(st)
}
