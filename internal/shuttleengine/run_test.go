// run_test.go covers Runner.Start: the happy path's exact AddSpec wiring (including
// SessionID/Display passthrough), validation short-circuiting before any reed call, run-dir cleanup
// on an AddStrand failure, and the opportunistic orphan sweep never blocking Start on its own
// failure.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// newTestRunner returns a Runner over reed/engine scoped to a fresh temp
// worktree, with tuning knobs small enough that any later Wait-driving test
// built on top of it runs fast. anchorPath and worktreeRoot are distinct
// values (never the same temp dir twice) so a swapped NewRunner argument
// pair fails a test rather than passing.
func newTestRunner(t *testing.T, reed ReedOps, engine Engine) (runner *Runner, anchorPath, worktreeRoot string) {
	t.Helper()
	worktreeRoot = t.TempDir()
	anchorPath = filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1}
	return NewRunner(reed, engine, anchorPath, worktreeRoot, cfg), anchorPath, worktreeRoot
}

// TestNewRunner_RefusesUnusableToldPaths pins the told-pair guard.
// anchorPath and worktreeRoot are adjacent parameters of the same type with four semantically
// distinct consumers, so a swap compiles cleanly and, in a subpath-anchored worktree, silently
// relocates the run-dir root, reed's state lookup, and the fork audit's transcript directory into
// the worktree root instead of the anchor. An empty or relative value fails the same way — it
// succeeds against whatever working directory the process happens to have.
// Every public entry point must refuse, not just Start: Interrupt/Send/Inject all resolve their run
// through the same anchorPath.
func TestNewRunner_RefusesUnusableToldPaths(t *testing.T) {
	worktreeRoot := t.TempDir()
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}

	tests := []struct {
		name         string
		anchorPath   string
		worktreeRoot string
		wantIn       string
	}{
		{"swapped_pair", worktreeRoot, anchorPath, "most likely swapped"},
		{"empty_anchor", "", worktreeRoot, "empty path"},
		{"empty_worktree_root", anchorPath, "", "empty path"},
		{"relative_anchor", filepath.Join("sub", "dir"), worktreeRoot, "relative path"},
		{"anchor_in_a_sibling_tree", t.TempDir(), worktreeRoot, "outside its worktree root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewRunner(&fakeReed{}, &fakeEngine{}, tt.anchorPath, tt.worktreeRoot, Config{RunTimeoutMin: 5})

			if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Start() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Interrupt("strand-1"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Send("strand-1", "hi"); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Send() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := runner.Inject("strand-1", []PaneInput{{Key: "Escape"}}); err == nil || !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Inject() error = %v; want it to name %q", err, tt.wantIn)
			}
		})
	}
}

// TestNewRunner_AcceptsHubGeometryShapes pins the other side: every pair hubgeom.ReedGeometry can
// produce must pass, including the anchor-at-worktree-root case (AnchorRel "."), where the two
// values are legitimately equal and a swap is a no-op.
func TestNewRunner_AcceptsHubGeometryShapes(t *testing.T) {
	worktreeRoot := t.TempDir()
	subpathAnchor := filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(subpathAnchor, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}

	tests := []struct {
		name       string
		anchorPath string
	}{
		{"anchored_at_worktree_root", worktreeRoot},
		{"subpath_anchored", subpathAnchor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewRunner(&fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}, &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}, tt.anchorPath, worktreeRoot, Config{RunTimeoutMin: 5})
			if runner.toldErr != nil {
				t.Errorf("NewRunner(%q, %q).toldErr = %v; want nil", tt.anchorPath, worktreeRoot, runner.toldErr)
			}
		})
	}
}

