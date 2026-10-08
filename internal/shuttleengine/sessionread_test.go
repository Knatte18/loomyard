// sessionread_test.go covers ReadSessionStates over run directories written by the test, with an engine double that parses session signals and answers the prober and the transcript reading.
// The reader consults no process-global seam, so these tests run in parallel.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// readEngine is a fakeEngine that parses session signals from lines of the form START@<time> or END@<time>, each optionally followed by |<session id>,
// and answers liveness, the interrupt marker and the transcript API-error marker from its fields.
type readEngine struct {
	*fakeEngine
	// livenessByID answers ProcessLiveness; an id it does not name reads alive.
	livenessByID map[string]Liveness
	interruptAt  time.Time
	apiError     bool
}

func (e *readEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	var signals []SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		body, sessionID, _ := strings.Cut(line, "|")
		kind, stamp, _ := strings.Cut(body, "@")
		at, _ := time.Parse(time.RFC3339, stamp)
		signal := SessionSignal{At: at, SessionID: sessionID, Raw: []byte(line)}
		switch kind {
		case "START":
			signal.Kind = SessionSignalTurnStart
		case "END":
			signal.Kind = SessionSignalTurnEnd
		}
		signals = append(signals, signal)
	}
	return signals, len(data)
}

func (e *readEngine) ProcessLiveness(sessionID string) Liveness {
	if liveness, ok := e.livenessByID[sessionID]; ok {
		return liveness
	}
	return LivenessAlive
}

func (e *readEngine) TurnStartInterrupt(SessionSignal) (time.Time, bool) {
	return e.interruptAt, !e.interruptAt.IsZero()
}

func (e *readEngine) TurnEndActivity(Event) TurnEndActivity {
	return TurnEndActivity{APIError: e.apiError, APIErrorText: "API Error: 529"}
}

// readRun describes one run directory under the run-directory root.
type readRun struct {
	outcome      string
	sessionID    string
	events       string
	promptOffset int64
	// outputExists makes the run declare one output file that exists.
	outputExists bool
}

