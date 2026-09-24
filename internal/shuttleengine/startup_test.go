// startup_test.go covers the startup step (awaitStartup, abandonStartup) that Start/StartGated/
// RunGated now run before ever issuing a run handle: the pending-to-ready path with a trust-gate
// dismissal recorded, the already-ready and already-satisfied-file-contract fast paths, the
// not-ready teardown (pane-died and undismissable-gate refusal, both wrapping ErrNotStarted with the
// strand removed and the run dir kept), the status-retry-cap paths (transient recovery, exhaustion
// with and without a satisfied file contract, and the untracked-strand mechanism failure), the
// tick-count cap, the run-deadline-shorter-than-the-startup-window interaction, RunGated's own
// not-ready and mechanism-failure mappings, startupTickCap's own boundary arithmetic, and the
// probe-cadence-matches-Wait assertion. Every case runs on a fake clock; none sleeps on the real one.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// frozenClock never advances on its own: Now always returns the same fixed instant and Sleep is a
// no-op, which is what the tick-count-cap cases need — a clock that never advances so the loop can
// only ever be bounded by startupTickCap, never by the startup deadline.
type frozenClock struct {
	now time.Time
}

func (c *frozenClock) Now() time.Time      { return c.now }
func (c *frozenClock) Sleep(time.Duration) {}

var _ clock = (*frozenClock)(nil)

// flakyStatusReed embeds *fakeReed and overrides Status to return a scripted error for its first
// failFirst calls before delegating to the embedded fakeReed.Status — the double the retry-cap tests
// need to script a bounded run of transient reed.Status failures.
type flakyStatusReed struct {
	*fakeReed
	failFirst int
	calls     int
	err       error
}

func (r *flakyStatusReed) Status() (reedengine.StatusResult, error) {
	r.calls++
	if r.calls <= r.failFirst {
		return reedengine.StatusResult{}, r.err
	}
	return r.fakeReed.Status()
}

var _ ReedOps = (*flakyStatusReed)(nil)

// undismissableEngine embeds *fakeEngine and overrides TrustDismissSequence to return no inputs —
// the shape an implementation that cannot locate the gate's accepting option takes, per Engine's own
// doc comment.
type undismissableEngine struct {
	*fakeEngine
}

func (e *undismissableEngine) TrustDismissSequence(capture string) []PaneInput {
	return nil
}

var _ Engine = (*undismissableEngine)(nil)

// defaultStartupConfig is the tuning knob set most startup tests use: fast enough that a scripted
// probe sequence runs at zero real wall-clock cost, and a RunTimeoutMin generous enough that
// Spec.validate's zero-Timeout default never binds the run deadline before a test's own scripted
// startup outcome does — a zero result would put run.deadline at start time, which the startup
// step's own run-deadline check would then hit after the very first pending probe (only the
// run-deadline tests below deliberately let that binding happen, via an explicit small Spec.Timeout).
func defaultStartupConfig() Config {
	return Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30, RunTimeoutMin: 5}
}

// newStartupWorktree creates a fresh temp worktree/anchor pair, matching newTestRunner's shape
// (run_test.go): anchorPath a real subdirectory of worktreeRoot, never the same value.
func newStartupWorktree(t *testing.T) (anchorPath, worktreeRoot string) {
	t.Helper()
	worktreeRoot = t.TempDir()
	anchorPath = filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	return anchorPath, worktreeRoot
}

// newStartupTestRunner builds a real *Runner via NewRunner over a fresh temp worktree and anchor
// (newStartupWorktree's shape) with cfg, sets the runner's clock field to clk so no test sleeps on
// the real one, and seeds reed.AddStrandResult and engine.PrepareLaunch when they are still zero, so
// a test that has already scripted its own values is left alone.
func newStartupTestRunner(t *testing.T, reed *fakeReed, engine *fakeEngine, cfg Config, clk clock) (runner *Runner, anchorPath string) {
	t.Helper()
	if reed.AddStrandResult == (reedengine.Strand{}) {
		reed.AddStrandResult = reedengine.Strand{GUID: "strand-1"}
	}
	if engine.PrepareLaunch == (Launch{}) {
		engine.PrepareLaunch = Launch{Cmd: "cmd", SessionID: "session-1"}
	}
	var wt string
	anchorPath, wt = newStartupWorktree(t)
	runner = NewRunner(reed, engine, anchorPath, wt, cfg)
	runner.clock = clk
	return runner, anchorPath
}