func TestRunner_Start_HappyPath_WiresAddSpecVerbatim(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "launch-cmd", ResumeCmd: "resume-cmd", SessionID: "session-1"}}
	runner, _, _ := newTestRunner(t, reed, engine)

	spec := Spec{
		Prompt:      "do the thing",
		OutputFiles: []string{"out.md"},
		Role:        "reviewer",
		Round:       "1",
		Parent:      "parent-guid",
		Display:     render.Display{Anchor: render.AnchorBelowParent},
	}

	run, err := runner.Start(spec)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if run == nil {
		t.Fatal("Start() returned nil run")
	}

	if len(reed.AddStrandCalls) != 1 {
		t.Fatalf("AddStrand calls = %d, want 1", len(reed.AddStrandCalls))
	}
	got := reed.AddStrandCalls[0]
	want := reedengine.AddSpec{
		Role:      "reviewer",
		Round:     "1",
		Parent:    "parent-guid",
		Cmd:       "launch-cmd",
		ResumeCmd: "resume-cmd",
		SessionID: "session-1",
		Display:   render.Display{Anchor: render.AnchorBelowParent},
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

func TestRunner_Start_ValidationFailure_ShortCircuitsBeforeReedCall(t *testing.T) {
	reed := &fakeReed{}
	engine := &fakeEngine{}
	runner, _, _ := newTestRunner(t, reed, engine)

	if _, err := runner.Start(Spec{}); err == nil {
		t.Fatal("Start() = nil error, want validation error for empty spec")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed calls = %v, want none (validation must short-circuit)", reed.CallLog)
	}
	if len(engine.PrepareCalls) != 0 {
		t.Errorf("engine.Prepare calls = %d, want 0", len(engine.PrepareCalls))
	}
}

func TestRunner_Start_AddStrandFailure_CleansRunDir(t *testing.T) {
	reed := &fakeReed{AddStrandErr: fmt.Errorf("boom")}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd"}}
	runner, anchorPath, _ := newTestRunner(t, reed, engine)

	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error, want AddStrand failure to propagate")
	}

	root := runDirRoot(runner.cfg, anchorPath)
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
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
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
	runner, anchorPath, _ := newTestRunner(t, reed, engine)

	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error, want save-run-state failure to propagate")
	}

	foundRemove := false
	for _, c := range reed.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded after save-state failure; strand leaked. calls = %+v", reed.RemoveStrandCalls)
	}

	root := runDirRoot(runner.cfg, anchorPath)
	entries, rerr := os.ReadDir(root)
	if rerr != nil && !os.IsNotExist(rerr) {
		t.Fatalf("read run dir root: %v", rerr)
	}
	if len(entries) != 0 {
		t.Errorf("run dir root has %d leftover entr(y/ies), want 0 (save-state failure must clean up)", len(entries))
	}
}

// TestRunner_Start_StrandTeardownFailure_LogsThroughLogger is one half of R2-F8's regression guard.
//
// By the time saveRunState fails, AddStrand has already put a real pane and a real provider process
// on the substrate, so the RemoveStrand that follows is a live-substrate teardown — and a
// RemoveStrand that ERRORS is precisely "a teardown that did not confirm clean", which
// CONSTRAINTS.md's Live-Substrate Spawn Observability invariant requires on internal/logger at Warn.
// It used to go to the bare log package, which the durable Info+ trace sink never captures, so the
// one record of a leaked live pane existed only on an ephemeral stderr with no trace correlation id
// on it. Verified live: a bare log.Printf from this package appeared on stderr and was absent from
// the trace file for the very same invocation.
func TestRunner_Start_StrandTeardownFailure_LogsThroughLogger(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		RemoveStrandErr: errors.New("reed: no session"),
	}
	engine := &fakeEngine{
		PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"},
		// Plant run.json as a DIRECTORY so saveRunState cannot succeed, which is what drives Start
		// into the teardown path under test.
		PrepareHook: func(runDir string) {
			if err := os.MkdirAll(filepath.Join(runDir, runStateFileName), 0o755); err != nil {
				t.Fatalf("plant run.json dir: %v", err)
			}
		},
	}
	runner, _, _ := newTestRunner(t, reed, engine)

	buf := captureLoggerOutput(t)
	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err == nil {
		t.Fatal("Start() = nil error; want the save-run-state failure to propagate")
	}

	logged := buf.String()
	for _, want := range []string{"remove strand after save-state failure", "strand-1", "reed: no session"} {
		if !strings.Contains(logged, want) {
			t.Errorf("logger output = %q; want it to contain %q", logged, want)
		}
	}
}

// TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger is the other half of R2-F8's guard, and
// the one that actually prevents reintroduction.
//
// This whole package is a live-substrate module: every operational message it emits is about a real
// pane or a real provider process, so every one of them belongs on internal/logger's durable sink
// rather than on the stdlib log package, whose output the trace file never sees and which carries no
// trace correlation id. R1-F10 migrated finalize's two sites and left five siblings behind in these
// same two files; a behavioural test can only ever pin the sites it happens to exercise, so the
// no-reintroduction half is a source scan, following the guard-test idiom cmd/lyx already uses.
//
// The scan reads this package's own production sources by name from the test's own working
// directory — no process is spawned and no scan root has to be resolved, so the Test Tier Purity
// Invariant is satisfied without an allowlist entry.
func TestShuttleengine_LiveSubstrateLoggingGoesThroughLogger(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("glob matched no .go files; the scan would pass vacuously")
	}

	scanned := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			t.Fatalf("read %s: %v", source, readErr)
		}
		scanned++
		for _, banned := range []string{"log.Printf(", "log.Println(", "log.Fatal", "fmt.Println("} {
			if strings.Contains(string(data), banned) {
				t.Errorf("%s uses %s — this package's operational messages are all about a real pane or provider process, so they belong on internal/logger (logger.Info for a normal spawn/teardown, logger.Warn for a retry or a teardown that did not confirm clean), whose durable Info+ trace sink the stdlib log package never reaches", source, banned)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no production source files scanned; the guard would pass vacuously")
	}
}

func TestRunner_Start_SweepErrorDoesNotBlockStart(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

	worktree := t.TempDir()
	// anchorPath is a real subpath of worktree here so reed's anchor-anchored
	// state lookup (as opposed to worktreeRoot) is actually observable.
	anchorPath := filepath.Join(worktree, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

	// Seed a corrupt reed.json so reedengine.LoadState errors during Start's
	// opportunistic orphan sweep — Start must log and continue rather than
	// fail the whole run over a housekeeping error.
	if err := os.MkdirAll(filepath.Join(anchorPath, lyxdirs.DotLyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "reed.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt reed.json: %v", err)
	}

	runner := NewRunner(reed, engine, anchorPath, worktree, cfg)
	run, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}})
	if err != nil {
		t.Fatalf("Start() error: %v, want sweep failure to be non-blocking", err)
	}
	if run == nil {
		t.Fatal("Start() returned nil run")
	}
}

func TestRunner_Start_SweepSkipsEntirelyOnReedStateReadError(t *testing.T) {
	// A LoadState ERROR must not degrade to "sweep with an empty live set":
	// that would delete every old-enough run dir, including a kept
	// asking/died/timeout dir reed still genuinely tracks, over an unrelated
	// I/O problem. It must skip the sweep for this Start entirely instead.
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

	// anchorPath is a real subpath of worktree, never the same value twice:
	// passing one directory for both fields would let a swapped NewRunner
	// argument pair pass this test, which is exactly the masking case
	// newTestRunner's own contract exists to prevent.
	worktree := t.TempDir()
	anchorPath := filepath.Join(worktree, "sub", "dir")
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

	if err := os.MkdirAll(filepath.Join(anchorPath, lyxdirs.DotLyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir .lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "reed.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt reed.json: %v", err)
	}

	// An old, kept run dir (as an asking/died/timeout outcome would leave
	// behind) whose strand is not in reed.json's live set — because reed.json
	// itself is unreadable, not because the strand is genuinely gone.
	shuttleRoot := runDirRoot(cfg, anchorPath)
	keptDir := seedRun(t, shuttleRoot, "kept-run", "some-other-strand")
	setDirMTime(t, keptDir, time.Now(), 10*time.Minute)

	runner := NewRunner(reed, engine, anchorPath, worktree, cfg)
	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if _, err := os.Stat(keptDir); err != nil {
		t.Errorf("kept run dir was removed despite a reed-state read error, want it preserved: %v", err)
	}
}

