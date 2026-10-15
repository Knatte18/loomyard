// agentwait_test.go covers ReadAgentWaits over run directories written by the test, with the liveness seam replaced and an engine double that parses session signals.
// Replacing the seam is process-global state,
// so these tests do not run in parallel.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// waitRun describes one run directory under the run-directory root.
type waitRun struct {
	// pid is the recorded waiting process; zero reads as the live one.
	pid         int
	events      string
	interactive bool
	// outputExists makes the run declare one output file that exists.
	outputExists bool
	// marker, when set, is the kind of a wait marker held by the live process.
	marker string
	// engine is the engine name the record carries; empty leaves the record naming none.
	engine string
}

func TestReadAgentWaits(t *testing.T) {
	const livePID = 4343
	prev := isAlive
	isAlive = func(pid int) bool { return pid == livePID }
	t.Cleanup(func() { isAlive = prev })

	now := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
	const (
		startLine = "START@2026-10-08T07:00:00Z\n"
		endLine   = "END@2026-10-08T07:05:00Z\n"
	)
	tests := []struct {
		name string
		run  waitRun
		want []AgentWait
	}{
		{"a held turn end waits", waitRun{events: startLine, marker: heldWaitLabel}, []AgentWait{{StrandName: "strand-run", Waiting: true}}},
		{"an interactive turn end without outputs is asking and waits", waitRun{events: startLine + endLine, interactive: true}, []AgentWait{{StrandName: "strand-run", Waiting: true}}},
		{"an autonomous turn end without outputs is stalled and waits", waitRun{events: startLine + endLine}, []AgentWait{{StrandName: "strand-run", Waiting: true}}},
		{"a busy session is busy", waitRun{events: startLine}, []AgentWait{{StrandName: "strand-run", Busy: true}}},
		{"a gate wait is no turn end, so the busy session stays busy", waitRun{events: startLine, marker: gateWaitLabelPrefix + "entry"}, []AgentWait{{StrandName: "strand-run", Busy: true}}},
		{"a finished session is neither", waitRun{events: startLine + endLine, outputExists: true}, []AgentWait{{StrandName: "strand-run"}}},
		{"a run whose waiting process is dead is absent", waitRun{pid: 1, events: startLine}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeWaitRun(t, root, "run", tt.run, livePID)
			engine := &readEngine{fakeEngine: &fakeEngine{}}

			got, err := ReadAgentWaits(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine), now)

			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadAgentWaits = (%+v, %v); want (%+v, nil)", got, err, tt.want)
			}
		})
	}

	t.Run("an absent root reads empty", func(t *testing.T) {
		got, err := ReadAgentWaits(Config{RunDir: filepath.Join(t.TempDir(), "missing")}, t.TempDir(), singleEngineSet(&fakeEngine{}), now)
		if err != nil || got != nil {
			t.Errorf("ReadAgentWaits = (%v, %v); want (nil, nil)", got, err)
		}
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("a run record and a wait marker that stay unreadable log once per episode", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		writeWaitRun(t, root, "live", waitRun{events: startLine}, livePID)
		markerPath := filepath.Join(root, "live", waitMarkerFileName)
		if err := os.WriteFile(markerPath, []byte("kind: ["), 0o644); err != nil {
			t.Fatal(err)
		}
		brokenDir := filepath.Join(root, "torn")
		if err := os.MkdirAll(brokenDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(brokenDir, runStateFileName), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		engine := &readEngine{fakeEngine: &fakeEngine{}}

		for range 3 {
			if _, err := ReadAgentWaits(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine), now); err != nil {
				t.Fatal(err)
			}
		}
		writeWaitRun(t, root, "live", waitRun{events: startLine, marker: heldWaitLabel}, livePID)
		writeWaitRun(t, root, "torn", waitRun{events: startLine}, livePID)
		if _, err := ReadAgentWaits(Config{RunDir: root}, t.TempDir(), singleEngineSet(engine), now); err != nil {
			t.Fatal(err)
		}

		requireFailureEpisode(t, buf.String(), "skipping an unreadable run record", 2, "run record readable again")
		requireFailureEpisode(t, buf.String(), "skipping an unreadable wait marker", 2, "wait marker readable again")
	})

	// It captures the process-global logger at Debug, so it runs serially like the test.
	t.Run("each run is read through the engine its record names", func(t *testing.T) {
		buf := logcapture.CaptureVerbose(t)
		logger.SetVerbosity(2)
		root := t.TempDir()
		// Only beta reads the recorded session's process as gone, so a run reads busy or neither by the engine that read it.
		engines := NewEngineSet("alpha", map[string]Engine{
			"alpha": &readEngine{fakeEngine: &fakeEngine{}},
			"beta":  &readEngine{fakeEngine: &fakeEngine{}, livenessByID: map[string]Liveness{"session": LivenessDead}},
		})
		for name, engine := range map[string]string{"a": "alpha", "b": "beta", "c": "", "d": "gamma"} {
			writeWaitRun(t, root, name, waitRun{events: startLine, engine: engine}, livePID)
		}
		read := func() []AgentWait {
			got, err := ReadAgentWaits(Config{RunDir: root}, t.TempDir(), engines, now)
			if err != nil {
				t.Fatalf("ReadAgentWaits error = %v; want nil", err)
			}
			return got
		}
		want := []AgentWait{{StrandName: "strand-a", Busy: true}, {StrandName: "strand-b"}, {StrandName: "strand-c", Busy: true}}

		for range 3 {
			if got := read(); !reflect.DeepEqual(got, want) {
				t.Fatalf("ReadAgentWaits = %+v; want %+v (alpha, beta, the default alpha; the gamma run skipped)", got, want)
			}
		}
		writeWaitRun(t, root, "d", waitRun{events: startLine, engine: "beta"}, livePID)
		want = append(want, AgentWait{StrandName: "strand-d"})
		if got := read(); !reflect.DeepEqual(got, want) {
			t.Errorf("ReadAgentWaits after d names beta = %+v; want %+v", got, want)
		}

		requireEngineSkipEpisode(t, buf.String(), filepath.Join(root, "d"))
	})
}

// writeWaitRun writes the run directory name under root: its run.json, its events file, an existing output file when the run declares one and its wait marker when it has one.
func writeWaitRun(t *testing.T, root, name string, run waitRun, livePID int) {
	t.Helper()
	runDir := filepath.Join(root, name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(runDir, eventsFileName)
	if run.pid == 0 {
		run.pid = livePID
	}
	state := RunState{
		RunID: name, StrandGUID: "guid-" + name, StrandName: "strand-" + name, SessionID: "session",
		EventsPath: eventsPath, Outcome: runOutcomeRunning, PID: run.pid, Interactive: run.interactive, Engine: run.engine,
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
	if run.marker == "" {
		return
	}
	data, err := yaml.Marshal(WaitMarker{Kind: run.marker, Started: time.Unix(1, 0).UTC(), PID: livePID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, waitMarkerFileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
