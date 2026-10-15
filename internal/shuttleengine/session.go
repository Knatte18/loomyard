// session.go holds Runner's session-cycling surface: reading a live run's events from a caller-held offset, and the SessionCycler-backed operations (context usage, idle probe, clear, compact) an orchestrator watcher needs.
// All of it is provider-invariant; provider specifics stay behind Engine and SessionCycler.

package shuttleengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// sessionCycler returns the engine's SessionCycler capability, or an error naming it when the engine lacks it.
func (r *Runner) sessionCycler(engine Engine) (SessionCycler, error) {
	cycler, ok := engine.(SessionCycler)
	if !ok {
		return nil, fmt.Errorf("shuttle: the engine does not implement the SessionCycler capability (ContextTokens, CompactedSince, IdleSession, ClearSessionSequence, CompactSessionSequence, ReloadPluginsSequence), so it cannot cycle a session")
	}
	return cycler, nil
}

// skillLoader returns the engine's SkillLoader capability, or an error naming it when the engine lacks it.
func (r *Runner) skillLoader(engine Engine) (SkillLoader, error) {
	loader, ok := engine.(SkillLoader)
	if !ok {
		return nil, fmt.Errorf("shuttle: the engine does not implement the SkillLoader capability (SkillLoadMessage, ClassifySkillLoad, DefaultSkillLoadTimeout), so it cannot load a skill")
	}
	return loader, nil
}

// LoadSkills types the provider's one-turn load message for skills into the live pane of the run identified by guid, through the verified send path.
// Like every verified send it waits for an idle session first and fails with ErrSessionBusy if it stays busy.
func (r *Runner) LoadSkills(guid string, skills []string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	state, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	engine, _, err := r.engineFor(state)
	if err != nil {
		return err
	}
	loader, err := r.skillLoader(engine)
	if err != nil {
		return err
	}
	sc, err := r.newSendContext(state, runDir)
	if err != nil {
		return err
	}
	return sendVerified(sc, loader.SkillLoadMessage(skills))
}

// ClassifySkillLoad returns the engine's classification of the load turn of skills that turnEnd ended.
func (r *Runner) ClassifySkillLoad(turnEnd Event, skills []string) (SkillLoadReport, error) {
	if r.toldErr != nil {
		return SkillLoadReport{}, r.toldErr
	}
	engine, _, err := r.engineFor(RunState{})
	if err != nil {
		return SkillLoadReport{}, err
	}
	loader, err := r.skillLoader(engine)
	if err != nil {
		return SkillLoadReport{}, err
	}
	return loader.ClassifySkillLoad(turnEnd, skills), nil
}

// ReadEvents returns the events of the run identified by guid that lie past byte offset, and the offset to resume from.
// A partial trailing line stays unconsumed, a parse error returns the original offset,
// and an absent events file returns no events and the original offset, matching pollEventsTick.
func (r *Runner) ReadEvents(guid string, offset int64) ([]Event, int64, error) {
	if r.toldErr != nil {
		return nil, offset, r.toldErr
	}
	state, _, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return nil, offset, fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	engine, _, err := r.engineFor(state)
	if err != nil {
		return nil, offset, err
	}
	data, newOffset, err := readEventsFrom(state.EventsPath, offset)
	if err != nil {
		return nil, offset, err
	}
	if len(data) == 0 {
		return nil, offset, nil
	}
	events, err := engine.ParseEvents(data)
	if err != nil {
		return nil, offset, err
	}
	return events, newOffset, nil
}

// SessionState returns the session state of the run identified by guid, read from files at the runner's clock.
func (r *Runner) SessionState(guid string) (RunSessionState, error) {
	if r.toldErr != nil {
		return RunSessionState{}, r.toldErr
	}
	rs, _, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return RunSessionState{}, fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	engine, _, err := r.engineFor(rs)
	if err != nil {
		return RunSessionState{}, err
	}
	return readRunSessionState(rs, engine, r.clock.Now()), nil
}

// SessionState returns the session state of this run, the same reading Runner.SessionState gives for its strand guid.
// It reads files only and writes nothing, so it is safe to call while the run's Wait runs on another goroutine.
func (run *Run) SessionState() (RunSessionState, error) {
	return run.runner.SessionState(run.StrandGUID())
}

// ContextTokens returns the provider's context usage as of turnEnd, via the engine's SessionCycler.
// A reading with Known false means usage could not be read.
func (r *Runner) ContextTokens(turnEnd Event) (ContextReading, error) {
	if r.toldErr != nil {
		return ContextReading{}, r.toldErr
	}
	engine, _, err := r.engineFor(RunState{})
	if err != nil {
		return ContextReading{}, err
	}
	cycler, err := r.sessionCycler(engine)
	if err != nil {
		return ContextReading{}, err
	}
	return cycler.ContextTokens(turnEnd), nil
}

// CompactedSince returns the newest compaction boundary after since in the transcript turnEnd names, with the turn ends that follow it, via the engine's SessionCycler.
// found is false when there is none or the transcript could not be read.
func (r *Runner) CompactedSince(turnEnd Event, since time.Time) (CompactionBoundary, bool, error) {
	if r.toldErr != nil {
		return CompactionBoundary{}, false, r.toldErr
	}
	engine, _, err := r.engineFor(RunState{})
	if err != nil {
		return CompactionBoundary{}, false, err
	}
	cycler, err := r.sessionCycler(engine)
	if err != nil {
		return CompactionBoundary{}, false, err
	}
	boundary, found := cycler.CompactedSince(turnEnd, since)
	return boundary, found, nil
}

