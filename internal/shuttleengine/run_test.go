// run_test.go covers Runner.Start: the happy path's exact AddSpec wiring
// (including SessionID/Display passthrough), validation short-circuiting
// before any mux call, run-dir cleanup on an AddStrand failure, and the
// opportunistic orphan sweep never blocking Start on its own failure.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestRunner_Start_SaveRunStateFailure_RemovesStrandAndRunDir(t *testing.T) {
	// AddStrand succeeds (the strand and its launching pane exist), but the
	// subsequent saveRunState fails. Start must tear the strand back down and
	// remove the run directory rather than leaking a live, untracked pane no
	// run.json can ever bind.
	mux := &fakeMux{AddStrandResult: muxengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{
		PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"},
		// Plant run.json as a DIRECTORY so the later saveRunState write can
		// never succeed, forcing the mid-op failure this test exercises.
		PrepareHook: func(runDir string) {
			if err := os.MkdirAll(filepath.Join(runDir, runStateFileName), 0o755); err != nil {
				t.Fatalf("plant run.json dir: %v", err)
			}
		},
	}
	runner, layout := newTestRunner(t, mux, engine)

	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error, want save-run-state failure to propagate")
	}

	foundRemove := false
	for _, c := range mux.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded after save-state failure; strand leaked. calls = %+v", mux.RemoveStrandCalls)
	}

	root := runDirRoot(runner.cfg, layout)
	entries, rerr := os.ReadDir(root)
	if rerr != nil && !os.IsNotExist(rerr) {
		t.Fatalf("read run dir root: %v", rerr)
	}
	if len(entries) != 0 {
		t.Errorf("run dir root has %d leftover entr(y/ies), want 0 (save-state failure must clean up)", len(entries))
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

func TestRunner_Start_SweepSkipsEntirelyOnMuxStateReadError(t *testing.T) {
	// A LoadState ERROR must not degrade to "sweep with an empty live set":
	// that would delete every old-enough run dir, including a kept
	// asking/died/timeout dir mux still genuinely tracks, over an unrelated
	// I/O problem. It must skip the sweep for this Start entirely instead.
	mux := &fakeMux{AddStrandResult: muxengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

	worktree := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: worktree, WorktreeRoot: worktree}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

	if err := os.MkdirAll(layout.DotLyxDir(), 0o755); err != nil {
		t.Fatalf("mkdir .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layout.DotLyxDir(), "mux.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt mux.json: %v", err)
	}

	// An old, kept run dir (as an asking/died/timeout outcome would leave
	// behind) whose strand is not in mux.json's live set — because mux.json
	// itself is unreadable, not because the strand is genuinely gone.
	shuttleRoot := runDirRoot(cfg, layout)
	keptDir := seedRun(t, shuttleRoot, "kept-run", "some-other-strand")
	setDirMTime(t, keptDir, time.Now(), 10*time.Minute)

	runner := NewRunner(mux, engine, layout, cfg)
	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if _, err := os.Stat(keptDir); err != nil {
		t.Errorf("kept run dir was removed despite a mux-state read error, want it preserved: %v", err)
	}
}

// newInterruptTestRun returns a bare Run handle wired to mux/engine, with
// no Start/Wait machinery involved — Interrupt/Send only ever touch
// runner.mux and runner.engine through run.state.StrandGUID.
func newInterruptTestRun(t *testing.T, mux MuxOps, engine Engine) *Run {
	t.Helper()
	root := t.TempDir()
	layout := &hubgeometry.Layout{Cwd: root, WorktreeRoot: root}
	runner := NewRunner(mux, engine, layout, Config{})
	return &Run{
		runner: runner,
		state:  RunState{StrandGUID: "strand-1"},
	}
}

// liveStrandStatus scripts a fakeMux Status answer reporting strand-1 with
// the given liveness — what the Interrupt/Send liveness guard consumes.
func liveStrandStatus(live bool) []muxengine.StatusResult {
	return []muxengine.StatusResult{
		{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: live}}},
	}
}

func TestRun_Interrupt_PlaysEscape(t *testing.T) {
	mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, mux, engine)

	if err := run.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error: %v", err)
	}

	if len(mux.SendKeyCalls) != 1 || mux.SendKeyCalls[0].Key != "Escape" {
		t.Errorf("SendKey calls = %+v, want exactly one Escape", mux.SendKeyCalls)
	}
	if len(mux.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", mux.SendTextCalls)
	}
}

func TestRun_Send_RejectsNewlines(t *testing.T) {
	mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, mux, engine)

	if err := run.Send("line one\nline two"); err == nil {
		t.Fatal("Send() = nil error, want rejection for multiline text")
	}
	if len(mux.CallLog) != 0 {
		t.Errorf("mux calls = %v, want none (rejected before any mux call)", mux.CallLog)
	}
}

