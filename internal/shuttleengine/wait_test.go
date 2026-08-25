// wait_test.go covers Run.Wait's poll loop against fakeReed/fakeEngine and a fake clock: all four
// outcome classifications, KeepPane skipping cleanup, the startup probe's trust-dismiss and
// fast-fail-on-timeout paths, multi-Stop offset tracking, events-offset resilience across a partial
// line, and finalize's fork-audit attach (only for a fork-mode spec's done classification).

package shuttleengine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// fakeClock is a virtual clock: Sleep instantly advances Now() by d instead
// of blocking, so Wait's poll loop runs an arbitrarily long scripted
// sequence at zero real wall-clock cost.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

var _ clock = (*fakeClock)(nil)

// scriptedClock wraps a fakeClock and runs onSleep once, after the first
// Sleep call, letting a test mutate on-disk fixtures (e.g. completing a
// partial events.jsonl line) exactly between two poll ticks.
type scriptedClock struct {
	*fakeClock
	onSleep func()
	fired   bool
}

func (c *scriptedClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	if !c.fired && c.onSleep != nil {
		c.fired = true
		c.onSleep()
	}
}

var _ clock = (*scriptedClock)(nil)

// newWaitTestRunner returns a Runner over reed/engine scoped to a fresh temp
// worktree, matching newTestRunner in run_test.go but kept local to this
// file since wait tests construct their Run handles directly rather than
// through Start.
func newWaitTestRunner(t *testing.T, reed ReedOps, engine Engine, cfg Config) *Runner {
	t.Helper()
	worktreeRoot := t.TempDir()
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path: %v", err)
	}
	return NewRunner(reed, engine, anchorPath, worktreeRoot, cfg)
}

// TestPollInterval_FloorsNonPositive pins the busy-spin guard: a configured poll_interval_ms of 0
// or below must fall back to the template default rather than making Wait tick with a zero sleep.
func TestPollInterval_FloorsNonPositive(t *testing.T) {
	tests := []struct {
		name       string
		intervalMS int
		want       time.Duration
	}{
		{"zero_floored", 0, defaultPollIntervalMS * time.Millisecond},
		{"negative_floored", -100, defaultPollIntervalMS * time.Millisecond},
		{"positive_passthrough", 250, 250 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pollInterval(Config{PollIntervalMS: tt.intervalMS})
			if got != tt.want {
				t.Errorf("pollInterval({PollIntervalMS: %d}) = %v; want %v", tt.intervalMS, got, tt.want)
			}
		})
	}
}

// captureLoggerOutput redirects internal/logger's stderr half into a buffer at Info verbosity for
// the duration of the calling test, restoring both when it ends.
// It is the seam the teardown-observability assertion below needs: Info is gated on verbosity for
// that half, and the durable trace file is not readable from a test.
func captureLoggerOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetVerbosity(0)
		logger.SetOutput(os.Stderr)
	})
	return &buf
}

// TestRun_Wait_MechanismFailure_KeepsRunIdentity pins that a Wait which reaches no classification
// still hands its caller the run's identity.
// A mechanism failure is exactly when those handles matter: finalize never ran, so the run
// directory is still on disk and the strand may still be registered, and a wholly zero Result
// leaves the caller unable to diagnose, resume, or tear down what it started.
// Reproduced live by tearing the reed session down under an in-flight run.
func TestRun_Wait_MechanismFailure_KeepsRunIdentity(t *testing.T) {
	// A permanently failing reed.Status is the live shape (a torn-down session answers every
	// Status the same way), so Wait gives up after maxStatusRetries consecutive failures.
	reed := &fakeReed{StatusErr: errors.New(`no reed session; run "lyx reed up"`)}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	runDir := t.TempDir()
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{filepath.Join(runDir, "out.md")}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: filepath.Join(runDir, "events.jsonl")},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err == nil {
		t.Fatal("Wait() = nil error, want the consecutive-status-failure mechanism error")
	}
	if result.Outcome != "" {
		t.Errorf("Outcome = %q; want empty — a mechanism failure reached no classification", result.Outcome)
	}
	if result.StrandGUID != "strand-1" {
		t.Errorf("StrandGUID = %q; want %q, so the caller can still reach the strand", result.StrandGUID, "strand-1")
	}
	if result.SessionID != "session-1" {
		t.Errorf("SessionID = %q; want %q, so the caller can still resume the session", result.SessionID, "session-1")
	}
	if result.RunDir != runDir {
		t.Errorf("RunDir = %q; want %q, so the caller can still diagnose and clean up the run dir", result.RunDir, runDir)
	}
}

