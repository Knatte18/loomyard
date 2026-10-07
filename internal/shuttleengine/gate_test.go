// gate_test.go covers the gate attempt loop end to end, driven over the package's existing
// fakeReed/fakeEngine fakes, newFixture, and the fakeClock/multiStepClock
// seams — hermetic, untagged: it spawns no external process, builds no real fixture hub, and never
// sleeps for real, per the Test Tier Purity Invariant.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

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

// TestGate_FirstDoneSettlesWithoutReprompt pins the ungated-behaviour-preserving happy path: a gate
// that passes the very first time finalize evaluates it never sends a re-prompt, reports zero
// attempts, and leaves cleanup identical to an ungated run's; a zero-value GateSpec, which every
// ungated run supplies, leaves Result.Gate nil and cleans up the same way.
//
//testtiming:keep pins that a gate passing on the first Done sends nothing and charges no attempt, that a zero GateSpec leaves Result.Gate nil, and that both clean the run dir
func TestGate_FirstDoneSettlesWithoutReprompt(t *testing.T) {
	tests := []struct {
		name  string
		gated bool
	}{
		{name: "gate passes the first time", gated: true},
		{name: "zero gate spec leaves Result.Gate nil", gated: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			opts := []runOpt{
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Hour)),
			}
			if tt.gated {
				opts = append(opts, withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))
			}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour}, opts...)

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}
			if tt.gated {
				if result.Gate == nil || !result.Gate.Passed {
					t.Errorf("Gate = %+v, want a passed verdict", result.Gate)
				}
				if result.Gate != nil && result.Gate.Attempts != 0 {
					t.Errorf("Attempts = %d, want 0", result.Gate.Attempts)
				}
				if gateCalls != 1 {
					t.Errorf("gate closure invoked %d times, want 1", gateCalls)
				}
			} else if result.Gate != nil {
				t.Errorf("Gate = %+v, want nil for an ungated run", result.Gate)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
			if _, err := os.Stat(runDir); !os.IsNotExist(err) {
				t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
			}
		})
	}
}

// TestGate_FailsOnceThenPassesOnNextTurn covers the core re-prompt loop: a failed first attempt sends
// exactly one re-prompt naming the findings file, and the agent's next turn passing settles the run.
//
//testtiming:keep pins that the re-prompt is a single line naming the findings file, and that the findings file holds the failed attempt's findings
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
	fx := newFixture(t, reed, engine, withConfig(gateConfig))
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { appendEventsLine(t, eventsPath, "STOP:turn2") },
	}}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour, KeepPane: true},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(mc, mc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

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

// TestGate_ClosureError covers the "a gate error is never not passed" decision: an infrastructure
// fault from a gate closure, whether it is the only entry or a later one after an earlier entry
// passed, fails the run as an ordinary error, sends no re-prompt, and charges no attempt or failure.
func TestGate_ClosureError(t *testing.T) {
	wantErr := errors.New("gate infrastructure fault")
	fault := func() (GateResult, error) { return GateResult{}, wantErr }
	pass := func() (GateResult, error) { return GateResult{Passed: true}, nil }

	tests := []struct {
		name   string
		events string
		spec   GateSpec
	}{
		{name: "only entry", events: "STOP:turn1\n", spec: GateSpec{{Gate: fault, Attempts: 3}}},
		{
			name:   "second entry after the first passes",
			events: "STOP:done\n",
			spec: GateSpec{
				{Name: "first", Gate: pass, Attempts: 3},
				{Name: "second", Gate: fault, Attempts: 3},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(eventsPath, []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Hour)),
				withRunGate(tt.spec))

			_, err := run.Wait()
			if err == nil || !errors.Is(err, wantErr) {
				t.Fatalf("Wait() error = %v, want it to wrap %v", err, wantErr)
			}
			if len(reed.SendTextCalls) != 0 {
				t.Errorf("SendText calls = %+v, want none", reed.SendTextCalls)
			}
			for i, sent := range run.gateSent {
				if sent != 0 {
					t.Errorf("gateSent[%d] = %d, want 0 (an infrastructure error charges no attempt)", i, sent)
				}
			}
			for i, fails := range run.gateFails {
				if fails != 0 {
					t.Errorf("gateFails[%d] = %d, want 0 (an infrastructure error counts no failure)", i, fails)
				}
			}
		})
	}
}

