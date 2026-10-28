// activity.go declares the provider-neutral reading of what a worktree's live shuttle runs have been doing:
// when each last showed activity, and whether its newest turn end is an API error.
// A caller that judges a run quiet, or stalled on an API error, reads it through ReadAgentActivity.
// Like ReadWaitMarker it reads files only, so any process may call it.

package shuttleengine

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ActivityReader is an optional capability beside Engine: the provider's reading of the session a turn end names, its last activity with its forks included.
// An Engine without it yields activity readings with no transcript part.
type ActivityReader interface {
	// TurnEndActivity returns the session's last activity, its forks included, and the API-error reading of the transcript turnEnd names.
	// A fork's pending tool call reads active up to a provider-defined ceiling past its last write.
	// It degrades to the zero value on every failure and never errors.
	TurnEndActivity(turnEnd Event) TurnEndActivity
}

// TurnEndActivity is a provider-neutral reading of a session at a turn end: its last activity, its forks included, and the API-error part, which reads the session's own transcript only.
type TurnEndActivity struct {
	// TranscriptModTime is the session's last activity, its forks included; zero when the transcript cannot be read.
	TranscriptModTime time.Time
	// APIError is true when the transcript's newest main-chain turn end is an API error rather than the agent's own message.
	APIError bool
	// APIErrorText is the error's text; empty unless APIError is true.
	APIErrorText string
	// UsageLimit is the usage-limit reading of the same turn end.
	UsageLimit UsageLimit
}

// UsageLimit is the provider's reading of whether its usage limit refused a session's newest turn end, and when that limit resets.
// The zero value is no usage limit.
type UsageLimit struct {
	// Limited is true when the newest main-chain turn end is an API error the provider classifies as its usage limit.
	Limited bool
	// ResetsAt is when the limit resets, as the provider's limit message states it; zero when the message states no readable time.
	ResetsAt time.Time
}

// AgentActivity is the reading of one live run.
type AgentActivity struct {
	// StrandName is the run's strand name, the strand guid for a record that predates the name.
	StrandName string
	// LastActivity is the newer of the session's last activity, its forks included, and the time of the events file's newest Stop or ask line.
	// A Stop or ask line with no time, an engine without a SessionSignalParser and a file yielding no signals count the events file's last write instead.
	// It is the run's creation time when none of these can be read.
	LastActivity time.Time
	// APIError is true when the run's newest turn end is an API error.
	APIError bool
	// APIErrorText is the error's text; empty unless APIError is true.
	APIErrorText string
	// UsageLimit is the usage-limit reading of the run's newest turn end.
	UsageLimit UsageLimit
}

// unreadableFiles keeps one failure episode per file the files-only readers skip, keyed by the path their log line names.
// It is package-level because the readers are free functions that loops in several processes poll: a file that stays unreadable warns once, and readers that skip the same path share its episode.
var unreadableFiles logger.EpisodeSet

// ReadAgentActivity returns one reading per live run under the run-directory root of anchorPath.
// A run is live when its record reads running and the pid of the process that waits on it is alive.
// A directory that is not a run directory, a run with no record, an undecodable record and a run that is not live are skipped,
// and an undecodable record is logged once per episode so one torn file cannot hide another run.
// A record that reads running while the process recorded as waiting on it is gone is skipped too, and logged once per episode, so a working agent that lost its waiter leaves a trace.
// Each run is read through the engine its record names, the set's default for a record naming none:
// its events past its prompt offset are parsed through that engine, and the newest turn end among them is handed to the engine's ActivityReader;
// a run with no turn end past the offset hands over the newest one before it, a skill-load turn end of the same session, for its last activity only, so a first turn spent in a fork still reads active.
// A run with no turn end at all, or an engine without the capability, is judged by its events file alone.
// A run whose record names an engine the set does not hold is skipped and logged once per episode.
// An absent root returns no readings and no error.
func ReadAgentActivity(cfg Config, anchorPath string, engines EngineSet) ([]AgentActivity, error) {
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var readings []AgentActivity
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(root, entry.Name())
		rs, found, err := loadRunState(runDir)
		if err != nil {
			unreadableFiles.Fail(runDir, "shuttle: skipping an unreadable run record", "runDir", runDir, "cause", err)
			continue
		}
		unreadableFiles.Recover(runDir, "shuttle: run record readable again", "runDir", runDir)
		if !found {
			waiterlessRuns.Recover(runDir, waiterlessRecovered, "runDir", runDir)
			continue
		}
		if !runIsLive(rs) {
			noteWaiterlessRun(runDir, rs)
			continue
		}
		waiterlessRuns.Recover(runDir, waiterlessRecovered, "runDir", runDir)
		engine, ok := engines.engineOfRun(runDir, rs)
		if !ok {
			continue
		}
		reader, _ := engine.(ActivityReader)
		readings = append(readings, readRunActivity(rs, engine, reader))
	}
	return readings, nil
}

// runIsLive reports whether rs names a run still waited on: it reads running and its recorded pid is alive.
func runIsLive(rs RunState) bool {
	return rs.Outcome == runOutcomeRunning && rs.PID > 0 && isAlive(rs.PID)
}

