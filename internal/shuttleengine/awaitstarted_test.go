// awaitstarted_test.go covers Run.AwaitStarted: the pending-to-ready path with a trust-gate dismissal
// recorded, the already-ready and already-satisfied-file-contract fast paths, the pane-died and
// undismissable-gate refusal paths, the status-retry-cap paths (transient recovery, exhaustion with
// and without a satisfied file contract, and the untracked-strand mechanism failure), the tick-count
// cap, the attached-and-started short-circuit, awaitStartedTickCap's own boundary arithmetic, and the
// probe-cadence-matches-Wait assertion. Every case runs on a fake clock; none sleeps on the real one.

package shuttleengine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// frozenClock never advances on its own: Now always returns the same fixed instant and Sleep is a
// no-op, which is what the tick-count-cap cases need — a clock that never advances so the loop can
// only ever be bounded by awaitStartedTickCap, never by the startup deadline.
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

// newAwaitStartedTestRun builds a *Run the way TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded
// does: runner, spec (OutputFiles naming a file under a fresh run directory), runDir, state (with
// StrandGUID and Outcome seeded runOutcomeRunning), and clock. It also seeds the run directory's
// run.json from state, so the run.json assertions each test makes have a baseline to compare against.
func newAwaitStartedTestRun(t *testing.T, reed ReedOps, engine Engine, cfg Config, clk clock) (*Run, string) {
	t.Helper()
	runDir := t.TempDir()
	outputFile := filepath.Join(runDir, "out.md")
	state := RunState{StrandGUID: "strand-1", EventsPath: filepath.Join(runDir, "events.jsonl"), Outcome: runOutcomeRunning}
	if err := saveRunState(runDir, state); err != nil {
		t.Fatalf("saveRunState: %v", err)
	}
	runner := newWaitTestRunner(t, reed, engine, cfg)
	return &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    state,
		clock:    clk,
		deadline: clk.Now().Add(time.Minute),
	}, outputFile
}

func defaultAwaitStartedConfig() Config {
	return Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30}
}

// TestAwaitStarted_PendingThenTrustPromptThenReady covers the readiness path that also dismisses a
// trust gate along the way.
func TestAwaitStarted_PendingThenTrustPromptThenReady(t *testing.T) {
	const gateCapture = "❯ No, exit\n  Yes, I trust this folder"
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
		CaptureQueue: []string{"", gateCapture},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupTrustPrompt, StartupReady}}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, defaultAwaitStartedConfig(), fc)

	buf := captureLoggerOutput(t)
	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
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

	rs, found, err := loadRunState(run.runDir)
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

// TestAwaitStarted_ReadyOnFirstTick covers the fast path: no dismissal ever attempted.
func TestAwaitStarted_ReadyOnFirstTick(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, defaultAwaitStartedConfig(), fc)

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
	}
	if len(reed.SendKeyCalls) != 0 {
		t.Errorf("SendKeyCalls = %+v; want none", reed.SendKeyCalls)
	}
	if len(engine.TrustDismissCaptures()) != 0 {
		t.Errorf("TrustDismissCaptures() = %v; want none", engine.TrustDismissCaptures())
	}
}

// TestAwaitStarted_PaneNotLiveMidStartup_NoOutputFiles covers the pane going not-live before
// StartupReady, with the file contract unsatisfied.
func TestAwaitStarted_PaneNotLiveMidStartup_NoOutputFiles(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{
		{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}},
		{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}},
	}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, defaultAwaitStartedConfig(), fc)

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if started {
		t.Errorf("AwaitStarted() started = true; want false")
	}

	rs, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
}