// SessionIdle reports whether the run identified by guid can take typed input.
// The answer is the readiness reading over the run's hook-derived session state and one pane capture:
// a ready reading is idle and a held one is not idle with its Reason, and the runner keeps one turn-start hold per strand across calls.
// A ready reading beside a pane that does not show the provider idle still answers idle, and logs the disagreement with the pane's last lines at Debug.
// An unknown reading, and a strand with no run record, fall back to the pane probe, which logs the cause at Debug.
// The engine is the one the run's record names, the set's default for a strand with no record.
// TooShort is filled only for an answer that is not idle.
func (r *Runner) SessionIdle(guid string) (IdleProbe, error) {
	if r.toldErr != nil {
		return IdleProbe{}, r.toldErr
	}
	state, runDir, findErr := FindRun(r.cfg, r.anchorPath, guid)
	if findErr != nil {
		state = RunState{}
	}
	engine, _, err := r.engineFor(state)
	if err != nil {
		return IdleProbe{}, err
	}
	cycler, err := r.sessionCycler(engine)
	if err != nil {
		return IdleProbe{}, err
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return IdleProbe{}, err
	}
	capture, err := r.reed.CapturePane(guid)
	if err != nil {
		return IdleProbe{}, fmt.Errorf("shuttle: capture strand %q's pane to probe idleness: %w", guid, err)
	}
	if findErr == nil {
		sc, err := r.newSendContext(state, runDir)
		if err != nil {
			return IdleProbe{}, err
		}
		reading := sessionReadiness(sc, cycler, r.holdFor(guid), capture)
		switch {
		case reading.ready:
			if !cycler.IdleSession(capture) {
				logger.Debug("shuttle: readiness reads ready beside a pane that does not show idle", "strandGUID", guid, "paneTail", paneTail(r.reed, guid))
			}
			if hold := operatorActivityHold(r.reed, guid, &r.activityFailures); hold != "" {
				logger.Debug("shuttle: session idle probe held", "strandGUID", guid, "reason", hold)
				return IdleProbe{Reason: hold}, nil
			}
			return IdleProbe{Idle: true}, nil
		case !reading.unknown:
			logger.Debug("shuttle: session idle probe held", "strandGUID", guid, "reason", reading.reason)
			return IdleProbe{Reason: reading.reason, Draft: reading.draft != "", TooShort: reading.reason == readinessReasonPaneTooShort}, nil
		}
		logger.Debug("shuttle: session readiness unknown, reading the pane", "strandGUID", guid, "cause", reading.cause)
	}
	probe := IdleProbe{Idle: cycler.IdleSession(capture)}
	if !probe.Idle {
		probe.TooShort = cycler.PaneTooShort(capture)
	}
	logger.Debug("shuttle: session idle probe", "strandGUID", guid, "idle", probe.Idle, "tooShort", probe.TooShort)
	return probe, nil
}

// sendSessionSequence sends the sequence that sequenceFor returns for the cycler into the live pane of the run identified by guid, through the guarded send path.
// A non-empty landing makes a session-start line of that source the send's confirmation.
func (r *Runner) sendSessionSequence(guid string, landing SessionStartSource, sequenceFor func(SessionCycler) []PaneInput) error {
	state, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	engine, _, err := r.engineFor(state)
	if err != nil {
		return err
	}
	cycler, err := r.sessionCycler(engine)
	if err != nil {
		return err
	}
	sc, err := r.newSendContext(state, runDir)
	if err != nil {
		return err
	}
	sc.hold = r.holdFor(guid)
	sc.landingStart = landing
	return sendSequence(sc, sequenceFor(cycler))
}

// ClearSession sends the provider's clear-session command into the live pane of the run identified by guid.
// Like every verified send it waits for an idle session first and fails with ErrSessionBusy if the session or its input box stays held, typing nothing.
func (r *Runner) ClearSession(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	return r.sendSessionSequence(guid, SessionStartClear, SessionCycler.ClearSessionSequence)
}

// ReloadPlugins sends the provider's reload-plugins command into the live pane of the run identified by guid.
// Like ClearSession it fails with ErrSessionBusy while the session or its input box is held.
func (r *Runner) ReloadPlugins(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	return r.sendSessionSequence(guid, "", SessionCycler.ReloadPluginsSequence)
}

// TypeColor sends the provider's color command for color into the live pane of the run identified by guid, for a caller that colors the session itself.
// A provider with no color command, or none for color, types nothing.
// Like every verified send it fails with ErrSessionBusy while the session or its input box is held, so the caller may retry.
func (r *Runner) TypeColor(guid string, color segmentcolor.Color) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	state, runDir, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	sc, err := r.newSendContext(state, runDir)
	if err != nil {
		return err
	}
	sc.hold = r.holdFor(guid)
	return typeColor(sc, color)
}

// CompactSession sends the provider's compact-session command, keeping what focus names, into the live pane of the run identified by guid.
// Like ClearSession it fails with ErrSessionBusy while the session or its input box is held.
// It refuses a focus containing a newline, since the focus is sent as one line.
func (r *Runner) CompactSession(guid, focus string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	if strings.ContainsAny(focus, "\r\n") {
		return fmt.Errorf("shuttle: compact focus for strand %q spans several lines; it is typed as one line, so join it into one", guid)
	}
	return r.sendSessionSequence(guid, SessionStartCompact, func(cycler SessionCycler) []PaneInput { return cycler.CompactSessionSequence(focus) })
}