func TestRun_Send_RejectsEmptyOrWhitespace(t *testing.T) {
	// An empty or whitespace-only send has nothing to deliver: it would still
	// play the Escape+submit choreography (a stray empty turn) while making
	// sendVerified's delivery check vacuous (an empty needle every capture
	// "contains"), so Send must refuse before touching the pane rather than
	// report a falsely-verified success.
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tab", "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := &fakeMux{StatusQueue: liveStrandStatus(true)}
			run := newInterruptTestRun(t, mux, &fakeEngine{})

			if err := run.Send(tt.text); err == nil {
				t.Fatalf("Send(%q) = nil error, want rejection for empty/whitespace text", tt.text)
			}
			if len(mux.CallLog) != 0 {
				t.Errorf("mux calls = %v, want none (rejected before any mux call)", mux.CallLog)
			}
		})
	}
}

// stubInputSleep replaces the package-level inputSleep seam with a no-op for
// the duration of the calling test, restoring the real implementation via
// t.Cleanup — so sendVerified's poll/replay loops (and playInputs' SettleMS
// pause) run at test speed instead of real wall-clock time.
func stubInputSleep(t *testing.T) {
	t.Helper()
	orig := inputSleep
	inputSleep = func(time.Duration) {}
	t.Cleanup(func() { inputSleep = orig })
}

func TestRun_Send_PlaysEscThenTextWithSubmit(t *testing.T) {
	stubInputSleep(t)
	// CapturePane must report the sent text back for sendVerified's
	// delivery check to succeed without a replay.
	mux := &fakeMux{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ updated instructions"}}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, mux, engine)

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// Status leads the log: the liveness guard must run before any key
	// reaches the pane. CapturePane follows the text step: that is the
	// delivery-verification poll.
	wantLog := []string{"Status", "SendKey:Escape", "SendText:updated instructions", "CapturePane"}
	if !reflect.DeepEqual(mux.CallLog, wantLog) {
		t.Errorf("call order = %v, want %v", mux.CallLog, wantLog)
	}
	if len(mux.SendTextCalls) != 1 || !mux.SendTextCalls[0].Submit {
		t.Errorf("SendText calls = %+v, want one call with Submit=true", mux.SendTextCalls)
	}
}

func TestRun_Send_SwallowedFirstAttempt_ReplaySucceeds(t *testing.T) {
	// The provider TUI can swallow the whole Escape+text chunk with no
	// error anywhere (observed live): the pane capture never shows the text
	// after the first attempt, but does after the replay. sendVerified must
	// replay once and succeed, rather than trusting the first attempt.
	stubInputSleep(t)
	mux := &fakeMux{
		StatusQueue: liveStrandStatus(true),
		// Every poll of the first attempt sees no trace of the text (still
		// showing the prompt); the replay's polls see it delivered.
		CaptureQueue: append(
			repeatCapture("❯ ", sendVerifyAttempts),
			repeatCapture("❯ updated instructions", sendVerifyAttempts)...,
		),
	}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, mux, engine)

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error: %v, want the replay to succeed", err)
	}
	if len(mux.SendTextCalls) != 2 {
		t.Errorf("SendText calls = %d, want 2 (initial attempt + one replay)", len(mux.SendTextCalls))
	}
}

func TestRun_Send_NeverDelivered_ReportsHonestFailure(t *testing.T) {
	// If the text never appears even after the replay, Send must report a
	// delivery failure rather than the "keys were emitted" false ok:true
	// observed live.
	stubInputSleep(t)
	mux := &fakeMux{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repeatCapture("❯ ", 2*sendVerifyAttempts+2),
	}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, mux, engine)

	err := run.Send("updated instructions")
	if err == nil {
		t.Fatal("Send() = nil error, want a delivery-failure error")
	}
	if !strings.Contains(err.Error(), "never appeared") {
		t.Errorf("Send() error = %q, want it to name the delivery failure", err)
	}
	if len(mux.SendTextCalls) != 1+sendReplays {
		t.Errorf("SendText calls = %d, want %d (initial attempt + all replays exhausted)", len(mux.SendTextCalls), 1+sendReplays)
	}
}

// repeatCapture returns n copies of capture, the fixture shape fakeMux's
// CaptureQueue consumes one-per-call.
func repeatCapture(capture string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = capture
	}
	return out
}

func TestRun_InterruptAndSend_RefuseDeadOrUntrackedStrand(t *testing.T) {
	// psmux send-keys against a dead or missing pane exits 0 while
	// delivering nothing (proven live), so Interrupt/Send must refuse
	// before touching the pane rather than report a silent-no-op success.
	tests := []struct {
		name   string
		status []muxengine.StatusResult
	}{
		{"dead_pane", liveStrandStatus(false)},
		{"untracked_strand", []muxengine.StatusResult{{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := &fakeMux{StatusQueue: tt.status}
			run := newInterruptTestRun(t, mux, &fakeEngine{})

			if err := run.Interrupt(); err == nil {
				t.Error("Interrupt() = nil error, want liveness refusal")
			}
			if err := run.Send("still there?"); err == nil {
				t.Error("Send() = nil error, want liveness refusal")
			}
			if len(mux.SendKeyCalls) != 0 || len(mux.SendTextCalls) != 0 {
				t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", mux.SendKeyCalls, mux.SendTextCalls)
			}
		})
	}
}
