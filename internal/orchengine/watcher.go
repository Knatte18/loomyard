// watcher.go is the decision core of the orch cycle:
// one poll of the persisted cycle, whose mode (State.CycleMode) picks between two machines.
// Both modes start with the note gate: idle, then handoff-requested.
// It posts the note request as creel mail and waits for a turn end with the note written.
// Clear mode then runs clearing and resuming.
// Compact mode then runs compacting, which injects `/compact` with a focus text.
// Neither clears nor compacts before the note gate has passed.
// The cycle's mode is the request's for an operator request, and Config.Mode for an automatic trigger.
// Both share the idle trigger selection, its gates, and its re-read of the context.
//
// Every provider and reed interaction goes through the Session seam,
// and every delivery into the orch goes through the Creel seam,
// so the whole state machine runs against fakes in untagged unit tests.
// The watcher types nothing into the pane itself: each step is a creel post or inject.
// Creel's own idle check gates its delivery.
// The watcher saves State before each send, with the step it is about to send as State.Pending,
// and a restarted watcher resumes from it:
// a pending step with no id is looked up in what creel journaled before it is sent again;
// one with an id has its outcome read.
// While a cycle runs the watcher holds the orch strand in creel;
// other mail waits until the cycle returns to idle.

package orchengine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// stepInjectTTL is how long a cycle step's inject may wait for the orch's next idle before creel expires it.
	// It is creel's default inject time to live.
	stepInjectTTL = 10 * time.Minute
	// cycleHoldDuration is how long one creel hold on the orch strand lasts.
	// Every tick outside idle renews it, so a hold a dead watcher leaves lapses on its own.
	cycleHoldDuration = 10 * time.Minute
)

// The creel command names the cycle injects, in creel's neutral spelling.
const (
	commandClear         = "clear"
	commandCompact       = "compact"
	commandReloadPlugins = "reload-plugins"
	commandColor         = "color"
	commandLoadSkills    = "load-skills"
)

// The two bodies of a reply to the note request the watcher recognises.
const (
	// replyWritten marks a reply the watcher ignores, since the note gate reads the note itself.
	replyWritten = "written"
	// replyDefer declines a soft cycle.
	replyDefer = "defer"
)

// errNotBound reports that the watcher has not yet set its events cursor for the recorded strand.
// It cannot act on a reply until then.
var errNotBound = errors.New("orch: the watcher is not bound to the recorded strand yet")

// errCompactWithdrawHeld reports that withdrawing the still-queued `/compact` was held by a creel failure.
// The compacting phase keeps its boundary for the next tick.
var errCompactWithdrawHeld = errors.New("orch: withdrawing the queued compact is held by a creel failure")

// errCompactInFlight reports that the queued `/compact` was in a creel delivery attempt the withdraw could not remove.
// The compacting phase keeps its boundary and the pending step for the next tick.
var errCompactInFlight = errors.New("orch: the queued compact is in a creel delivery attempt")

// errResetPending reports that a phase belonging to another strand could not be reset because a creel call failed;
// the next tick runs the reset again.
var errResetPending = errors.New("orch: resetting a phase of another strand is held by a creel failure")

// Session is the seam between the cycle machine and the live orchestrator session.
// Production satisfies it with an adapter over shuttleengine.Runner and reed.
type Session interface {
	// StrandAlive reports whether the strand exists and its pane is alive.
	StrandAlive(guid string) (bool, error)
	// ReadEvents returns the events past offset and the offset read through.
	ReadEvents(guid string, offset int64) ([]shuttleengine.Event, int64, error)
	// ContextTokens returns the context reading of the run identified by guid as of turnEnd;
	// a reading with Known false means unreadable.
	ContextTokens(guid string, turnEnd shuttleengine.Event) (shuttleengine.ContextReading, error)
	// SessionIdle probes whether the session shows an empty input box with no turn in progress.
	// It also reports whether the pane is too short to tell.
	// Only the hub pause's handling reads it.
	SessionIdle(guid string) (shuttleengine.IdleProbe, error)
	// SessionState returns the session's state as read from the run's files;
	// the watcher logs its session start and records its session id.
	SessionState(guid string) (shuttleengine.RunSessionState, error)
	// StrandName returns the full name of the strand guid, read from reed at each call.
	// The watcher addresses its creel calls with this name.
	// It fails when guid names no tracked strand.
	StrandName(guid string) (string, error)
	// StrandColor returns the strand's palette color from reed, empty for none.
	StrandColor(guid string) (string, error)
	// ClassifySkillLoad classifies the load turn of skills that turnEnd ended in the run identified by guid.
	ClassifySkillLoad(guid string, turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error)
	// CompactedSince returns the newest compaction boundary after since in the transcript turnEnd names.
	// It also returns the turn ends that follow it, for the run identified by guid.
	CompactedSince(guid string, turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool, error)
	// UsageLimit returns the usage-limit reading of the transcript turnEnd names, for the run identified by guid.
	UsageLimit(guid string, turnEnd shuttleengine.Event) (shuttleengine.UsageLimit, error)
	// Interrupt stops the session's running turn with the provider's interrupt key, sent only into a pane showing one.
	// It fails with shuttleengine.ErrNoRunningTurn when the pane shows none.
	// It fails with shuttleengine.ErrRewindPickerOpen when it closed an open rewind picker.
	Interrupt(guid string) error
	// InterruptSettle is the wait after an interrupt key before the pane of the run identified by guid is read again.
	// It fails when guid resolves to no run or to an engine the set does not hold.
	InterruptSettle(guid string) (time.Duration, error)
}

// Clock supplies the current time, settable in tests.
type Clock interface {
	Now() time.Time
}

// Watcher runs the orch cycle, one Tick per poll.
type Watcher struct {
	session     Session
	creel       Creel
	cfg         Config
	paths       Paths
	stencilsDir string
	skills      []string // Skills the reload sequence's skills step loads in one turn after a clear, before the pointer.
	clock       Clock

	// compactedAt is the time of an auto-compaction boundary a turn end read confirmed fresh and not yet reloaded from;
	// zero when none.
	// A tick that reads a turn end replaces it from its own evaluation,
	// and binding to another strand clears it.
	// It is memory only.
	// A restarted watcher finds the boundary again at its next turn end, since the baseline has not moved.
	compactedAt time.Time

	// colorPending is true from binding to a session the color was not yet queued for until its inject settles.
	colorPending bool
	// colorInjectID is the id of the bind color's inject, zero while none was queued.
	// It is memory only: a restarted watcher may queue the color once more, which is display only.
	colorInjectID int64

	started bool   // Whether the cursor has been initialised from state.
	strand  string // Strand the cursor belongs to.
	cursor  int64  // Events-file position read through.

	newest     *shuttleengine.Event // Newest event read since the last return to idle.
	newestRead time.Time            // When newest was first read.

	seen phaseEvents // What the current non-idle phase has read so far.

	// sessionStartLogged is the newest session-start time logged; memory only, so a restarted watcher logs again.
	sessionStartLogged time.Time

	// creelFailedTick is true once a creel call failed in the running tick.
	// It keeps that tick from clearing a creel failure.
	creelFailedTick bool
	// lastCreelError is the text of the creel failure last logged, empty while the calls succeed.
	lastCreelError string
	// repliesRead is true when the running tick's mailbox read handed over messages.
	repliesRead bool

	// pause is the told pause-record reading, nil when the watcher was never told one.
	pause *PauseWatch
	// ownOrchName is the agent name of the orch the watcher watches, empty for the prime's orch.
	// The pause checks match an orch caller or marker against it.
	ownOrchName string
	// binaryCheck re-execs the process onto a replaced binary, nil when the watcher was never told one.
	binaryCheck func()
	// firstTick is the time of the watcher's first tick, zero before it.
	firstTick time.Time
	// pauseHandled holds the PausedAt, as Unix nanoseconds, of each hub entry already handled.
	pauseHandled map[int64]bool
	// pauseFirstEsc holds, per hub entry key, when the first interrupt key went.
	// It is memory only: a watcher restarted during the settle has lost the time.
	// It finds the orch recorded in the entry, settles it through settledWithoutInterrupt, and sends no second key.
	pauseFirstEsc map[int64]time.Time
	// pauseReadError is the text of the record read error last logged, empty while the record reads.
	pauseReadError string
	// netOpsFailures is the failure episode of the pause's network operation marker reads.
	// The reads repeat on every tick of a pause's grace.
	netOpsFailures logger.Episode
	// owner is the told owner reading, nil when the watcher was never told one.
	owner *OwnerWatch
	// ownerReadFailures is the failure episode of the owner reads.
	// They repeat on every tick while the board is unreadable.
	ownerReadFailures logger.Episode
	// ownerAdoptHeld reports whether the last owner reading found the beam owned elsewhere under an adopt in flight here.
	ownerAdoptHeld bool
}