// TestRun_Wait_LogsTeardownThroughLogger pins the teardown half of the Live-Substrate Spawn
// Observability invariant: Start already logs "run started" through internal/logger, so a finalize
// that logs nothing would leave the durable Info+ trace file showing every shuttle run beginning
// and none of them ending — and the bare log package finalize's cleanup failures used before never
// reaches that sink at all.
func TestRun_Wait_LogsTeardownThroughLogger(t *testing.T) {
	tests := []struct {
		name     string
		keepPane bool
		want     string
	}{
		{"cleaned_up", false, "cleanedUp=true"},
		{"kept_pane", true, "cleanedUp=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			outputFile := filepath.Join(runDir, "out.md")
			if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
				t.Fatalf("seed output file: %v", err)
			}
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
			runner := newWaitTestRunner(t, reed, &fakeEngine{StartupScript: []StartupState{StartupReady}}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: tt.keepPane},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Minute),
			}

			buf := captureLoggerOutput(t)
			if _, err := run.Wait(); err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			logged := buf.String()
			for _, want := range []string{"shuttle: run finished", `outcome=done`, "strand-1", tt.want} {
				if !strings.Contains(logged, want) {
					t.Errorf("teardown log = %q; want it to contain %q", logged, want)
				}
			}
		})
	}
}

func TestRun_Wait_DoneHappyPath_CleansUp(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}

	foundRemove := false
	for _, c := range reed.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded, calls = %+v", reed.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after done cleanup, stat err = %v", err)
	}
}

func TestRun_Wait_DoneWithKeepPane_SkipsCleanup(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, KeepPane: true},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (KeepPane)", reed.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed despite KeepPane: %v", err)
	}
}

func TestRun_Wait_Asking_CarriesMessageKeepsStrand(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created

	if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "need operator input" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "need operator input")
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (asking keeps the strand)", reed.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for asking outcome: %v", err)
	}
}

// multiStepClock wraps a fakeClock and runs the next entry of steps, in order, once per Sleep call —
// a generalization of scriptedClock's single-shot onSleep for tests that need to mutate on-disk
// fixtures between several ticks, not just the first two.
// Once steps is exhausted, further Sleep calls run no step.
type multiStepClock struct {
	*fakeClock
	steps []func()
	next  int
}

func (c *multiStepClock) Sleep(d time.Duration) {
	c.fakeClock.Sleep(d)
	if c.next < len(c.steps) {
		step := c.steps[c.next]
		c.next++
		step()
	}
}

var _ clock = (*multiStepClock)(nil)

