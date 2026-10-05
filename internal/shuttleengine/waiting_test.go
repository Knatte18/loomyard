package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// waitingEngine is a fakeEngine whose ParseEvents also maps a "WAIT:<message>" line to an
// EventWaiting, so the wait loop's handling of the new kind is exercised without a provider.
type waitingEngine struct {
	fakeEngine
	// outstanding is attached to every EventWaiting the engine parses.
	outstanding []BackgroundTask
}

func (e *waitingEngine) ParseEvents(data []byte) ([]Event, error) {
	var events []Event
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if msg, ok := strings.CutPrefix(trimmed, "WAIT:"); ok {
			events = append(events, Event{Kind: EventWaiting, Message: msg, Raw: []byte(trimmed), Outstanding: e.outstanding})
			continue
		}
		parsed, err := e.fakeEngine.ParseEvents([]byte(line))
		if err != nil {
			return nil, err
		}
		events = append(events, parsed...)
	}
	return events, nil
}

func TestPollEventsTick_WaitingIsStillRunning(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
		withRunEvents("WAIT:background work\n"),
		withRunClock(fc, fc.Now().Add(time.Hour)))

	outcome, _, err := run.pollEventsTick()
	if err != nil {
		t.Fatalf("pollEventsTick error: %v", err)
	}
	if outcome != "" {
		t.Errorf("outcome = %q, want empty (still running)", outcome)
	}
	if want := int64(len("WAIT:background work\n")); run.offset != want {
		t.Errorf("offset = %d, want %d (advanced past the parsed bytes)", run.offset, want)
	}
}

func TestPollEventsTick_StopAfterWaitingClassifiesAsking(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
		withRunEvents("WAIT:background work\n"),
		withRunClock(fc, fc.Now().Add(time.Hour)))
	if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
		t.Fatalf("first tick = (%q, %v), want still running", outcome, err)
	}

	f, err := os.OpenFile(run.state.EventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open events: %v", err)
	}
	if _, err := f.WriteString("STOP:what now?\n"); err != nil {
		t.Fatalf("append events: %v", err)
	}
	f.Close()

	outcome, message, err := run.pollEventsTick()
	if err != nil {
		t.Fatalf("second tick error: %v", err)
	}
	if outcome != OutcomeAsking || message != "what now?" {
		t.Errorf("second tick = (%q, %q), want (%q, %q)", outcome, message, OutcomeAsking, "what now?")
	}
}

func TestPollEventsTick_WaitingWithOutputFilesIsDone(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	touchOutputFile(t, outputFile)
	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
		withRunEvents("WAIT:background work\n"),
		withRunClock(fc, fc.Now().Add(time.Hour)))

	outcome, _, err := run.pollEventsTick()
	if err != nil {
		t.Fatalf("pollEventsTick error: %v", err)
	}
	if outcome != OutcomeDone {
		t.Errorf("outcome = %q, want %q", outcome, OutcomeDone)
	}
}

