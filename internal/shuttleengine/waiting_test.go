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

// TestPollEventsTick_Waiting drives pollEventsTick over an events file holding one waiting turn end
// ("WAIT:background work"): with no output files it is still running and the offset advances past
// the parsed bytes, a Stop after it classifies asking, and with the output files present it is done.
//
//testtiming:keep pins that a waiting turn end is still running and advances the offset, becomes asking on a later Stop, and is done when the output files exist
func TestPollEventsTick_Waiting(t *testing.T) {
	const waitLine = "WAIT:background work\n"
	tests := []struct {
		name         string
		touchOutput  bool
		wantFirst    Outcome
		checkOffset  bool
		stopAfter    string
		wantSecond   Outcome
		wantSecondIn string
	}{
		{name: "waiting is still running", wantFirst: "", checkOffset: true},
		{
			name: "a stop after waiting classifies asking", wantFirst: "", checkOffset: true,
			stopAfter: "STOP:what now?", wantSecond: OutcomeAsking, wantSecondIn: "what now?",
		},
		{name: "waiting with output files is done", touchOutput: true, wantFirst: OutcomeDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFile := filepath.Join(t.TempDir(), "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
				withRunEvents(waitLine),
				withRunClock(fc, fc.Now().Add(time.Hour)))

			outcome, _, err := run.pollEventsTick()
			if err != nil {
				t.Fatalf("pollEventsTick error: %v", err)
			}
			if outcome != tt.wantFirst {
				t.Errorf("outcome = %q, want %q", outcome, tt.wantFirst)
			}
			if tt.checkOffset {
				if want := int64(len(waitLine)); run.offset != want {
					t.Errorf("offset = %d, want %d (advanced past the parsed bytes)", run.offset, want)
				}
			}
			if tt.stopAfter == "" {
				return
			}

			appendEventsLine(t, run.state.EventsPath, tt.stopAfter)
			outcome, message, err := run.pollEventsTick()
			if err != nil {
				t.Fatalf("second tick error: %v", err)
			}
			if outcome != tt.wantSecond || message != tt.wantSecondIn {
				t.Errorf("second tick = (%q, %q), want (%q, %q)", outcome, message, tt.wantSecond, tt.wantSecondIn)
			}
		})
	}
}

//testtiming:keep pins that a gated waiting turn end is not an arrival: the gate is not evaluated on it and runs exactly once on the next Stop
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

// TestPollEventsTick_ShellExpiry covers a waiting turn end whose outstanding list is one background
// shell: the turn keeps waiting until the bound, and at the bound it ends done when the output files
// exist and asking, with the waiting message, when they do not.
//
//testtiming:keep pins the background-shell wait bound: still waiting until the bound, then done with output files or asking with the waiting message without them
func TestPollEventsTick_ShellExpiry(t *testing.T) {
	tests := []struct {
		name string
		// gated keeps the files-exist shortcut out of the way, so the expiry alone ends the turn.
		gated       bool
		touchOutput bool
		wantOutcome Outcome
		wantMessage string
	}{
		{name: "expires after the bound", gated: true, touchOutput: true, wantOutcome: OutcomeDone},
		{name: "expiry with missing output is asking", wantOutcome: OutcomeAsking, wantMessage: "background work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFile := filepath.Join(t.TempDir(), "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			run, fc := shellWaitFixture(t, outputFile, oneShell, Spec{})
			if tt.gated {
				run.gate = GateSpec{{Gate: func() (GateResult, error) { return GateResult{Passed: true}, nil }, Attempts: 1}}
			}

			if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
				t.Fatalf("first tick = (%q, %v), want still waiting", outcome, err)
			}
			fc.Sleep(10*time.Minute - time.Second)
			if outcome, _, err := run.pollEventsTick(); err != nil || outcome != "" {
				t.Fatalf("tick just before the bound = (%q, %v), want still waiting", outcome, err)
			}
			fc.Sleep(time.Second)
			outcome, message, err := run.pollEventsTick()
			if err != nil || outcome != tt.wantOutcome || message != tt.wantMessage {
				t.Errorf("tick at the bound = (%q, %q, %v), want (%q, %q, nil)", outcome, message, err, tt.wantOutcome, tt.wantMessage)
			}
		})
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

//testtiming:keep pins that a gated Wait evaluates the gate once at a shell expiry and reports the expired shell's label
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