// TestRun_Wait_AwaitOperator_AskingNonTerminal is the defect-A coverage: an ask that is terminal
// today must become non-terminal once Spec.AwaitOperator is set, while every other exit stays
// exactly as it was.
func TestRun_Wait_AwaitOperator_AskingNonTerminal(t *testing.T) {
	t.Run("AwaitOperatorFalse_PinsTodaysAskingBehaviour", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl")
		outputFile := filepath.Join(runDir, "out.md") // never created

		if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: false},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
			clock:    fc,
			deadline: fc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Outcome != OutcomeAsking {
			t.Errorf("Outcome = %q, want %q (AwaitOperator false must keep an ask terminal)", result.Outcome, OutcomeAsking)
		}
	})

	t.Run("AwaitOperatorTrue_DropsAskAndFinalizesOnceOutputFilesAppear", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl")
		outputFile := filepath.Join(runDir, "out.md")

		if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		mc := &multiStepClock{fakeClock: fc, steps: []func(){
			func() {
				// Fires between tick 1 (which observed the dropped ask) and tick 2: the agent
				// finishes, writes its output file, and appends the terminating Stop event.
				if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
					t.Fatalf("write output file: %v", err)
				}
				f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open events file to append: %v", err)
				}
				defer f.Close()
				if _, err := f.WriteString("STOP:done\n"); err != nil {
					t.Fatalf("append done event: %v", err)
				}
			},
		}}

		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
			clock:    mc,
			deadline: mc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Outcome != OutcomeDone {
			t.Errorf("Outcome = %q, want %q (the ask must be dropped and polling must continue to the later Done)", result.Outcome, OutcomeDone)
		}
	})

	t.Run("AwaitOperatorTrue_SeveralAsksInARowThenDone", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl")
		outputFile := filepath.Join(runDir, "out.md")

		if err := os.WriteFile(eventsPath, []byte("STOP:question batch one\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		appendEvent := func(line string) func() {
			return func() {
				f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open events file to append: %v", err)
				}
				defer f.Close()
				if _, err := f.WriteString(line); err != nil {
					t.Fatalf("append event: %v", err)
				}
			}
		}

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		mc := &multiStepClock{fakeClock: fc, steps: []func(){
			appendEvent("STOP:question batch two\n"),
			appendEvent("STOP:question batch three\n"),
			func() {
				if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
					t.Fatalf("write output file: %v", err)
				}
				appendEvent("STOP:done\n")()
			},
		}}

		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
			clock:    mc,
			deadline: mc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Outcome != OutcomeDone {
			t.Errorf("Outcome = %q, want %q (a multi-batch interview must survive every ask and finalize on the eventual Done)", result.Outcome, OutcomeDone)
		}
	})

	t.Run("AwaitOperatorTrue_StillTimesOutWithFilesAbsent", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl")
		outputFile := filepath.Join(runDir, "out.md") // never created

		if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
		engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
		runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Second, AwaitOperator: true},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
			clock:    fc,
			deadline: fc.Now().Add(time.Second),
		}

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Outcome != OutcomeTimeout {
			t.Errorf("Outcome = %q, want %q (AwaitOperator does not extend the run deadline, only drops asks)", result.Outcome, OutcomeTimeout)
		}
	})

	t.Run("AwaitOperatorTrue_StillDiesOnDeadPane", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl") // never created
		outputFile := filepath.Join(runDir, "out.md")       // never created

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}}}}
		engine := &fakeEngine{}
		runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
			clock:    fc,
			deadline: fc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Outcome != OutcomeDied {
			t.Errorf("Outcome = %q, want %q (a dead pane still terminates the wait under AwaitOperator)", result.Outcome, OutcomeDied)
		}
	})

	t.Run("AwaitOperatorTrue_StillSurfacesUntrackedStrandMechanismFailure", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl") // never created
		outputFile := filepath.Join(runDir, "out.md")       // never created

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}}}}}
		runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
			clock:    fc,
			deadline: fc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err == nil {
			t.Fatalf("Wait() = (%+v, nil); want the untracked-strand mechanism error", result)
		}
		if !errors.Is(err, errStrandNotTracked) {
			t.Errorf("Wait() error = %v; want one wrapping errStrandNotTracked", err)
		}
		if result.Outcome != "" {
			t.Errorf("Outcome = %q; want empty — a mechanism failure reached no classification", result.Outcome)
		}
	})

	t.Run("AwaitOperatorTrue_StillSurfacesClearedPaneBindingMechanismFailure", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, "events.jsonl") // never created
		outputFile := filepath.Join(runDir, "out.md")       // never created

		reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
			Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}},
		}}}
		runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
		fc := newFakeClock(time.Now())
		run := &Run{
			runner:   runner,
			spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
			runDir:   runDir,
			state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
			clock:    fc,
			deadline: fc.Now().Add(time.Minute),
		}

		result, err := run.Wait()
		if err == nil {
			t.Fatalf("Wait() = (%+v, nil); want the cleared-pane-binding mechanism error", result)
		}
		if !errors.Is(err, errStrandPaneBindingCleared) {
			t.Errorf("Wait() error = %v; want one wrapping errStrandPaneBindingCleared", err)
		}
		if result.Outcome != "" {
			t.Errorf("Outcome = %q; want empty — a mechanism failure reached no classification", result.Outcome)
		}
	})
}

