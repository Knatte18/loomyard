//go:build tier0

// activity_test.go covers ReadAgentActivity over run directories written by the test, with the liveness seam replaced so no process is spawned and an engine double that answers the transcript reading.
// Replacing the seam is process-global state,
// so these tests do not run in parallel.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// activityEngine is a fakeEngine that also reads transcript activity from a table keyed by the turn end's message.
type activityEngine struct {
	*fakeEngine
	byMessage map[string]TurnEndActivity
}

func (e *activityEngine) TurnEndActivity(turnEnd Event) TurnEndActivity {
	return e.byMessage[turnEnd.Message]
}

// signalActivityEngine is an activityEngine that also reads session signals:
// STOP: and ASK: lines are unstamped Stop and ask records, SIG-STOP@<time> is a stamped Stop record and SIG-IDLE@<time> a stamped idle notice, which is not a record.
type signalActivityEngine struct {
	*activityEngine
}

func (e signalActivityEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	var signals []SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "STOP:"), strings.HasPrefix(line, "ASK:"):
			signals = append(signals, SessionSignal{Kind: SessionSignalTurnEnd, Event: true})
		case strings.HasPrefix(line, "SIG-STOP@"):
			at, _ := time.Parse(time.RFC3339, strings.TrimPrefix(line, "SIG-STOP@"))
			signals = append(signals, SessionSignal{Kind: SessionSignalTurnEnd, At: at, Event: true})
		case strings.HasPrefix(line, "SIG-IDLE@"):
			at, _ := time.Parse(time.RFC3339, strings.TrimPrefix(line, "SIG-IDLE@"))
			signals = append(signals, SessionSignal{Kind: SessionSignalIdleNotice, At: at})
		}
	}
	return signals, len(data)
}

// failingPartEngine is a fakeEngine whose ParseEvents fails on any bytes containing failOn.
type failingPartEngine struct {
	*fakeEngine
	failOn string
}

func (e *failingPartEngine) ParseEvents(data []byte) ([]Event, error) {
	if strings.Contains(string(data), e.failOn) {
		return nil, errors.New("failingPartEngine: scripted parse failure")
	}
	return e.fakeEngine.ParseEvents(data)
}

// activityRun describes one run directory under the run-directory root.
type activityRun struct {
	outcome      string
	pid          int
	strandName   string
	events       string
	promptOffset int64
	// noEventsFile leaves the events file unwritten.
	noEventsFile bool
	// engine is the engine name the record carries; empty leaves the record naming none.
	engine string
}

