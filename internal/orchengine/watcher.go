// watcher.go — the watcher's decision core: one poll of the idle check and the persisted cycle, which Config.Mode picks between two machines.
// Clear mode runs four phases (idle, handoff-requested, clearing, resuming);
// compact mode runs two (idle, compacting), types `/compact` with a focus text and writes no handoff.
// Both share the idle trigger selection, its gates and its re-read of the context.
//
// Every provider and reed interaction goes through the Session seam, so the whole state machine runs against a fake in untagged unit tests.
// The watcher saves State before every side effect, and a restarted watcher resumes from it:
// a non-idle phase's injection is treated as unconfirmed until a turn end proves it landed or a passing idle probe shows it did not, and then it is sent again;
// in compacting, a compaction boundary read from the transcript proves it instead of a turn end.
// Nothing is typed into the pane, text, `/clear` or `/compact`, unless the idle probe passed on the same tick.

package orchengine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// deferReply is the whole turn-end message with which a session declines a soft cycle.
const deferReply = "DEFER"

// Session is the seam between the cycle machine and the live orchestrator session.
// Production satisfies it with an adapter over shuttleengine.Runner and reed.
type Session interface {
	// StrandAlive reports whether the strand exists and its pane is alive.
	StrandAlive(guid string) (bool, error)
	// ReadEvents returns the events past offset and the offset read through.
	ReadEvents(guid string, offset int64) ([]shuttleengine.Event, int64, error)
	// ContextTokens returns the context reading as of turnEnd; a reading with Known false means unreadable.
	ContextTokens(turnEnd shuttleengine.Event) (shuttleengine.ContextReading, error)
	// SessionIdle probes whether the session shows an empty input box with no turn in progress, and whether the pane is too short to tell.
	SessionIdle(guid string) (shuttleengine.IdleProbe, error)
	// Send types text into the session as a new turn.
	Send(guid, text string) error
	// ClearSession types the provider's clear command into the session.
	ClearSession(guid string) error
	// CompactSession types the provider's compact command into the session, with focus as its single-line instruction.
	CompactSession(guid, focus string) error
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

	// replaying is true from a restart into a non-idle phase until the first read,
	// whose events predate the restart in unknown order against the handoff file.
	replaying bool

	newest     *shuttleengine.Event // Newest event read since the last return to idle.
	newestRead time.Time            // When newest was first read.

	seen phaseEvents // What the current non-idle phase has read so far.
}

