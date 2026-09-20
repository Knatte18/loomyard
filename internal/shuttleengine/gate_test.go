// gate_test.go covers the gate attempt loop end to end, driven over the package's existing
// fakeReed/fakeEngine fakes, newWaitTestRunner, newAttachTestRunner, and the fakeClock/multiStepClock
// seams — hermetic, untagged, no exec.Command, no gitexec, no hubforge.NewHub, no real sleeping, per
// the Test Tier Purity Invariant.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// appendEventsLine appends line (with a trailing newline) to path, standing in for the agent's next
// turn appending a fresh events.jsonl record.
func appendEventsLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open events file for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append events line: %v", err)
	}
}

// repromptCaptureSequence returns the n*3 fakeReed.CaptureQueue entries a test expects across n
// Send() calls that each succeed on their first delivery poll: per call, a probe capture (answers
// requireReadyAgentPane), a clean baseline capture (answers sendVerified's pre-send snapshot, holding
// no copy of the re-prompt needle), and a delivery capture (the re-prompt text itself, so the
// needle's count rises above the baseline's on the very first poll).
func repromptCaptureSequence(findingsPath string, n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "idle pane", "idle pane", gateRepromptText(findingsPath))
	}
	return out
}

// TestGate_PassesOnFirstDone pins the ungated-behaviour-preserving happy path: a gate that passes the
// very first time finalize evaluates it never sends a re-prompt, reports zero attempts, and leaves
// cleanup identical to an ungated run's.
func TestGate_PassesOnFirstDone(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}

	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || !result.Gate.Passed {
		t.Errorf("Gate = %+v, want a passed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", result.Gate.Attempts)
	}
	if gateCalls != 1 {
		t.Errorf("gate closure invoked %d times, want 1", gateCalls)
	}
	if len(reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
	}
}

// TestGate_FailsOnceThenPassesOnNextTurn covers the core re-prompt loop: a failed first attempt sends
// exactly one re-prompt naming the findings file, and the agent's next turn passing settles the run.
func TestGate_FailsOnceThenPassesOnNextTurn(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	findingsPath := filepath.Join(runDir, gateFindingsFileName)
	wantFindings := "fix the thing"
	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		if gateCalls == 1 {
			return GateResult{Passed: false, Findings: wantFindings}, nil
		}
		return GateResult{Passed: true}, nil
	}

	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repromptCaptureSequence(findingsPath, 1),
	}
	engine := readyAgentEngine()
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { appendEventsLine(t, eventsPath, "STOP:turn2") },
	}}
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour, KeepPane: true},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    mc,
		deadline: mc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || !result.Gate.Passed {
		t.Errorf("Gate = %+v, want a passed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Gate.Attempts)
	}
	if len(reed.SendTextCalls) != 1 {
		t.Fatalf("SendText calls = %+v, want exactly one", reed.SendTextCalls)
	}
	sent := reed.SendTextCalls[0].Text
	if strings.ContainsAny(sent, "\n\r") {
		t.Errorf("re-prompt text = %q, want a single line", sent)
	}
	if !strings.Contains(sent, findingsPath) {
		t.Errorf("re-prompt text = %q, want it to name the findings file %q", sent, findingsPath)
	}

	gotFindings, err := os.ReadFile(findingsPath)
	if err != nil {
		t.Fatalf("read findings file: %v", err)
	}
	if string(gotFindings) != wantFindings {
		t.Errorf("findings file = %q, want the first attempt's findings %q", gotFindings, wantFindings)
	}
}

// TestGate_FailsEveryAttempt_ExhaustsBudget covers the exhaustion path: a gate that never passes
// sends exactly the configured budget's worth of re-prompts and then settles Done with a failed
// verdict, rather than looping forever.
func TestGate_FailsEveryAttempt_ExhaustsBudget(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	findingsPath := filepath.Join(runDir, gateFindingsFileName)
	const budget = 2
	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: false, Findings: "still bad"}, nil
	}

	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repromptCaptureSequence(findingsPath, budget),
	}
	engine := readyAgentEngine()
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { appendEventsLine(t, eventsPath, "STOP:turn2") },
		func() { appendEventsLine(t, eventsPath, "STOP:turn3") },
	}}
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    mc,
		deadline: mc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate, Attempts: budget},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != budget {
		t.Errorf("Attempts = %d, want %d (the exhausted budget)", result.Gate.Attempts, budget)
	}
	if result.Gate != nil && result.Gate.FindingsPath != findingsPath {
		t.Errorf("FindingsPath = %q, want %q", result.Gate.FindingsPath, findingsPath)
	}
	if len(reed.SendTextCalls) != budget {
		t.Errorf("SendText calls = %d, want %d (exactly the budget, no more)", len(reed.SendTextCalls), budget)
	}
}