// waiterlessRuns keeps one episode per run directory whose record reads running while the process recorded as waiting on it is gone.
// Its key space is its own because the unreadable-record episode of the same run directory recovers on every readable read.
var waiterlessRuns logger.EpisodeSet

// waiterlessRecovered is the recovery line of a waiterless-run episode.
const waiterlessRecovered = "shuttle: run no longer has a gone waiting process"

// noteWaiterlessRun logs the skip of the run at runDir, whose record rs is not live, once per episode when rs reads running with a recorded waiter that is gone.
// Such a run's agent may still be working where no reader sees it.
// It ends the episode when rs reads anything else.
// A record with no pid predates the field and is not counted.
func noteWaiterlessRun(runDir string, rs RunState) {
	if rs.Outcome == runOutcomeRunning && rs.PID > 0 {
		waiterlessRuns.Fail(runDir, "shuttle: skipping a run that reads running but whose waiting process is gone", "runDir", runDir, "strand", rs.StrandName, "pid", rs.PID)
		return
	}
	waiterlessRuns.Recover(runDir, waiterlessRecovered, "runDir", runDir)
}

// readRunActivity builds the reading of the live run rs; reader is nil when the engine has no ActivityReader.
func readRunActivity(rs RunState, engine Engine, reader ActivityReader) AgentActivity {
	reading := AgentActivity{StrandName: rs.StrandName}
	if reading.StrandName == "" {
		reading.StrandName = rs.StrandGUID
	}

	var eventsActivity time.Time
	var turnEnd *Event
	var ownTurnEnd bool
	if info, err := os.Stat(rs.EventsPath); err == nil {
		eventsActivity = eventsFileActivity(rs, engine, info.ModTime())
		turnEnd, ownTurnEnd = newestTurnEnd(rs, engine)
	}

	var activity TurnEndActivity
	if reader != nil && turnEnd != nil {
		activity = reader.TurnEndActivity(*turnEnd)
	}
	// A turn end before the prompt offset stands in only for the session's last activity: its API error is not the run's own.
	if ownTurnEnd {
		reading.APIError, reading.APIErrorText, reading.UsageLimit = activity.APIError, activity.APIErrorText, activity.UsageLimit
	}

	reading.LastActivity = activity.TranscriptModTime
	if eventsActivity.After(reading.LastActivity) {
		reading.LastActivity = eventsActivity
	}
	if reading.LastActivity.IsZero() {
		if createdAt, err := time.Parse(time.RFC3339, rs.CreatedAt); err == nil {
			reading.LastActivity = createdAt
		}
	}
	return reading
}

// eventsFileActivity returns the events file's part of a run's last activity.
// An engine with a SessionSignalParser reads the time of the newest Stop or ask line, so an idle notice, a prompt submission or a stamp line never moves it;
// the line's own time stands in as mtime only when the line carries none.
// The lines past the prompt offset are read first, and the whole file when none of them is a Stop or ask line.
// A file whose lines yield signals but no Stop or ask line reads as no activity.
// An engine without the parser, or a file that yields no signals at all, reads modTime.
func eventsFileActivity(rs RunState, engine Engine, modTime time.Time) time.Time {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return modTime
	}
	data, err := os.ReadFile(rs.EventsPath)
	if err != nil {
		return modTime
	}
	anySignal := false
	for _, from := range []int64{max(0, min(rs.PromptOffset, int64(len(data)))), 0} {
		signals, _ := parser.ParseSessionSignals(data[from:])
		anySignal = anySignal || len(signals) > 0
		for i := len(signals) - 1; i >= 0; i-- {
			if !signals[i].Event {
				continue
			}
			if signals[i].At.IsZero() {
				return modTime
			}
			return signals[i].At
		}
	}
	if anySignal {
		return time.Time{}
	}
	return modTime
}

// newestTurnEnd returns the newest turn end in the run's events file past its prompt offset, with own true.
// When none follows the offset it returns the newest turn end before it, with own false: that one names the same session, so its transcript and forks can be read before the run's first own turn ends.
// It returns nil when the file holds no turn end or cannot be parsed.
func newestTurnEnd(rs RunState, engine Engine) (turnEnd *Event, own bool) {
	data, err := os.ReadFile(rs.EventsPath)
	if err != nil {
		return nil, false
	}
	offset := min(max(rs.PromptOffset, 0), int64(len(data)))
	for _, part := range []struct {
		data []byte
		own  bool
	}{{data[offset:], true}, {data[:offset], false}} {
		events, err := engine.ParseEvents(part.data)
		if err != nil {
			unreadableFiles.Fail(rs.EventsPath, "shuttle: could not parse a run's events for its activity", "eventsPath", rs.EventsPath, "cause", err)
			return nil, false
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Kind == EventStop || events[i].Kind == EventWaiting {
				// Recovery is logged only when the parts read so far have all parsed,
				// so a part that fails cannot end and restart the episode on each poll.
				unreadableFiles.Recover(rs.EventsPath, "shuttle: run events parse again", "eventsPath", rs.EventsPath)
				return &events[i], part.own
			}
		}
	}
	unreadableFiles.Recover(rs.EventsPath, "shuttle: run events parse again", "eventsPath", rs.EventsPath)
	return nil, false
}