// TestAwaitStarted_UndismissableGate covers a gate whose accepting option cannot be located: no key
// is ever sent, and the startup window expires with the pane never reaching StartupReady.
func TestAwaitStarted_UndismissableGate(t *testing.T) {
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
		CaptureQueue: []string{"❯ No, exit\n  Yes, I trust this folder"},
	}
	engine := &undismissableEngine{fakeEngine: &fakeEngine{StartupScript: []StartupState{StartupTrustPrompt}}}
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, cfg, fc)
	run.deadline = fc.Now().Add(10 * time.Minute) // run deadline stays well beyond the startup window
	start := fc.Now()

	buf := captureLoggerOutput(t)
	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if started {
		t.Errorf("AwaitStarted() started = true; want false")
	}
	if len(reed.SendKeyCalls) != 0 {
		t.Errorf("SendKeyCalls = %+v; want none — the gate could not be dismissed", reed.SendKeyCalls)
	}
	maxElapsed := time.Duration(cfg.StartupTimeoutS)*time.Second + pollInterval(cfg)*time.Duration(cfg.LivenessEveryNPolls)
	if elapsed := fc.Now().Sub(start); elapsed > maxElapsed {
		t.Errorf("virtual elapsed time = %s; want at most the startup timeout plus one probe interval (%s)", elapsed, maxElapsed)
	}
	if strings.Contains(buf.String(), "shuttle: dismissed startup gate") {
		t.Errorf("logger output = %q; want no dismissal line", buf.String())
	}

	rs, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
}

// TestAwaitStarted_OutputFilesPresent_PanePendingForever covers the file contract winning through
// classifyStartupWindow while the pane never becomes ready.
func TestAwaitStarted_OutputFilesPresent_PanePendingForever(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
	fc := newFakeClock(time.Now())
	run, outputFile := newAwaitStartedTestRun(t, reed, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1}, fc)
	touchOutputFile(t, outputFile)

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true (file contract satisfied)")
	}

	rs, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
}

// TestAwaitStarted_OneTransientStatusErrorThenReady covers a single transient reed.Status failure
// recovering before the retry cap is reached.
func TestAwaitStarted_OneTransientStatusErrorThenReady(t *testing.T) {
	inner := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: 1, err: errors.New("reed status: transient")}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, defaultAwaitStartedConfig(), fc)

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
	}

	rs, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
}

// TestAwaitStarted_StatusErrorsExhaustRetryCap_NoOutputFiles covers the retry-cap error path.
func TestAwaitStarted_StatusErrorsExhaustRetryCap_NoOutputFiles(t *testing.T) {
	scriptedErr := errors.New("reed status: unavailable")
	inner := &fakeReed{}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: maxStatusRetries, err: scriptedErr}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, &fakeEngine{}, defaultAwaitStartedConfig(), fc)

	started, err := run.AwaitStarted()
	if err == nil {
		t.Fatalf("AwaitStarted() = (%v, nil); want a non-nil error", started)
	}
	if !errors.Is(err, scriptedErr) {
		t.Errorf("AwaitStarted() error = %v; want one wrapping %v", err, scriptedErr)
	}
	if started {
		t.Errorf("AwaitStarted() started = true; want false")
	}

	rs, found, err := loadRunState(run.runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	if rs.Outcome != runOutcomeRunning {
		t.Errorf("loadRunState().Outcome = %q; want %q", rs.Outcome, runOutcomeRunning)
	}
}

// TestAwaitStarted_StatusErrorsExhaustRetryCap_OutputFilePresent covers the same exhaustion, but with
// the file contract already satisfied: (true, nil), not the error.
func TestAwaitStarted_StatusErrorsExhaustRetryCap_OutputFilePresent(t *testing.T) {
	scriptedErr := errors.New("reed status: unavailable")
	inner := &fakeReed{}
	reed := &flakyStatusReed{fakeReed: inner, failFirst: maxStatusRetries, err: scriptedErr}
	fc := newFakeClock(time.Now())
	run, outputFile := newAwaitStartedTestRun(t, reed, &fakeEngine{}, defaultAwaitStartedConfig(), fc)
	touchOutputFile(t, outputFile)

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v; want nil (file contract satisfied)", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
	}
}

// TestAwaitStarted_ReedNeverTracksStrand_NoOutputFiles covers the untracked-strand mechanism failure.
func TestAwaitStarted_ReedNeverTracksStrand_NoOutputFiles(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}}}}}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, &fakeEngine{}, defaultAwaitStartedConfig(), fc)

	started, err := run.AwaitStarted()
	if err == nil {
		t.Fatalf("AwaitStarted() = (%v, nil); want the untracked-strand mechanism error", started)
	}
	if !errors.Is(err, errStrandNotTracked) {
		t.Errorf("AwaitStarted() error = %v; want one wrapping errStrandNotTracked", err)
	}
	if started {
		t.Errorf("AwaitStarted() started = true; want false")
	}
}