// TestGate_NoLiveSessionDonePaths covers the Done classifications a gated run can reach with no
// live session at all: the three checkLivenessTick-driven ones (not-tracked, not-live (a dead
// pane), an expired startup window), the run deadline's own classifyDeadlineExpiry, and the
// events-unreadable finishedDespiteMechanismFailure exit. Each finalizes through Wait's liveness,
// deadline or mechanism-failure branch rather than the events-tick Send loop, so each asserts a
// failed verdict, zero sends, zero attempts charged, and the gate running exactly once.
//
//testtiming:keep pins that every exit with no live session finalizes through its own branch with the gate run once, a failed verdict, and no re-prompt or attempt charged
func TestGate_NoLiveSessionDonePaths(t *testing.T) {
	liveness := Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 0}
	tests := []struct {
		name          string
		cfg           Config
		statusQueue   []reedengine.StatusResult
		startupScript []StartupState
		// eventsSeed is written to the events file; empty leaves it unwritten, so no Stop event arrives.
		eventsSeed     string
		parseEventsErr error
		// deadlineOffset is where the run deadline sits relative to the clock's start.
		deadlineOffset time.Duration
	}{
		{name: "not_tracked", cfg: liveness, statusQueue: []reedengine.StatusResult{{Strands: nil}}, deadlineOffset: time.Hour},
		{name: "not_live_dead_pane", cfg: liveness, statusQueue: []reedengine.StatusResult{deadStatus("strand-1", "%1")}, deadlineOffset: time.Hour},
		{
			name: "startup_window_expires", cfg: liveness, deadlineOffset: time.Hour,
			statusQueue:   []reedengine.StatusResult{liveStatus("strand-1", "%1")},
			startupScript: []StartupState{StartupPending},
		},
		// The deadline is already expired: the very first deadline check trips it, and the gate
		// still runs once through classifyDeadlineExpiry -> finalize with no session addressed.
		{name: "run_deadline_expires", cfg: gateConfig, deadlineOffset: -time.Minute},
		// The events-unreadable mechanism-failure exit still runs the gate once, through
		// finishedDespiteMechanismFailure -> finalize.
		{
			name: "events_unreadable_mechanism_failure", cfg: gateConfig, deadlineOffset: time.Hour,
			eventsSeed: "STOP:x\n", parseEventsErr: errors.New("events file unparseable"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			if tt.eventsSeed != "" {
				if err := os.WriteFile(eventsPath, []byte(tt.eventsSeed), 0o644); err != nil {
					t.Fatalf("seed events: %v", err)
				}
			}
			outputFile := filepath.Join(runDir, "out.md")
			touchOutputFile(t, outputFile)

			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: false, Findings: "n/a"}, nil
			}

			reed := &fakeReed{StatusQueue: tt.statusQueue}
			engine := &fakeEngine{StartupScript: tt.startupScript, ParseEventsErr: tt.parseEventsErr}
			fx := newFixture(t, reed, engine, withConfig(tt.cfg))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(tt.deadlineOffset)),
				withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

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

