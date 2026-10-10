// watcher.go — the watcher's decision core: one poll of the idle check and the persisted cycle, whose mode (State.CycleMode) picks between two machines.
// Both modes start with the note gate: idle, then handoff-requested, which writes the note.
// Clear mode then runs clearing and resuming;
// compact mode then runs compacting, which types `/compact` with a focus text.
// Neither clears or compacts before the note gate passed.
// The cycle's mode is the request's for an operator request and Config.Mode for an automatic trigger.
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
	// SessionState returns the session's state as read from the run's files; the watcher only logs it.
	SessionState(guid string) (shuttleengine.RunSessionState, error)
	// Send types text into the session as a new turn.
	Send(guid, text string) error
	// ClearSession types the provider's clear command into the session.
	ClearSession(guid string) error
	// ReloadPlugins types the provider's plugin reload command into the session.
	ReloadPlugins(guid string) error
	// TypeColor types the provider's color command for the strand's palette color into the session; a strand with no color types nothing.
	TypeColor(guid string) error
	// CompactSession types the provider's compact command into the session, with focus as its single-line instruction.
	CompactSession(guid, focus string) error
	// LoadSkills types the provider's one-turn load message for skills into the session.
	LoadSkills(guid string, skills []string) error
	// ClassifySkillLoad classifies the load turn of skills that turnEnd ended.
	ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error)
	// CompactedSince returns the newest compaction boundary after since in the transcript turnEnd names, with the turn ends that follow it.
	CompactedSince(turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool, error)
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
	skills      []string // Skills the reload sequence's skills step loads in one turn after a clear, before the pointer.
	// index returns the operator command index each role-file render fills in; an error from it fails that render.
	index func() (string, error)
	clock Clock

	// compactedAt is the time of an auto-compaction boundary a turn end read confirmed fresh and not yet reloaded from;
	// zero when none.
	// A tick that reads a turn end replaces it from its own evaluation,
	// and binding to another strand clears it.
	// It is memory only: a restarted watcher finds the boundary again at its next turn end, since the baseline has not moved.
	compactedAt time.Time

	// colorPending is true from binding to a strand until the first idle tick types the strand's color.
	// It is memory only: a restarted watcher types the color again, which is harmless.
	colorPending bool

	started bool   // Whether the cursor has been initialised from state.
	strand  string // Strand the cursor belongs to.
	cursor  int64  // Events-file position read through.

	// replaying is true from a restart into a non-idle phase until the first read,
	// whose events predate the restart in unknown order against the handoff file.
	replaying bool

	newest     *shuttleengine.Event // Newest event read since the last return to idle.
	newestRead time.Time            // When newest was first read.

	seen phaseEvents // What the current non-idle phase has read so far.

	// idleDisagreement is the pair last logged as a disagreement between the idle probe and the session state, valid while hasIdleDisagreement.
	// It is memory only, and binding to another strand clears it.
	idleDisagreement    idleStatePair
	hasIdleDisagreement bool

	// A hold is a run of consecutive probes that are not idle, ended by the next passing probe.
	// lastHeld is the reason the current hold last held an injection for, as logged, and heldSince when the hold began; empty and zero outside a hold.
	// heldWaited is how long the hold the last passing probe ended lasted, zero when it ended none, and sessionStartLogged the newest session-start time logged.
	// All four are memory only: binding to another strand clears them, and a restarted watcher logs again.
	lastHeld           string
	heldSince          time.Time
	heldWaited         time.Duration
	sessionStartLogged time.Time
}