// TestAwaitStarted_TickCountCap covers the tick-count cap under a frozen clock, with a subtest for
// the file-contract-satisfied variant.
func TestAwaitStarted_TickCountCap(t *testing.T) {
	cfg := Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1}

	t.Run("NoOutputFiles", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
		fc := &frozenClock{now: time.Now()}
		run, _ := newAwaitStartedTestRun(t, reed, engine, cfg, fc)
		run.deadline = fc.Now().Add(10 * time.Minute)

		started, err := run.AwaitStarted()
		if err != nil {
			t.Fatalf("AwaitStarted() error: %v", err)
		}
		if started {
			t.Errorf("AwaitStarted() started = true; want false")
		}

		wantTicks := awaitStartedTickCap(time.Duration(cfg.StartupTimeoutS)*time.Second, pollInterval(cfg)*time.Duration(cfg.LivenessEveryNPolls))
		gotTicks := 0
		for _, c := range reed.CallLog {
			if c == "Status" {
				gotTicks++
			}
		}
		if gotTicks != wantTicks {
			t.Errorf("Status call count = %d; want %d (awaitStartedTickCap)", gotTicks, wantTicks)
		}
	})

	t.Run("OutputFilePresentAtCap", func(t *testing.T) {
		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupPending}}
		fc := &frozenClock{now: time.Now()}
		run, outputFile := newAwaitStartedTestRun(t, reed, engine, cfg, fc)
		run.deadline = fc.Now().Add(10 * time.Minute)
		touchOutputFile(t, outputFile)

		started, err := run.AwaitStarted()
		if err != nil {
			t.Fatalf("AwaitStarted() error: %v", err)
		}
		if !started {
			t.Errorf("AwaitStarted() started = false; want true (file contract satisfied at the cap)")
		}
	})
}

// TestAwaitStarted_AttachedAndStarted_ShortCircuits covers the run.attached && run.state.Started
// short-circuit: no Status call at all.
func TestAwaitStarted_AttachedAndStarted_ShortCircuits(t *testing.T) {
	reed := &fakeReed{}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, &fakeEngine{}, defaultAwaitStartedConfig(), fc)
	run.attached = true
	run.state.Started = true

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
	}
	if len(reed.CallLog) != 0 {
		t.Errorf("reed.CallLog = %v; want none — the short-circuit must never call reed", reed.CallLog)
	}
}

// TestAwaitStartedTickCap covers awaitStartedTickCap's own boundary arithmetic.
func TestAwaitStartedTickCap(t *testing.T) {
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
			got := awaitStartedTickCap(tt.timeout, tt.interval)
			if got != tt.want {
				t.Errorf("awaitStartedTickCap(%v, %v) = %d; want %d", tt.timeout, tt.interval, got, tt.want)
			}
		})
	}
}

// TestAwaitStarted_ProbeCadenceMatchesWait pins that AwaitStarted probes at pollInterval x
// LivenessEveryNPolls, never at the bare poll interval: with PollIntervalMS 100 and
// LivenessEveryNPolls 10, a ready-on-third-Status-call script must advance the virtual clock by
// exactly two probe intervals (2 x 100ms x 10 = 2s), never by a bare 100ms step.
func TestAwaitStarted_ProbeCadenceMatchesWait(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupPending, StartupReady}}
	cfg := Config{PollIntervalMS: 100, LivenessEveryNPolls: 10, StartupTimeoutS: 30}
	fc := newFakeClock(time.Now())
	run, _ := newAwaitStartedTestRun(t, reed, engine, cfg, fc)
	start := fc.Now()

	started, err := run.AwaitStarted()
	if err != nil {
		t.Fatalf("AwaitStarted() error: %v", err)
	}
	if !started {
		t.Errorf("AwaitStarted() started = false; want true")
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