// TestRun_Wait_LiveAsk_ClassifiesRealTimeAsking verifies a live ask (an EventAsk with no output
// files present) classifies OutcomeAsking carrying the question as the message, keeping the pane
// and run dir just like the existing turn-end asking case — proving the unchanged pollEventsTick
// branch also covers the live-ask signal ParseEvents now emits.
func TestRun_Wait_LiveAsk_ClassifiesRealTimeAsking(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created

	if err := os.WriteFile(eventsPath, []byte("ASK:which approach?\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "which approach?" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "which approach?")
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (asking keeps the strand)", reed.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for asking outcome: %v", err)
	}
}

// TestRun_Wait_LiveAsk_DoneFirstStillWins verifies that when a live ask arrives but the output
// files already exist, done-first classification still wins — an EventAsk never overrides an
// already-satisfied file contract.
func TestRun_Wait_LiveAsk_DoneFirstStillWins(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("ASK:which approach?\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q (done-first over a live ask when output files exist)", result.Outcome, OutcomeDone)
	}
}

func TestRun_Wait_Died_ViaStatusNotLive(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}}}}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDied {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDied)
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (died keeps the strand)", reed.RemoveStrandCalls)
	}
}

// TestRun_Wait_UntrackedStrand_IsMechanismFailureNotDied is R3-F1's regression guard.
//
// Wait used to derive one boolean from reed's strand table and treat both negative answers alike, so
// a Status that succeeded with the run's guid simply ABSENT — reed's bookkeeping reset under a run
// whose agent is still working — classified OutcomeDied. Reproduced live twice: renaming the worktree
// under an in-flight run, and deleting .lyx/reed.json (the remedy reed's own corrupt-state error
// recommends) both returned ok:true/outcome:"died" ~6 s later while the claude process kept working
// in its pane.
// The first subtest is the defect; the second pins that a strand reed DOES track whose pane is not
// alive still classifies OutcomeDied, so the fix narrows the branch rather than removing it.
func TestRun_Wait_UntrackedStrand_IsMechanismFailureNotDied(t *testing.T) {
	tests := []struct {
		name        string
		strands     []reedengine.StrandStatus
		wantOutcome Outcome
		wantErr     bool
	}{
		{
			name:        "absent_from_table_is_a_mechanism_failure",
			strands:     []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}},
			wantOutcome: "",
			wantErr:     true,
		},
		{
			name:        "tracked_but_pane_not_live_is_still_died",
			strands:     []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}},
			wantOutcome: OutcomeDied,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl") // never created
			outputFile := filepath.Join(runDir, "out.md")       // never created

			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: tt.strands}}}
			runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Minute),
			}

			result, err := run.Wait()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Wait() = (%+v, nil); want the untracked-strand mechanism error", result)
				}
				if !errors.Is(err, errStrandNotTracked) {
					t.Errorf("Wait() error = %v; want one wrapping errStrandNotTracked", err)
				}
				if !strings.Contains(err.Error(), "strand-1") {
					t.Errorf("Wait() error = %v; want it to name the run's guid so the operator can find the pane", err)
				}
				// A mechanism failure must keep the run's identity (R1-F2) — the agent may still be
				// live, so the caller needs the handles to reach it.
				if result.StrandGUID != "strand-1" || result.SessionID != "session-1" || result.RunDir != runDir {
					t.Errorf("Wait() result = %+v; want the run's identity preserved (guid strand-1, session session-1, runDir %s)", result, runDir)
				}
			} else if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q", result.Outcome, tt.wantOutcome)
			}
			if len(reed.RemoveStrandCalls) != 0 {
				t.Errorf("RemoveStrand calls = %+v; want none — neither exit cleans up", reed.RemoveStrandCalls)
			}
			if _, err := os.Stat(runDir); err != nil {
				t.Errorf("run dir removed: %v; want it kept for diagnosis", err)
			}
		})
	}
}