// TestRunner_Start_SweepSkipsEntirelyOnAbsentReedState is R3-F2's regression guard.
//
// An ABSENT reed.json used to fall through to a sweep with an EMPTY live-guid set, so every run dir
// past the age guard was deleted — including one whose agent is still working. That state is not
// exotic: it is exactly what reed's own corrupt-state error recommends ("delete <path> by hand to
// keep the session (its panes and their processes keep running, untracked)") and what a
// `git clean -xdf` of .lyx leaves behind. The sweep then removes the live run's events.jsonl (the
// file the Stop hook is still appending to) and its run.json, after which interrupt/send answer "is
// not a shuttle strand" for a running agent.
// Absence must skip the sweep for the same reason unreadability already does — see the sibling test
// above.
func TestRunner_Start_SweepSkipsEntirelyOnAbsentReedState(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "sess"}}

	worktree := t.TempDir()
	anchorPath := filepath.Join(worktree, "sub", "dir")
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5}

	// .lyx exists (reed created it) but holds NO reed.json — the hand-deleted / git-cleaned shape.
	if err := os.MkdirAll(filepath.Join(anchorPath, lyxdirs.DotLyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir .lyx: %v", err)
	}

	shuttleRoot := runDirRoot(cfg, anchorPath)
	liveRunDir := seedRun(t, shuttleRoot, "live-run", "still-running-strand")
	setDirMTime(t, liveRunDir, time.Now(), 10*time.Minute)

	runner := NewRunner(reed, engine, anchorPath, worktree, cfg)
	if _, err := runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}}); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if _, err := os.Stat(liveRunDir); err != nil {
		t.Errorf("run dir was swept with no reed.json to prove it orphaned, want it preserved: %v", err)
	}
}

// newInterruptTestRun returns a bare Run handle wired to reed/engine, with
// no Start/Wait machinery involved — Interrupt/Send only ever touch
// runner.reed and runner.engine through run.state.StrandGUID.
func newInterruptTestRun(t *testing.T, reed ReedOps, engine Engine) *Run {
	t.Helper()
	worktreeRoot := t.TempDir()
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, Config{})
	return &Run{
		runner: runner,
		state:  RunState{StrandGUID: "strand-1"},
	}
}

// liveStrandStatus scripts a fakeReed Status answer reporting strand-1 with
// the given liveness — what the Interrupt/Send liveness guard consumes.
func liveStrandStatus(live bool) []reedengine.StatusResult {
	return []reedengine.StatusResult{
		{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: live}}},
	}
}

// readyAgentEngine returns a fakeEngine whose Startup classification sticks
// at StartupReady — the pane state requireReadyAgentPane demands before any
// Interrupt/Send key is played, so tests of the key choreography itself pass
// the guard without scripting captures per call.
func readyAgentEngine() *fakeEngine {
	return &fakeEngine{StartupScript: []StartupState{StartupReady}}
}

func TestRun_Interrupt_PlaysEscape(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := readyAgentEngine()
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error: %v", err)
	}

	if len(reed.SendKeyCalls) != 1 || reed.SendKeyCalls[0].Key != "Escape" {
		t.Errorf("SendKey calls = %+v, want exactly one Escape", reed.SendKeyCalls)
	}
	if len(reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
	}
}