// TestGate_ClosureError covers the "a gate error is never not passed" decision: an infrastructure
// fault from the gate closure fails the run as an ordinary error, sends no re-prompt, and charges no
// attempt.
func TestGate_ClosureError(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	wantErr := errors.New("gate infrastructure fault")
	gate := func() (GateResult, error) { return GateResult{}, wantErr }

	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate},
	}

	_, err := run.Wait()
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("Wait() error = %v, want it to wrap %v", err, wantErr)
	}
	if len(reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
	}
	if run.gateSent != 0 {
		t.Errorf("gateSent = %d, want 0 (an infrastructure error charges no attempt)", run.gateSent)
	}
}

// TestGate_NoLiveSessionDonePaths covers the three checkLivenessTick-driven Done classifications a
// gated run can reach with no live session at all: not-tracked, not-live (a dead pane), and an
// expired startup window. Each finalizes through Wait's liveness branch rather than the events-tick
// Send loop, so each asserts a failed verdict, zero sends, and zero attempts charged.
func TestGate_NoLiveSessionDonePaths(t *testing.T) {
	tests := []struct {
		name          string
		statusQueue   []reedengine.StatusResult
		startupScript []StartupState
	}{
		{"not_tracked", []reedengine.StatusResult{{Strands: nil}}, nil},
		{"not_live_dead_pane", []reedengine.StatusResult{deadStatus("strand-1", "%1")}, nil},
		{"startup_window_expires", []reedengine.StatusResult{liveStatus("strand-1", "%1")}, []StartupState{StartupPending}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName) // never written: no Stop event arrives
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)

			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: false, Findings: "n/a"}, nil
			}

			reed := &fakeReed{StatusQueue: tt.statusQueue}
			engine := &fakeEngine{StartupScript: tt.startupScript}
			runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 0})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Hour),
				gate:     GateSpec{Gate: gate},
			}

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if result.Gate == nil || result.Gate.Passed {
				t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
			}
			if result.Gate != nil && result.Gate.Attempts != 0 {
				t.Errorf("Attempts = %d, want 0 (no live session ever received a re-prompt)", result.Gate.Attempts)
			}
			if gateCalls != 1 {
				t.Errorf("gate closure invoked %d times, want 1", gateCalls)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
		})
	}
}

// TestGate_RunDeadlineExpires_NoLiveSession covers the run deadline's own Done classification: a
// gate still runs once, through classifyDeadlineExpiry -> finalize, with no session ever addressed.
func TestGate_RunDeadlineExpires_NoLiveSession(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName) // never written
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)

	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: false, Findings: "n/a"}, nil
	}

	reed := &fakeReed{}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner: runner,
		spec:   Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir: runDir,
		state:  RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:  fc,
		// Already expired: the very first deadline check trips it.
		deadline: fc.Now().Add(-time.Minute),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", result.Gate.Attempts)
	}
	if gateCalls != 1 {
		t.Errorf("gate closure invoked %d times, want 1", gateCalls)
	}
	if len(reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
	}
}

// TestGate_FinishedDespiteMechanismFailure_NoLiveSession covers the events-unreadable mechanism-
// failure exit: a gate still runs once, through finishedDespiteMechanismFailure -> finalize.
func TestGate_FinishedDespiteMechanismFailure_NoLiveSession(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	if err := os.WriteFile(eventsPath, []byte("STOP:x\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)

	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: false, Findings: "n/a"}, nil
	}

	reed := &fakeReed{}
	engine := &fakeEngine{ParseEventsErr: errors.New("events file unparseable")}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", result.Gate.Attempts)
	}
	if gateCalls != 1 {
		t.Errorf("gate closure invoked %d times, want 1", gateCalls)
	}
	if len(reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
	}
}

