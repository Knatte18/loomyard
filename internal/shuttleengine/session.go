// session.go holds Runner's session-cycling surface: reading a live run's events from a caller-held offset, and the SessionCycler-backed operations (context usage, idle probe, clear, compact) an orchestrator watcher needs.
// All of it is provider-invariant; provider specifics stay behind Engine and SessionCycler.

package shuttleengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// sessionCycler returns the engine's SessionCycler capability, or an error naming it when the engine lacks it.
func (r *Runner) sessionCycler() (SessionCycler, error) {
	cycler, ok := r.engine.(SessionCycler)
	if !ok {
		return nil, fmt.Errorf("shuttle: the engine does not implement the SessionCycler capability (ContextTokens, CompactedSince, IdleSession, ClearSessionSequence, CompactSessionSequence), so it cannot cycle a session")
	}
	return cycler, nil
}

// skillLoader returns the engine's SkillLoader capability, or an error naming it when the engine lacks it.
func (r *Runner) skillLoader() (SkillLoader, error) {
	loader, ok := r.engine.(SkillLoader)
	if !ok {
		return nil, fmt.Errorf("shuttle: the engine does not implement the SkillLoader capability (SkillLoadSequence, SkillUnknown, DefaultSkillLoadTimeout), so it cannot load a skill")
	}
	return loader, nil
}

// LoadSkill plays the provider's load choreography for skill into the live pane of the run identified by guid.
// Like ClearSession it has no idle precondition of its own, since the caller has already probed idleness.
func (r *Runner) LoadSkill(guid, skill string) error {
	if r.toldErr != nil {
		return r.toldErr
	}
	loader, err := r.skillLoader()
	if err != nil {
		return err
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return err
	}
	return playInputs(r.reed, guid, loader.SkillLoadSequence(skill))
}

// SkillUnknown captures the pane of the run identified by guid and reports whether the provider shows skill as unknown.
func (r *Runner) SkillUnknown(guid, skill string) (bool, error) {
	if r.toldErr != nil {
		return false, r.toldErr
	}
	loader, err := r.skillLoader()
	if err != nil {
		return false, err
	}
	if _, _, err := FindRun(r.cfg, r.anchorPath, guid); err != nil {
		return false, fmt.Errorf("shuttle: %q is not a shuttle strand: %w", guid, err)
	}
	if err := requireLiveStrand(r.reed, guid); err != nil {
		return false, err
	}
	capture, err := r.reed.CapturePane(guid)
	if err != nil {
		return false, fmt.Errorf("shuttle: capture strand %q's pane to check skill %q: %w", guid, skill, err)
	}
	return loader.SkillUnknown(capture, skill), nil
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

// CompactedSince returns the timestamp of the newest compaction boundary after since in the transcript turnEnd names, via the engine's SessionCycler.
// found is false when there is none or the transcript could not be read.
func (r *Runner) CompactedSince(turnEnd Event, since time.Time) (time.Time, bool, error) {
	if r.toldErr != nil {
		return time.Time{}, false, r.toldErr
	}
	cycler, err := r.sessionCycler()
	if err != nil {
		return time.Time{}, false, err
	}
	at, found := cycler.CompactedSince(turnEnd, since)
	return at, found, nil
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