// soleStartupRunDir returns the one run directory under cfg/anchorPath's run-dir root, failing the
// test if there is not exactly one — the way a test recovers the run directory a failed Start built,
// since a StartGated error return carries no *Run handle to read RunDir() from.
func soleStartupRunDir(t *testing.T, cfg Config, anchorPath string) string {
	t.Helper()
	root := runDirRoot(cfg, anchorPath)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read run dir root %s: %v", root, err)
	}
	if len(entries) != 1 {
		t.Fatalf("run dir root %s has %d entries; want exactly 1", root, len(entries))
	}
	return filepath.Join(root, entries[0].Name())
}

// TestStartup_TrustPromptThenReady covers the readiness path that also dismisses a trust gate along
// the way, driven through the real Start/StartGated flow: a handle is returned once StartupReady
// lands, run.json records Started/running, and the capture handed to TrustDismissSequence is the one
// Startup classified StartupTrustPrompt from.
func TestStartup_TrustPromptThenReady(t *testing.T) {
	const gateCapture = "❯ No, exit\n  Yes, I trust this folder"
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		CaptureQueue: []string{"", gateCapture},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupTrustPrompt, StartupReady}}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, defaultStartupConfig(), fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	buf := captureLoggerOutput(t)
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}

	foundEnter := false
	for _, c := range reed.SendKeyCalls {
		if c.GUID == "strand-1" && c.Key == "Enter" {
			foundEnter = true
		}
	}
	if !foundEnter {
		t.Errorf("SendKey(strand-1, Enter) not recorded (trust dismiss), calls = %+v", reed.SendKeyCalls)
	}
	captures := engine.TrustDismissCaptures()
	if len(captures) == 0 || captures[0] != gateCapture {
		t.Errorf("TrustDismissCaptures() = %v; want [%q]", captures, gateCapture)
	}

	rs, found, err := loadRunState(run.RunDir())
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if !rs.Started {
		t.Errorf("loadRunState().Started = false; want true")
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
	if !strings.Contains(buf.String(), "shuttle: dismissed startup gate") {
		t.Errorf("logger output = %q; want it to contain %q", buf.String(), "shuttle: dismissed startup gate")
	}
}

// TestStartup_ReadyOnFirstProbe covers the fast path: no dismissal ever attempted and no Sleep call
// before the handle is returned, proven by the virtual clock never advancing.
func TestStartup_ReadyOnFirstProbe(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, defaultStartupConfig(), fc)
	start := fc.Now()

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}
	if len(reed.SendKeyCalls) != 0 {
		t.Errorf("SendKeyCalls = %+v; want none", reed.SendKeyCalls)
	}
	if elapsed := fc.Now().Sub(start); elapsed != 0 {
		t.Errorf("virtual elapsed = %s; want exactly 0 — ready on the first probe must never Sleep before returning", elapsed)
	}
}

// TestStartup_PaneNotLiveMidStartup covers the pane going not-live before StartupReady: the run is
// torn down as ErrNotStarted, the strand is removed, run.json's Outcome is died, the run directory is
// kept, and startup-capture.txt holds the last successful capture.
func TestStartup_PaneNotLiveMidStartup(t *testing.T) {
	const lastCapture = "still booting..."
	reed := &fakeReed{
		StatusQueue: []reedengine.StatusResult{
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		},
		CaptureQueue: []string{lastCapture},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	fc := newFakeClock(time.Now())
	cfg := defaultStartupConfig()
	runner, anchorPath := newStartupTestRunner(t, reed, engine, cfg, fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}

	if len(reed.RemoveStrandCalls) != 1 || reed.RemoveStrandCalls[0].GUID != "strand-1" {
		t.Errorf("RemoveStrandCalls = %+v; want exactly one for strand-1", reed.RemoveStrandCalls)
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	rs, found, rerr := loadRunState(runDir)
	if rerr != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
	}
	if rs.Outcome != string(OutcomeDied) {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, OutcomeDied)
	}
	if _, statErr := os.Stat(runDir); statErr != nil {
		t.Errorf("run dir removed; want it kept: %v", statErr)
	}
	capture, rerr := os.ReadFile(filepath.Join(runDir, startupCaptureFileName))
	if rerr != nil {
		t.Fatalf("read %s: %v", startupCaptureFileName, rerr)
	}
	if string(capture) != lastCapture {
		t.Errorf("%s = %q; want %q", startupCaptureFileName, capture, lastCapture)
	}
}

