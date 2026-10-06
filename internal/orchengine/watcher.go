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
	// Send types text into the session as a new turn.
	Send(guid, text string) error
	// ClearSession types the provider's clear command into the session.
	ClearSession(guid string) error
	// CompactSession types the provider's compact command into the session, with focus as its single-line instruction.
	CompactSession(guid, focus string) error
	// LoadSkills types the provider's one-turn load message for skills into the session.
	LoadSkills(guid string, skills []string) error
	// ClassifySkillLoad classifies the load turn of skills that turnEnd ended.
	ClassifySkillLoad(turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error)
	// CompactedSince returns the time of the newest compaction boundary after since in the transcript turnEnd names.
	CompactedSince(turnEnd shuttleengine.Event, since time.Time) (time.Time, bool, error)
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
	skills      []string // Skills the reload sequence's skills step loads in one turn, before the pointer.
	clock       Clock

	// compactedAt is the time of an auto-compaction boundary read at a turn end and not yet reloaded from; zero when none.
	// It is memory only: a restarted watcher finds the boundary again at its next turn end, since the baseline has not moved.
	compactedAt time.Time

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
// skills is the orch skill list the reload sequence types after a clear, a compaction and an auto-compaction.
func NewWatcher(session Session, cfg Config, paths Paths, stencilsDir string, skills []string, clock Clock) *Watcher {
	return &Watcher{session: session, cfg: cfg, paths: paths, stencilsDir: stencilsDir, skills: skills, clock: clock}
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
		at, found, err := w.session.CompactedSince(*lastTurnEnd, st.CompactionBaseline)
		if err != nil {
			return false, err
		}
		if found {
			compactedAt = at
		}
	}
	// No error point remains before the events are committed to memory.
	w.cursor = next
	if !compactedAt.IsZero() {
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
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry = 0, time.Time{}, nil
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
	if err := RenderRoleFile(w.stencilsDir, w.paths.RolePath); err != nil {
		return w.toIdle(st, fmt.Sprintf("role stencil %s failed to render: %v", roleStencilName, err))
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
	}
	return w.startReload(st, now)
}

// startAutoReload reloads the skills and the role after an auto-compaction read at a turn end, once the idle probe passes.
// The baseline moves to the boundary when the phase is entered, so no boundary reloads twice.
func (w *Watcher) startAutoReload(st State, now time.Time) error {
	probe, err := w.probeIdle(&st)
	if err != nil {
		return err
	}
	if !probe.Idle {
		return nil
	}
	if err := RenderRoleFile(w.stencilsDir, w.paths.RolePath); err != nil {
		return err
	}
	pointer, err := RenderReloadPrompt(w.stencilsDir, w.paths.RolePath)
	if err != nil {
		return err
	}
	st.CompactionBaseline = w.compactedAt
	st.PendingResume = pointer
	w.compactedAt = time.Time{}
	return w.startReload(st, now)
}

// startReload enters the resuming phase at its first step and types it.
// The caller must have seen the session idle on this tick and set st.PendingResume to the pointer line.
func (w *Watcher) startReload(st State, now time.Time) error {
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry = ReloadStepSkills, time.Time{}, nil
	st, err := w.enter(st, PhaseResuming, now)
	if err != nil {
		return err
	}
	w.newest = nil
	return w.typeReloadStep(st, now)
}

// reloadStep returns the step st is in, normalised: an empty skill list has no skills step, a retry step with nothing to retry has no retry,
// and any value that is neither is the pointer step.
func (w *Watcher) reloadStep(st State) int {
	switch {
	case st.ReloadStep == ReloadStepSkills && len(w.skills) > 0:
		return ReloadStepSkills
	case st.ReloadStep == ReloadStepRetry && len(st.ReloadRetry) > 0:
		return ReloadStepRetry
	}
	return ReloadStepPointer
}

// typeReloadStep types the current step, the skills load, the retry load or the pointer: the caller must have seen the session idle on this tick.
// The first typing persists the step's time and events offset first, so a turn end read before it never confirms the step.
// A re-typing after a restart keeps both, so the step's timeout never restarts.
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
	switch w.reloadStep(st) {
	case ReloadStepSkills:
		err = w.session.LoadSkills(st.Strand, w.skills)
	case ReloadStepRetry:
		err = w.session.LoadSkills(st.Strand, st.ReloadRetry)
	default:
		err = w.session.Send(st.Strand, st.PendingResume)
	}
	if err != nil {
		return err
	}
	return w.confirm(st)
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

// tickResuming walks the reload sequence: the skills step, the retry step when skills were left missing, then the pointer.
// A skills or retry step is confirmed by a turn end read after it was typed and settled from that turn; past its timeout, from its first typing, every skill it loads is skipped with no retry.
// The pointer step ends the phase at its first turn end and times out the same way.
// Nothing is typed unless the idle probe passed on the same tick, and a step typed before a restart is typed again until confirmed.
func (w *Watcher) tickResuming(st State, now time.Time) error {
	typed := !st.ReloadTypedAt.IsZero()
	timedOut := typed && now.Sub(st.ReloadTypedAt) >= w.cfg.HandoffTimeout()
	step := w.reloadStep(st)
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
	if err := RenderRoleFile(w.stencilsDir, w.paths.RolePath); err != nil {
		return w.toIdle(st, fmt.Sprintf("role stencil %s failed to render: %v", roleStencilName, err))
	}
	resume, err := RenderResumePrompt(w.stencilsDir, w.paths.RolePath, st.LastHandoff)
	if err != nil {
		return w.toIdle(st, fmt.Sprintf("resume stencil %s failed to render: %v", resumeStencilName, err))
	}
	st.PendingResume = resume
	return w.startReload(st, now)
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
			st.CompactionBaseline = reading.BoundaryAt
			return w.reloadAfterCompaction(st, now)
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