// TestGate_EvaluateOncePerAttempt_Memoized pins that the gate runs exactly once per settling: a
// second evaluateGate() call within the same attempt reads the stored memo rather than re-invoking
// the closure.
//
//testtiming:keep pins that evaluateGate memoizes within an attempt, returning the same pointer without re-running the closure
func TestGate_EvaluateOncePerAttempt_Memoized(t *testing.T) {
	callCount := 0
	gate := func() (GateResult, error) {
		callCount++
		return GateResult{Passed: true}, nil
	}
	run := newFixture(t, &fakeReed{}, &fakeEngine{}).newRun(Spec{}, withRunDir(t.TempDir()), withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

	first, err := run.evaluateGate(false)
	if err != nil {
		t.Fatalf("evaluateGate() error: %v", err)
	}
	second, err := run.evaluateGate(false)
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
//
//testtiming:keep pins that a failed re-prompt send ends the loop without a retry and charges no attempt
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
	fx := newFixture(t, reed, engine, withConfig(gateConfig))
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
		withRunClock(fc, fc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

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
//
//testtiming:keep pins that a deadline expiring after one re-prompt runs the gate once more through the deadline's own Done, with Attempts still honest at one
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
	fx := newFixture(t, reed, engine, withConfig(Config{PollIntervalMS: 5, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30}))
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
		withRunClock(fc, fc.Now().Add(2*time.Millisecond)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

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

// TestAttach_GateThreading covers a resumed run's gate: AttachGated gates it exactly as a fresh
// RunGated run is gated, and the plain Attach entry point never carries a gate.
//
//testtiming:keep pins that AttachGated gates a resumed run exactly as a fresh one is gated, and that plain Attach leaves Result.Gate nil
func TestAttach_GateThreading(t *testing.T) {
	tests := []struct {
		name  string
		gated bool
	}{
		{name: "AttachGated threads the gate through reconstruct and wait", gated: true},
		{name: "Attach leaves Result.Gate nil", gated: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
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

			spec := Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute}
			gateCalls := 0
			gate := func() (GateResult, error) {
				gateCalls++
				return GateResult{Passed: true}, nil
			}

			var (
				result Result
				found  bool
				err    error
			)
			if tt.gated {
				result, found, err = runner.AttachGated(spec, GateSpec{{Gate: gate, Attempts: 3}})
			} else {
				result, found, err = runner.Attach(spec)
			}
			if err != nil {
				t.Fatalf("attach error = %v; want nil", err)
			}
			if !found {
				t.Fatal("attach found = false; want true")
			}
			if !tt.gated {
				if result.Gate != nil {
					t.Errorf("Gate = %+v; want nil for the ungated Attach delegation", result.Gate)
				}
				return
			}
			if result.Gate == nil || !result.Gate.Passed {
				t.Errorf("Gate = %+v; want a passed verdict, proving a resumed run is gated exactly as a fresh one is", result.Gate)
			}
			if gateCalls != 1 {
				t.Errorf("gate closure invoked %d times; want 1", gateCalls)
			}
		})
	}
}

// TestGate_WaitMarkAroundEntry pins that a gate entry is marked on screen and on disk while its closure runs and unmarked on every path out,
// and that a failing SetWaitMark changes neither the verdict nor the error the closure produced.
func TestGate_WaitMarkAroundEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     GateResult
		closureErr error
		markErr    error
		wantPassed bool
		wantErr    bool
	}{
		{name: "passing entry", result: GateResult{Passed: true}, wantPassed: true},
		{name: "failing entry", result: GateResult{Findings: "no"}},
		{name: "pending entry", result: GateResult{Pending: true}},
		{name: "erroring entry", closureErr: errors.New("boom"), wantErr: true},
		{name: "a failing SetWaitMark leaves a passing verdict", result: GateResult{Passed: true}, markErr: errors.New("no tmux"), wantPassed: true},
		{name: "a failing SetWaitMark leaves a failing verdict", result: GateResult{Findings: "no"}, markErr: errors.New("no tmux")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			markerPath := filepath.Join(runDir, waitMarkerFileName)
			reed := &fakeReed{WaitMarkErr: tt.markErr}
			var markerSeen WaitMarker
			gate := func() (GateResult, error) {
				data, err := os.ReadFile(markerPath)
				if err != nil {
					t.Errorf("wait marker unreadable while the closure runs: %v", err)
				}
				if err := yaml.Unmarshal(data, &markerSeen); err != nil {
					t.Errorf("wait marker undecodable: %v", err)
				}
				if got := reed.WaitMarkCalls; len(got) != 1 || got[0].Label != "gate review" {
					t.Errorf("mark calls while the closure runs = %+v; want one set of %q", got, "gate review")
				}
				return tt.result, tt.closureErr
			}
			run := newFixture(t, reed, &fakeEngine{}).newRun(Spec{}, withRunDir(runDir),
				withRunGate(GateSpec{{Name: "review", Gate: gate, Attempts: 3, MayHold: true}}))

			outcome, err := run.evaluateGate(false)

			if (err != nil) != tt.wantErr {
				t.Fatalf("evaluateGate error = %v; want error %v", err, tt.wantErr)
			}
			if err == nil && outcome.Passed != tt.wantPassed {
				t.Errorf("Passed = %v; want %v", outcome.Passed, tt.wantPassed)
			}
			if markerSeen.Kind != "gate review" || markerSeen.PID != os.Getpid() {
				t.Errorf("marker seen = %+v; want kind %q and this process's pid", markerSeen, "gate review")
			}
			if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
				t.Errorf("wait marker survives the closure: %v", statErr)
			}
			if got := reed.WaitMarkCalls; len(got) != 2 || got[1].Label != "" {
				t.Errorf("mark calls = %+v; want a set then a clear", got)
			}
		})
	}
}