// TestStartup_UndismissableGateUntilWindowExpires covers a gate whose accepting option cannot be
// located: no key is ever sent, and the startup window expires with the same not-ready teardown
// TestStartup_PaneNotLiveMidStartup pins.
func TestStartup_UndismissableGateUntilWindowExpires(t *testing.T) {
	innerReed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		CaptureQueue: []string{"❯ No, exit\n  Yes, I trust this folder"},
	}
	engine := &undismissableEngine{fakeEngine: &fakeEngine{StartupScript: []StartupState{StartupTrustPrompt}}}
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1, RunTimeoutMin: 5}
	fc := newFakeClock(time.Now())
	anchorPath, worktreeRoot := newStartupWorktree(t)
	innerReed.AddStrandResult = reedengine.Strand{GUID: "strand-1"}
	runner := NewRunner(innerReed, engine, anchorPath, worktreeRoot, cfg)
	runner.clock = fc

	outputFile := filepath.Join(t.TempDir(), "out.md")
	buf := captureLoggerOutput(t)
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}
	if len(innerReed.SendKeyCalls) != 0 {
		t.Errorf("SendKeyCalls = %+v; want none — the gate could not be dismissed", innerReed.SendKeyCalls)
	}
	if strings.Contains(buf.String(), "shuttle: dismissed startup gate") {
		t.Errorf("logger output = %q; want no dismissal line", buf.String())
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	rs, found, rerr := loadRunState(runDir)
	if rerr != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
	}
	if rs.Outcome != string(OutcomeDied) {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, OutcomeDied)
	}
	if len(innerReed.RemoveStrandCalls) != 1 {
		t.Errorf("RemoveStrandCalls = %+v; want exactly one", innerReed.RemoveStrandCalls)
	}
}

// TestStartup_FileContractSatisfiedWhilePending covers the file contract winning through
// classifyStartupWindow while the pane stays pending forever: a handle is returned and the strand is
// never torn down. The output file cannot exist before Start (Spec.validate refuses a pre-existing
// entry), so a multiStepClock writes it from inside the first Sleep call, between two probes.
func TestStartup_FileContractSatisfiedWhilePending(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1, RunTimeoutMin: 5}
	fc := newFakeClock(time.Now())
	anchorPath, worktreeRoot := newStartupWorktree(t)
	reed.AddStrandResult = reedengine.Strand{GUID: "strand-1"}
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { touchOutputFile(t, outputFile) },
	}}
	runner.clock = mc

	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil (file contract satisfied)", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrandCalls = %+v; want none", reed.RemoveStrandCalls)
	}
}

// TestStartup_OneTransientStatusErrorThenReady covers a single transient reed.Status failure
// recovering before the retry cap is reached.
func TestStartup_OneTransientStatusErrorThenReady(t *testing.T) {
	inner := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: 1, err: errors.New("reed status: transient")}
	inner.AddStrandResult = reedengine.Strand{GUID: "strand-1"}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	anchorPath, worktreeRoot := newStartupWorktree(t)
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, defaultStartupConfig())
	runner.clock = fc

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}
}