// phaseEvents records what the current non-idle phase has observed, which the cursor has moved past and a later tick must still know.
type phaseEvents struct {
	turnEnd      bool
	firstTurnEnd shuttleengine.Event
	ask          bool

	// handoffWritten is set once a stat taken before a tick's event read finds the handoff file written.
	handoffWritten bool
	// turnEndAfterHandoff is set by a turn end read after handoffWritten was set,
	// so a turn end that predates the file never opens the clear gate.
	turnEndAfterHandoff bool

	// deferred is set by the first turn end read in a soft cycle's handoff-requested phase whose message is exactly DEFER;
	// deferRead is when it was read.
	deferred  bool
	deferRead time.Time
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
// A tick whose state is bound to another strand while it runs saves nothing and returns no error,
// and the next tick follows the new strand.
func (w *Watcher) Tick() (done bool, err error) {
	done, err = w.tick()
	if errors.Is(err, errStrandReplaced) {
		logger.Info("orch: state was bound to another strand mid-tick; the next tick follows it", "strandGUID", w.strand)
		return false, nil
	}
	return done, err
}

// save persists st unless the state has been bound to another strand since this tick loaded it.
func (w *Watcher) save(st State) error {
	return saveStateForStrand(w.paths, st)
}

func (w *Watcher) tick() (done bool, err error) {
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
		return true, w.save(st)
	}

	if !w.started || w.strand != st.Strand {
		if st, err = w.initCursor(st); err != nil {
			return false, err
		}
	}

	if st.Phase == PhaseHandoffRequested && !w.seen.handoffWritten {
		// Stat before the event read, so every turn end read from here on was read after the file was seen written.
		if w.seen.handoffWritten, err = handoffWritten(st.PendingHandoff); err != nil {
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
			reading, err := w.session.ContextTokens(ev)
			if err != nil {
				return false, err
			}
			storeReading(&st, reading, ev)
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
		if !isTurnEnd(ev) {
			continue
		}
		if !w.seen.turnEnd {
			w.seen.turnEnd, w.seen.firstTurnEnd = true, ev
		}
		if w.seen.handoffWritten && !w.replaying {
			w.seen.turnEndAfterHandoff = true
		}
		if st.Phase == PhaseHandoffRequested && st.CycleTrigger == TriggerSoft && !w.seen.deferred && strings.TrimSpace(ev.Message) == deferReply {
			w.seen.deferred, w.seen.deferRead = true, now
		}
	}
	w.replaying = false
	if readingChanged {
		if err := w.save(st); err != nil {
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
	case PhaseCompacting:
		return false, w.tickCompacting(st, now)
	}
	return false, fmt.Errorf("orch: unknown phase %q", st.Phase)
}

// initCursor sets the read cursor from st on the watcher's first tick for a strand.
// A phase belonging to another strand is reset to idle,
// and a same-strand phase's injection is marked unconfirmed so the landed check runs again.
func (w *Watcher) initCursor(st State) (State, error) {
	w.started, w.strand = true, st.Strand
	w.newest, w.seen, w.replaying = nil, phaseEvents{}, false
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
		w.replaying = true
	}
	return st, w.save(st)
}

// toIdle returns st to idle, persisting the cursor read through and forgetting the newest event,
// so only a turn end read after the return can qualify for the next injection.
func (w *Watcher) toIdle(st State, abortReason string) error {
	st.Phase = PhaseIdle
	st.PhaseInjected = false
	st.PendingHandoff, st.PendingResume = "", ""
	st.Stuck = ""
	st.LastInjectionOffset = w.cursor
	if abortReason != "" {
		st.LastAbortReason = abortReason
	}
	w.newest, w.seen = nil, phaseEvents{}
	return w.save(st)
}

// enter persists a new non-idle phase, unconfirmed, before its side effect.
func (w *Watcher) enter(st State, phase Phase, now time.Time) (State, error) {
	st.Phase = phase
	st.PhaseStrand = st.Strand
	st.PhaseEnteredAt = now
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	st.Stuck = ""
	w.seen = phaseEvents{}
	return st, w.save(st)
}

// confirm persists that the phase's injection landed.
func (w *Watcher) confirm(st State) error {
	st.PhaseInjected = true
	return w.save(st)
}

// markStuck records reason as the current phase's stuck condition for `status`,
// saving and logging it only when it changes so a long wait logs once.
func (w *Watcher) markStuck(st State, reason string) error {
	if st.Stuck == reason {
		return nil
	}
	logger.Warn("orch: cycle phase stuck", "phase", string(st.Phase), "reason", reason, "strandGUID", st.Strand)
	st.Stuck = reason
	return w.save(st)
}

// paneTooShortReason is the State.Stuck text recorded while the idle probe reports a pane too short to draw an input box.
const paneTooShortReason = "orch pane too short for the idle probe; resize or use the larger client"

// probeIdle runs the idle probe, the one door every watcher probe goes through.
// A probe reporting TooShort records paneTooShortReason in st.Stuck, saved and logged once;
// the next probe that does not report it clears that reason, and only that reason.
func (w *Watcher) probeIdle(st *State) (shuttleengine.IdleProbe, error) {
	probe, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return probe, err
	}
	switch {
	case probe.TooShort && st.Stuck != paneTooShortReason:
		logger.Warn("orch: pane too short for the idle probe", "phase", string(st.Phase), "strandGUID", st.Strand)
		st.Stuck = paneTooShortReason
		return probe, w.save(*st)
	case !probe.TooShort && st.Stuck == paneTooShortReason:
		st.Stuck = ""
		return probe, w.save(*st)
	}
	return probe, nil
}