func TestReadAgentActivity(t *testing.T) {
	const livePID = 4242
	prev := isAlive
	isAlive = func(pid int) bool { return pid == livePID }
	t.Cleanup(func() { isAlive = prev })

	created := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	eventsWritten := created.Add(10 * time.Minute)
	transcriptWritten := created.Add(20 * time.Minute)
	live := activityRun{outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", events: "STOP:turn\n"}

	tests := []struct {
		name string
		runs map[string]activityRun
		// transcript answers the engine's transcript reading by turn-end message; nil runs the plain engine with no ActivityReader.
		transcript map[string]TurnEndActivity
		// signals gives the engine a session-signal parser.
		signals bool
		want    []AgentActivity
	}{
		{
			name:    "a stamped stop followed by an idle notice reads the stop's stamp, not the file's write time",
			runs:    map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", events: "SIG-STOP@2026-10-07T09:05:00Z\nSIG-IDLE@2026-10-07T09:15:00Z\n"}},
			signals: true,
			want:    []AgentActivity{{StrandName: "hub:task:driver", LastActivity: created.Add(5 * time.Minute)}},
		},
		{
			name:    "an unstamped stop reads the file's write time as before",
			runs:    map[string]activityRun{"run-a": live},
			signals: true,
			want:    []AgentActivity{{StrandName: "hub:task:driver", LastActivity: eventsWritten}},
		},
		{
			name:    "a file with only an idle notice reads no events activity",
			runs:    map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", events: "SIG-IDLE@2026-10-07T09:15:00Z\n"}},
			signals: true,
			want:    []AgentActivity{{StrandName: "hub:task:driver", LastActivity: created}},
		},
		{
			name:    "a stop before the prompt offset still counts when none follows it",
			runs:    map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", events: "SIG-STOP@2026-10-07T09:05:00Z\nSIG-IDLE@2026-10-07T09:15:00Z\n", promptOffset: int64(len("SIG-STOP@2026-10-07T09:05:00Z\n"))}},
			signals: true,
			want:    []AgentActivity{{StrandName: "hub:task:driver", LastActivity: created.Add(5 * time.Minute)}},
		},
		{
			name:       "transcript newer than the events file reports the transcript time",
			runs:       map[string]activityRun{"run-a": live},
			transcript: map[string]TurnEndActivity{"turn": {TranscriptModTime: transcriptWritten}},
			want:       []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten}},
		},
		{
			name:       "an unreadable transcript falls back to the events file",
			runs:       map[string]activityRun{"run-a": live},
			transcript: map[string]TurnEndActivity{},
			want:       []AgentActivity{{StrandName: "hub:task:driver", LastActivity: eventsWritten}},
		},
		{
			name: "an engine without the capability is judged by the events file",
			runs: map[string]activityRun{"run-a": live},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: eventsWritten}},
		},
		{
			name: "a run with no events file falls back to its creation time",
			runs: map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", noEventsFile: true}},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: created}},
		},
		{
			name: "a run with no turn end yet is judged by its events file alone",
			runs: map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver", events: "ASK:question\n"}},
			transcript: map[string]TurnEndActivity{
				"": {TranscriptModTime: transcriptWritten},
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: eventsWritten}},
		},
		{
			name:       "an API-error turn end is reported with its text",
			runs:       map[string]activityRun{"run-a": live},
			transcript: map[string]TurnEndActivity{"turn": {TranscriptModTime: transcriptWritten, APIError: true, APIErrorText: "API Error: 529 overloaded"}},
			want:       []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten, APIError: true, APIErrorText: "API Error: 529 overloaded"}},
		},
		{
			name:       "a usage-limited turn end carries its limit",
			runs:       map[string]activityRun{"run-a": live},
			transcript: map[string]TurnEndActivity{"turn": {TranscriptModTime: transcriptWritten, APIError: true, APIErrorText: "You've hit your weekly limit", UsageLimit: UsageLimit{Limited: true, ResetsAt: transcriptWritten.Add(time.Hour)}}},
			want:       []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten, APIError: true, APIErrorText: "You've hit your weekly limit", UsageLimit: UsageLimit{Limited: true, ResetsAt: transcriptWritten.Add(time.Hour)}}},
		},
		{
			name: "a usage limit on a turn end before the prompt offset is not the run's own",
			runs: map[string]activityRun{"run-a": {
				outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver",
				events: "STOP:skill\nSTOP:turn\n", promptOffset: int64(len("STOP:skill\n")),
			}},
			transcript: map[string]TurnEndActivity{
				"skill": {UsageLimit: UsageLimit{Limited: true, ResetsAt: transcriptWritten.Add(time.Hour)}},
				"turn":  {TranscriptModTime: transcriptWritten},
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten}},
		},
		{
			name: "turn ends before the prompt offset are not the run's own",
			runs: map[string]activityRun{"run-a": {
				outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver",
				events: "STOP:skill\nSTOP:turn\n", promptOffset: int64(len("STOP:skill\n")),
			}},
			transcript: map[string]TurnEndActivity{
				"skill": {APIError: true, APIErrorText: "from the skill load"},
				"turn":  {TranscriptModTime: transcriptWritten},
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten}},
		},
		{
			name: "a first turn not yet ended reads the session's activity through the turn end before the prompt offset, without its API error",
			runs: map[string]activityRun{"run-a": {
				outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver",
				events: "STOP:skill\n", promptOffset: int64(len("STOP:skill\n")),
			}},
			transcript: map[string]TurnEndActivity{
				"skill": {TranscriptModTime: transcriptWritten, APIError: true, APIErrorText: "from the skill load"},
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten}},
		},
		{
			name: "a prompt offset past the end of the file reads the whole file as the session's earlier activity",
			runs: map[string]activityRun{"run-a": {
				outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver",
				events: "STOP:skill\n", promptOffset: 1000,
			}},
			transcript: map[string]TurnEndActivity{
				"skill": {TranscriptModTime: transcriptWritten, APIError: true, APIErrorText: "from the skill load"},
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: transcriptWritten}},
		},
		{
			name: "a record without a strand name reports the strand guid",
			runs: map[string]activityRun{"run-a": {outcome: runOutcomeRunning, pid: livePID, events: "STOP:turn\n"}},
			want: []AgentActivity{{StrandName: "guid-run-a", LastActivity: eventsWritten}},
		},
		{
			name: "dead-pid, pid-less and finished runs are skipped",
			runs: map[string]activityRun{
				"run-a": {outcome: runOutcomeRunning, pid: 4343, events: "STOP:turn\n"},
				"run-b": {outcome: runOutcomeRunning, events: "STOP:turn\n"},
				"run-c": {outcome: string(OutcomeDone), pid: livePID, events: "STOP:turn\n"},
				"run-d": live,
			},
			want: []AgentActivity{{StrandName: "hub:task:driver", LastActivity: eventsWritten}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, run := range tt.runs {
				writeActivityRun(t, root, name, run, created, eventsWritten)
			}
			var engine Engine = &fakeEngine{}
			if tt.transcript != nil || tt.signals {
				engine = &activityEngine{fakeEngine: &fakeEngine{}, byMessage: tt.transcript}
			}
			if tt.signals {
				engine = signalActivityEngine{engine.(*activityEngine)}
			}

			got, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine))

			if err != nil {
				t.Fatalf("ReadAgentActivity error = %v; want nil", err)
			}
			// File modification times come back in the local zone; compare them as instants.
			for i := range got {
				got[i].LastActivity = got[i].LastActivity.UTC()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadAgentActivity = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("an absent root returns none", func(t *testing.T) {
		got, err := ReadAgentActivity(Config{RunDir: filepath.Join(t.TempDir(), "missing")}, t.TempDir(), singleEngineSet(&fakeEngine{}))
		if err != nil || got != nil {
			t.Errorf("ReadAgentActivity = (%v, %v); want (nil, nil)", got, err)
		}
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("events that fail to parse log once per episode", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		writeActivityRun(t, root, "run-a", live, created, eventsWritten)
		engine := &fakeEngine{ParseEventsFailCount: 3}

		for range 4 {
			if _, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine)); err != nil {
				t.Fatal(err)
			}
		}

		requireFailureEpisode(t, buf.String(), "could not parse a run's events", 2, "run events parse again")
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("a pre-offset part that fails to parse after the own part parsed logs one episode", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		run := activityRun{
			outcome: runOutcomeRunning, pid: livePID, strandName: "hub:task:driver",
			events: "BAD\nnoise\n", promptOffset: int64(len("BAD\n")),
		}
		writeActivityRun(t, root, "run-a", run, created, eventsWritten)
		engine := &failingPartEngine{fakeEngine: &fakeEngine{}, failOn: "BAD"}

		for range 4 {
			if _, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine)); err != nil {
				t.Fatal(err)
			}
		}

		failure := "could not parse a run's events"
		if got := countLevelLines(buf.String(), "WARN", failure); got != 1 {
			t.Errorf("WARN lines of %q = %d, want 1; log: %s", failure, got, buf.String())
		}
		if got := countLevelLines(buf.String(), "INFO", "run events parse again"); got != 0 {
			t.Errorf("INFO recovery lines = %d, want 0; log: %s", got, buf.String())
		}
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("a running record whose waiter is gone logs once per episode until it reads live", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		waiterless := live
		waiterless.pid = 4343
		writeActivityRun(t, root, "run-a", waiterless, created, eventsWritten)
		writeActivityRun(t, root, "run-b", activityRun{outcome: runOutcomeRunning, events: "STOP:turn\n"}, created, eventsWritten)
		writeActivityRun(t, root, "run-c", activityRun{outcome: string(OutcomeDone), pid: 4343, events: "STOP:turn\n"}, created, eventsWritten)
		read := func() []AgentActivity {
			got, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), singleEngineSet(&fakeEngine{}))
			if err != nil {
				t.Fatalf("ReadAgentActivity error = %v; want nil", err)
			}
			return got
		}

		for range 3 {
			if got := read(); len(got) != 0 {
				t.Fatalf("ReadAgentActivity = %+v; want no readings while no run has a live waiter", got)
			}
		}
		writeActivityRun(t, root, "run-a", live, created, eventsWritten)
		if got := read(); len(got) != 1 {
			t.Fatalf("ReadAgentActivity = %+v; want the one run whose waiter is live again", got)
		}

		log := buf.String()
		requireFailureEpisode(t, log, "reads running but whose waiting process is gone", 2, waiterlessRecovered)

		// A record that stops reading running, or vanishes, ends an open episode with one Info each.
		writeActivityRun(t, root, "run-a", waiterless, created, eventsWritten)
		read()
		finished := waiterless
		finished.outcome = string(OutcomeDone)
		writeActivityRun(t, root, "run-a", finished, created, eventsWritten)
		read()
		read()
		if got := countLevelLines(buf.String(), "INFO", waiterlessRecovered); got != 2 {
			t.Errorf("INFO lines of %q after the record turned terminal = %d, want 2; log: %s", waiterlessRecovered, got, buf.String())
		}
		writeActivityRun(t, root, "run-a", waiterless, created, eventsWritten)
		read()
		if err := os.Remove(filepath.Join(root, "run-a", runStateFileName)); err != nil {
			t.Fatal(err)
		}
		read()
		read()
		if got := countLevelLines(buf.String(), "INFO", waiterlessRecovered); got != 3 {
			t.Errorf("INFO lines of %q after the record vanished = %d, want 3; log: %s", waiterlessRecovered, got, buf.String())
		}

		if !strings.Contains(log, filepath.Join(root, "run-a")) {
			t.Errorf("log names no run-a directory; log: %s", log)
		}
		for _, other := range []string{"run-b", "run-c"} {
			if strings.Contains(log, filepath.Join(root, other)) {
				t.Errorf("log names %s, a pid-less or finished record; log: %s", other, log)
			}
		}
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("each run is read through the engine its record names", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		alphaWritten, betaWritten := created.Add(20*time.Minute), created.Add(30*time.Minute)
		engines := NewEngineSet("alpha", map[string]Engine{
			"alpha": &activityEngine{fakeEngine: &fakeEngine{}, byMessage: map[string]TurnEndActivity{"turn": {TranscriptModTime: alphaWritten}}},
			"beta":  &activityEngine{fakeEngine: &fakeEngine{}, byMessage: map[string]TurnEndActivity{"turn": {TranscriptModTime: betaWritten}}},
		})
		for name, engine := range map[string]string{"run-a": "alpha", "run-b": "beta", "run-c": "", "run-d": "gamma"} {
			run := live
			run.engine = engine
			writeActivityRun(t, root, name, run, created, eventsWritten)
		}
		read := func() []AgentActivity {
			got, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), engines)
			if err != nil {
				t.Fatalf("ReadAgentActivity error = %v; want nil", err)
			}
			for i := range got {
				got[i].LastActivity = got[i].LastActivity.UTC()
			}
			return got
		}
		want := []AgentActivity{
			{StrandName: "hub:task:driver", LastActivity: alphaWritten},
			{StrandName: "hub:task:driver", LastActivity: betaWritten},
			{StrandName: "hub:task:driver", LastActivity: alphaWritten},
		}

		for range 3 {
			if got := read(); !reflect.DeepEqual(got, want) {
				t.Fatalf("ReadAgentActivity = %+v; want %+v (alpha, beta, the default alpha; the gamma run skipped)", got, want)
			}
		}
		rewritten := live
		rewritten.engine = "beta"
		writeActivityRun(t, root, "run-d", rewritten, created, eventsWritten)
		want = append(want, AgentActivity{StrandName: "hub:task:driver", LastActivity: betaWritten})
		if got := read(); !reflect.DeepEqual(got, want) {
			t.Errorf("ReadAgentActivity after run-d names beta = %+v; want %+v", got, want)
		}

		requireEngineSkipEpisode(t, buf.String(), filepath.Join(root, "run-d"))
	})
}

// writeActivityRun writes the run directory name under root: its run.json, and its events file stamped with eventsWritten unless the run has none.
func writeActivityRun(t *testing.T, root, name string, run activityRun, created, eventsWritten time.Time) {
	t.Helper()
	runDir := filepath.Join(root, name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(runDir, eventsFileName)
	state := RunState{
		RunID: name, StrandGUID: "guid-" + name, StrandName: run.strandName, EventsPath: eventsPath,
		CreatedAt: created.Format(time.RFC3339), Outcome: run.outcome, PID: run.pid, PromptOffset: run.promptOffset, Engine: run.engine,
	}
	if err := saveRunState(runDir, state); err != nil {
		t.Fatal(err)
	}
	if run.noEventsFile {
		return
	}
	if err := os.WriteFile(eventsPath, []byte(run.events), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(eventsPath, eventsWritten, eventsWritten); err != nil {
		t.Fatal(err)
	}
}