// TestStartup_StatusErrorsExhaustRetryCap_NoOutputFiles covers the retry-cap mechanism-failure path:
// a non-nil error that is NOT ErrNotStarted, no teardown, and run.json's Outcome left at running.
func TestStartup_StatusErrorsExhaustRetryCap_NoOutputFiles(t *testing.T) {
	scriptedErr := errors.New("reed status: unavailable")
	inner := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: maxStatusRetries, err: scriptedErr}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	cfg := defaultStartupConfig()
	anchorPath, worktreeRoot := newStartupWorktree(t)
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
	runner.clock = fc

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if err == nil {
		t.Fatal("StartGated() error = nil; want a non-nil error")
	}
	if !errors.Is(err, scriptedErr) {
		t.Errorf("StartGated() error = %v; want one wrapping %v", err, scriptedErr)
	}
	if errors.Is(err, ErrNotStarted) {
		t.Errorf("StartGated() error = %v; want it NOT to wrap ErrNotStarted -- this is a mechanism failure, not a not-ready teardown", err)
	}
	if len(inner.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrandCalls = %+v; want none -- a mechanism failure tears nothing down", inner.RemoveStrandCalls)
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	rs, found, rerr := loadRunState(runDir)
	if rerr != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q -- a mechanism failure writes no Outcome", rs.Outcome, runOutcomeRunning)
	}
}

// TestStartup_StatusErrorsExhaustRetryCap_OutputFilePresent covers the same exhaustion, but with the
// file contract already satisfied by the time the cap is reached: a handle is returned, not the
// error. The output file is written from inside the clock's Sleep, since Spec.validate refuses a
// pre-existing entry.
func TestStartup_StatusErrorsExhaustRetryCap_OutputFilePresent(t *testing.T) {
	scriptedErr := errors.New("reed status: unavailable")
	inner := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: maxStatusRetries + 5, err: scriptedErr}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	anchorPath, worktreeRoot := newStartupWorktree(t)
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, defaultStartupConfig())

	outputFile := filepath.Join(t.TempDir(), "out.md")
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { touchOutputFile(t, outputFile) },
	}}
	runner.clock = mc

	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil (file contract satisfied)", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}
}

// TestStartup_ReedNeverTracksStrand covers the untracked-strand mechanism failure.
func TestStartup_ReedNeverTracksStrand(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}}}},
	}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	cfg := defaultStartupConfig()
	runner, anchorPath := newStartupTestRunner(t, reed, engine, cfg, fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if err == nil {
		t.Fatal("StartGated() error = nil; want the untracked-strand mechanism error")
	}
	if !errors.Is(err, errStrandNotTracked) {
		t.Errorf("StartGated() error = %v; want one wrapping errStrandNotTracked", err)
	}
	if errors.Is(err, ErrNotStarted) {
		t.Errorf("StartGated() error = %v; want it NOT to wrap ErrNotStarted", err)
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrandCalls = %+v; want none", reed.RemoveStrandCalls)
	}
	_ = anchorPath
}

