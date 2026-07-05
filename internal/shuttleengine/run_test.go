// run_test.go covers Runner.Start: the happy path's exact AddSpec wiring
// (including SessionID/Display passthrough), validation short-circuiting
// before any mux call, run-dir cleanup on an AddStrand failure, and the
// opportunistic orphan sweep never blocking Start on its own failure.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// newTestRunner returns a Runner over mux/engine scoped to a fresh temp
// worktree, with tuning knobs small enough that any later Wait-driving test
// built on top of it runs fast.
func newTestRunner(t *testing.T, mux MuxOps, engine Engine) (*Runner, *hubgeometry.Layout) {
	t.Helper()
	root := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: root, WorktreeRoot: root}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1}
	return NewRunner(mux, engine, layout, cfg), layout
}

func TestRunner_Start_HappyPath_WiresAddSpecVerbatim(t *testing.T) {
	mux := &fakeMux{AddStrandResult: muxengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd", ResumeCmd: "resume-cmd", SessionID: "session-1"}}
	runner, _ := newTestRunner(t, mux, engine)

	spec := Spec{
		Prompt:      "do the thing",
		OutputFiles: []string{"out.md"},
		Role:        "reviewer",
		Round:       "1",
		Parent:      "parent-guid",
		Display:     render.Display{Anchor: render.AnchorTop},
	}

	run, err := runner.Start(spec)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if run == nil {
		t.Fatal("Start() returned nil run")
	}

	if len(mux.AddStrandCalls) != 1 {
		t.Fatalf("AddStrand calls = %d, want 1", len(mux.AddStrandCalls))
	}
	got := mux.AddStrandCalls[0]
	want := muxengine.AddSpec{
		Role:      "reviewer",
		Round:     "1",
		Parent:    "parent-guid",
		Cmd:       "launch-cmd",
		ResumeCmd: "resume-cmd",
		SessionID: "session-1",
		Display:   render.Display{Anchor: render.AnchorTop},
	}
	if got != want {
		t.Errorf("AddStrand spec = %+v, want %+v", got, want)
	}

	if run.state.SessionID != "session-1" {
		t.Errorf("state.SessionID = %q, want %q", run.state.SessionID, "session-1")
	}
	if run.state.StrandGUID != "strand-1" {
		t.Errorf("state.StrandGUID = %q, want %q", run.state.StrandGUID, "strand-1")
	}
}

func TestRunner_Start_ValidationFailure_ShortCircuitsBeforeMuxCall(t *testing.T) {
	mux := &fakeMux{}
	engine := &fakeEngine{}
	runner, _ := newTestRunner(t, mux, engine)

	if _, err := runner.Start(Spec{}); err == nil {
		t.Fatal("Start() = nil error, want validation error for empty spec")
	}
	if len(mux.CallLog) != 0 {
		t.Errorf("mux calls = %v, want none (validation must short-circuit)", mux.CallLog)
	}
	if len(engine.PrepareCalls) != 0 {
		t.Errorf("engine.Prepare calls = %d, want 0", len(engine.PrepareCalls))
	}
}

func TestRunner_Start_AddStrandFailure_CleansRunDir(t *testing.T) {
	mux := &fakeMux{AddStrandErr: fmt.Errorf("boom")}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
	runner, layout := newTestRunner(t, mux, engine)

	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error, want AddStrand failure to propagate")
	}

	root := runDirRoot(runner.cfg, layout)
	entries, rerr := os.ReadDir(root)
	if rerr != nil && !os.IsNotExist(rerr) {
		t.Fatalf("read run dir root: %v", rerr)
	}
	if len(entries) != 0 {
		t.Errorf("run dir root has %d leftover entr(y/ies), want 0 (AddStrand failure must clean up)", len(entries))
	}
}

func TestRunner_Start_SweepErrorDoesNotBlockStart(t *testing.T) {
	mux := &fakeMux{AddStrandResult: muxengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

	worktree := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: worktree, WorktreeRoot: worktree}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

	// Seed a corrupt mux.json so muxengine.LoadState errors during Start's
	// opportunistic orphan sweep — Start must log and continue rather than
	// fail the whole run over a housekeeping error.
	if err := os.MkdirAll(layout.DotLyxDir(), 0o755); err != nil {
		t.Fatalf("mkdir .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layout.DotLyxDir(), "mux.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt mux.json: %v", err)
	}

	runner := NewRunner(mux, engine, layout, cfg)
	run, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}})
	if err != nil {
		t.Fatalf("Start() error: %v, want sweep failure to be non-blocking", err)
	}
	if run == nil {
		t.Fatal("Start() returned nil run")
	}
}
