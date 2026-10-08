// activity_test.go covers ReadAgentActivity over run directories written by the test, with the liveness seam replaced so no process is spawned and an engine double that answers the transcript reading.
// Replacing the seam is process-global state,
// so these tests do not run in parallel.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// activityEngine is a fakeEngine that also reads transcript activity from a table keyed by the turn end's message.
type activityEngine struct {
	*fakeEngine
	byMessage map[string]TurnEndActivity
}

func (e *activityEngine) TurnEndActivity(turnEnd Event) TurnEndActivity {
	return e.byMessage[turnEnd.Message]
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
		want       []AgentActivity
	}{
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
			if tt.transcript != nil {
				engine = &activityEngine{fakeEngine: &fakeEngine{}, byMessage: tt.transcript}
			}

			got, err := ReadAgentActivity(Config{RunDir: root}, t.TempDir(), engine)

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
		got, err := ReadAgentActivity(Config{RunDir: filepath.Join(t.TempDir(), "missing")}, t.TempDir(), &fakeEngine{})
		if err != nil || got != nil {
			t.Errorf("ReadAgentActivity = (%v, %v); want (nil, nil)", got, err)
		}
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
		CreatedAt: created.Format(time.RFC3339), Outcome: run.outcome, PID: run.pid, PromptOffset: run.promptOffset,
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