// TestStartup_TickCap covers the tick-count cap under a frozen clock, with a subtest for the
// file-contract-satisfied variant.
func TestStartup_TickCap(t *testing.T) {
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1, RunTimeoutMin: 5}

	t.Run("NoOutputFiles_ErrNotStartedWithTeardown", func(t *testing.T) {
		reed := &fakeReed{
			AddStrandResult: reedengine.Strand{GUID: "strand-1"},
			StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
		fc := &frozenClock{now: time.Now()}
		anchorPath, worktreeRoot := newStartupWorktree(t)
		runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
		runner.clock = fc

		outputFile := filepath.Join(t.TempDir(), "out.md")
		run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute}, GateSpec{})
		if run != nil {
			t.Errorf("StartGated() run = %+v; want nil", run)
		}
		if !errors.Is(err, ErrNotStarted) {
			t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
		}

		wantTicks := startupTickCap(time.Duration(cfg.StartupTimeoutS)*time.Second, pollInterval(cfg)*time.Duration(cfg.LivenessEveryNPolls))
		gotTicks := 0
		for _, c := range reed.CallLog {
			if c == "Status" {
				gotTicks++
			}
		}
		if gotTicks != wantTicks {
			t.Errorf("Status call count = %d; want %d (startupTickCap)", gotTicks, wantTicks)
		}
		if len(reed.RemoveStrandCalls) != 1 {
			t.Errorf("RemoveStrandCalls = %+v; want exactly one", reed.RemoveStrandCalls)
		}

		runDir := soleStartupRunDir(t, cfg, anchorPath)
		rs, found, rerr := loadRunState(runDir)
		if rerr != nil || !found {
			t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
		}
		if rs.Outcome != string(OutcomeDied) {
			t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, OutcomeDied)
		}
	})

	t.Run("OutputFilePresentAtCap_HandleReturned", func(t *testing.T) {
		reed := &fakeReed{
			AddStrandResult: reedengine.Strand{GUID: "strand-1"},
			StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
		fc := &frozenClock{now: time.Now()}
		anchorPath, worktreeRoot := newStartupWorktree(t)
		runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
		runner.clock = fc

		outputFile := filepath.Join(t.TempDir(), "out.md")
		// A frozen clock never advances via Sleep, and the run dir cannot be predicted before Start
		// returns, so the file is created via the engine's PrepareHook -- which runs synchronously
		// inside Start, before the startup probe's very first tick -- rather than a clock step.
		var createdOnce bool
		engine.PrepareHook = func(runDir string) {
			if createdOnce {
				return
			}
			createdOnce = true
			touchOutputFile(t, outputFile)
		}

		run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute}, GateSpec{})
		if err != nil {
			t.Fatalf("StartGated() error = %v; want nil (file contract satisfied at the cap)", err)
		}
		if run == nil {
			t.Fatal("StartGated() returned a nil run")
		}
	})
}

// TestStartupTickCap covers startupTickCap's own boundary arithmetic.
func TestStartupTickCap(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		interval time.Duration
		want     int
	}{
		{"zero_timeout", 0, time.Second, 1 + maxStatusRetries},
		{"negative_timeout", -5 * time.Second, time.Second, 1 + maxStatusRetries},
		{"exact_multiple", 10 * time.Second, 5 * time.Second, 2 + 1 + maxStatusRetries},
		{"needs_ceiling", 11 * time.Second, 5 * time.Second, 3 + 1 + maxStatusRetries},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := startupTickCap(tt.timeout, tt.interval)
			if got != tt.want {
				t.Errorf("startupTickCap(%v, %v) = %d; want %d", tt.timeout, tt.interval, got, tt.want)
			}
		})
	}
}

// TestStartup_ProbeCadenceMatchesWait pins that the startup step probes at pollInterval x
// LivenessEveryNPolls, never at the bare poll interval: with PollIntervalMS 100 and
// LivenessEveryNPolls 10, a ready-on-third-Status-call script must advance the virtual clock by
// exactly two probe intervals (2 x 100ms x 10 = 2s), never by a bare 100ms step.
func TestStartup_ProbeCadenceMatchesWait(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupPending, StartupReady}}
	cfg := Config{PollIntervalMS: 100, LivenessEveryNPolls: 10, StartupTimeoutS: 30, RunTimeoutMin: 5}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, cfg, fc)
	start := fc.Now()

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error = %v; want nil", err)
	}
	if run == nil {
		t.Fatal("StartGated() returned a nil run")
	}

	statusCalls := 0
	for _, c := range reed.CallLog {
		if c == "Status" {
			statusCalls++
		}
	}
	if statusCalls != 3 {
		t.Errorf("Status call count = %d; want 3", statusCalls)
	}
	wantElapsed := 2 * 100 * time.Millisecond * 10
	if elapsed := fc.Now().Sub(start); elapsed != wantElapsed {
		t.Errorf("virtual elapsed = %s; want exactly %s (two probe intervals, not a bare poll-interval step)", elapsed, wantElapsed)
	}
}