// storeReading records reading in st, taken through turnEnd; an unknown reading is stored as zero tokens.
func storeReading(st *State, reading shuttleengine.ContextReading, turnEnd shuttleengine.Event) {
	st.ReadingTurnEnd = &turnEnd
	st.LastContextTokens, st.LastContextKnown = reading.Tokens, reading.Known
	if !reading.Known {
		st.LastContextTokens = 0
	}
}

func (w *Watcher) tickIdle(st State, now time.Time) error {
	requested, err := CycleRequested(w.paths)
	if err != nil {
		return err
	}
	// The trigger is chosen in a fixed order: the hard cap wins over a request, which wins over the soft threshold.
	var trigger string
	switch {
	case st.LastContextKnown && st.LastContextTokens >= w.cfg.Threshold():
		trigger = TriggerHard
	case requested:
		trigger = TriggerRequested
	case st.LastContextKnown && st.LastContextTokens >= w.cfg.SoftThreshold():
		trigger = TriggerSoft
	default:
		return nil
	}
	if w.newest == nil || !isTurnEnd(*w.newest) {
		return nil
	}
	compact := w.cfg.Mode() == CycleCompact
	deferralHolds := !st.LastDeferral.IsZero() && now.Sub(st.LastDeferral) < w.cfg.SoftIdle()
	quiet := w.cfg.IdleGrace()
	if trigger == TriggerSoft {
		quiet = w.cfg.SoftIdle()
		if deferralHolds {
			return nil
		}
	}
	if compact && trigger == TriggerHard && deferralHolds {
		// A failed compaction holds every automatic trigger for the soft idle; a requested cycle is never held.
		return nil
	}
	if now.Sub(w.newestRead) < quiet {
		return nil
	}
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	if trigger != TriggerRequested {
		// The transcript can change without a turn end, so the saved reading may be stale: fire only on a fresh one.
		reading, err := w.session.ContextTokens(*w.newest)
		if err != nil {
			return err
		}
		storeReading(&st, reading, *w.newest)
		if err := w.save(st); err != nil {
			return err
		}
		threshold := w.cfg.SoftThreshold()
		if trigger == TriggerHard {
			threshold = w.cfg.Threshold()
		}
		if !st.LastContextKnown || st.LastContextTokens < threshold {
			return nil
		}
	}

	if compact {
		return w.startCompacting(st, trigger, now)
	}

	path := NewHandoffPath(w.paths, now)
	text, err := renderHandoffRequest(w.stencilsDir, trigger, path)
	if err != nil {
		return err
	}
	st.PendingHandoff = path
	st.CycleTrigger = trigger
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

// renderHandoffRequest renders the handoff request for trigger: the soft stencil for a soft cycle, the plain one otherwise.
func renderHandoffRequest(stencilsDir, trigger, handoffPath string) (string, error) {
	if trigger == TriggerSoft {
		return RenderSoftHandoffInstruction(stencilsDir, handoffPath)
	}
	return RenderHandoffInstruction(stencilsDir, handoffPath)
}

func (w *Watcher) tickHandoff(st State, now time.Time) error {
	if w.seen.ask {
		return w.toIdle(st, "session asked a question during the handoff")
	}
	if w.seen.turnEndAfterHandoff {
		probe, err := w.probeIdle(&st)
		if err != nil {
			return err
		}
		if probe.Idle {
			return w.startClearing(st, now)
		}
	}
	if w.seen.deferred {
		// A file written between the stat and this tick wins over the DEFER, so the file is re-statted before returning to idle.
		if !w.seen.handoffWritten {
			written, err := handoffWritten(st.PendingHandoff)
			if err != nil {
				return err
			}
			w.seen.handoffWritten = written
			if !written {
				st.LastDeferral = w.seen.deferRead
				return w.toIdle(st, "deferred")
			}
		}
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
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	text, err := renderHandoffRequest(w.stencilsDir, st.CycleTrigger, st.PendingHandoff)
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

// startClearing renders the resume prompt first, so the text the cleared session needs is known good before /clear runs, then persists clearing and types /clear.
// The caller must have seen the session idle on this tick.
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

// tickClearing types nothing while the session is not idle, however long that lasts;
// past the handoff timeout it records the wait in State.Stuck instead.
// Once idle, an unconfirmed clear is typed again before the timeout and skipped after it, and a confirmed one moves on to resuming.
func (w *Watcher) tickClearing(st State, now time.Time) error {
	timedOut := now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout()
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		if timedOut && !probe.TooShort {
			return w.markStuck(st, "clearing timed out with the session not idle; nothing is typed until it is idle")
		}
		return nil
	}
	if !st.PhaseInjected && !timedOut {
		// A clear that errored never reached the pane, and a pre-clear pane passes the idle probe too.
		if err := w.session.ClearSession(st.Strand); err != nil {
			return err
		}
		return w.confirm(st)
	}

	if st.PhaseInjected {
		st.CycleCount++
	}
	st, err = w.enter(st, PhaseResuming, now)
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
		reading, err := w.session.ContextTokens(w.seen.firstTurnEnd)
		if err != nil {
			return err
		}
		storeReading(&st, reading, w.seen.firstTurnEnd)
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
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	if err := w.session.Send(st.Strand, st.PendingResume); err != nil {
		return err
	}
	return w.confirm(st)
}

// startCompacting renders the focus first, so a stencil failure changes nothing, then persists compacting and types `/compact`.
// The caller must have seen the session idle on this tick.
func (w *Watcher) startCompacting(st State, trigger string, now time.Time) error {
	focus, err := RenderCompactFocus(w.stencilsDir)
	if err != nil {
		return err
	}
	st.CycleTrigger = trigger
	if st, err = w.enter(st, PhaseCompacting, now); err != nil {
		return err
	}
	w.newest = nil
	if err := ClearCycleRequest(w.paths); err != nil {
		return err
	}
	if err := w.session.CompactSession(st.Strand, focus); err != nil {
		return err
	}
	return w.confirm(st)
}

// tickCompacting re-reads the context through State.ReadingTurnEnd every tick, since a compaction ends without a turn end.
// The phase completes on a compaction boundary at or after the phase was entered, once the idle probe passes; an earlier boundary never completes it.
// Past the handoff timeout it returns to idle and holds the next automatic trigger for the soft idle, through LastDeferral.
// An unconfirmed `/compact` is typed again only when the idle probe passes and no qualifying boundary has been read.
func (w *Watcher) tickCompacting(st State, now time.Time) error {
	var reading shuttleengine.ContextReading
	qualifying := false
	if st.ReadingTurnEnd != nil {
		var err error
		if reading, err = w.session.ContextTokens(*st.ReadingTurnEnd); err != nil {
			return err
		}
		qualifying = reading.Known && reading.Compacted && !reading.BoundaryAt.Before(st.PhaseEnteredAt)
	}

	idleProbed, idle := false, false
	probe := func() (bool, error) {
		if !idleProbed {
			p, err := w.probeIdle(&st)
			if err != nil {
				return false, err
			}
			idle, idleProbed = p.Idle, true
		}
		return idle, nil
	}

	if qualifying {
		idle, err := probe()
		if err != nil {
			return err
		}
		if idle {
			storeReading(&st, reading, *st.ReadingTurnEnd)
			st.CycleCount++
			return w.toIdle(st, "")
		}
	}
	if now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		if st.ReadingTurnEnd != nil {
			storeReading(&st, reading, *st.ReadingTurnEnd)
		}
		st.LastDeferral = now
		return w.toIdle(st, "compaction timed out")
	}
	if qualifying || st.PhaseInjected {
		return nil
	}
	idle, err := probe()
	if err != nil {
		return err
	}
	if !idle {
		return nil
	}
	focus, err := RenderCompactFocus(w.stencilsDir)
	if err != nil {
		return err
	}
	if err := w.session.CompactSession(st.Strand, focus); err != nil {
		return err
	}
	return w.confirm(st)
}
