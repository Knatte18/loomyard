package shuttleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitingEngine is a fakeEngine whose ParseEvents also maps a "WAIT:<message>" line to an
// EventWaiting, so the wait loop's handling of the new kind is exercised without a provider.
type waitingEngine struct {
	fakeEngine
}

func (e *waitingEngine) ParseEvents(data []byte) ([]Event, error) {
	var events []Event
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if msg, ok := strings.CutPrefix(trimmed, "WAIT:"); ok {
			events = append(events, Event{Kind: EventWaiting, Message: msg, Raw: []byte(trimmed)})
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

// newPollTickRun builds a Run over a fresh events file seeded with events, and an output file that
// exists only when withOutput is true.
func newPollTickRun(t *testing.T, events string, withOutput bool) *Run {
	t.Helper()
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	if withOutput {
		touchOutputFile(t, outputFile)
	}
	if err := os.WriteFile(eventsPath, []byte(events), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	runner := newWaitTestRunner(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	return &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Hour),
	}
}

func TestPollEventsTick_WaitingIsStillRunning(t *testing.T) {
	run := newPollTickRun(t, "WAIT:background work\n", false)

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
	run := newPollTickRun(t, "WAIT:background work\n", false)
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
	run := newPollTickRun(t, "WAIT:background work\n", true)

	outcome, _, err := run.pollEventsTick()
	if err != nil {
		t.Fatalf("pollEventsTick error: %v", err)
	}
	if outcome != OutcomeDone {
		t.Errorf("outcome = %q, want %q", outcome, OutcomeDone)
	}
}