// phaseEvents records what the current non-idle phase has observed.
// It keeps what the cursor has moved past, which a later tick must still know.
type phaseEvents struct {
	// turnEnds lists the turn ends read since the phase or its current step began, oldest first.
	turnEnds []shuttleengine.Event
	ask      bool
}

// NewWatcher builds a watcher over session and creel.
// skills is the orch skill list the reload sequence loads after a clear.
func NewWatcher(session Session, creel Creel, cfg Config, paths Paths, stencilsDir string, skills []string, clock Clock) *Watcher {
	return &Watcher{session: session, creel: creel, cfg: cfg, paths: paths, stencilsDir: stencilsDir, skills: skills, clock: clock}
}

// renderRoleFile renders the role file, with its error worded as an abort reason.
func (w *Watcher) renderRoleFile() error {
	if err := RenderRoleFile(w.paths.RolePath); err != nil {
		return fmt.Errorf("role file failed to render: %w", err)
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
// A failed creel call is no error.
// It is recorded in State.Stuck and holds the cycle until a tick whose creel calls all succeed clears it.
func (w *Watcher) Tick() (done bool, err error) {
	w.creelFailedTick = false
	done, err = w.tick()
	if errors.Is(err, errStrandReplaced) {
		logger.Info("orch: state was bound to another strand mid-tick; the next tick follows it", "strandGUID", w.strand)
		return false, nil
	}
	if err != nil || done || w.creelFailedTick {
		return done, err
	}
	w.lastCreelError = ""
	return false, w.clearCreelStuck()
}

// save persists st unless the state has been bound to another strand since this tick loaded it.
func (w *Watcher) save(st State) error {
	return saveStateForStrand(w.paths, st)
}

// clearCreelStuck ends a creel failure recorded in the state, after a tick whose creel calls all succeeded.
func (w *Watcher) clearCreelStuck() error {
	st, err := LoadState(w.paths)
	if err != nil || !st.StuckByCreel {
		return err
	}
	st.Stuck, st.StuckByCreel = "", false
	if err := w.save(st); err != nil && !errors.Is(err, errStrandReplaced) {
		return err
	}
	return nil
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

	if done, err := w.ownerTick(st); done || err != nil {
		return done, err
	}

	if !w.started || w.strand != st.Strand {
		if st, err = w.initCursor(st); err != nil {
			if errors.Is(err, errResetPending) {
				return false, nil
			}
			return false, err
		}
	}

	if err := w.recordSessionID(&st); err != nil {
		return false, err
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
			reading, err := w.session.ContextTokens(st.Strand, ev)
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
	var limit shuttleengine.UsageLimit
	if lastTurnEnd != nil {
		if limit, err = w.session.UsageLimit(st.Strand, *lastTurnEnd); err != nil {
			return false, err
		}
		boundary, found, err := w.session.CompactedSince(st.Strand, *lastTurnEnd, st.CompactionBaseline)
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
		if isTurnEnd(ev) {
			w.seen.turnEnds = append(w.seen.turnEnds, ev)
		}
	}
	limitEnded := false
	if lastTurnEnd != nil {
		limitEnded = recordUsageLimit(&st, limit, now)
	}
	if readingChanged {
		if err := w.save(st); err != nil {
			return false, err
		}
	}
	if limitEnded {
		if err := w.releaseHold(&st); err != nil {
			return false, err
		}
	}

	if st.FreshLaunchFailure != "" {
		retried, err := w.retryFreshLaunch(&st)
		if err != nil || !retried {
			return false, err
		}
	}

	if st.Phase != PhaseIdle {
		held, err := w.renewHold(&st)
		if err != nil || !held {
			return false, err
		}
	}

	held, err := w.pauseTick(st, now)
	if err != nil {
		return false, err
	}
	if held {
		return false, nil
	}

	if st, err = w.readMailbox(st); err != nil {
		return false, err
	}

	if err := w.typeStartColor(&st); err != nil {
		return false, err
	}

	if st.Phase != PhaseIdle && st.Pending != nil && st.Pending.ID == 0 {
		if err := w.recoverPending(&st, now); err != nil {
			return false, err
		}
	}

	switch st.Phase {
	case PhaseIdle:
		if !st.UsageLimitAt.IsZero() {
			return false, w.tickUsageLimit(st, now)
		}
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

// recordSessionID stores in st the session id from shuttle's files-only reading of the strand, if it differs.
// It also logs the reading's session-start time once for each new value.
// An unreadable reading, such as a bound strand with no shuttle run, leaves the recorded id and never fails the tick.
// An empty reading id writes nothing.
// The save is strand-checked, so a strand a concurrent start bound is never written onto.
func (w *Watcher) recordSessionID(st *State) error {
	reading, err := w.session.SessionState(st.Strand)
	if err != nil {
		logger.Debug("orch: session id unreadable", "strandGUID", st.Strand, "cause", err)
		return nil
	}
	if at := reading.SessionStartAt; !at.IsZero() && !at.Equal(w.sessionStartLogged) {
		w.sessionStartLogged = at
		logger.Info("orch: session start signal read", "strandGUID", st.Strand, "sessionStartAt", at)
	}
	if reading.SessionID == "" || reading.SessionID == st.SessionID {
		return nil
	}
	logger.Info("orch: session id recorded", "strandGUID", st.Strand, "previous", st.SessionID, "sessionID", reading.SessionID)
	st.SessionID = reading.SessionID
	return w.save(*st)
}

// orchName returns the full name of the orch strand the state records, read from reed at each call.
func (w *Watcher) orchName(st State) (string, error) {
	return w.session.StrandName(st.Strand)
}

// creelFailed records a failed creel call.
// It is logged once per change of error and written to st.Stuck with StuckByCreel set.
// The failure is no tick error, so a creel outage holds the cycle and never ends the watcher;
// the returned error is only a failed save.
func (w *Watcher) creelFailed(st *State, err error) error {
	w.creelFailedTick = true
	text := err.Error()
	if text != w.lastCreelError {
		logger.Warn("orch: a creel call failed; the cycle is held until creel answers", "strandGUID", st.Strand, "phase", string(st.Phase), "cause", text)
		w.lastCreelError = text
	}
	if st.Stuck == text && st.StuckByCreel {
		return nil
	}
	st.Stuck, st.StuckByCreel = text, true
	return w.save(*st)
}

// retryFreshLaunch retries the withdraw and the release made around a fresh launch.
// It acts on the orch strand's name, for the launch State.FreshLaunchFailure records as failed.
// It answers true once both succeeded.
// It clears the field and State.Stuck in one save,
// and it hands a withdrawn resume line to ClearResumeNoticed.
// A failure, a failed ClearResumeNoticed included, rewrites State.Stuck with the failure.
// It leaves State.StuckByCreel unset, so no successful creel call clears it while the field is set.
// It answers false with no error.
// A resume line the withdraw removed whose clear failed is kept in State.ResumeLineWithdrawn,
// so a later retry clears the mark although its own withdraw finds nothing.
func (w *Watcher) retryFreshLaunch(st *State) (bool, error) {
	var failures []string
	lineWithdrawnBefore := st.ResumeLineWithdrawn
	name, err := w.orchName(*st)
	if err != nil {
		failures = append(failures, err.Error())
	} else {
		withdrawn, err := w.creel.Withdraw(name)
		if err != nil {
			failures = append(failures, err.Error())
		} else if w.pause != nil {
			if st.ResumeLineWithdrawn {
				withdrawn = append(withdrawn, CreelWithdrawn{Subject: pauserecord.ResumeLine})
			}
			err := ClearResumeNoticed(w.pause.RecordPath, st.Strand, withdrawn, w.clock.Now())
			st.ResumeLineWithdrawn = err != nil && slices.ContainsFunc(withdrawn, func(message CreelWithdrawn) bool { return message.Subject == pauserecord.ResumeLine })
			if err != nil {
				failures = append(failures, err.Error())
			}
		}
		if err := w.creel.Release(name); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		w.creelFailedTick = true
		text := strings.Join(failures, "; ")
		if text != w.lastCreelError {
			logger.Warn("orch: retrying the fresh launch's withdraw failed; nothing is posted or queued until it succeeds", "strandGUID", st.Strand, "cause", text)
			w.lastCreelError = text
		}
		if st.Stuck == text && !st.StuckByCreel && st.ResumeLineWithdrawn == lineWithdrawnBefore {
			return false, nil
		}
		st.Stuck, st.StuckByCreel = text, false
		return false, w.save(*st)
	}
	logger.Info("orch: the fresh launch's withdraw and release succeeded on retry", "strandGUID", st.Strand)
	st.FreshLaunchFailure, st.Stuck, st.StuckByCreel = "", "", false
	return true, w.save(*st)
}

// renewHold takes or renews the watcher's creel hold on the orch strand for cycleHoldDuration.
// It reports whether the hold stands.
// A creel refusal or failure is recorded through `creelFailed`, and it reports false with no error.
func (w *Watcher) renewHold(st *State) (bool, error) {
	name, err := w.orchName(*st)
	if err != nil {
		return false, w.creelFailed(st, err)
	}
	if err := w.creel.Hold(name, cycleHoldDuration); err != nil {
		return false, w.creelFailed(st, err)
	}
	return true, nil
}

// releaseHold ends the watcher's creel hold on the orch strand.
// A failed release is recorded through `creelFailed` and changes nothing else:
// the hold lapses within `cycleHoldDuration`.
func (w *Watcher) releaseHold(st *State) error {
	name, err := w.orchName(*st)
	if err != nil {
		return w.creelFailed(st, err)
	}
	if err := w.creel.Release(name); err != nil {
		return w.creelFailed(st, err)
	}
	return nil
}

// readMailbox reads the watcher's own creel mailbox without waiting.
// It hands the messages to `readReplies` and returns the state as the handler left it.
// A handler error is a local failure and fails the tick;
// a failed read is a creel failure and holds the cycle.
func (w *Watcher) readMailbox(st State) (State, error) {
	w.repliesRead = false
	var handlerErr error
	err := w.creel.ReadMailbox(func(messages []CreelMessage) error {
		w.repliesRead = true
		handlerErr = w.readReplies(messages)
		return handlerErr
	})
	if handlerErr != nil {
		return st, handlerErr
	}
	if err != nil {
		if w.repliesRead {
			// The handler already moved the persisted state; saving the pre-handler copy would move the phase back.
			reloaded, loadErr := LoadState(w.paths)
			if loadErr != nil {
				return st, loadErr
			}
			st = reloaded
		}
		return st, w.creelFailed(&st, err)
	}
	if !w.repliesRead {
		return st, nil
	}
	return LoadState(w.paths)
}

// readReplies handles the messages of the watcher's creel mailbox before the client acknowledges the read.
// A failure leaves the messages unread.
// Only a reply from the orch strand whose `reply_to` is the pending note request counts;
// every other message is logged and dropped.
// A `defer` body declines a soft cycle whose note is still unwritten.
// It is ignored in every other case.
// A body starting with `written` is logged and dropped, since the note gate reads the note itself.
// Any other body is logged and dropped too.
func (w *Watcher) readReplies(messages []CreelMessage) error {
	st, err := LoadState(w.paths)
	if err != nil {
		return err
	}
	if !w.started || w.strand != st.Strand {
		return errNotBound
	}
	now := w.clock.Now()
	for _, message := range messages {
		if st.Phase != PhaseHandoffRequested || st.RequestID == 0 || message.ReplyTo != st.RequestID {
			logger.Info("orch: creel message dropped, it answers no pending request", "messageID", message.ID, "from", message.From, "replyTo", message.ReplyTo)
			continue
		}
		name, err := w.orchName(st)
		if err != nil {
			return err
		}
		if message.From != name {
			logger.Info("orch: creel message dropped, it is not from the orch strand", "messageID", message.ID, "from", message.From, "orch", name)
			continue
		}
		body := strings.TrimSpace(message.Body)
		if body == replyDefer {
			if st.CycleTrigger != TriggerSoft {
				logger.Info("orch: defer ignored, only a soft cycle may be declined", "messageID", message.ID, "trigger", st.CycleTrigger)
				continue
			}
			written, err := handoffWritten(st.PendingHandoff)
			if err != nil {
				return err
			}
			if written {
				logger.Info("orch: defer ignored, the note is already written", "messageID", message.ID)
				continue
			}
			st.LastDeferral = now
			return w.toIdle(st, "deferred")
		}
		if strings.HasPrefix(body, replyWritten) {
			logger.Info("orch: a written reply is ignored, the note gate reads the note itself", "messageID", message.ID)
			continue
		}
		logger.Info("orch: reply dropped, only `defer` is acted on", "messageID", message.ID)
	}
	return nil
}

// sendStep persists the step as `State.Pending` with no id, then posts it as mail or injects it into the orch strand.
// A mail posts `body`, and an inject sends `step`; the id creel answers is saved either way.
// A mail ends as the note request: its id becomes `State.RequestID` and nothing stays pending.
// A step is sent only after a re-read of the state file finds its phase still non-idle on its strand;
// otherwise nothing is persisted or sent and the tick ends.
// A failed call is recorded through `creelFailed` and leaves the step pending with no id;
// a later tick's `adoptPending` looks the step up.
func (w *Watcher) sendStep(st *State, mail bool, step CreelStep, body string, now time.Time) error {
	current, err := LoadState(w.paths)
	if err != nil {
		return err
	}
	if current.Phase == PhaseIdle || current.Phase == "" || current.Strand != st.Strand {
		return fmt.Errorf("orch: the phase ended or moved to another strand before its step was sent: %w", errStrandReplaced)
	}
	pending := PendingStep{Mail: mail, PersistedAt: now}
	if mail {
		pending.Text = body
	} else {
		pending.Command, pending.Argument, pending.Text = step.Command, step.Argument, step.Text
	}
	st.Pending = &pending
	if err := w.save(*st); err != nil {
		return err
	}
	name, err := w.orchName(*st)
	if err != nil {
		return w.creelFailed(st, err)
	}
	var id int64
	if mail {
		id, err = w.creel.Post(name, body)
	} else {
		id, err = w.creel.Inject(name, step, stepInjectTTL)
	}
	if err != nil {
		return w.creelFailed(st, err)
	}
	logger.Info("orch: cycle step sent to creel", "strandGUID", st.Strand, "phase", string(st.Phase), "mail", mail, "command", pending.Command, "id", id, "sincePhase", w.sincePhase(*st, now))
	if mail {
		st.Pending, st.RequestID, st.PhaseInjected = nil, id, true
	} else {
		st.Pending.ID = id
	}
	return w.save(*st)
}

// recoverPending resolves a pending step that has no id.
// It adopts what creel journaled for the step, and it sends the step again only when creel journaled nothing.
func (w *Watcher) recoverPending(st *State, now time.Time) error {
	settled, err := w.adoptPending(st)
	if err != nil || settled {
		return err
	}
	pending := st.Pending
	return w.sendStep(st, pending.Mail, CreelStep{Command: pending.Command, Argument: pending.Argument, Text: pending.Text}, pending.Text, now)
}

// adoptPending looks up what creel journaled for a pending step that has no id.
// The step was persisted before its call, and a failed call may still have journaled its request.
// It adopts only a post or inject whose command and argument, or text (a post's first line), match the step.
// It reports whether the tick must not send the step:
// an id was adopted, or the lookup failed and is held through `creelFailed`.
func (w *Watcher) adoptPending(st *State) (bool, error) {
	pending := st.Pending
	if pending == nil || pending.ID != 0 {
		return false, nil
	}
	name, err := w.orchName(*st)
	if err != nil {
		return true, w.creelFailed(st, err)
	}
	sent, err := w.creel.Sent(name, pending.PersistedAt)
	if err != nil {
		return true, w.creelFailed(st, err)
	}
	for _, candidate := range sent {
		if candidate.At.Before(pending.PersistedAt) || !pendingMatches(*pending, candidate) {
			continue
		}
		logger.Info("orch: pending step adopted from creel's journal", "strandGUID", st.Strand, "phase", string(st.Phase), "id", candidate.ID)
		if pending.Mail {
			st.Pending, st.RequestID, st.PhaseInjected = nil, candidate.ID, true
		} else {
			pending.ID = candidate.ID
		}
		return true, w.save(*st)
	}
	return false, nil
}

// pendingMatches reports whether sent is the post or inject the pending step would have made.
func pendingMatches(pending PendingStep, sent CreelSent) bool {
	switch {
	case pending.Mail:
		firstLine, _, _ := strings.Cut(pending.Text, "\n")
		return sent.Mail && sent.Subject == firstLine
	case pending.Command != "":
		return !sent.Mail && sent.Step.Command == pending.Command && sent.Step.Argument == pending.Argument
	}
	return !sent.Mail && sent.Step.Command == "" && sent.Subject == pending.Text
}

// stepOutcome reads the pending inject's outcome from creel without blocking the tick.
// It reports whether the inject was typed (injected or unconfirmed), and the outcome says which.
// An expired inject is sent again with its stall reason recorded in `State.Stuck`;
// a withdrawn inject is left to the caller, which returns the cycle to idle.
// A pending step with no id, or a post, has no outcome to read, so the zero outcome is returned.
func (w *Watcher) stepOutcome(st *State) (CreelOutcome, bool, error) {
	pending := st.Pending
	if pending == nil || pending.ID == 0 || pending.Mail {
		return CreelOutcome{}, false, nil
	}
	outcome, err := w.creel.InjectOutcome(pending.ID)
	if err != nil {
		return CreelOutcome{}, false, w.creelFailed(st, err)
	}
	switch outcome.State {
	case CreelInjected, CreelUnconfirmed:
		if st.Stuck != "" && !st.StuckByCreel {
			st.Stuck = ""
			return outcome, true, w.save(*st)
		}
		return outcome, true, nil
	case CreelExpired:
		reason := outcome.Stall
		if reason == "" {
			reason = "the inject expired before the orch session took it"
		}
		if err := w.markStuck(st, reason); err != nil {
			return outcome, false, err
		}
		step := CreelStep{Command: pending.Command, Argument: pending.Argument, Text: pending.Text}
		return outcome, false, w.sendStep(st, false, step, "", w.clock.Now())
	}
	return outcome, false, nil
}

// turnEndAfterSending returns the first turn end read since the current step began.
// The turn end's hook-side second must be after the second of `sendingAt`.
// `sendingAt` is the newest sending record of the step's inject.
// A turn end with no time never counts, and none counts while `sendingAt` is zero.
func (w *Watcher) turnEndAfterSending(sendingAt time.Time) (shuttleengine.Event, bool) {
	if sendingAt.IsZero() {
		return shuttleengine.Event{}, false
	}
	for _, ev := range w.seen.turnEnds {
		if !ev.At.IsZero() && ev.At.Unix() > sendingAt.Unix() {
			return ev, true
		}
	}
	return shuttleengine.Event{}, false
}

// typeStartColor queues the strand's color on the first tick after binding, before anything else that tick queues.
// It then reads the color's outcome until it settles.
// A session resumed at the reload sequence's color step is left to that step.
// A failed or expired color is logged and dropped, since the color is display only; an unconfirmed one counts as typed.
func (w *Watcher) typeStartColor(st *State) error {
	if !w.colorPending {
		return nil
	}
	if st.Phase == PhaseResuming && w.reloadStep(*st) == ReloadStepColor {
		w.colorPending = false
		return nil
	}
	if w.colorInjectID == 0 {
		return w.typeColor(st)
	}
	outcome, err := w.creel.InjectOutcome(w.colorInjectID)
	if err != nil {
		logger.Warn("orch: reading the strand color's outcome failed; trying again", "strandGUID", st.Strand, "cause", err)
		return nil
	}
	switch outcome.State {
	case CreelInjected, CreelUnconfirmed:
		w.colorPending, w.colorInjectID = false, 0
		return w.recordColorTyped(st)
	case CreelExpired, CreelWithdrawnState:
		logger.Warn("orch: the strand color was not typed", "strandGUID", st.Strand, "outcome", string(outcome.State), "reason", outcome.Stall)
		w.colorPending, w.colorInjectID = false, 0
	}
	return nil
}

// typeColor queues the strand's color as a creel command inject and remembers the inject's id.
// Its outcome is read on a later tick.
// A strand with no color queues nothing and counts as done.
// A failure is logged and drops the color instead of failing the tick, so the color never wedges a start or a reload.
func (w *Watcher) typeColor(st *State) error {
	color, err := w.session.StrandColor(st.Strand)
	if err != nil {
		logger.Warn("orch: reading the strand color failed", "strandGUID", st.Strand, "cause", err)
		w.colorPending = false
		return nil
	}
	if color == "" {
		w.colorPending = false
		return w.recordColorTyped(st)
	}
	name, err := w.orchName(*st)
	if err == nil {
		w.colorInjectID, err = w.creel.Inject(name, CreelStep{Command: commandColor, Argument: color}, stepInjectTTL)
	}
	if err != nil {
		logger.Warn("orch: queueing the strand color failed", "strandGUID", st.Strand, "cause", err)
		w.colorPending, w.colorInjectID = false, 0
	}
	return nil
}

// recordColorTyped records in st that the strand's color counts as typed for its current strand and launch,
// so a watcher binding to the same session queues no start color.
func (w *Watcher) recordColorTyped(st *State) error {
	st.ColorStrand, st.ColorLaunchedAt = st.Strand, st.LaunchedAt
	return w.save(*st)
}

// initCursor sets the read cursor from st on the watcher's first tick for a strand.
// A phase that belongs to another strand is reset to idle, which withdraws the watcher's queued injects and unread mail.
// A Stuck that neither a pending step, a creel failure nor a recorded fresh-launch failure owns is cleared.
// Only an earlier watcher's hold bookkeeping could have left it.
func (w *Watcher) initCursor(st State) (State, error) {
	w.started, w.strand = true, st.Strand
	w.newest, w.seen = nil, phaseEvents{}
	w.compactedAt = time.Time{}
	w.sessionStartLogged = time.Time{}
	w.colorInjectID = 0
	staleStuck := st.Stuck != "" && st.Pending == nil && !st.StuckByCreel && st.FreshLaunchFailure == ""
	if staleStuck {
		st.Stuck = ""
	}
	switch {
	case st.Phase == PhaseIdle:
		w.cursor = st.LastInjectionOffset
		w.colorPending = !colorTyped(st)
		if staleStuck {
			return st, w.save(st)
		}
		return st, nil
	case st.PhaseStrand != st.Strand:
		return w.resetForOtherStrand(st)
	}
	w.cursor = st.PhaseEventsOffset
	w.colorPending = !colorTyped(st)
	return st, w.save(st)
}

// resetForOtherStrand resets a phase that belongs to another strand to idle.
// It withdraws the watcher's queued injects and unread mail to the orch strand.
// It then releases the hold and drops the pending ids.
// The resume-noticed mark of each pause entry whose resume line the withdraw removed is cleared,
// so a continued session gets the line posted again.
// A failed withdraw or release leaves the watcher unbound and saves no reset.
// The next tick runs the reset again and then answers errResetPending.
func (w *Watcher) resetForOtherStrand(st State) (State, error) {
	held := func(err error) (State, error) {
		w.started = false
		if saveErr := w.creelFailed(&st, err); saveErr != nil {
			return st, saveErr
		}
		return st, errResetPending
	}
	name, err := w.orchName(st)
	if err != nil {
		return held(err)
	}
	withdrawn, err := w.creel.Withdraw(name)
	if err != nil {
		return held(err)
	}
	if w.pause != nil {
		if st.ResumeLineWithdrawn {
			withdrawn = append(withdrawn, CreelWithdrawn{Subject: pauserecord.ResumeLine})
		}
		if err := ClearResumeNoticed(w.pause.RecordPath, st.Strand, withdrawn, w.clock.Now()); err != nil {
			// The withdraw removed the line for good; keep that fact so the next tick's reset still clears the mark.
			st.ResumeLineWithdrawn = slices.ContainsFunc(withdrawn, func(message CreelWithdrawn) bool { return message.Subject == pauserecord.ResumeLine })
			w.started = false
			if saveErr := w.save(st); saveErr != nil {
				return st, saveErr
			}
			return st, err
		}
	}
	if err := w.creel.Release(name); err != nil {
		return held(err)
	}
	st = ResetForFreshLaunch(st, st.Strand, w.clock.Now())
	w.cursor = 0
	w.colorPending = !colorTyped(st)
	return st, w.save(st)
}

// colorTyped reports whether st records the strand's color as typed for its current strand and launch.
func colorTyped(st State) bool {
	return st.ColorStrand == st.Strand && st.ColorLaunchedAt.Equal(st.LaunchedAt)
}

// toIdle returns st to idle, persisting the cursor read through and forgetting the newest event,
// so only a turn end read after the return can qualify for the next injection.
// It releases the creel hold; a failed release is recorded and does not keep the phase.
func (w *Watcher) toIdle(st State, abortReason string) error {
	st.Phase = PhaseIdle
	st.PhaseInjected = false
	st.Pending, st.RequestID, st.RequestReadAt = nil, 0, time.Time{}
	st.PendingHandoff, st.PendingResume = "", ""
	st.PointerMarkSince = time.Time{}
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry, st.ReloadSkipsSkills = ReloadStepSkills, time.Time{}, nil, false
	st.Stuck, st.StuckByCreel = "", false
	st.LastInjectionOffset = w.cursor
	if abortReason != "" {
		st.LastAbortReason = abortReason
	}
	w.newest, w.seen = nil, phaseEvents{}
	if err := ClearResumeMark(w.paths); err != nil {
		return err
	}
	if err := w.releaseHold(&st); err != nil {
		return err
	}
	return w.save(st)
}

// enter persists a new non-idle phase before its first step is sent.
func (w *Watcher) enter(st State, phase Phase, now time.Time) (State, error) {
	st.Phase = phase
	st.PhaseStrand = st.Strand
	st.PhaseEnteredAt = now
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	st.Pending, st.RequestID, st.RequestReadAt = nil, 0, time.Time{}
	st.Stuck, st.StuckByCreel = "", false
	w.seen = phaseEvents{}
	return st, w.save(st)
}

// markStuck records reason as the stall creel reported for the pending inject, so `status` can show it.
// It saves and logs the stall only when it changes, so a long wait logs once.
func (w *Watcher) markStuck(st *State, reason string) error {
	if st.Stuck == reason && !st.StuckByCreel {
		return nil
	}
	logger.Warn("orch: cycle step stalled", "phase", string(st.Phase), "reason", reason, "strandGUID", st.Strand)
	st.Stuck, st.StuckByCreel = reason, false
	return w.save(*st)
}

// logTyped logs that step of the cycle was typed into the session.
func (w *Watcher) logTyped(st State, step string) {
	logger.Info("orch: injection typed", "strandGUID", st.Strand, "phase", string(st.Phase), "step", step, "sincePhase", w.sincePhase(st, w.clock.Now()))
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

// storeReading records reading in st, taken through turnEnd; an unknown reading is stored as zero tokens.
func storeReading(st *State, reading shuttleengine.ContextReading, turnEnd shuttleengine.Event) {
	st.ReadingTurnEnd = &turnEnd
	st.LastContextTokens, st.LastContextKnown = reading.Tokens, reading.Known
	if !reading.Known {
		st.LastContextTokens = 0
	}
}

func (w *Watcher) tickIdle(st State, now time.Time) error {
	if !w.compactedAt.IsZero() {
		return w.startAutoReload(st, now)
	}
	req, requested, err := CycleRequested(w.paths)
	if err != nil {
		return err
	}
	if requested {
		if age := w.unpausedSince(req.RequestedAt, now); age >= w.cfg.HandoffTimeout() {
			// The time is on disk, so this covers a marker a dead watcher left.
			// Only the request this tick read is removed: one written since stays pending and the cycle start below finds it changed.
			removed, err := RemoveCycleRequestIf(w.paths, req)
			if err != nil {
				return err
			}
			if removed {
				logger.Warn("orch: stale cycle request removed", "mode", req.Mode, "age", age.String(), "strandGUID", st.Strand)
			}
			req, requested = CycleRequest{}, false
		}
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
		// The operator asked for this cycle at a moment of their choosing; creel's idle check alone guards the pane.
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
	if trigger != TriggerRequested {
		// The transcript can change without a turn end, so the saved reading may be stale: fire only on a fresh one.
		reading, err := w.session.ContextTokens(st.Strand, *w.newest)
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
	body, err := w.renderHandoffRequest(trigger, path)
	if err != nil {
		return err
	}
	held, err := w.renewHold(&st)
	if err != nil || !held {
		return err
	}
	st.PendingHandoff = path
	st.CycleTrigger = trigger
	st.CycleMode, st.CycleRequestedAt = mode, requestedAt
	// The phase fields below are saved with the request marker's removal,
	// so a withdrawn or replaced request is never consumed.
	st.Phase = PhaseHandoffRequested
	st.PhaseStrand = st.Strand
	st.PhaseEnteredAt = now
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	st.Pending, st.RequestID, st.RequestReadAt = nil, 0, time.Time{}
	st.Stuck, st.StuckByCreel = "", false
	if err := saveCycleStart(w.paths, st, req, requested); err != nil {
		if !errors.Is(err, errMarkerChanged) {
			return err
		}
		logger.Info("orch: requested cycle not started, the request changed", "strandGUID", st.Strand)
		return w.releaseHold(&st)
	}
	w.seen = phaseEvents{}
	w.newest = nil
	return w.sendStep(&st, true, CreelStep{}, body, now)
}

// unpausedSince returns the time from `from` to `now`, less the time a hub pause held the watcher's cycle machine.
// A watcher with no pause record, or one whose record cannot be read, discounts nothing.
func (w *Watcher) unpausedSince(from, now time.Time) time.Duration {
	if w.pause == nil {
		return now.Sub(from)
	}
	record, err := pauserecord.Read(w.pause.RecordPath)
	if err != nil {
		return now.Sub(from)
	}
	return pauserecord.AwakeSince(from, now, record.CoverageOf("", now).Intervals)
}

// renderHandoffRequest renders the note template file, then returns the note request for trigger.
// A soft cycle gets the soft stencil; any other trigger gets the plain one.
func (w *Watcher) renderHandoffRequest(trigger, handoffPath string) (string, error) {
	if err := RenderNoteTemplateFile(w.stencilsDir, w.paths.NoteTemplatePath); err != nil {
		return "", err
	}
	if trigger == TriggerSoft {
		return RenderSoftHandoffInstruction(w.stencilsDir, handoffPath, w.paths.NoteTemplatePath)
	}
	return RenderHandoffInstruction(w.stencilsDir, handoffPath, w.paths.NoteTemplatePath)
}

// notePassed reports whether the note gate passes.
// The gate passes when the newest session event read since the phase began is a turn end,
// and the requested note file exists and is not empty.
func (w *Watcher) notePassed(st State) (bool, error) {
	if w.newest == nil || !isTurnEnd(*w.newest) {
		return false, nil
	}
	return handoffWritten(st.PendingHandoff)
}

// tickHandoff passes the note gate once the orch ends a turn with the requested note written, and times the phase out.
// A request not yet posted, because a restart came before the post was persisted, is posted again.
func (w *Watcher) tickHandoff(st State, now time.Time) error {
	if w.seen.ask {
		return w.toIdle(st, "session asked a question during the handoff")
	}
	readAt, err := w.requestReadAt(&st)
	if err != nil {
		return err
	}
	// The clock starts when the orch read the request, so a request creel held behind a long turn still gets the full time.
	if !readAt.IsZero() && w.unpausedSince(readAt, now) >= w.cfg.HandoffTimeout() {
		return w.toIdle(st, "handoff timed out")
	}
	passed, err := w.notePassed(st)
	if err != nil {
		return err
	}
	if passed {
		// The new phase's first tick runs in this call, so entering the phase and its first tick happen in one step.
		start := w.startClearing
		if st.CycleMode == CycleCompact {
			start = w.startCompacting
		}
		if err := start(st, now); err != nil {
			return err
		}
		w.newest = nil
		if st, err = LoadState(w.paths); err != nil {
			return err
		}
		switch st.Phase {
		case PhaseClearing:
			return w.tickClearing(st, now)
		case PhaseCompacting:
			return w.tickCompacting(st, now)
		}
		return nil
	}
	if st.Pending != nil || st.RequestID != 0 {
		return nil
	}
	body, err := w.renderHandoffRequest(st.CycleTrigger, st.PendingHandoff)
	if err != nil {
		return err
	}
	return w.sendStep(&st, true, CreelStep{}, body, now)
}

// requestReadAt returns when the orch read the posted note request.
// It returns zero while the request is unread, when none was posted, or when creel could not be asked.
// The first time creel answers a read it is recorded in State.RequestReadAt, so a restarted watcher keeps it.
// A failed lookup is recorded through creelFailed and answers zero, holding the timeout until creel answers.
func (w *Watcher) requestReadAt(st *State) (time.Time, error) {
	if !st.RequestReadAt.IsZero() || st.RequestID == 0 {
		return st.RequestReadAt, nil
	}
	readAt, err := w.creel.MailReadAt(st.RequestID)
	if err != nil {
		return time.Time{}, w.creelFailed(st, err)
	}
	if readAt.IsZero() {
		return readAt, nil
	}
	logger.Info("orch: note request read by the orch", "strandGUID", st.Strand, "id", st.RequestID, "readAt", readAt)
	st.RequestReadAt = readAt
	return readAt, w.save(*st)
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

// startClearing renders the role file and the resume prompt, then persists the clearing phase.
// Rendering first means what the cleared session needs is known good before /clear is injected.
// The `/clear` itself is sent by the next tickClearing.
func (w *Watcher) startClearing(st State, now time.Time) error {
	if err := w.renderRoleFile(); err != nil {
		return w.toIdle(st, err.Error())
	}
	resume, err := RenderResumePrompt(w.stencilsDir, w.paths.RolePath, st.PendingHandoff)
	if err != nil {
		return w.toIdle(st, fmt.Sprintf("resume stencil %s failed to render: %v", resumeStencilName, err))
	}
	st.LastHandoff = st.PendingHandoff
	st.PendingResume = WithPresence(w.paths, resume)
	_, err = w.enter(st, PhaseClearing, now)
	return err
}

// tickClearing injects `/clear` and waits for creel to type it, however long the orch stays busy;
// `lym orch status` shows the stall creel reports meanwhile.
// An injected or unconfirmed `/clear` moves on to resuming, and a withdrawn one returns to idle.
func (w *Watcher) tickClearing(st State, now time.Time) error {
	if st.Pending == nil {
		return w.sendStep(&st, false, CreelStep{Command: commandClear}, "", now)
	}
	outcome, typed, err := w.stepOutcome(&st)
	if err != nil {
		return err
	}
	if outcome.State == CreelWithdrawnState {
		return w.toIdle(st, "the clear was withdrawn")
	}
	if !typed {
		return nil
	}
	st.CycleCount++
	if err := w.removeSatisfiedRequest(CycleClear, now); err != nil {
		return err
	}
	st.PointerMarkSince = st.CompactionBaseline
	return w.startReload(st, now, false)
}

// removeSatisfiedRequest removes a pending cycle request of mode made no later than effectAt.
// effectAt is the time the running cycle's action took effect.
// A cycle start clears the request pending at that moment, so a request found during the cycle was made after it started;
// the cycle's action did what that request asks, so a second cycle for it is pointless.
// The removal reaches only a request made after the cycle started and before its action took effect.
// The request must also be of the same mode as the action.
// The request time is CLI-written and the boundary time is the transcript's, both wall clock on one host.
func (w *Watcher) removeSatisfiedRequest(mode string, effectAt time.Time) error {
	req, pending, err := CycleRequested(w.paths)
	if err != nil || !pending || req.Mode != mode || req.RequestedAt.After(effectAt) {
		return err
	}
	removed, err := RemoveCycleRequestIf(w.paths, req)
	if err != nil || !removed {
		return err
	}
	logger.Info("orch: cycle request satisfied by the running cycle and removed", "mode", req.Mode, "requestedAt", req.RequestedAt)
	return nil
}

// startAutoReload reloads the plugins and the role after an auto-compaction read at a turn end, once the creel hold is taken.
// The baseline moves to the boundary when the phase is entered, so no boundary reloads twice.
// A role-file render failure returns to idle with the reason.
// It also forgets the boundary, so the failure is reported once and the next turn end retries it.
// A refused hold keeps the boundary and tries again on the next tick.
func (w *Watcher) startAutoReload(st State, now time.Time) error {
	held, err := w.renewHold(&st)
	if err != nil || !held {
		return err
	}
	if err := w.renderRoleFile(); err != nil {
		w.compactedAt = time.Time{}
		return w.toIdle(st, err.Error())
	}
	pointer, err := RenderReloadPrompt(w.stencilsDir, w.paths.RolePath)
	if err != nil {
		return err
	}
	st.PointerMarkSince = st.CompactionBaseline
	st.CompactionBaseline = w.compactedAt
	st.PendingResume = WithPresence(w.paths, pointer)
	w.compactedAt = time.Time{}
	return w.startReload(st, now, true)
}

// startReload enters the resuming phase at its color step and queues it.
// skipsSkills is true for a reload after a compaction, which keeps the session's skills;
// it is false after `/clear`, which loses them.
// The caller must have set st.PendingResume to the pointer line.
func (w *Watcher) startReload(st State, now time.Time, skipsSkills bool) error {
	st.ReloadStep, st.ReloadTypedAt, st.ReloadRetry, st.ReloadSkipsSkills = ReloadStepColor, time.Time{}, nil, skipsSkills
	st, err := w.enter(st, PhaseResuming, now)
	if err != nil {
		return err
	}
	w.newest = nil
	return w.typeReloadStep(st, now)
}

// reloadStep returns the step st is in, normalised.
// A reload that skips the skills has no skills step, and an empty skill list has none either.
// A retry step with nothing to retry has no retry, and any value that is none of these is the pointer step.
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

// typeReloadStep queues the current step as one creel inject.
// The step is the color, the plugins reload, the skills load, the retry load or the pointer.
// The first queueing persists the step's time and events offset first,
// so a turn end read before it never confirms the step.
// A requeue after a restart or an expiry keeps both,
// so the step's timeout never restarts.
// A strand with no color has no color step to queue and moves straight to the plugins step.
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
	switch step := w.reloadStep(st); step {
	case ReloadStepColor:
		color, err := w.session.StrandColor(st.Strand)
		if err != nil {
			logger.Warn("orch: reading the strand color failed; the reload goes on without it", "strandGUID", st.Strand, "cause", err)
			color = ""
		}
		if color == "" {
			return w.finishColorStep(st, now)
		}
		return w.sendStep(&st, false, CreelStep{Command: commandColor, Argument: color}, "", now)
	case ReloadStepPlugins:
		return w.sendStep(&st, false, CreelStep{Command: commandReloadPlugins}, "", now)
	case ReloadStepSkills:
		return w.sendStep(&st, false, CreelStep{Command: commandLoadSkills, Argument: strings.Join(w.skills, " ")}, "", now)
	case ReloadStepRetry:
		return w.sendStep(&st, false, CreelStep{Command: commandLoadSkills, Argument: strings.Join(st.ReloadRetry, " ")}, "", now)
	}
	if mark, delivered := w.hookDelivered(st); delivered {
		logger.Info("orch: pointer delivered by the session-start hook", "strandGUID", st.Strand, "markAt", mark.At)
		return w.toIdle(st, "")
	}
	return w.sendStep(&st, false, CreelStep{Text: st.PendingResume}, "", now)
}

// finishColorStep moves the reload from its color step to the plugins step.
// It records the strand's color as typed for the session.
func (w *Watcher) finishColorStep(st State, now time.Time) error {
	st.ColorStrand, st.ColorLaunchedAt = st.Strand, st.LaunchedAt
	return w.advanceReload(st, now, ReloadStepPlugins, nil)
}

// hookDelivered reports the delivery mark and whether the session-start hook already delivered the pending pointer.
// A mark counts when it is dated at or after State.PointerMarkSince and its text is the pending pointer.
// An unreadable mark counts as no delivery, so the pointer is typed.
func (w *Watcher) hookDelivered(st State) (ResumeMark, bool) {
	mark, found, err := ReadResumeMark(w.paths)
	if err != nil {
		logger.Warn("orch: resume mark unreadable; the pointer is typed", "strandGUID", st.Strand, "cause", err)
		return ResumeMark{}, false
	}
	return mark, found && !mark.At.Before(st.PointerMarkSince) && mark.Text == st.PendingResume
}

// advanceReload persists the move to step, with retry as the skills its retry step loads.
// It takes the events offset at the cursor and goes on to queue the step.
// The move ends the stall reason of the step it leaves.
func (w *Watcher) advanceReload(st State, now time.Time, step int, retry []string) error {
	st.ReloadStep, st.ReloadRetry = step, retry
	st.ReloadTypedAt = time.Time{}
	st.PhaseEventsOffset = w.cursor
	st.PhaseInjected = false
	st.Pending = nil
	if !st.StuckByCreel {
		st.Stuck = ""
	}
	w.seen = phaseEvents{}
	if err := w.save(st); err != nil {
		return err
	}
	return w.tickResuming(st, now)
}

func (w *Watcher) skipSkill(st State, skill, cause string) {
	logger.Warn("orch: skill skipped", "skill", skill, "cause", cause, "strandGUID", st.Strand)
}

// settleSkillTurn classifies the load turn of skills that turnEnd ended and logs what is skipped.
// It moves to the retry step when the skills step left skills missing, and to the pointer otherwise.
// An unreadable turn is confirmed unverified, with no retry.
func (w *Watcher) settleSkillTurn(st State, now time.Time, step int, skills []string, turnEnd shuttleengine.Event) error {
	report, err := w.session.ClassifySkillLoad(st.Strand, turnEnd, skills)
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

// tickResuming walks the reload sequence one step at a time.
// The sequence runs the color step, the plugins step, and the skills step after `/clear`.
// Then come the retry step, when skills were left missing, and the pointer step.
// The color and plugins steps move on once creel typed their inject, injected or unconfirmed.
// A skills or retry step is confirmed by a turn end read after its inject's newest sending record.
// It is then settled from that turn.
// The pointer step ends the phase at such a turn end.
// Every step times out from its first queueing.
// A timed-out color or plugins step is logged as skipped.
// A timed-out skills or retry step skips every skill it loads, and the pointer ends the phase as `resume timed out`.
// A withdrawn inject returns to idle.
func (w *Watcher) tickResuming(st State, now time.Time) error {
	step := w.reloadStep(st)
	skills := w.skills
	if step == ReloadStepRetry {
		skills = st.ReloadRetry
	}
	outcome, typed, err := w.stepOutcome(&st)
	if err != nil {
		return err
	}
	if outcome.State == CreelWithdrawnState {
		return w.toIdle(st, "a reload step was withdrawn")
	}
	if typed {
		switch step {
		case ReloadStepColor:
			w.logTyped(st, reloadStepName(step))
			return w.finishColorStep(st, now)
		case ReloadStepPlugins:
			w.logTyped(st, reloadStepName(step))
			next := ReloadStepSkills
			if st.ReloadSkipsSkills {
				next = ReloadStepPointer
			}
			return w.advanceReload(st, now, next, nil)
		case ReloadStepPointer:
			if turnEnd, ok := w.turnEndAfterSending(outcome.SendingAt); ok {
				reading, err := w.session.ContextTokens(st.Strand, turnEnd)
				if err != nil {
					return err
				}
				storeReading(&st, reading, turnEnd)
				return w.toIdle(st, "")
			}
		default:
			if turnEnd, ok := w.turnEndAfterSending(outcome.SendingAt); ok {
				return w.settleSkillTurn(st, now, step, skills, turnEnd)
			}
		}
	}
	if !st.ReloadTypedAt.IsZero() && now.Sub(st.ReloadTypedAt) >= w.cfg.HandoffTimeout() {
		switch step {
		case ReloadStepColor, ReloadStepPlugins:
			logger.Warn("orch: reload step skipped", "step", reloadStepName(step), "cause", "timeout", "strandGUID", st.Strand)
			next := ReloadStepPlugins
			if step == ReloadStepPlugins {
				next = ReloadStepSkills
				if st.ReloadSkipsSkills {
					next = ReloadStepPointer
				}
			}
			return w.advanceReload(st, now, next, nil)
		case ReloadStepPointer:
			// The persisted reading predates the reload and says nothing about the new context.
			st.LastContextTokens, st.LastContextKnown = 0, false
			return w.toIdle(st, "resume timed out")
		}
		for _, skill := range skills {
			w.skipSkill(st, skill, "timeout")
		}
		return w.advanceReload(st, now, ReloadStepPointer, nil)
	}
	if st.Pending == nil {
		return w.typeReloadStep(st, now)
	}
	return nil
}

// startCompacting renders the focus first, so a stencil failure changes nothing, then persists compacting.
// The caller must have seen the note gate pass, so the note becomes LastHandoff.
// The `/compact` itself is sent by the next tickCompacting.
func (w *Watcher) startCompacting(st State, now time.Time) error {
	if _, err := RenderCompactFocus(w.stencilsDir); err != nil {
		return err
	}
	st.LastHandoff = st.PendingHandoff
	if _, err := w.enter(st, PhaseCompacting, now); err != nil {
		return err
	}
	w.newest = nil
	return nil
}

// reloadAfterCompaction renders the role file and the resume pointer naming the cycle's note.
// It then enters the reload sequence.
// A stencil failure returns to idle with the reason.
func (w *Watcher) reloadAfterCompaction(st State, now time.Time) error {
	if err := w.renderRoleFile(); err != nil {
		return w.toIdle(st, err.Error())
	}
	resume, err := RenderResumePrompt(w.stencilsDir, w.paths.RolePath, st.LastHandoff)
	if err != nil {
		return w.toIdle(st, fmt.Sprintf("resume stencil %s failed to render: %v", resumeStencilName, err))
	}
	st.PendingResume = WithPresence(w.paths, resume)
	return w.startReload(st, now, true)
}

// withdrawQueuedCompact withdraws the `/compact` inject still queued after the session compacted on its own.
// This keeps the session from compacting a second time.
// The inject counts as still queued when its step is pending and not yet typed.
// That holds when creel never answered its send, or when creel reads it queued or sending.
// The withdraw is receiver-wide: it also removes any other queued watcher item to the orch.
// Creel's withdraw leaves an inject a delivery attempt holds,
// and a busy attempt puts it back in the queue,
// so the outcome is read again after the withdraw.
// One still queued or sending keeps the step pending and answers errCompactInFlight, so the next call withdraws again.
// Past the handoff timeout from the phase entry, such a step is dropped with a warning.
// A resume line the withdraw removed has its pause-record mark cleared;
// a failed clear drops a withdrawn step, keeps the line in State.ResumeLineWithdrawn, and fails the call,
// and a later call clears the mark although no withdraw runs.
// A creel failure is recorded through creelFailed.
// It is answered with errCompactWithdrawHeld, which leaves the phase as it was for the next call to retry.
// A failure of the read after the withdraw also keeps a withdrawn resume line in State.ResumeLineWithdrawn.
func (w *Watcher) withdrawQueuedCompact(st *State) error {
	if st.Pending == nil || st.PhaseInjected {
		return w.clearWithdrawnResumeLine(st)
	}
	queued := st.Pending.ID == 0
	if !queued {
		outcome, err := w.creel.InjectOutcome(st.Pending.ID)
		if err != nil {
			return w.holdOnCreelFailure(st, err)
		}
		queued = outcome.State == CreelQueued || outcome.State == CreelSending
	}
	if !queued {
		return w.clearWithdrawnResumeLine(st)
	}
	name, err := w.orchName(*st)
	if err != nil {
		return w.holdOnCreelFailure(st, err)
	}
	withdrawn, err := w.creel.Withdraw(name)
	if err != nil {
		return w.holdOnCreelFailure(st, err)
	}
	inFlight := false
	if st.Pending.ID != 0 {
		outcome, err := w.creel.InjectOutcome(st.Pending.ID)
		if err != nil {
			st.ResumeLineWithdrawn = st.ResumeLineWithdrawn || slices.ContainsFunc(withdrawn, func(message CreelWithdrawn) bool { return message.Subject == pauserecord.ResumeLine })
			if saveErr := w.save(*st); saveErr != nil {
				return saveErr
			}
			return w.holdOnCreelFailure(st, err)
		}
		inFlight = outcome.State == CreelQueued || outcome.State == CreelSending
	}
	if inFlight && w.clock.Now().Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		logger.Warn("orch: the queued compact stayed in a delivery attempt past the handoff timeout; the reload goes on", "strandGUID", st.Strand, "id", st.Pending.ID)
		inFlight = false
	}
	if !inFlight {
		st.Pending = nil
	}
	if w.pause != nil {
		if st.ResumeLineWithdrawn {
			withdrawn = append(withdrawn, CreelWithdrawn{Subject: pauserecord.ResumeLine})
		}
		if err := ClearResumeNoticed(w.pause.RecordPath, st.Strand, withdrawn, w.clock.Now()); err != nil {
			st.ResumeLineWithdrawn = slices.ContainsFunc(withdrawn, func(message CreelWithdrawn) bool { return message.Subject == pauserecord.ResumeLine })
			if saveErr := w.save(*st); saveErr != nil {
				return saveErr
			}
			return err
		}
		st.ResumeLineWithdrawn = false
	}
	if inFlight {
		logger.Debug("orch: the queued compact is in a delivery attempt; withdrawing again at the next tick", "strandGUID", st.Strand, "id", st.Pending.ID)
		return errCompactInFlight
	}
	return nil
}

// clearWithdrawnResumeLine retries the pause-record mark clear of a resume line that an earlier withdraw removed.
// On success it unsets State.ResumeLineWithdrawn.
// It does nothing when no such line is recorded.
func (w *Watcher) clearWithdrawnResumeLine(st *State) error {
	if !st.ResumeLineWithdrawn || w.pause == nil {
		return nil
	}
	if err := ClearResumeNoticed(w.pause.RecordPath, st.Strand, []CreelWithdrawn{{Subject: pauserecord.ResumeLine}}, w.clock.Now()); err != nil {
		return err
	}
	st.ResumeLineWithdrawn = false
	return nil
}

// holdOnCreelFailure records a failed creel call through creelFailed and answers errCompactWithdrawHeld;
// when saving the record fails, it answers that save error instead.
func (w *Watcher) holdOnCreelFailure(st *State, err error) error {
	if saveErr := w.creelFailed(st, err); saveErr != nil {
		return saveErr
	}
	return errCompactWithdrawHeld
}

// tickCompacting searches the transcript from State.ReadingTurnEnd for a compaction boundary on every tick.
// It searches every tick because a compaction ends without a turn end.
// The boundary is searched for rather than read off the newest entry.
// A message answered right after the boundary would hide it there.
// The phase completes on a boundary after the phase was entered, whatever follows it;
// an earlier boundary never completes it.
// Past the handoff timeout it returns to idle.
// It sets LastDeferral, which holds the next automatic trigger for the soft idle.
// The `/compact` is injected once: an expired one is queued again,
// and an unconfirmed one keeps the search going for the boundary.
func (w *Watcher) tickCompacting(st State, now time.Time) error {
	var boundary shuttleengine.CompactionBoundary
	qualifying := false
	if st.ReadingTurnEnd != nil {
		var err error
		if boundary, qualifying, err = w.session.CompactedSince(st.Strand, *st.ReadingTurnEnd, st.PhaseEnteredAt); err != nil {
			return err
		}
	}
	storeCurrentReading := func() error {
		if st.ReadingTurnEnd == nil {
			return nil
		}
		reading, err := w.session.ContextTokens(st.Strand, *st.ReadingTurnEnd)
		if err != nil {
			return err
		}
		storeReading(&st, reading, *st.ReadingTurnEnd)
		return nil
	}

	if qualifying {
		logger.Info("orch: compaction boundary found", "strandGUID", st.Strand, "boundaryAt", boundary.At)
		if err := w.withdrawQueuedCompact(&st); err != nil {
			if errors.Is(err, errCompactWithdrawHeld) || errors.Is(err, errCompactInFlight) {
				return nil
			}
			return err
		}
		if err := storeCurrentReading(); err != nil {
			return err
		}
		st.CycleCount++
		st.PointerMarkSince = st.CompactionBaseline
		st.CompactionBaseline = boundary.At
		if err := w.removeSatisfiedRequest(CycleCompact, boundary.At); err != nil {
			return err
		}
		return w.reloadAfterCompaction(st, now)
	}
	outcome, typed, err := w.stepOutcome(&st)
	if err != nil {
		return err
	}
	if outcome.State == CreelWithdrawnState {
		return w.toIdle(st, "the compact was withdrawn")
	}
	if typed {
		st.Pending, st.PhaseInjected = nil, true
		if err := w.save(st); err != nil {
			return err
		}
	}
	if now.Sub(st.PhaseEnteredAt) >= w.cfg.HandoffTimeout() {
		if err := storeCurrentReading(); err != nil {
			return err
		}
		st.LastDeferral = now
		return w.toIdle(st, "compaction timed out")
	}
	if st.Pending != nil || st.PhaseInjected {
		return nil
	}
	focus, err := RenderCompactFocus(w.stencilsDir)
	if err != nil {
		return err
	}
	return w.sendStep(&st, false, CreelStep{Command: commandCompact, Argument: focus}, "", now)
}