// TestRun_Wait_UntrackedStrand_OutputFilesStillWin pins that the file contract outranks reed's
// bookkeeping: an agent that wrote every output file and was then untracked (its pane removed by a
// `lyx reed remove`, say) finished its work, and a caller must not be told to go diagnose a run that
// actually succeeded.
func TestRun_Wait_UntrackedStrand_OutputFilesStillWin(t *testing.T) {
	runDir := t.TempDir()
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: nil}}}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: filepath.Join(runDir, "events.jsonl")},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q (the file contract is satisfied, whatever reed still tracks)", result.Outcome, OutcomeDone)
	}
}

func TestRun_Wait_Died_ButOutputFilesExist_ClassifiesDone(t *testing.T) {
	// The pane died (reed.Status reports not live) but every output file
	// already exists on disk — the agent must have written its result and
	// then been killed (or exited) before its Stop hook ever appended a
	// turn-end line, so pollEventsTick had nothing to classify from. The
	// file contract is still satisfied: this must report done, not died, so
	// a caller does not needlessly respawn already-completed work.
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created: no Stop event fired
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}}}}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q (file contract satisfied despite a dead pane and no Stop event)", result.Outcome, OutcomeDone)
	}
	// A "done" outcome without KeepPane still runs the normal cleanup path.
	foundRemove := false
	for _, c := range reed.RemoveStrandCalls {
		if c.GUID == "strand-1" && !c.Recursive {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Errorf("RemoveStrand(strand-1, false) not recorded, calls = %+v", reed.RemoveStrandCalls)
	}
}

// TestRun_Wait_StartupDeadline_BindsEveryNotReadyPath is R2-F9's regression guard.
//
// The startup deadline used to be consulted from ONE arm of checkLivenessTick's switch,
// StartupPending, so the two other ways a run can sit in the startup window without ever reaching
// StartupReady — a trust prompt whose dismissal never takes, and a pane that fails every capture —
// escaped it entirely and ran on to the full run deadline instead. Both subtests below keep the run
// deadline (10 minutes) an order of magnitude beyond the startup deadline (1 second), so a run that
// only ever classifies OutcomeTimeout is exactly the pre-fix behaviour and a run that classifies
// OutcomeDied proves the startup window bound the path under test.
func TestRun_Wait_StartupDeadline_BindsEveryNotReadyPath(t *testing.T) {
	tests := []struct {
		name string
		// startupScript drains FIFO and its last entry then repeats forever, so a single-entry
		// script pins the pane in that state for the whole run.
		startupScript []StartupState
		captureErr    error
	}{
		{"trust_prompt_that_never_clears", []StartupState{StartupTrustPrompt}, nil},
		{"pane_capture_fails_every_probe", nil, errors.New("capture pane: no such pane")},
		{"still_booting", []StartupState{StartupPending}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl") // never created
			outputFile := filepath.Join(runDir, "out.md")       // never created

			reed := &fakeReed{
				StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
				CaptureErr:  tt.captureErr,
			}
			engine := &fakeEngine{StartupScript: tt.startupScript}
			runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(10 * time.Minute),
			}

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDied {
				t.Errorf("Outcome = %q; want %q — the 1s startup deadline must bind this path, not the 10m run deadline", result.Outcome, OutcomeDied)
			}
			// A run that reached the RUN deadline instead would have burned the whole 10 minutes of
			// virtual time; the startup deadline is 1s, so anything past a few seconds means the
			// window did not bind.
			if elapsed := fc.Now().Sub(run.deadline.Add(-10 * time.Minute)); elapsed > time.Minute {
				t.Errorf("virtual time elapsed = %s; want well under a minute (the 1s startup window), not the 10m run window", elapsed)
			}
		})
	}
}