func TestRun_Send_RejectsNewlines(t *testing.T) {
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := &fakeEngine{}
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Send("line one\nline two"); err == nil {
		t.Fatal("Send() = nil error, want rejection for multiline text")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed calls = %v, want none (rejected before any reed call)", reed.CallLog)
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
			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			run := newInterruptTestRun(t, reed, &fakeEngine{})

			if err := run.Send(tt.text); err == nil {
				t.Fatalf("Send(%q) = nil error, want rejection for empty/whitespace text", tt.text)
			}
			if len(reed.CallLog) != 0 {
				t.Errorf("reed calls = %v, want none (rejected before any reed call)", reed.CallLog)
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
	// The first capture answers requireReadyAgentPane's TUI probe, the
	// second is sendVerified's pre-send baseline (both a ready pane without
	// the text yet); the later ones must report the sent text back for the
	// delivery check to succeed without a replay.
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ ", "❯ updated instructions"}}
	engine := readyAgentEngine()
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// Status then CapturePane lead the log: the liveness guard and the
	// ready-TUI probe must both run before any key reaches the pane; the
	// second CapturePane is the pre-send baseline snapshot. The final
	// CapturePane follows the text step: that is the delivery-verification
	// poll.
	wantLog := []string{"Status", "CapturePane", "CapturePane", "SendKey:Escape", "SendText:updated instructions", "CapturePane"}
	if !reflect.DeepEqual(reed.CallLog, wantLog) {
		t.Errorf("call order = %v, want %v", reed.CallLog, wantLog)
	}
	if len(reed.SendTextCalls) != 1 || !reed.SendTextCalls[0].Submit {
		t.Errorf("SendText calls = %+v, want one call with Submit=true", reed.SendTextCalls)
	}
}

func TestRun_Send_SwallowedFirstAttempt_ReplaySucceeds(t *testing.T) {
	// The provider TUI can swallow the whole Escape+text chunk with no
	// error anywhere (observed live): the pane capture never shows the text
	// after the first attempt, but does after the replay. sendVerified must
	// replay once and succeed, rather than trusting the first attempt.
	stubInputSleep(t)
	reed := &fakeReed{
		StatusQueue: liveStrandStatus(true),
		// The two leading captures answer the ready-TUI probe and the
		// pre-send baseline; every poll of the first attempt then sees no
		// trace of the text (still showing the prompt); the replay's polls
		// see it delivered.
		CaptureQueue: append(
			repeatCapture("❯ ", 2+sendVerifyAttempts),
			repeatCapture("❯ updated instructions", sendVerifyAttempts)...,
		),
	}
	engine := readyAgentEngine()
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error: %v, want the replay to succeed", err)
	}
	if len(reed.SendTextCalls) != 2 {
		t.Errorf("SendText calls = %d, want 2 (initial attempt + one replay)", len(reed.SendTextCalls))
	}
}

func TestRun_Send_NeverDelivered_ReportsHonestFailure(t *testing.T) {
	// If the text never appears even after the replay, Send must report a
	// delivery failure rather than the "keys were emitted" false ok:true
	// observed live.
	stubInputSleep(t)
	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repeatCapture("❯ ", 2*sendVerifyAttempts+2),
	}
	engine := readyAgentEngine()
	run := newInterruptTestRun(t, reed, engine)

	err := run.Send("updated instructions")
	if err == nil {
		t.Fatal("Send() = nil error, want a delivery-failure error")
	}
	if !strings.Contains(err.Error(), "never appeared") {
		t.Errorf("Send() error = %q, want it to name the delivery failure", err)
	}
	if len(reed.SendTextCalls) != 1+sendReplays {
		t.Errorf("SendText calls = %d, want %d (initial attempt + all replays exhausted)", len(reed.SendTextCalls), 1+sendReplays)
	}
}