// TestStartup_CaptureAlwaysErroringUntilWindowExpires covers a pane whose CapturePane always fails:
// no capture is ever recorded, so the not-ready teardown saves nothing and says so.
func TestStartup_CaptureAlwaysErroringUntilWindowExpires(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		CaptureErr:      errors.New("tmux: capture-pane failed"),
	}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1, RunTimeoutMin: 5}
	fc := newFakeClock(time.Now())
	anchorPath, worktreeRoot := newStartupWorktree(t)
	runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
	runner.clock = fc

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}
	if !strings.Contains(err.Error(), "no pane capture was saved") {
		t.Errorf("StartGated() error = %v; want it to say no pane capture was saved", err)
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	if _, statErr := os.Stat(filepath.Join(runDir, startupCaptureFileName)); !os.IsNotExist(statErr) {
		t.Errorf("%s exists; want it absent (no capture ever succeeded)", startupCaptureFileName)
	}
}

// TestStartup_RemoveStrandFailureDuringTeardown covers a teardown whose own strand removal fails:
// ErrNotStarted is still reported, the message says the strand could not be removed and carries
// reed's own error text, and run.json's Outcome is still persisted.
func TestStartup_RemoveStrandFailureDuringTeardown(t *testing.T) {
	removeErr := errors.New("tmux: kill-pane failed")
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue: []reedengine.StatusResult{
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		},
		RemoveStrandErr: removeErr,
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	cfg := defaultStartupConfig()
	runner, anchorPath := newStartupTestRunner(t, reed, engine, cfg, fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}
	if !strings.Contains(err.Error(), "could NOT be removed") || !strings.Contains(err.Error(), removeErr.Error()) {
		t.Errorf("StartGated() error = %v; want it to say the strand could not be removed and carry %q", err, removeErr.Error())
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	rs, found, rerr := loadRunState(runDir)
	if rerr != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, rerr)
	}
	if rs.Outcome != string(OutcomeDied) {
		t.Errorf("loadRunState().Outcome = %q; want %q -- the Outcome write is unaffected by the removal failing", rs.Outcome, OutcomeDied)
	}
}

// TestStartup_KeepPaneOnNotReadyStart pins that Spec.KeepPane is irrelevant to the not-ready
// teardown: the strand is removed whatever KeepPane says, since KeepPane governs a COMPLETED run's
// pane retention, not a startup failure's.
func TestStartup_KeepPaneOnNotReadyStart(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue: []reedengine.StatusResult{
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, defaultStartupConfig(), fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, KeepPane: true}, GateSpec{})
	if run != nil {
		t.Errorf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}
	if len(reed.RemoveStrandCalls) != 1 {
		t.Errorf("RemoveStrandCalls = %+v; want exactly one -- KeepPane does not apply to a not-ready teardown", reed.RemoveStrandCalls)
	}
}

// TestStartup_RunGated_NotReady pins RunGated's own not-ready mapping: errors.Is(err, ErrNotStarted)
// is swallowed into a died Result with a nil error, carrying the run's identity, and the gate closure
// never runs (its counter stays at zero) since finalize never evaluates a gate for a non-Done
// outcome.
func TestStartup_RunGated_NotReady(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue: []reedengine.StatusResult{
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, defaultStartupConfig(), fc)

	gateCalls := 0
	gate := GateSpec{Gate: func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}}

	outputFile := filepath.Join(t.TempDir(), "out.md")
	result, err := runner.RunGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, gate)
	if err != nil {
		t.Fatalf("RunGated() error = %v; want nil (a not-ready start is not surfaced as an error)", err)
	}
	if result.Outcome != OutcomeDied {
		t.Errorf("RunGated() Outcome = %q; want %q", result.Outcome, OutcomeDied)
	}
	if result.StrandGUID != "strand-1" || result.RunDir == "" || result.SessionID == "" {
		t.Errorf("RunGated() result = %+v; want StrandGUID/RunDir/SessionID all populated", result)
	}
	if gateCalls != 0 {
		t.Errorf("gate closure called %d time(s); want 0", gateCalls)
	}
}