func TestRun_Wait_Died_ViaStartupTimeout_TrustDismissRecorded(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	// First probe sees the trust prompt (dismissed with Enter); every probe
	// after that sees a still-booting pane, so the run never becomes ready
	// and eventually fast-fails once the startup deadline passes.
	engine := &fakeEngine{StartupScript: []StartupState{StartupTrustPrompt, StartupPending}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 1})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(10 * time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDied {
		t.Errorf("Outcome = %q, want %q (startup deadline expiry)", result.Outcome, OutcomeDied)
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
}

func TestRun_Wait_Timeout_KeepsStrand(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")       // never created

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Second},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Second),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeTimeout {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeTimeout)
	}
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v, want none (timeout keeps the strand)", reed.RemoveStrandCalls)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Errorf("run dir removed for timeout outcome: %v", err)
	}
}

// TestRun_Wait_ForkAudit_AttachedOnlyForForkModeDone proves finalize's AuditForks wiring: a
// fork-mode spec's done classification calls engine.AuditForks(sessionID, layout.AnchorPath()) and
// attaches its result to Result.ForkAudit, while a non-fork spec's done classification never calls
// AuditForks at all and leaves Result.ForkAudit nil.
func TestRun_Wait_ForkAudit_AttachedOnlyForForkModeDone(t *testing.T) {
	tests := []struct {
		name          string
		forkSubagents bool
	}{
		{"fork_mode_on_attaches_audit", true},
		{"fork_mode_off_no_audit_call", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			outputFile := filepath.Join(runDir, "out.md")
			if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
				t.Fatalf("seed output file: %v", err)
			}
			if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			cannedAudit := ForkAudit{SpawnCalls: 1, NamedSpawns: 0}
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
			engine := &fakeEngine{StartupScript: []StartupState{StartupReady}, AuditForksResult: cannedAudit}
			runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: tt.forkSubagents},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Minute),
			}

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}

			if tt.forkSubagents {
				if len(engine.AuditForksCalls) != 1 {
					t.Fatalf("AuditForksCalls = %v; want exactly one call", engine.AuditForksCalls)
				}
				call := engine.AuditForksCalls[0]
				if call.SessionID != "session-1" || call.Workdir != runner.anchorPath {
					t.Errorf("AuditForks called with (%q, %q); want (%q, %q)", call.SessionID, call.Workdir, "session-1", runner.anchorPath)
				}
				if result.ForkAudit == nil || !reflect.DeepEqual(*result.ForkAudit, cannedAudit) {
					t.Errorf("Result.ForkAudit = %+v; want it to carry the fake's canned audit %+v", result.ForkAudit, cannedAudit)
				}
			} else {
				if len(engine.AuditForksCalls) != 0 {
					t.Errorf("AuditForksCalls = %v; want none for a non-fork spec", engine.AuditForksCalls)
				}
				if result.ForkAudit != nil {
					t.Errorf("Result.ForkAudit = %+v; want nil for a non-fork spec", result.ForkAudit)
				}
			}
		})
	}
}

