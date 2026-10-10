// sessionread.go declares the files-only reading of a run's session state: it reads a run's signals and facts from disk and folds them once.
// Like ReadAgentActivity it reads files only, so any process may call it, and it writes nothing.

package shuttleengine

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// SessionCauseUnsupported is the cause of an unknown state read from an engine that cannot parse session signals.
const SessionCauseUnsupported = "unsupported"

// RunSessionState is the session state read for one run.
type RunSessionState struct {
	// StrandName is the run's strand name, the strand guid for a record that predates the name.
	StrandName string
	// StrandGUID is the run's strand guid.
	StrandGUID string
	// State is the session's current state.
	State SessionState
	// History is every state the reading passed through, in order.
	History []SessionState
	// SessionStartAt is the time of the newest session-start signal read; zero when none.
	SessionStartAt time.Time
}

// ReadSessionStates returns the session state of every run under the run-directory root of anchorPath whose record reads running.
// The process waiting on a run need not be alive, so a run whose Wait died still shows.
// A directory that is not a run directory, a run with no record and an undecodable record are skipped, and an undecodable record is logged.
// now is the reading's time.
// An absent root returns no readings and no error.
func ReadSessionStates(cfg Config, anchorPath string, engine Engine, now time.Time) ([]RunSessionState, error) {
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var readings []RunSessionState
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil {
			logger.Warn("shuttle: skipping an unreadable run record", "runDir", runDir, "cause", err)
			continue
		}
		if !found || rs.Outcome != runOutcomeRunning {
			continue
		}
		readings = append(readings, readRunSessionState(rs, engine, now))
	}
	return readings, nil
}

// readRunSessionState folds the signals past rs's prompt offset, with the facts read for them, into one state.
// An engine without a SessionSignalParser reads unknown with cause unsupported.
func readRunSessionState(rs RunState, engine Engine, now time.Time) RunSessionState {
	reading := RunSessionState{StrandName: rs.StrandName, StrandGUID: rs.StrandGUID}
	if reading.StrandName == "" {
		reading.StrandName = rs.StrandGUID
	}

	parser, ok := engine.(SessionSignalParser)
	if !ok {
		reading.State = SessionState{Name: SessionUnknown, Cause: SessionCauseUnsupported, Since: now}
		reading.History = []SessionState{reading.State}
		return reading
	}

	var signals []SessionSignal
	eventsUnreadable := false
	if data, err := os.ReadFile(rs.EventsPath); err != nil {
		eventsUnreadable = true
	} else {
		signals, _ = parser.ParseSessionSignals(data[max(0, min(rs.PromptOffset, int64(len(data)))):])
	}

	facts := readSessionFacts(rs, engine, signals, now)
	facts.EventsUnreadable = eventsUnreadable
	var fold SessionFold
	fold.Fold(signals, facts)
	reading.State, reading.History, reading.SessionStartAt = fold.State(), fold.History(), fold.SessionStartAt()
	return reading
}

// readSessionFacts reads the facts of a fold over signals that the signals themselves do not carry, at the time now.
// Interactive and the output files come from the record, the stat taken once per call, and a run that declares no output file has none that exist.
// Liveness comes from the engine's SessionProber for the session id of the newest signal that names one, else the record's;
// the API-error marker from the engine's ActivityReader over the newest turn end among signals, and the interrupt marker from the SessionProber over the newest turn start.
// A fact its capability cannot give stays at its zero value, which reads as unproven or absent.
// EventsUnreadable is left for the caller, which knows how it read the events file.
func readSessionFacts(rs RunState, engine Engine, signals []SessionSignal, now time.Time) SessionFacts {
	facts := SessionFacts{
		Interactive:  rs.Interactive,
		OutputsExist: len(rs.OutputFiles) > 0 && allOutputFilesExist(rs.OutputFiles),
		ReadAt:       now,
	}
	prober, _ := engine.(SessionProber)
	reader, _ := engine.(ActivityReader)

	sessionID := ""
	var newestTurnEnd, newestTurnStart *SessionSignal
	for i := len(signals) - 1; i >= 0; i-- {
		signal := &signals[i]
		if sessionID == "" {
			sessionID = signal.SessionID
		}
		switch signal.Kind {
		case SessionSignalTurnEnd, SessionSignalAPIErrorTurnEnd:
			if newestTurnEnd == nil {
				newestTurnEnd = signal
			}
		case SessionSignalTurnStart:
			if newestTurnStart == nil {
				newestTurnStart = signal
			}
		}
	}

	if sessionID == "" {
		sessionID = rs.SessionID
	}
	if prober != nil && sessionID != "" {
		facts.Liveness = prober.ProcessLiveness(sessionID)
	}
	if reader != nil && newestTurnEnd != nil {
		activity := reader.TurnEndActivity(Event{Raw: newestTurnEnd.Raw})
		facts.APIError, facts.APIErrorText = activity.APIError, activity.APIErrorText
	}
	if prober != nil && newestTurnStart != nil {
		facts.InterruptAt, facts.Interrupted = prober.TurnStartInterrupt(*newestTurnStart)
	}
	return facts
}