// TestStartup_RunGated_MechanismFailure pins RunGated's mapping for a startup mechanism failure
// (never ErrNotStarted): the error is returned unchanged, alongside a Result carrying the run's
// identity fields with an empty Outcome.
func TestStartup_RunGated_MechanismFailure(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}}}},
	}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	runner, _ := newStartupTestRunner(t, reed, engine, defaultStartupConfig(), fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	result, err := runner.RunGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err == nil {
		t.Fatal("RunGated() error = nil; want the mechanism-failure error")
	}
	if errors.Is(err, ErrNotStarted) {
		t.Errorf("RunGated() error = %v; want it NOT to wrap ErrNotStarted", err)
	}
	if result.Outcome != "" {
		t.Errorf("RunGated() Outcome = %q; want empty (no classification was ever reached)", result.Outcome)
	}
	if result.StrandGUID != "strand-1" || result.RunDir == "" || result.SessionID == "" {
		t.Errorf("RunGated() result = %+v; want StrandGUID/RunDir/SessionID all populated", result)
	}
}

// TestStartup_RunDeadlineShorterThanWindow_NeverReady pins the run-deadline-anchoring decision: a
// Spec.Timeout far shorter than startup_timeout_s must still expire the run on its OWN schedule,
// classified OutcomeTimeout, never OutcomeDied from the (much longer) startup window.
func TestStartup_RunDeadlineShorterThanWindow_NeverReady(t *testing.T) {
	newRig := func(t *testing.T) (*Runner, *fakeReed, time.Time) {
		reed := &fakeReed{
			AddStrandResult: reedengine.Strand{GUID: "strand-1"},
			StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
		cfg := Config{PollIntervalMS: 500, LivenessEveryNPolls: 1, StartupTimeoutS: 3600, RunTimeoutMin: 5}
		fc := newFakeClock(time.Now())
		anchorPath, worktreeRoot := newStartupWorktree(t)
		runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
		runner.clock = fc
		return runner, reed, fc.Now()
	}

	t.Run("StartGated", func(t *testing.T) {
		runner, reed, start := newRig(t)
		fc := runner.clock.(*fakeClock)
		outputFile := filepath.Join(t.TempDir(), "out.md")
		specTimeout := 2 * time.Second
		run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: specTimeout}, GateSpec{})
		if run != nil {
			t.Errorf("StartGated() run = %+v; want nil", run)
		}
		if !errors.Is(err, ErrNotStarted) {
			t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
		}
		if len(reed.RemoveStrandCalls) != 1 {
			t.Errorf("RemoveStrandCalls = %+v; want exactly one -- the not-ready teardown still runs", reed.RemoveStrandCalls)
		}
		if elapsed := fc.Now().Sub(start); elapsed < specTimeout {
			t.Errorf("virtual elapsed = %s; want at least Spec.Timeout (%s) -- the run deadline, not the startup window, must be what expired", elapsed, specTimeout)
		}
	})

	t.Run("RunGated", func(t *testing.T) {
		runner, _, start := newRig(t)
		fc := runner.clock.(*fakeClock)
		outputFile := filepath.Join(t.TempDir(), "out.md")
		specTimeout := 2 * time.Second
		result, err := runner.RunGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: specTimeout}, GateSpec{})
		if err != nil {
			t.Fatalf("RunGated() error = %v; want nil", err)
		}
		if result.Outcome != OutcomeTimeout {
			t.Errorf("RunGated() Outcome = %q; want %q -- the run's own deadline expired, not the startup window", result.Outcome, OutcomeTimeout)
		}
		elapsed := fc.Now().Sub(start)
		// "Near Spec.Timeout" rather than exact: the loop only samples the deadline once per probe
		// interval (500ms here), so it can overshoot by up to one interval, but must land nowhere
		// close to the 3600s startup window.
		if elapsed < specTimeout || elapsed > specTimeout+5*time.Second {
			t.Errorf("virtual elapsed = %s; want it near Spec.Timeout (%s), not the startup window", elapsed, specTimeout)
		}
	})
}