// TestRun_Wait_ForkAuditFailure_KeepsTheClassifiedOutcome is R2-F2's regression guard.
//
// A fork-mode run that satisfies the file contract has reached OutcomeDone before AuditForks is ever
// called, so an audit failure is a failure of the AUDIT, not of the run. finalize used to hand back
// run.identity() here — a Result with an empty Outcome — which is the shape Wait reserves for a
// mechanism failure that reached no classification at all. Because this branch also skips cleanup,
// leaving the strand and run dir in place exactly as a mechanism failure does, a caller had nothing
// left to tell the two apart with.
func TestRun_Wait_ForkAuditFailure_KeepsTheClassifiedOutcome(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{
		StartupScript: []StartupState{StartupReady},
		AuditForksErr: errors.New("read parent transcript: no such file or directory"),
	}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: true},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err == nil {
		t.Fatalf("Wait() error = nil; want the audit failure surfaced")
	}
	if !strings.Contains(err.Error(), "audit forks for session") {
		t.Errorf("Wait() error = %v; want it to name the fork audit", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q — the run satisfied the file contract before the audit was attempted", result.Outcome, OutcomeDone)
	}
	if result.StrandGUID != "strand-1" || result.SessionID != "session-1" || result.RunDir != runDir {
		t.Errorf("identity = (%q, %q, %q); want (%q, %q, %q)", result.StrandGUID, result.SessionID, result.RunDir, "strand-1", "session-1", runDir)
	}
	if result.ForkAudit != nil {
		t.Errorf("Result.ForkAudit = %+v; want nil — nil is what \"not audited\" means", result.ForkAudit)
	}
	// The audit failure must not have triggered the done-outcome cleanup: both the strand and the
	// run dir have to survive for the caller to diagnose what the audit could not read.
	if len(reed.RemoveStrandCalls) != 0 {
		t.Errorf("RemoveStrand calls = %+v; want none — an audit failure must not tear the run down", reed.RemoveStrandCalls)
	}
	if _, statErr := os.Stat(runDir); statErr != nil {
		t.Errorf("run dir removed after an audit failure: %v", statErr)
	}
}

func TestRun_Wait_MultiStopOffsetTracking(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking

	fixture := "STOP:first\nSTOP:second\n"
	if err := os.WriteFile(eventsPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	reed := &fakeReed{}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "second" {
		t.Errorf("LastAssistantMessage = %q, want %q (the LAST of the two Stop events)", result.LastAssistantMessage, "second")
	}
	if run.offset != int64(len(fixture)) {
		t.Errorf("offset = %d, want %d (both events consumed)", run.offset, len(fixture))
	}
}

func TestRun_Wait_ParseEventsFailure_BytesReReadOnRetry(t *testing.T) {
	// A ParseEvents error must NOT advance run.offset past the bytes it
	// failed to parse: if it did, the batch's Stop event would be discarded
	// unread once ParseEvents starts succeeding on the NEXT tick's (empty)
	// read, and the run would never classify. This proves the fix: the
	// same fixture is retried and DOES classify once the transient failure
	// clears.
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	fixture := "STOP:hello\n"
	if err := os.WriteFile(eventsPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking once classified

	reed := &fakeReed{}
	// Fail the first two ParseEvents calls; the third (retrying the SAME
	// unconsumed bytes) succeeds. maxEventsReadRetries is 3, so this must
	// stay under that budget to prove a retry recovers rather than erroring.
	engine := &fakeEngine{ParseEventsFailCount: 2}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v, want the retry to recover and classify", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "hello" {
		t.Errorf("LastAssistantMessage = %q, want %q — the batch a failed parse left unconsumed must still be classified once parsing succeeds", result.LastAssistantMessage, "hello")
	}
	if run.offset != int64(len(fixture)) {
		t.Errorf("offset = %d, want %d (bytes consumed only after a successful parse)", run.offset, len(fixture))
	}
}

func TestRun_Wait_EventsOffsetResilience_PartialLine(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("STOP:partial"), 0o644); err != nil { // no trailing newline yet
		t.Fatalf("seed partial events: %v", err)
	}
	outputFile := filepath.Join(runDir, "out.md") // never created -> asking once classified

	reed := &fakeReed{}
	engine := &fakeEngine{}
	runner := newWaitTestRunner(t, reed, engine, Config{PollIntervalMS: 1, LivenessEveryNPolls: 100, StartupTimeoutS: 30})

	fc := newFakeClock(time.Now())
	sc := &scriptedClock{fakeClock: fc, onSleep: func() {
		// Complete the partial line between tick 1 and tick 2 so the next
		// read sees a full Stop event.
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open events file to append: %v", err)
		}
		defer f.Close()
		if _, err := f.WriteString("\n"); err != nil {
			t.Fatalf("append newline: %v", err)
		}
	}}

	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: eventsPath},
		clock:    sc,
		deadline: sc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if result.LastAssistantMessage != "partial" {
		t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, "partial")
	}
}