func TestPollEventsTick_GatedWaitingWithOutputFilesIsNotAnArrival(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("WAIT:background work\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}

	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	mc := &multiStepClock{fakeClock: fc, steps: []func(){
		func() { appendEventsLine(t, eventsPath, "STOP:done") },
	}}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(mc, fc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

	outcome, _, err := run.pollEventsTick()
	if err != nil {
		t.Fatalf("pollEventsTick error: %v", err)
	}
	if outcome != "" {
		t.Errorf("outcome = %q, want empty (a gated waiting turn end is not an arrival)", outcome)
	}
	if gateCalls != 0 {
		t.Errorf("gate evaluated %d times on the waiting turn end, want 0", gateCalls)
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if gateCalls != 1 {
		t.Errorf("gate evaluated %d times, want exactly 1 after the next stop", gateCalls)
	}
}

// isZeroResult reports whether r is the zero Result; Result holds a slice, so it cannot be compared with ==.
func isZeroResult(r Result) bool { return reflect.DeepEqual(r, Result{}) }

// shellWaitFixture returns a Run over a waiting turn end whose outstanding list is tasks,
// with a fake clock and a one-hour run timeout; the first tick has not run yet.
func shellWaitFixture(t *testing.T, outputFile string, tasks []BackgroundTask, spec Spec) (*Run, *fakeClock) {
	t.Helper()
	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{outstanding: tasks}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	spec.OutputFiles = []string{outputFile}
	spec.Timeout = time.Hour
	run := fx.newRun(spec,
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
		withRunEvents("WAIT:background work\n"),
		withRunClock(fc, fc.Now().Add(time.Hour)))
	return run, fc
}

var oneShell = []BackgroundTask{{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999"}}

func TestPollEventsTick_ShellExpiresAfterBound(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	touchOutputFile(t, outputFile)
	// A gated run keeps the files-exist shortcut out of the way, so the expiry alone ends the turn.
	run, fc := shellWaitFixture(t, outputFile, oneShell, Spec{})
	run.gate = GateSpec{{Gate: func() (GateResult, error) { return GateResult{Passed: true}, nil }, Attempts: 1}}

	if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
		t.Fatalf("first tick = (%q, %v), want still waiting", outcome, err)
	}
	fc.Sleep(10*time.Minute - time.Second)
	if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
		t.Fatalf("tick just before the bound = (%q, %v), want still waiting", outcome, err)
	}
	fc.Sleep(time.Second)
	outcome, _, err := run.pollEventsTick()
	if err != nil || outcome != OutcomeDone {
		t.Fatalf("tick at the bound = (%q, %v), want %q", outcome, err, OutcomeDone)
	}
}

func TestPollEventsTick_ShellExpiryWithMissingOutputIsAsking(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, fc := shellWaitFixture(t, outputFile, oneShell, Spec{})

	if outcome, _, _ := run.pollEventsTick(); outcome != "" {
		t.Fatalf("first tick outcome = %q, want still waiting", outcome)
	}
	fc.Sleep(10 * time.Minute)
	outcome, message, err := run.pollEventsTick()
	if err != nil || outcome != OutcomeAsking || message != "background work" {
		t.Errorf("tick at the bound = (%q, %q, %v), want (%q, %q, nil)", outcome, message, err, OutcomeAsking, "background work")
	}
}

func TestPollEventsTick_ForkOutstandingKeepsWaitingPastBound(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	tasks := []BackgroundTask{oneShell[0], {Kind: BackgroundFork, ID: "fork-1"}}
	run, fc := shellWaitFixture(t, outputFile, tasks, Spec{})

	run.pollEventsTick()
	fc.Sleep(time.Hour)
	if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
		t.Errorf("tick past the bound with a fork outstanding = (%q, %v), want still waiting", outcome, err)
	}
}

func TestPollEventsTick_AwaitedShellKeepsWaitingPastBound(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	tasks := []BackgroundTask{{Kind: BackgroundShell, ID: "sh-2", Label: "lyx webster recover-batch 03"}}
	run, fc := shellWaitFixture(t, outputFile, tasks, Spec{AwaitedShellPrefixes: []string{"lyx webster recover-batch"}})

	run.pollEventsTick()
	fc.Sleep(time.Hour)
	if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
		t.Errorf("tick past the bound with an awaited shell = (%q, %v), want still waiting", outcome, err)
	}
}

func TestPollEventsTick_LaterTurnEndListingExpiredShellEndsWithoutNewWait(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, fc := shellWaitFixture(t, outputFile, oneShell, Spec{})

	run.pollEventsTick()
	fc.Sleep(10 * time.Minute)
	if outcome, _, _ := run.pollEventsTick(); outcome != OutcomeAsking {
		t.Fatalf("expiry outcome = %q, want %q", outcome, OutcomeAsking)
	}

	appendEventsLine(t, run.state.EventsPath, "WAIT:still there")
	outcome, message, err := run.pollEventsTick()
	if err != nil || outcome != OutcomeAsking || message != "still there" {
		t.Errorf("later turn end = (%q, %q, %v), want asking at once with its message", outcome, message, err)
	}
}

// jumpClock is a fake clock whose Sleep advances a fixed jump, so Wait reaches an expiry bound in a few ticks.
type jumpClock struct {
	*fakeClock
	jump time.Duration
}

func (c *jumpClock) Sleep(time.Duration) { c.fakeClock.Sleep(c.jump) }

func TestWait_GatedShellExpiryEvaluatesGate(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("WAIT:background work\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	gateCalls := 0
	gate := func() (GateResult, error) {
		gateCalls++
		return GateResult{Passed: true}, nil
	}

	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{outstanding: oneShell}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	jc := &jumpClock{fakeClock: fc, jump: 6 * time.Minute}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(jc, fc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if gateCalls != 1 {
		t.Errorf("gate evaluated %d times, want 1 at the expiry", gateCalls)
	}
	if want := []string{"sleep 9999"}; !slices.Equal(result.ExpiredShells, want) {
		t.Errorf("ExpiredShells = %v, want %v", result.ExpiredShells, want)
	}
}