// TestStartup_RunDeadlineShorterThanWindow_OutputFilePresentAtDeadline covers the same shorter-
// deadline shape with the file contract satisfied by the time the run deadline arrives: a handle is
// returned from StartGated, and RunGated goes on to classify OutcomeDone through the same run
// deadline exit.
func TestStartup_RunDeadlineShorterThanWindow_OutputFilePresentAtDeadline(t *testing.T) {
	newRig := func(t *testing.T, outputFile string) *Runner {
		reed := &fakeReed{
			AddStrandResult: reedengine.Strand{GUID: "strand-1"},
			StatusQueue:     []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}},
		}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
		cfg := Config{PollIntervalMS: 500, LivenessEveryNPolls: 1, StartupTimeoutS: 3600, RunTimeoutMin: 5}
		fc := newFakeClock(time.Now())
		anchorPath, worktreeRoot := newStartupWorktree(t)
		runner := NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
		mc := &multiStepClock{fakeClock: fc, steps: []func(){
			func() { touchOutputFile(t, outputFile) },
		}}
		runner.clock = mc
		return runner
	}

	t.Run("StartGated_HandleReturned", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "out.md")
		runner := newRig(t, outputFile)
		run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: 2 * time.Second}, GateSpec{})
		if err != nil {
			t.Fatalf("StartGated() error = %v; want nil (file contract satisfied at the run deadline)", err)
		}
		if run == nil {
			t.Fatal("StartGated() returned a nil run")
		}
	})

	t.Run("RunGated_ClassifiesDone", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "out.md")
		runner := newRig(t, outputFile)
		result, err := runner.RunGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}, Timeout: 2 * time.Second}, GateSpec{})
		if err != nil {
			t.Fatalf("RunGated() error = %v; want nil", err)
		}
		if result.Outcome != OutcomeDone {
			t.Errorf("RunGated() Outcome = %q; want %q", result.Outcome, OutcomeDone)
		}
	})
}

// TestStartup_FailedStartThenAttach_RespawnEligible covers the seam between card 4's not-ready
// teardown and card 1's terminal-Outcome respawn rule: a failed start leaves a died run.json behind
// with the strand removed, and a subsequent Attach over the same output files -- with reed rescripted
// to no longer track the strand, and its state file now present -- does not harvest it as a leftover
// to wait on (found == false, nil error), since a terminal Outcome makes it respawn-eligible
// regardless of its directory's age.
func TestStartup_FailedStartThenAttach_RespawnEligible(t *testing.T) {
	reed := &fakeReed{
		AddStrandResult: reedengine.Strand{GUID: "strand-1"},
		StatusQueue: []reedengine.StatusResult{
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}},
			{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}},
		},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}, PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	fc := newFakeClock(time.Now())
	cfg := defaultStartupConfig()
	runner, anchorPath := newStartupTestRunner(t, reed, engine, cfg, fc)

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if run != nil {
		t.Fatalf("StartGated() run = %+v; want nil", run)
	}
	if !errors.Is(err, ErrNotStarted) {
		t.Fatalf("StartGated() error = %v; want one wrapping ErrNotStarted", err)
	}

	runDir := soleStartupRunDir(t, cfg, anchorPath)
	// The run dir is younger than 2*StartupTimeoutS -- the leftover-then-age rule's own minAge --
	// which is what proves the terminal-Outcome check, not the age escape, is what makes this
	// respawn-eligible.
	if info, statErr := os.Stat(runDir); statErr != nil || time.Since(info.ModTime()) >= 2*time.Duration(cfg.StartupTimeoutS)*time.Second {
		t.Fatalf("run dir age check invalid: statErr=%v", statErr)
	}

	// Reed no longer tracks the strand (it was just removed); rescript Status() to reflect that for
	// Attach's own liveness read, and seed a present reed state file for the first gate.
	reed.StatusQueue = []reedengine.StatusResult{{Strands: nil}}
	dotLyxDir := filepath.Join(anchorPath, lyxdirs.DotLyxDirName)
	seedPresentReedState(t, dotLyxDir)

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if found {
		t.Errorf("Attach() found = true; want false -- a terminal-Outcome record is respawn-eligible, not attachable")
	}
	if result != (Result{}) {
		t.Errorf("Attach() result = %+v; want zero Result", result)
	}
}