func TestReadSessionStates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	start := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	const (
		startLine = "START@2026-10-08T07:00:00Z\n"
		endLine   = "END@2026-10-08T07:05:00Z\n"
	)
	finished := readRun{outcome: runOutcomeRunning, events: startLine + endLine, outputExists: true}

	type wanted struct {
		name  string
		state SessionStateName
		cause string
		since time.Time
	}
	tests := []struct {
		name string
		runs map[string]readRun
		// stray adds a non-run file and a run directory with an undecodable record.
		stray bool
		// plain gives the engine no session-signal parser.
		plain  bool
		engine readEngine
		want   []wanted
	}{
		{
			name: "a stamped turn end with outputs present reads idle-done at the turn end's time",
			runs: map[string]readRun{"run-a": finished},
			want: []wanted{{"hub:task:driver", SessionIdleDone, SessionCauseDone, end}},
		},
		{
			name: "a non-running record, a non-run file and an undecodable record are skipped",
			runs: map[string]readRun{"run-a": finished, "run-b": {outcome: string(OutcomeDone), events: startLine + endLine}},
			// the stray entries sort around run-a and must not hide it.
			stray: true,
			want:  []wanted{{"hub:task:driver", SessionIdleDone, SessionCauseDone, end}},
		},
		{
			name: "signals before the prompt offset are not folded",
			runs: map[string]readRun{"run-a": {outcome: runOutcomeRunning, events: endLine + startLine, promptOffset: int64(len(endLine))}},
			want: []wanted{{"hub:task:driver", SessionBusy, SessionCauseTurn, start}},
		},
		{
			name:   "liveness is looked up by the newest signal's session id, not the record's",
			runs:   map[string]readRun{"run-a": {outcome: runOutcomeRunning, sessionID: "recorded", events: "START@2026-10-08T07:00:00Z|recorded\nEND@2026-10-08T07:05:00Z|newer\n", outputExists: true}},
			engine: readEngine{livenessByID: map[string]Liveness{"recorded": LivenessDead}},
			want:   []wanted{{"hub:task:driver", SessionIdleDone, SessionCauseDone, end}},
		},
		{
			name:   "a dead process reads dead",
			runs:   map[string]readRun{"run-a": {outcome: runOutcomeRunning, sessionID: "recorded", events: startLine}},
			engine: readEngine{livenessByID: map[string]Liveness{"recorded": LivenessDead}},
			want:   []wanted{{"hub:task:driver", SessionDead, SessionCauseProcessGone, now}},
		},
		{
			name:   "the transcript's API-error marker makes the turn end a stall",
			runs:   map[string]readRun{"run-a": {outcome: runOutcomeRunning, events: startLine + endLine}},
			engine: readEngine{apiError: true},
			want:   []wanted{{"hub:task:driver", SessionIdleStalled, SessionCauseAPIError, end}},
		},
		{
			name:   "an interrupt marker on the turn start makes the turn a stall at the interrupt's time",
			runs:   map[string]readRun{"run-a": {outcome: runOutcomeRunning, events: startLine}},
			engine: readEngine{interruptAt: start.Add(time.Minute)},
			want:   []wanted{{"hub:task:driver", SessionIdleStalled, SessionCauseInterrupt, start.Add(time.Minute)}},
		},
		{
			name:  "an engine without a signal parser reads unknown unsupported",
			runs:  map[string]readRun{"run-a": finished},
			plain: true,
			want:  []wanted{{"hub:task:driver", SessionUnknown, SessionCauseUnsupported, now}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, run := range tt.runs {
				writeReadRun(t, root, name, run)
			}
			if tt.stray {
				if err := os.WriteFile(filepath.Join(root, "a-stray.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				badDir := filepath.Join(root, "run-bad")
				if err := os.MkdirAll(badDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(badDir, runStateFileName), []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var engine Engine = &tt.engine
			tt.engine.fakeEngine = &fakeEngine{}
			if tt.plain {
				engine = tt.engine.fakeEngine
			}

			got, err := ReadSessionStates(Config{RunDir: root}, t.TempDir(), engine, now)
			if err != nil {
				t.Fatalf("ReadSessionStates error = %v; want nil", err)
			}
			var gotWanted []wanted
			for _, reading := range got {
				gotWanted = append(gotWanted, wanted{reading.StrandName, reading.State.Name, reading.State.Cause, reading.State.Since})
			}
			if !reflect.DeepEqual(gotWanted, tt.want) {
				t.Errorf("ReadSessionStates = %+v; want %+v", gotWanted, tt.want)
			}
		})
	}

	t.Run("an absent root returns none", func(t *testing.T) {
		t.Parallel()
		got, err := ReadSessionStates(Config{RunDir: filepath.Join(t.TempDir(), "missing")}, t.TempDir(), &fakeEngine{}, now)
		if err != nil || got != nil {
			t.Errorf("ReadSessionStates = (%v, %v); want (nil, nil)", got, err)
		}
	})
}

// writeReadRun writes the run directory name under root: its run.json, its events file and, when the run declares one, an existing output file.
func writeReadRun(t *testing.T, root, name string, run readRun) {
	t.Helper()
	runDir := filepath.Join(root, name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(runDir, eventsFileName)
	if run.sessionID == "" {
		run.sessionID = "recorded-session"
	}
	state := RunState{
		RunID: name, StrandGUID: "guid-" + name, StrandName: "hub:task:driver", SessionID: run.sessionID,
		EventsPath: eventsPath, Outcome: run.outcome, PID: 4343, PromptOffset: run.promptOffset,
	}
	if run.outputExists {
		output := filepath.Join(runDir, "report.md")
		if err := os.WriteFile(output, []byte("done"), 0o644); err != nil {
			t.Fatal(err)
		}
		state.OutputFiles = []string{output}
	}
	if err := saveRunState(runDir, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventsPath, []byte(run.events), 0o644); err != nil {
		t.Fatal(err)
	}
}
