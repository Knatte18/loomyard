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
func (r *Runner) sessionCycler() (SessionCycler, error) {
	cycler, ok := r.engine.(SessionCycler)
	if !ok {
		return nil, fmt.Errorf("shuttle: the engine does not implement the SessionCycler capability (ContextTokens, CompactedSince, IdleSession, ClearSessionSequence, CompactSessionSequence, ReloadPluginsSequence), so it cannot cycle a session")
	}
	return cycler, nil
}

// skillLoader returns the engine's SkillLoader capability, or an error naming it when the engine lacks it.
func (r *Runner) skillLoader() (SkillLoader, error) {
	loader, ok := r.engine.(SkillLoader)
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
	loader, err := r.skillLoader()
	if err != nil {
		return err
	}
	state, _, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	return sendVerified(r.newSendContext(state), loader.SkillLoadMessage(skills))
}

// ClassifySkillLoad returns the engine's classification of the load turn of skills that turnEnd ended.
func (r *Runner) ClassifySkillLoad(turnEnd Event, skills []string) (SkillLoadReport, error) {
	if r.toldErr != nil {
		return SkillLoadReport{}, r.toldErr
	}
	loader, err := r.skillLoader()
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
	data, newOffset, err := readEventsFrom(state.EventsPath, offset)
	if err != nil {
		return nil, offset, err
	}
	if len(data) == 0 {
		return nil, offset, nil
	}
	events, err := r.engine.ParseEvents(data)
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
	return readRunSessionState(rs, r.engine, r.clock.Now()), nil
}

// ContextTokens returns the provider's context usage as of turnEnd, via the engine's SessionCycler.
// A reading with Known false means usage could not be read.
func (r *Runner) ContextTokens(turnEnd Event) (ContextReading, error) {
	if r.toldErr != nil {
		return ContextReading{}, r.toldErr
	}
	cycler, err := r.sessionCycler()
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
	cycler, err := r.sessionCycler()
	if err != nil {
		return CompactionBoundary{}, false, err
	}
	boundary, found := cycler.CompactedSince(turnEnd, since)
	return boundary, found, nil
}

// SessionIdle probes the live pane of the run identified by guid for the provider idle.
// TooShort is filled only for a pane that is not idle.
func (r *Runner) SessionIdle(guid string) (IdleProbe, error) {
	if r.toldErr != nil {
		return IdleProbe{}, r.toldErr
	}
	cycler, err := r.sessionCycler()
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
	probe := IdleProbe{Idle: cycler.IdleSession(capture)}
	if !probe.Idle {
		probe.TooShort = cycler.PaneTooShort(capture)
	}
	logger.Debug("shuttle: session idle probe", "strandGUID", guid, "idle", probe.Idle, "tooShort", probe.TooShort)
	return probe, nil
}

// ClearSession plays the provider's clear-session key choreography into the live pane of the run identified by guid.
// It skips requireReadyAgentPane on purpose: right after a clear the pane may briefly show no ready marker,
// and the caller has already probed idleness itself.
func (r *Runner) ClearSession(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	cycler, err := r.sessionCycler()
	if err != nil {
		return err
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, cycler.ClearSessionSequence())
}

// ReloadPlugins plays the provider's reload-plugins key choreography into the live pane of the run identified by guid.
// Like ClearSession it skips requireReadyAgentPane, since the caller has already probed idleness itself.
func (r *Runner) ReloadPlugins(guid string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	cycler, err := r.sessionCycler()
	if err != nil {
		return err
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, cycler.ReloadPluginsSequence())
}

// TypeColor plays the provider's color command for color into the live pane of the run identified by guid, for a caller that colors the session itself.
// A provider with no color command, or none for color, types nothing.
// Like start's color step it waits out the command before returning, and an unconfirmed color is only logged.
func (r *Runner) TypeColor(guid string, color segmentcolor.Color) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	state, _, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return typeColor(r.newSendContext(state), color)
}

// CompactSession plays the provider's compact-session key choreography, keeping what focus names, into the live pane of the run identified by guid.
// Like ClearSession it skips requireReadyAgentPane, since the caller has already probed idleness itself.
// It refuses a focus containing a newline, since the focus is typed as one line.
func (r *Runner) CompactSession(guid, focus string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	cycler, err := r.sessionCycler()
	if err != nil {
		return err
	}
	if strings.ContainsAny(focus, "\r\n") {
		return fmt.Errorf("shuttle: compact focus for strand %q spans several lines; it is typed as one line, so join it into one", guid)
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, cycler.CompactSessionSequence(focus))
}