func TestRun_Send_PreexistingText_RequiresNewOccurrence(t *testing.T) {
	// The sent text can already be on screen before the send — an operator
	// retrying the same instruction after an uncertain first attempt, or
	// text quoting the agent's own visible output. Presence alone would then
	// "verify" even a swallowed send; delivery must be judged by the
	// occurrence count RISING above the pre-send baseline.
	stubInputSleep(t)

	t.Run("SwallowedSend_ReportsFailure", func(t *testing.T) {
		// Every capture — probe, baseline, and all verification polls —
		// shows the text exactly once: it was already there, and the send
		// never lands. A presence check would falsely verify this.
		reed := &fakeReed{
			StatusQueue:  liveStrandStatus(true),
			CaptureQueue: []string{"❯ do it again"},
		}
		run := newInterruptTestRun(t, reed, readyAgentEngine())

		if err := run.Send("do it again"); err == nil {
			t.Fatal("Send() = nil error, want a delivery failure when the occurrence count never rises")
		}
	})

	t.Run("DeliveredSend_CountRises", func(t *testing.T) {
		// Probe and baseline see one pre-existing occurrence; the
		// verification polls see a second one appear — a genuine delivery.
		reed := &fakeReed{
			StatusQueue:  liveStrandStatus(true),
			CaptureQueue: []string{"❯ do it again", "❯ do it again", "do it again …\n❯ do it again"},
		}
		run := newInterruptTestRun(t, reed, readyAgentEngine())

		if err := run.Send("do it again"); err != nil {
			t.Fatalf("Send() error: %v, want the risen occurrence count to verify delivery", err)
		}
	})
}

// repeatCapture returns n copies of capture, the fixture shape fakeReed's
// CaptureQueue consumes one-per-call.
// TestRun_Send_BaselineOccurrencesScrolledAway_NoDuplicateDelivery is R2-F6's regression guard.
//
// CapturePane returns the pane's visible VIEWPORT, not its scrollback, so an occurrence counted at
// baseline time can scroll off while the agent works. The delivery check demanded a count strictly
// ABOVE that baseline forever, so once the baseline was inflated by copies that later scrolled away,
// no poll could ever satisfy it — every one of the 20 attempts failed and the try loop REPLAYED the
// whole ComposeSend choreography, typing the instruction into the pane a second time.
//
// A false negative here is not a harmless retry: it is a duplicate agent turn, from a send that
// actually landed. Sending the same text twice in one session is ordinary (an operator re-issuing a
// nudge, loom's repeated one-line pointers), which is what makes an inflated baseline reachable.
func TestRun_Send_BaselineOccurrencesScrolledAway_NoDuplicateDelivery(t *testing.T) {
	stubInputSleep(t)
	reed := &fakeReed{
		StatusQueue: liveStrandStatus(true),
		CaptureQueue: []string{
			// Ready-TUI probe and the pre-send baseline: the text is already on screen TWICE from
			// earlier turns, so baseline starts at 2.
			"❯ do it again do it again",
			"❯ do it again do it again",
			// First verification poll: the agent's output has pushed both earlier copies off the
			// top of the viewport, and the delivered copy is not rendered yet. Count 0.
			"❯ working...",
			// Second poll: the delivered copy is on screen. Count 1 — below the ORIGINAL baseline of
			// 2, which is exactly why the pre-fix check could never pass. The last queue entry
			// sticks, so every later poll sees this too.
			"❯ do it again",
		},
	}
	run := newInterruptTestRun(t, reed, readyAgentEngine())

	if err := run.Send("do it again"); err != nil {
		t.Fatalf("Send() error: %v; want the delivery recognised once the scrolled-away baseline is re-lowered", err)
	}
	if len(reed.SendTextCalls) != 1 {
		t.Errorf("SendText calls = %d; want exactly 1 — a replay here re-types an instruction the agent already received", len(reed.SendTextCalls))
	}
}

func repeatCapture(capture string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = capture
	}
	return out
}

