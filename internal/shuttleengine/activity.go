// activity.go declares the provider-neutral reading of what a worktree's live shuttle runs have been doing:
// when each last wrote anything, and whether its newest turn end is an API error.
// A caller that judges a run quiet, or stalled on an API error, reads it through ReadAgentActivity.
// Like ReadWaitMarker it reads files only, so any process may call it.

package shuttleengine

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ActivityReader is an optional capability beside Engine: the provider's reading of the transcript a turn end names.
// An Engine without it yields activity readings with no transcript part.
type ActivityReader interface {
	// TurnEndActivity returns the activity of the transcript turnEnd names.
	// It degrades to the zero value on every failure and never errors.
	TurnEndActivity(turnEnd Event) TurnEndActivity
}

// TurnEndActivity is a provider-neutral reading of a session's transcript at a turn end.
type TurnEndActivity struct {
	// TranscriptModTime is the transcript's last write time; zero when the transcript cannot be read.
	TranscriptModTime time.Time
	// APIError is true when the transcript's newest main-chain turn end is an API error rather than the agent's own message.
	APIError bool
	// APIErrorText is the error's text; empty unless APIError is true.
	APIErrorText string
}

// AgentActivity is the reading of one live run.
type AgentActivity struct {
	// StrandName is the run's strand name, the strand guid for a record that predates the name.
	StrandName string
	// LastActivity is the newer of the transcript's last write and the events file's last write.
	// It is the run's creation time when neither can be read.
	LastActivity time.Time
	// APIError is true when the run's newest turn end is an API error.
	APIError bool
	// APIErrorText is the error's text; empty unless APIError is true.
	APIErrorText string
}

// ReadAgentActivity returns one reading per live run under the run-directory root of anchorPath.
// A run is live when its record reads running and the pid of the process that waits on it is alive.
// A directory that is not a run directory, a run with no record, an undecodable record and a run that is not live are skipped,
// and an undecodable record is logged so one torn file cannot hide another run.
// A run's events past its prompt offset are parsed through engine, and the newest turn end among them is handed to engine's ActivityReader;
// a run with no turn end yet, or an engine without the capability, is judged by its events file alone.
// An absent root returns no readings and no error.
func ReadAgentActivity(cfg Config, anchorPath string, engine Engine) ([]AgentActivity, error) {
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reader, _ := engine.(ActivityReader)
	var readings []AgentActivity
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
		if !found || !runIsLive(rs) {
			continue
		}
		readings = append(readings, readRunActivity(rs, engine, reader))
	}
	return readings, nil
}

// runIsLive reports whether rs names a run still waited on: it reads running and its recorded pid is alive.
func runIsLive(rs RunState) bool {
	return rs.Outcome == runOutcomeRunning && rs.PID > 0 && isAlive(rs.PID)
}

// readRunActivity builds the reading of the live run rs; reader is nil when the engine has no ActivityReader.
func readRunActivity(rs RunState, engine Engine, reader ActivityReader) AgentActivity {
	reading := AgentActivity{StrandName: rs.StrandName}
	if reading.StrandName == "" {
		reading.StrandName = rs.StrandGUID
	}

	var eventsModTime time.Time
	var turnEnd *Event
	if info, err := os.Stat(rs.EventsPath); err == nil {
		eventsModTime = info.ModTime()
		turnEnd = newestTurnEnd(rs, engine)
	}

	var activity TurnEndActivity
	if reader != nil && turnEnd != nil {
		activity = reader.TurnEndActivity(*turnEnd)
	}
	reading.APIError, reading.APIErrorText = activity.APIError, activity.APIErrorText

	reading.LastActivity = activity.TranscriptModTime
	if eventsModTime.After(reading.LastActivity) {
		reading.LastActivity = eventsModTime
	}
	if reading.LastActivity.IsZero() {
		if createdAt, err := time.Parse(time.RFC3339, rs.CreatedAt); err == nil {
			reading.LastActivity = createdAt
		}
	}
	return reading
}

// newestTurnEnd returns the newest turn end in the run's events file past its prompt offset, or nil when there is none or the file cannot be parsed.
func newestTurnEnd(rs RunState, engine Engine) *Event {
	data, err := os.ReadFile(rs.EventsPath)
	if err != nil {
		return nil
	}
	if rs.PromptOffset > 0 {
		data = data[min(rs.PromptOffset, int64(len(data))):]
	}
	events, err := engine.ParseEvents(data)
	if err != nil {
		logger.Warn("shuttle: could not parse a run's events for its activity", "eventsPath", rs.EventsPath, "cause", err)
		return nil
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == EventStop || events[i].Kind == EventWaiting {
			return &events[i]
		}
	}
	return nil
}