// TestRun_Wait_ClearedPaneBinding_IsMechanismFailureNotDied is R4-F2's regression guard.
//
// Reed clears every pane binding in a state file whose recorded pane generation is not the session
// incarnation now running, and its Status then reports the strand with an EMPTY PaneID — which its
// liveness lookup answers false for, since no pane is bound to look up. Wait read that not-live
// answer as a dead pane and returned ok:true/outcome:"died". Reproduced live in round 4: reed logged
// the clear, shuttle answered "died" 4 s later, and the agent was still working in a pane tmux
// reported alive — proven by restoring the stamp, after which the same strand reported live:true on
// the same pane again.
//
// The hidden row is the one case that must NOT change: an anchor:hidden strand is never given a pane,
// so its empty PaneID is normal rather than cleared. The genuinely-dead-pane case (a bound pane id
// that is not alive) is pinned by TestRun_Wait_UntrackedStrand_IsMechanismFailureNotDied's second row.
func TestRun_Wait_ClearedPaneBinding_IsMechanismFailureNotDied(t *testing.T) {
	tests := []struct {
		name        string
		anchor      render.Anchor
		wantOutcome Outcome
		wantErr     bool
	}{
		{
			name:        "cleared_binding_under_an_ordinary_run_is_a_mechanism_failure",
			anchor:      render.AnchorBelowParent,
			wantOutcome: "",
			wantErr:     true,
		},
		{
			name:        "hidden_strand_never_had_a_pane_and_is_still_died",
			anchor:      render.AnchorHidden,
			wantOutcome: OutcomeDied,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl") // never created
			outputFile := filepath.Join(runDir, "out.md")       // never created

			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
				Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}},
			}}}
			runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
			fc := newFakeClock(time.Now())
			run := &Run{
				runner:   runner,
				spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: tt.anchor}},
				runDir:   runDir,
				state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
				clock:    fc,
				deadline: fc.Now().Add(time.Minute),
			}

			result, err := run.Wait()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Wait() = (%+v, nil); want the cleared-pane-binding mechanism error", result)
				}
				if !errors.Is(err, errStrandPaneBindingCleared) {
					t.Errorf("Wait() error = %v; want one wrapping errStrandPaneBindingCleared", err)
				}
				if result.StrandGUID != "strand-1" || result.SessionID != "session-1" || result.RunDir != runDir {
					t.Errorf("Wait() result = %+v; want the run's identity preserved (guid strand-1, session session-1, runDir %s)", result, runDir)
				}
			} else if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q", result.Outcome, tt.wantOutcome)
			}
			if len(reed.RemoveStrandCalls) != 0 {
				t.Errorf("RemoveStrand calls = %+v; want none — neither exit cleans up", reed.RemoveStrandCalls)
			}
			if _, err := os.Stat(runDir); err != nil {
				t.Errorf("run dir removed: %v; want it kept for diagnosis", err)
			}
		})
	}
}

// TestRun_Wait_ClearedPaneBinding_OutputFilesStillWin pins that the file contract outranks a cleared
// binding exactly as it outranks the other two negative liveness answers: an agent that wrote every
// output file finished its work, whether or not reed can still address its pane.
func TestRun_Wait_ClearedPaneBinding_OutputFilesStillWin(t *testing.T) {
	runDir := t.TempDir()
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
		t.Fatalf("seed output file: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{
		Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}},
	}}}
	runner := newWaitTestRunner(t, reed, &fakeEngine{}, Config{PollIntervalMS: 1, LivenessEveryNPolls: 1, StartupTimeoutS: 30})
	fc := newFakeClock(time.Now())
	run := &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, Display: render.Display{Anchor: render.AnchorBelowParent}},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", EventsPath: filepath.Join(runDir, "events.jsonl")},
		clock:    fc,
		deadline: fc.Now().Add(time.Minute),
	}

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v; want the satisfied file contract to classify done", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q — the output files ARE the run's return value", result.Outcome, OutcomeDone)
	}
}