func TestRun_InterruptAndSend_RefuseDeadOrUntrackedStrand(t *testing.T) {
	// tmux send-keys against a dead or missing pane exits 0 while
	// delivering nothing (proven live), so Interrupt/Send must refuse
	// before touching the pane rather than report a silent-no-op success.
	tests := []struct {
		name   string
		status []reedengine.StatusResult
		// wantIn is a phrase the refusal must name. The untracked case pins the
		// state-reset cause explicitly: reed's table is also emptied by a remove,
		// a down/up cycle, and a server rebirth, so a refusal claiming only that
		// "its run has completed and been cleaned up" misdirects an operator whose
		// agent is still running in its pane (proven live).
		wantIn string
	}{
		{"dead_pane", liveStrandStatus(false), "no live pane"},
		{"untracked_strand", []reedengine.StatusResult{{}}, "reed's strand table was reset under it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: tt.status}
			run := newInterruptTestRun(t, reed, &fakeEngine{})

			if err := run.Interrupt(); err == nil {
				t.Error("Interrupt() = nil error, want liveness refusal")
			} else if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("Interrupt() error = %v; want it to name %q", err, tt.wantIn)
			}
			if err := run.Send("still there?"); err == nil {
				t.Error("Send() = nil error, want liveness refusal")
			}
			if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
				t.Errorf("keys reached the pane despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
			}
		})
	}
}

func TestRun_InterruptAndSend_RefuseAgentlessShellPane(t *testing.T) {
	// A live pane is not enough: a provider that failed at launch (or was
	// killed while its shell survived) leaves a live pane whose keys land at
	// the shell prompt — proven live, a send against a kept "died" run
	// reported ok while its text was EXECUTED as a pwsh command. The engine
	// classifying every probe capture as still-booting (a bare shell prompt
	// shows no ready marker) must therefore refuse before any key is played.
	stubInputSleep(t)
	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: []string{"PS C:\\Code\\hub> "},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Interrupt(); err == nil || !strings.Contains(err.Error(), "no input-ready provider TUI") {
		t.Errorf("Interrupt() error = %v, want the no-ready-TUI refusal", err)
	}
	if err := run.Send("echo poked"); err == nil || !strings.Contains(err.Error(), "no input-ready provider TUI") {
		t.Errorf("Send() error = %v, want the no-ready-TUI refusal", err)
	}
	// The refusal must own BOTH readings of a non-Ready capture — a provider
	// still starting up and one that exited behind a surviving shell — since
	// the probe cannot distinguish them and misdiagnosing a booting pane as a
	// dead agent steers an operator toward tearing down a healthy run.
	if err := run.Interrupt(); err == nil || !strings.Contains(err.Error(), "still starting up") || !strings.Contains(err.Error(), "exited") {
		t.Errorf("Interrupt() error = %v, want it to name both the still-starting and exited readings", err)
	}
	if len(reed.SendKeyCalls) != 0 || len(reed.SendTextCalls) != 0 {
		t.Errorf("keys reached the agent-less shell despite refusal: SendKey=%+v SendText=%+v", reed.SendKeyCalls, reed.SendTextCalls)
	}
}

func TestRun_Interrupt_ReadyProbeRetriesTransientBoot(t *testing.T) {
	// One capture can land mid-redraw and classify a healthy TUI as still
	// booting; the guard must retry within agentPaneProbeAttempts and pass
	// once a later capture classifies Ready, rather than refusing on the
	// first inconclusive frame.
	stubInputSleep(t)
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupReady}}
	run := newInterruptTestRun(t, reed, engine)

	if err := run.Interrupt(); err != nil {
		t.Fatalf("Interrupt() error: %v, want the retried probe to pass", err)
	}
	if len(reed.SendKeyCalls) != 1 || reed.SendKeyCalls[0].Key != "Escape" {
		t.Errorf("SendKey calls = %+v, want exactly one Escape after the probe passes", reed.SendKeyCalls)
	}
}