// TestGate_EvaluateOncePerAttempt_Memoized pins that the gate runs exactly once per settling: a
// second evaluateGate() call within the same attempt reads the stored memo rather than re-invoking
// the closure.
func TestGate_EvaluateOncePerAttempt_Memoized(t *testing.T) {
	callCount := 0
	gate := func() (GateResult, error) {
		callCount++
		return GateResult{Passed: true}, nil
	}
	run := &Run{runDir: t.TempDir(), gate: GateSpec{Gate: gate}}

	first, err := run.evaluateGate()
	if err != nil {
		t.Fatalf("evaluateGate() error: %v", err)
	}
	second, err := run.evaluateGate()
	if err != nil {
		t.Fatalf("evaluateGate() error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("gate closure invoked %d times, want 1 (memoised per attempt)", callCount)
	}
	if first != second {
		t.Errorf("evaluateGate() = %+v then %+v, want the same memoised pointer both times", first, second)
	}
}

// TestGate_SendFailsMidLoop_EndsLoopWithAttemptsSoFar covers a re-prompt Send that itself fails: the
// loop ends there rather than retrying, and the failed send charges no attempt.
func TestGate_SendFailsMidLoop_EndsLoopWithAttemptsSoFar(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	gate := func() (GateResult, error) { return GateResult{Passed: false, Findings: "bad"}, nil }

	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: []string{"idle pane", "idle pane"},
		SendTextErr:  errors.New("pane swallowed input"),
	}
	engine := readyAgentEngine()
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0 (the failed send charges no attempt)", result.Gate.Attempts)
	}
	if len(reed.SendTextCalls) != 1 {
		t.Errorf("SendText calls = %d, want 1 (no retry after a failed send)", len(reed.SendTextCalls))
	}
}

// TestGate_DeadlineExpiresBetweenAttempts covers the run deadline expiring mid-loop, after one
// successful re-prompt: the loop stops, the deadline's own Done classification runs the gate once
// more, and the reported Attempts stays honest about the one send that preceded it.
func TestGate_DeadlineExpiresBetweenAttempts(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	findingsPath := filepath.Join(runDir, gateFindingsFileName)
	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: false, Findings: "still bad"}, nil
	}

	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repromptCaptureSequence(findingsPath, 1),
	}
	engine := readyAgentEngine()
	// PollIntervalMS's 5ms Sleep after the one successful re-prompt crosses the 2ms-out deadline
	// below, so the SECOND tick's deadline check trips with no further event ever appended.
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 5, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(2 * time.Millisecond),
		gate:     GateSpec{Gate: gate},
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Errorf("Gate = %+v, want a failed verdict", result.Gate)
	}
	if result.Gate != nil && result.Gate.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 (the one re-prompt sent before the deadline expired)", result.Gate.Attempts)
	}
	if gateCalls != 2 {
		t.Errorf("gate closure invoked %d times, want 2 (the failed attempt, then once more on the deadline's own Done)", gateCalls)
	}
	if len(reed.SendTextCalls) != 1 {
		t.Errorf("SendText calls = %d, want 1 (the deadline path never sends)", len(reed.SendTextCalls))
	}
}

// TestGate_ZeroGateSpec_RegressionGuard pins the zero-value GateSpec every ungated row supplies:
// Result.Gate stays nil and cleanup runs exactly as it did before the gate existed.
func TestGate_ZeroGateSpec_RegressionGuard(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
		// gate left at its zero value, exactly as Run/Attach's own delegation constructs it.
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if result.Gate != nil {
		t.Errorf("Gate = %+v, want nil for an ungated run", result.Gate)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
	}
}

// TestAttachGated_ThreadsGateThroughReconstructAndWait covers AttachGated's own contract: a resumed
// run is gated exactly as a fresh RunGated one is.
func TestAttachGated_ThreadsGateThroughReconstructAndWait(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", sessionID: "session-1",
		outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
	})
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}

	result, found, err := runner.AttachGated(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute}, GateSpec{Gate: gate})
	if err != nil {
		t.Fatalf("AttachGated() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("AttachGated() found = false; want true")
	}
	if result.Gate == nil || !result.Gate.Passed {
		t.Errorf("Gate = %+v; want a passed verdict, proving a resumed run is gated exactly as a fresh one is", result.Gate)
	}
	if gateCalls != 1 {
		t.Errorf("gate closure invoked %d times; want 1", gateCalls)
	}
}

// TestAttach_LeavesResultGateNil pins the ungated Attach delegation: a resumed run through the plain
// Attach entry point never carries a gate.
func TestAttach_LeavesResultGateNil(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", sessionID: "session-1",
		outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
	})
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatal("Attach() found = false; want true")
	}
	if result.Gate != nil {
		t.Errorf("Gate = %+v; want nil for the ungated Attach delegation", result.Gate)
	}
}