// idleStatePair is one idle probe answer beside the session state it disagreed with.
type idleStatePair struct {
	idle  bool
	state shuttleengine.SessionStateName
	cause string
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
// skills is the orch skill list the reload sequence types after a clear.
// index supplies the operator command index each role-file render fills in.
func NewWatcher(session Session, cfg Config, paths Paths, stencilsDir string, skills []string, index func() (string, error), clock Clock) *Watcher {
	return &Watcher{session: session, cfg: cfg, paths: paths, stencilsDir: stencilsDir, skills: skills, index: index, clock: clock}
}

// renderRoleFile fetches the command index and renders the role file with it.
// Its error is worded as an abort reason naming the failing source, the index or the role stencil.
func (w *Watcher) renderRoleFile() error {
	index, err := w.index()
	if err != nil {
		return fmt.Errorf("command index unavailable: %w", err)
	}
	if err := RenderRoleFile(w.stencilsDir, w.paths.RolePath, index); err != nil {
		return fmt.Errorf("role stencil %s failed to render: %w", roleStencilName, err)
	}
	return nil
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
	var lastTurnEnd *shuttleengine.Event
	for _, ev := range events {
		if st.Phase == PhaseIdle && isTurnEnd(ev) {
			reading, err := w.session.ContextTokens(ev)
			if err != nil {
				return false, err
			}
			storeReading(&st, reading, ev)
			readingChanged = true
			end := ev
			lastTurnEnd = &end
		}
	}
	var compactedAt time.Time
	if lastTurnEnd != nil {
		boundary, found, err := w.session.CompactedSince(*lastTurnEnd, st.CompactionBaseline)
		if err != nil {
			return false, err
		}
		if found {
			switch {
			case boundary.TurnEndsAfter > 1:
				logger.Info("orch: stale compaction boundary passed without a reload", "strandGUID", st.Strand, "boundaryAt", boundary.At, "turnEndsAfter", boundary.TurnEndsAfter)
				st.CompactionBaseline = boundary.At
			case boundary.TurnEndsAfter == 1 && boundary.ReadTurnEndAfter:
				logger.Info("orch: compaction boundary found", "strandGUID", st.Strand, "boundaryAt", boundary.At)
				compactedAt = boundary.At
			}
		}
	}
	// No error point remains before the events are committed to memory.
	w.cursor = next
	if lastTurnEnd != nil {
		w.compactedAt = compactedAt
	}
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

	if err := w.typeStartColor(&st); err != nil {
		return false, err
	}

	switch st.Phase {
	case PhaseIdle:
		if err := w.tickIdle(st, now); err != nil {
			return false, err
		}
		return false, w.deliverNotice()
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

// typeStartColor types the strand's color on the first tick whose idle probe passes after binding, before anything else that tick types, then clears the mark.
// A session resumed at the reload sequence's color step is left to that step.
// A failed typing is logged and dropped, since the color is display only.
func (w *Watcher) typeStartColor(st *State) error {
	if !w.colorPending {
		return nil
	}
	if st.Phase == PhaseResuming && w.reloadStep(*st) == ReloadStepColor {
		w.colorPending = false
		return nil
	}
	probe, err := w.probeIdle(st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	w.colorPending = false
	w.typeColor(st.Strand)
	return nil
}

// typeColor types the strand's color and logs a failure instead of returning it, so the color never wedges a start or a reload.
func (w *Watcher) typeColor(strand string) {
	if err := w.session.TypeColor(strand); err != nil {
		logger.Warn("orch: typing the strand color failed", "strandGUID", strand, "cause", err)
	}
}

// initCursor sets the read cursor from st on the watcher's first tick for a strand.
// A phase belonging to another strand is reset to idle,
// and a same-strand phase's injection is marked unconfirmed so the landed check runs again.
func (w *Watcher) initCursor(st State) (State, error) {
	w.started, w.strand = true, st.Strand
	w.newest, w.seen, w.replaying = nil, phaseEvents{}, false
	w.compactedAt = time.Time{}
	w.colorPending = true
	w.idleDisagreement, w.hasIdleDisagreement = idleStatePair{}, false
	w.lastHeld, w.heldSince, w.heldWaited, w.sessionStartLogged = "", time.Time{}, 0, time.Time{}
	switch {
	case st.Phase == PhaseIdle:
		w.cursor = st.LastInjectionOffset
		return st, nil
	case st.PhaseStrand != st.Strand:
		st = ResetForFreshLaunch(st, st.Strand, w.clock.Now())
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
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry, st.ReloadSkipsSkills = ReloadStepSkills, time.Time{}, nil, false
	st.Stuck, st.StuckByHold = "", false
	st.LastInjectionOffset = w.cursor
	if abortReason != "" {
		st.LastAbortReason = abortReason
	}
	w.newest, w.seen = nil, phaseEvents{}
	if err := ClearResumeMark(w.paths); err != nil {
		return err
	}
	return w.save(st)
}

// enter persists a new non-idle phase, unconfirmed, before its side effect.
func (w *Watcher) enter(st State, phase Phase, now time.Time) (State, error) {
	st.Phase = phase
	st.PhaseStrand = st.Strand
	st.PhaseEnteredAt = now
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	st.Stuck, st.StuckByHold = "", false
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
	st.Stuck, st.StuckByHold = reason, false
	return w.save(st)
}

// paneTooShortReason is the State.Stuck text recorded while the idle probe reports a pane too short to draw an input box.
const paneTooShortReason = "orch pane too short for the idle probe; resize or use the larger client"

// paneNotIdleReason is the hold reason of a probe that is not idle and gives no reason of its own: the pane probe decided.
const paneNotIdleReason = "the pane does not show an empty input box with no turn in progress"

// heldReason returns why probe holds an injection: its own reason, else the pane-too-short text, else the pane fallback.
func heldReason(probe shuttleengine.IdleProbe) string {
	switch {
	case probe.Reason != "":
		return probe.Reason
	case probe.TooShort:
		return paneTooShortReason
	}
	return paneNotIdleReason
}

// probeIdle runs the idle probe, the one door every watcher probe goes through.
// A probe that is not idle is logged and recorded in st.Stuck through logHeld;
// a passing probe ends the hold and clears a Stuck that a hold wrote, and only that.
func (w *Watcher) probeIdle(st *State) (shuttleengine.IdleProbe, error) {
	probe, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return probe, err
	}
	w.logIdleStateDisagreement(st.Strand, probe)
	if !probe.Idle {
		return probe, w.logHeld(st, probe)
	}
	w.heldWaited = 0
	if !w.heldSince.IsZero() {
		w.heldWaited = w.clock.Now().Sub(w.heldSince)
	}
	w.lastHeld, w.heldSince = "", time.Time{}
	if st.StuckByHold {
		st.Stuck, st.StuckByHold = "", false
		return probe, w.save(*st)
	}
	return probe, nil
}

// logHeld logs why the probe holds the injection, once per change of reason, and records the reason in st.Stuck.
// A Stuck written by anything but a hold is left alone.
func (w *Watcher) logHeld(st *State, probe shuttleengine.IdleProbe) error {
	reason := heldReason(probe)
	now := w.clock.Now()
	if w.heldSince.IsZero() {
		w.heldSince = now
	}
	if reason != w.lastHeld {
		logger.Info("orch: injection held", "strandGUID", st.Strand, "phase", string(st.Phase), "step", w.stepName(*st), "reason", reason, "sincePhase", w.sincePhase(*st, now))
	}
	w.lastHeld = reason
	if st.Stuck != "" && !st.StuckByHold || st.Stuck == reason {
		return nil
	}
	st.Stuck, st.StuckByHold = reason, true
	return w.save(*st)
}

// logTyped logs that step was typed into the session, with how long the hold the caller's passing probe ended lasted.
func (w *Watcher) logTyped(st State, step string) {
	now := w.clock.Now()
	logger.Info("orch: injection typed", "strandGUID", st.Strand, "phase", string(st.Phase), "step", step, "waited", w.heldWaited, "sincePhase", w.sincePhase(st, now))
	w.heldWaited = 0
}

// stepName names the reload step st is in while it is resuming, and is empty in every other phase.
func (w *Watcher) stepName(st State) string {
	if st.Phase != PhaseResuming {
		return ""
	}
	return reloadStepName(w.reloadStep(st))
}

// sincePhase returns how long st's phase has run at now, or since the newest event was read while idle.
func (w *Watcher) sincePhase(st State, now time.Time) time.Duration {
	switch {
	case st.Phase != PhaseIdle:
		return now.Sub(st.PhaseEnteredAt)
	case w.newest != nil:
		return now.Sub(w.newestRead)
	}
	return 0
}

// reloadStepName returns the name of a reload step as the logs spell it.
func reloadStepName(step int) string {
	switch step {
	case ReloadStepColor:
		return "color"
	case ReloadStepPlugins:
		return "plugins"
	case ReloadStepSkills:
		return "skills"
	case ReloadStepRetry:
		return "retry"
	}
	return "pointer"
}

// logIdleStateDisagreement warns once when probe reads idle beside the state busy, or not idle beside idle-done, idle-stalled or asking, and again only after either side changes.
// It also logs the hook's session-start event once per new time.
// A probe that reports the pane too short is not compared.
// A state that cannot be read is logged at Debug, and nothing the watcher decides depends on the state.
func (w *Watcher) logIdleStateDisagreement(strand string, probe shuttleengine.IdleProbe) {
	reading, err := w.session.SessionState(strand)
	if err != nil {
		logger.Debug("orch: session state unreadable", "strandGUID", strand, "cause", err)
		return
	}
	if at := reading.SessionStartAt; !at.IsZero() && !at.Equal(w.sessionStartLogged) {
		w.sessionStartLogged = at
		logger.Info("orch: session start signal read", "strandGUID", strand, "sessionStartAt", at)
	}
	if probe.TooShort {
		return
	}
	state := reading.State
	var disagrees bool
	if probe.Idle {
		disagrees = state.Name == shuttleengine.SessionBusy
	} else {
		disagrees = state.Name == shuttleengine.SessionIdleDone || state.Name == shuttleengine.SessionIdleStalled || state.Name == shuttleengine.SessionAsking
	}
	if !disagrees {
		w.hasIdleDisagreement = false
		return
	}
	pair := idleStatePair{idle: probe.Idle, state: state.Name, cause: state.Cause}
	if w.hasIdleDisagreement && w.idleDisagreement == pair {
		return
	}
	w.idleDisagreement, w.hasIdleDisagreement = pair, true
	logger.Warn("orch: session state disagrees with the idle probe", "strandGUID", strand, "idle", probe.Idle, "state", string(state.Name), "cause", state.Cause, "since", state.Since)
}

// storeReading records reading in st, taken through turnEnd; an unknown reading is stored as zero tokens.
func storeReading(st *State, reading shuttleengine.ContextReading, turnEnd shuttleengine.Event) {
	st.ReadingTurnEnd = &turnEnd
	st.LastContextTokens, st.LastContextKnown = reading.Tokens, reading.Known
	if !reading.Known {
		st.LastContextTokens = 0
	}
}

// deliverNotice types the oldest queued notice into the session as a turn, then removes its file.
// It runs after tickIdle and delivers only when the persisted phase is still idle, so a tick that started a cycle delivers nothing,
// and only when the idle probe passes, so a notice is never typed over a draft or a running turn.
// Delivery is at least once: a watcher that dies between typing and removing types the notice again after restart.
func (w *Watcher) deliverNotice() error {
	notices, err := ListNotices(w.paths)
	if err != nil || len(notices) == 0 {
		return err
	}
	st, err := LoadState(w.paths)
	if err != nil {
		return err
	}
	if st.Phase != PhaseIdle {
		return nil
	}
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	if err := w.session.Send(st.Strand, notices[0].Line); err != nil {
		return err
	}
	return RemoveNotice(notices[0])
}

func (w *Watcher) tickIdle(st State, now time.Time) error {
	if !w.compactedAt.IsZero() {
		return w.startAutoReload(st, now)
	}
	req, requested, err := CycleRequested(w.paths)
	if err != nil {
		return err
	}
	if requested && now.Sub(req.RequestedAt) >= w.cfg.HandoffTimeout() {
		// The time is on disk, so this covers a request whose idle probe never passed and a marker a dead watcher left.
		logger.Warn("orch: stale cycle request removed", "mode", req.Mode, "age", now.Sub(req.RequestedAt).String(), "strandGUID", st.Strand)
		if err := ClearCycleRequest(w.paths); err != nil {
			return err
		}
		requested = false
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
	mode, requestedAt := w.cfg.Mode(), time.Time{}
	if trigger == TriggerRequested {
		mode, requestedAt = req.Mode, req.RequestedAt
	}
	deferralHolds := !st.LastDeferral.IsZero() && now.Sub(st.LastDeferral) < w.cfg.SoftIdle()
	quiet := w.cfg.IdleGrace()
	if trigger == TriggerRequested {
		// The operator asked for this cycle at a moment of their choosing; the idle probe alone guards the pane.
		quiet = 0
	}
	if trigger == TriggerSoft {
		quiet = w.cfg.SoftIdle()
		if deferralHolds {
			return nil
		}
	}
	if mode == CycleCompact && trigger == TriggerHard && deferralHolds {
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

	path := NewHandoffPath(w.paths, now)
	text, err := w.renderHandoffRequest(trigger, path)
	if err != nil {
		return err
	}
	st.PendingHandoff = path
	st.CycleTrigger = trigger
	st.CycleMode, st.CycleRequestedAt = mode, requestedAt
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

// renderHandoffRequest renders the note template file, then the note request for trigger: the soft stencil for a soft cycle, the plain one otherwise.
func (w *Watcher) renderHandoffRequest(trigger, handoffPath string) (string, error) {
	if err := RenderNoteTemplateFile(w.stencilsDir, w.paths.NoteTemplatePath); err != nil {
		return "", err
	}
	if trigger == TriggerSoft {
		return RenderSoftHandoffInstruction(w.stencilsDir, handoffPath, w.paths.NoteTemplatePath)
	}
	return RenderHandoffInstruction(w.stencilsDir, handoffPath, w.paths.NoteTemplatePath)
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
			if st.CycleMode == CycleCompact {
				return w.startCompacting(st, now)
			}
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
	// A requested cycle's clock starts at the request, which predates the phase entry.
	started := st.PhaseEnteredAt
	if !st.CycleRequestedAt.IsZero() && st.CycleRequestedAt.Before(started) {
		started = st.CycleRequestedAt
	}
	if now.Sub(started) >= w.cfg.HandoffTimeout() {
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
	text, err := w.renderHandoffRequest(st.CycleTrigger, st.PendingHandoff)
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

// startClearing renders the role file and the resume prompt first, so what the cleared session needs is known good before /clear runs, then persists clearing and types /clear.
// The caller must have seen the session idle on this tick.
func (w *Watcher) startClearing(st State, now time.Time) error {
	if err := w.renderRoleFile(); err != nil {
		return w.toIdle(st, err.Error())
	}
	resume, err := RenderResumePrompt(w.stencilsDir, w.paths.RolePath, st.PendingHandoff)
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
		if err := w.removeSatisfiedRequest(CycleClear, now); err != nil {
			return err
		}
	}
	return w.startReload(st, now, false)
}

// removeSatisfiedRequest removes a pending cycle request of mode made no later than effectAt, the time the running cycle's action took effect.
// A cycle start clears the request pending at that moment, so a request found during the cycle was made after it started;
// the cycle's action did what that request asks, so a second cycle for it is pointless.
// Bound: it removes only a request made after the cycle started and before its action took effect, for the same thing the action did;
// the request time is CLI-written and the boundary time is the transcript's, both wall clock on one host.
func (w *Watcher) removeSatisfiedRequest(mode string, effectAt time.Time) error {
	req, pending, err := CycleRequested(w.paths)
	if err != nil || !pending || req.Mode != mode || req.RequestedAt.After(effectAt) {
		return err
	}
	logger.Info("orch: cycle request satisfied by the running cycle and removed", "mode", req.Mode, "requestedAt", req.RequestedAt)
	return ClearCycleRequest(w.paths)
}

// startAutoReload reloads the plugins and the role after an auto-compaction read at a turn end, once the idle probe passes.
// The baseline moves to the boundary when the phase is entered, so no boundary reloads twice.
// A role-file render failure, the command index's included, returns to idle with the reason, as a clear cycle's does.
// It also forgets the boundary, so the failure is reported once and the next turn end retries it.
func (w *Watcher) startAutoReload(st State, now time.Time) error {
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	if err := w.renderRoleFile(); err != nil {
		w.compactedAt = time.Time{}
		return w.toIdle(st, err.Error())
	}
	pointer, err := RenderReloadPrompt(w.stencilsDir, w.paths.RolePath)
	if err != nil {
		return err
	}
	st.CompactionBaseline = w.compactedAt
	st.PendingResume = pointer
	w.compactedAt = time.Time{}
	return w.startReload(st, now, true)
}

// startReload enters the resuming phase at its color step and types it.
// skipsSkills is true for a reload after a compaction, which keeps the session's skills, and false after `/clear`, which loses them.
// The caller must have seen the session idle on this tick and set st.PendingResume to the pointer line.
func (w *Watcher) startReload(st State, now time.Time, skipsSkills bool) error {
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry, st.ReloadSkipsSkills = ReloadStepColor, time.Time{}, nil, skipsSkills
	st, err := w.enter(st, PhaseResuming, now)
	if err != nil {
		return err
	}
	w.newest = nil
	return w.typeReloadStep(st, now)
}

// reloadStep returns the step st is in, normalised:
// a reload that skips the skills has no skills step, an empty skill list has none either,
// a retry step with nothing to retry has no retry,
// and any value that is none of these is the pointer step.
func (w *Watcher) reloadStep(st State) int {
	switch {
	case st.ReloadStep == ReloadStepColor:
		return ReloadStepColor
	case st.ReloadStep == ReloadStepPlugins:
		return ReloadStepPlugins
	case st.ReloadStep == ReloadStepSkills && len(w.skills) > 0 && !st.ReloadSkipsSkills:
		return ReloadStepSkills
	case st.ReloadStep == ReloadStepRetry && len(st.ReloadRetry) > 0:
		return ReloadStepRetry
	}
	return ReloadStepPointer
}

// typeReloadStep types the current step, the color, the plugins reload, the skills load, the retry load or the pointer: the caller must have seen the session idle on this tick.
// The first typing persists the step's time and events offset first, so a turn end read before it never confirms the step.
// A re-typing after a restart keeps both, so the step's timeout never restarts.
// The color and plugins steps end no turn,
// so each persists the move to the next step itself and types nothing else on this tick.
func (w *Watcher) typeReloadStep(st State, now time.Time) error {
	if st.ReloadTypedAt.IsZero() {
		st.ReloadTypedAt = now
		st.PhaseEventsOffset = w.cursor
		st.PhaseInjected = false
		w.seen = phaseEvents{}
		if err := w.save(st); err != nil {
			return err
		}
	}
	var err error
	step := w.reloadStep(st)
	switch step {
	case ReloadStepColor:
		w.typeColor(st.Strand)
		w.logTyped(st, reloadStepName(step))
		st.ReloadStep, st.ReloadTypedAt = ReloadStepPlugins, time.Time{}
		st.PhaseEventsOffset = w.cursor
		st.PhaseInjected = false
		w.seen = phaseEvents{}
		return w.save(st)
	case ReloadStepPlugins:
		if err := w.session.ReloadPlugins(st.Strand); err != nil {
			return err
		}
		w.logTyped(st, reloadStepName(step))
		next := ReloadStepSkills
		if st.ReloadSkipsSkills {
			next = ReloadStepPointer
		}
		st.ReloadStep, st.ReloadTypedAt = next, time.Time{}
		st.PhaseEventsOffset = w.cursor
		st.PhaseInjected = false
		w.seen = phaseEvents{}
		return w.save(st)
	case ReloadStepSkills:
		err = w.session.LoadSkills(st.Strand, w.skills)
	case ReloadStepRetry:
		err = w.session.LoadSkills(st.Strand, st.ReloadRetry)
	default:
		if mark, delivered := w.hookDelivered(st); delivered {
			logger.Info("orch: pointer delivered by the session-start hook", "strandGUID", st.Strand, "markAt", mark.At)
			return w.toIdle(st, "")
		}
		err = w.session.Send(st.Strand, st.PendingResume)
	}
	if err != nil {
		return err
	}
	w.logTyped(st, reloadStepName(step))
	return w.confirm(st)
}

// hookDelivered returns the delivery mark and true when the session-start hook already delivered the pending pointer:
// a mark dated at or after the compaction baseline whose text is the pending pointer.
// An unreadable mark counts as no delivery, so the pointer is typed.
func (w *Watcher) hookDelivered(st State) (ResumeMark, bool) {
	mark, found, err := ReadResumeMark(w.paths)
	if err != nil {
		logger.Warn("orch: resume mark unreadable; the pointer is typed", "strandGUID", st.Strand, "cause", err)
		return ResumeMark{}, false
	}
	return mark, found && !mark.At.Before(st.CompactionBaseline) && mark.Text == st.PendingResume
}

// advanceReload persists the move to step, with retry as the skills its retry step loads and its offset taken at the cursor, and goes on to type it when the idle probe passes.
func (w *Watcher) advanceReload(st State, now time.Time, step int, retry []string) error {
	st.ReloadStep, st.ReloadRetry = step, retry
	st.ReloadTypedAt = time.Time{}
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	w.seen = phaseEvents{}
	if err := w.save(st); err != nil {
		return err
	}
	return w.tickResuming(st, now)
}

// skipSkill logs the skipped skill and its cause.
func (w *Watcher) skipSkill(st State, skill, cause string) {
	logger.Warn("orch: skill skipped", "skill", skill, "cause", cause, "strandGUID", st.Strand)
}

// settleSkillTurn classifies the load turn of skills that the step's first turn end ended, logs what is skipped and moves on:
// to the retry step when the skills step left skills missing, else to the pointer.
// An unreadable turn is confirmed unverified, with no retry.
func (w *Watcher) settleSkillTurn(st State, now time.Time, step int, skills []string) error {
	report, err := w.session.ClassifySkillLoad(w.seen.firstTurnEnd, skills)
	if err != nil {
		return err
	}
	if !report.Verified {
		logger.Warn("orch: skill load unverified", "skills", skills, "strandGUID", st.Strand)
		return w.advanceReload(st, now, ReloadStepPointer, nil)
	}
	for _, skill := range report.Unknown {
		w.skipSkill(st, skill, "unknown")
	}
	if step == ReloadStepSkills && len(report.Missing) > 0 {
		return w.advanceReload(st, now, ReloadStepRetry, report.Missing)
	}
	for _, skill := range report.Missing {
		w.skipSkill(st, skill, "not loaded")
	}
	return w.advanceReload(st, now, ReloadStepPointer, nil)
}

// tickResuming walks the reload sequence: the color step, the plugins step, the skills step after `/clear`, the retry step when skills were left missing, then the pointer.
// The color and plugins steps have no confirmation and no timeout: each is typed again until the move off it is persisted.
// A skills or retry step is confirmed by a turn end read after it was typed, and settled from that turn.
// Past its timeout, from its first typing, every skill it loads is skipped with no retry.
// The pointer step ends the phase at its first turn end and times out the same way.
// Nothing is typed unless the idle probe passed on the same tick, and a step typed before a restart is typed again until confirmed.
func (w *Watcher) tickResuming(st State, now time.Time) error {
	typed := !st.ReloadTypedAt.IsZero()
	timedOut := typed && now.Sub(st.ReloadTypedAt) >= w.cfg.HandoffTimeout()
	step := w.reloadStep(st)
	if step == ReloadStepColor || step == ReloadStepPlugins {
		return w.typeReloadStepWhenIdle(st, now)
	}
	if step != ReloadStepPointer {
		skills := w.skills
		if step == ReloadStepRetry {
			skills = st.ReloadRetry
		}
		if typed {
			if w.seen.turnEnd {
				return w.settleSkillTurn(st, now, step, skills)
			}
			if timedOut {
				for _, skill := range skills {
					w.skipSkill(st, skill, "timeout")
				}
				return w.advanceReload(st, now, ReloadStepPointer, nil)
			}
			if st.PhaseInjected {
				return nil
			}
		}
	} else if typed {
		if w.seen.turnEnd {
			reading, err := w.session.ContextTokens(w.seen.firstTurnEnd)
			if err != nil {
				return err
			}
			storeReading(&st, reading, w.seen.firstTurnEnd)
			return w.toIdle(st, "")
		}
		if timedOut {
			// The persisted reading predates the reload and says nothing about the new context.
			st.LastContextTokens, st.LastContextKnown = 0, false
			return w.toIdle(st, "resume timed out")
		}
		if st.PhaseInjected {
			return nil
		}
	}
	return w.typeReloadStepWhenIdle(st, now)
}

// typeReloadStepWhenIdle types the current step when the idle probe passes on this tick, and types nothing otherwise.
func (w *Watcher) typeReloadStepWhenIdle(st State, now time.Time) error {
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	return w.typeReloadStep(st, now)
}

// startCompacting renders the focus first, so a stencil failure changes nothing, then persists compacting and types `/compact`.
// The caller must have seen the session idle on this tick and the note gate pass, so the note becomes LastHandoff.
func (w *Watcher) startCompacting(st State, now time.Time) error {
	focus, err := RenderCompactFocus(w.stencilsDir)
	if err != nil {
		return err
	}
	st.LastHandoff = st.PendingHandoff
	if st, err = w.enter(st, PhaseCompacting, now); err != nil {
		return err
	}
	w.newest = nil
	if err := w.session.CompactSession(st.Strand, focus); err != nil {
		return err
	}
	return w.confirm(st)
}

// reloadAfterCompaction renders the role file and the resume pointer naming the cycle's note, then enters the reload sequence.
// The caller must have seen the session idle on this tick.
// A stencil failure returns to idle with the reason, as a clear cycle's does.
func (w *Watcher) reloadAfterCompaction(st State, now time.Time) error {
	if err := w.renderRoleFile(); err != nil {
		return w.toIdle(st, err.Error())
	}
	resume, err := RenderResumePrompt(w.stencilsDir, w.paths.RolePath, st.LastHandoff)
	if err != nil {
		return w.toIdle(st, fmt.Sprintf("resume stencil %s failed to render: %v", resumeStencilName, err))
	}
	st.PendingResume = resume
	return w.startReload(st, now, true)
}

// tickCompacting searches the transcript State.ReadingTurnEnd names for a compaction boundary every tick, since a compaction ends without a turn end.
// The boundary is searched for rather than read off the newest entry, because a message answered right after the boundary hides it there.
// The phase completes on a boundary after the phase was entered, whatever follows it, once the idle probe passes; an earlier boundary never completes it.
// Past the handoff timeout it returns to idle and holds the next automatic trigger for the soft idle, through LastDeferral.
// An unconfirmed `/compact` is typed again only when the idle probe passes and no qualifying boundary has been found.
func (w *Watcher) tickCompacting(st State, now time.Time) error {
	var boundary shuttleengine.CompactionBoundary
	qualifying := false
	if st.ReadingTurnEnd != nil {
		var err error
		if boundary, qualifying, err = w.session.CompactedSince(*st.ReadingTurnEnd, st.PhaseEnteredAt); err != nil {
			return err
		}
	}
	storeCurrentReading := func() error {
		if st.ReadingTurnEnd == nil {
			return nil
		}
		reading, err := w.session.ContextTokens(*st.ReadingTurnEnd)
		if err != nil {
			return err
		}
		storeReading(&st, reading, *st.ReadingTurnEnd)
		return nil
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
			logger.Info("orch: compaction boundary found", "strandGUID", st.Strand, "boundaryAt", boundary.At)
			if err := storeCurrentReading(); err != nil {
				return err
			}
			st.CycleCount++
			st.CompactionBaseline = boundary.At
			if err := w.removeSatisfiedRequest(CycleCompact, boundary.At); err != nil {
				return err
			}
			return w.reloadAfterCompaction(st, now)
		}
	}
	if now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		if err := storeCurrentReading(); err != nil {
			return err
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
