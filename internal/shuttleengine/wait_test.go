// wait_test.go covers Run.Wait's poll loop against fakeReed/fakeEngine and a fake clock: all four
// outcome classifications, KeepPane skipping cleanup, the startup probe's trust-dismiss and
// fast-fail-on-timeout paths, multi-Stop offset tracking, events-offset resilience across a partial
// line, and finalize's fork-audit attach (only for a fork-mode spec's done classification).

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// TestPollInterval_FloorsNonPositive pins the busy-spin guard: a configured poll_interval_ms of 0
// or below must fall back to the template default rather than making Wait tick with a zero sleep.
//
//testtiming:keep pins the busy-spin guard: a non-positive poll_interval_ms falls back to the template default, which no Wait test measures
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
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig))
	runDir := t.TempDir()
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{filepath.Join(runDir, "out.md")}, Timeout: time.Minute},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: filepath.Join(runDir, "events.jsonl")}),
		withRunClock(fc, fc.Now().Add(time.Minute)))

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

// cleanupExpectation says what a Wait that finalized must have done to the strand and run dir.
type cleanupExpectation int

const (
	// cleanupUnchecked leaves the strand and run dir unasserted.
	cleanupUnchecked cleanupExpectation = iota
	// cleanupPerformed expects the strand removed (non-recursively) and the run dir gone.
	cleanupPerformed
	// cleanupSkipped expects the strand kept and the run dir kept for diagnosis.
	cleanupSkipped
)

// TestRun_Wait_Classification drives Run.Wait over the pane, events and file states that decide an
// outcome, and what each leaves behind. The file contract outranks every negative liveness answer:
// an agent that wrote every output file finished its work, whatever reed still tracks — an
// untracked strand, a dead pane, a cleared pane binding, a reed.Status that errors past its retry
// cap, or an unparseable events file past its cap — and the same contract wins over a live ask.
// The "died" and "mechanism failure" rows are R3-F1's and R4-F2's guards, which narrow the
// not-live branch rather than remove it:
//
//   - Wait used to derive one boolean from reed's strand table and treat both negative answers
//     alike, so a Status that succeeded with the run's guid simply ABSENT — reed's bookkeeping reset
//     under a run whose agent is still working — classified OutcomeDied. Reproduced live twice:
//     renaming the worktree under an in-flight run, and deleting .lyx/reed.json both returned
//     outcome:"died" ~6 s later while the claude process kept working in its pane.
//   - Reed clears every pane binding in a state file whose recorded pane generation is not the
//     session incarnation now running, and its Status then reports the strand with an EMPTY PaneID,
//     which its liveness lookup answers false for. Wait read that as a dead pane. The hidden row is
//     the one case that must NOT change: an anchor:hidden strand is never given a pane, so its empty
//     PaneID is normal rather than cleared.
//
// A mechanism failure must keep the run's identity: the agent may still be live, so the caller
// needs the handles to reach it. Finalize also logs the teardown through internal/logger, so the
// durable Info+ trace file shows every shuttle run ending as well as beginning.
//
//testtiming:keep pins the outcome, message and cleanup of every pane, events and file state Wait classifies, of which the AwaitOperator test reaches only a few
func TestRun_Wait_Classification(t *testing.T) {
	liveStrands := []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}
	deadPane := []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}}}
	clearedBinding := []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "", Live: false}}}}
	ready := []StartupState{StartupReady}

	tests := []struct {
		name           string
		cfg            Config
		timeout        time.Duration
		status         []reedengine.StatusResult
		statusErr      error
		startup        []StartupState
		parseEventsErr error
		// events is the events file's seed; empty leaves it uncreated.
		events      string
		output      bool
		keepPane    bool
		anchor      render.Anchor
		wantOutcome Outcome
		wantMessage string
		// wantErr is the error Wait must wrap; wantErrIn a fragment it must name, wantIdentity that
		// the Result still carries the run's identity.
		wantErr      error
		wantErrIn    string
		wantIdentity bool
		wantCleanup  cleanupExpectation
		wantLogged   []string
	}{
		{
			name: "done cleans up", status: liveStrands, startup: ready, events: "STOP:done\n", output: true,
			wantOutcome: OutcomeDone, wantCleanup: cleanupPerformed,
			wantLogged: []string{"shuttle: run finished", "outcome=done", "strand-1", "cleanedUp=true"},
		},
		{
			name: "done with KeepPane skips cleanup", status: liveStrands, startup: ready, events: "STOP:done\n", output: true, keepPane: true,
			wantOutcome: OutcomeDone, wantCleanup: cleanupSkipped,
			wantLogged: []string{"shuttle: run finished", "outcome=done", "strand-1", "cleanedUp=false"},
		},
		{
			name: "asking carries the message and keeps the strand", status: liveStrands, startup: ready, events: "STOP:need operator input\n",
			wantOutcome: OutcomeAsking, wantMessage: "need operator input", wantCleanup: cleanupSkipped,
		},
		{
			// An EventAsk with no output files present classifies asking just like the turn-end case,
			// proving the unchanged pollEventsTick branch also covers the live-ask signal ParseEvents emits.
			name: "live ask classifies real-time asking", status: liveStrands, startup: ready, events: "ASK:which approach?\n",
			wantOutcome: OutcomeAsking, wantMessage: "which approach?", wantCleanup: cleanupSkipped,
		},
		{
			// An EventAsk never overrides an already-satisfied file contract.
			name: "live ask with output files already present is done first", status: liveStrands, startup: ready, events: "ASK:which approach?\n", output: true,
			wantOutcome: OutcomeDone,
		},
		{name: "dead pane is died and keeps the strand", status: deadPane, wantOutcome: OutcomeDied, wantCleanup: cleanupSkipped},
		{
			name:        "strand absent from reed's table is a mechanism failure, not died",
			status:      []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "someone-elses-strand", Live: true}}}},
			wantOutcome: "", wantErr: errStrandNotTracked, wantErrIn: "strand-1", wantIdentity: true, wantCleanup: cleanupSkipped,
		},
		{
			// An agent that wrote every output file and was then untracked (its pane removed by a
			// `lyx reed remove`, say) finished its work, and a caller must not be told to go
			// diagnose a run that actually succeeded.
			name: "untracked strand with output files is still done", status: []reedengine.StatusResult{{Strands: nil}}, output: true,
			wantOutcome: OutcomeDone,
		},
		{
			// The pane died but every output file already exists on disk: the agent must have
			// written its result and then been killed (or exited) before its Stop hook ever appended
			// a turn-end line, so a caller must not needlessly respawn already-completed work. A done
			// outcome without KeepPane still runs the normal cleanup path.
			name: "dead pane with output files is done", status: deadPane, output: true,
			wantOutcome: OutcomeDone, wantCleanup: cleanupPerformed,
		},
		{
			name: "cleared pane binding under an ordinary run is a mechanism failure", status: clearedBinding, anchor: render.AnchorBelowParent,
			wantOutcome: "", wantErr: errStrandPaneBindingCleared, wantIdentity: true, wantCleanup: cleanupSkipped,
		},
		{
			name: "hidden strand never had a pane and is still died", status: clearedBinding, anchor: render.AnchorHidden,
			wantOutcome: OutcomeDied, wantCleanup: cleanupSkipped,
		},
		{
			// The output files ARE the run's return value.
			name: "cleared pane binding with output files is still done", status: clearedBinding, anchor: render.AnchorBelowParent, output: true,
			wantOutcome: OutcomeDone,
		},
		{
			// reed.Status errors on every call (the shape a crash-corrupted or truncated reed.json, or
			// a torn-down session, produces), so the run reaches maxStatusRetries consecutive liveness
			// failures — but every declared output file is on disk, so it finished. events.jsonl is
			// never created, so the ONLY path to done is the mechanism-failure cap's own file-contract
			// check.
			name:      "status failure cap with output files is done",
			statusErr: errors.New(`reed state file is unreadable: unmarshal state: unexpected end of JSON input`), output: true,
			wantOutcome: OutcomeDone,
		},
		{
			// ParseEvents fails on every call, so the run reaches maxEventsReadRetries consecutive
			// parse failures — but every declared output file is on disk. LivenessEveryNPolls is high
			// so the events cap, not a liveness tick, is what fires.
			name: "events unreadable cap with output files is done", cfg: sparseProbeConfig, status: boundLivePane(), events: "garbage that never parses\n", output: true,
			parseEventsErr: errors.New("parse events: malformed"), wantOutcome: OutcomeDone,
		},
		{
			name: "timeout keeps the strand", cfg: Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 30}, timeout: time.Second,
			status: liveStrands, startup: ready, wantOutcome: OutcomeTimeout, wantCleanup: cleanupSkipped,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			outputFile := filepath.Join(runDir, "out.md")
			if tt.events != "" {
				if err := os.WriteFile(eventsPath, []byte(tt.events), 0o644); err != nil {
					t.Fatalf("seed events: %v", err)
				}
			}
			if tt.output {
				touchOutputFile(t, outputFile)
			}
			cfg := tt.cfg
			if cfg == (Config{}) {
				cfg = fastConfig
			}
			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Minute
			}

			reed := &fakeReed{StatusQueue: tt.status, StatusErr: tt.statusErr}
			engine := &fakeEngine{StartupScript: tt.startup, ParseEventsErr: tt.parseEventsErr}
			fx := newFixture(t, reed, engine, withConfig(cfg))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: timeout, KeepPane: tt.keepPane, Display: render.Display{Anchor: tt.anchor}},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(timeout)))

			buf := logcapture.CaptureVerbose(t)
			result, err := run.Wait()
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Wait() = (%+v, nil); want an error wrapping %v", result, tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Wait() error = %v; want one wrapping %v", err, tt.wantErr)
				}
				if tt.wantErrIn != "" && !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Errorf("Wait() error = %v; want it to name %q so the operator can find the pane", err, tt.wantErrIn)
				}
			} else if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if tt.wantIdentity && (result.StrandGUID != "strand-1" || result.SessionID != "session-1" || result.RunDir != runDir) {
				t.Errorf("Wait() result = %+v; want the run's identity preserved (guid strand-1, session session-1, runDir %s)", result, runDir)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q", result.Outcome, tt.wantOutcome)
			}
			if tt.wantMessage != "" && result.LastAssistantMessage != tt.wantMessage {
				t.Errorf("LastAssistantMessage = %q; want %q", result.LastAssistantMessage, tt.wantMessage)
			}

			switch tt.wantCleanup {
			case cleanupPerformed:
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
					t.Errorf("run dir still exists after cleanup, stat err = %v", err)
				}
			case cleanupSkipped:
				if len(reed.RemoveStrandCalls) != 0 {
					t.Errorf("RemoveStrand calls = %+v; want none — this exit keeps the strand", reed.RemoveStrandCalls)
				}
				if _, err := os.Stat(runDir); err != nil {
					t.Errorf("run dir removed: %v; want it kept for diagnosis", err)
				}
			}
			logged := buf.String()
			for _, want := range tt.wantLogged {
				if !strings.Contains(logged, want) {
					t.Errorf("teardown log = %q; want it to contain %q", logged, want)
				}
			}
		})
	}
}

// boundLivePane is a reed Status answer for strand-1 with a bound pane that is live.
func boundLivePane() []reedengine.StatusResult {
	return []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: true}}}}
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
		fx := newFixture(t, reed, engine, withConfig(fastConfig))
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: false},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Minute)))

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
		fx := newFixture(t, reed, engine, withConfig(sparseProbeConfig))
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

		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(mc, mc.Now().Add(time.Minute)))

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
		fx := newFixture(t, reed, engine, withConfig(sparseProbeConfig))
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

		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(mc, mc.Now().Add(time.Minute)))

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
		fx := newFixture(t, reed, engine, withConfig(Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 30}))
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Second, AwaitOperator: true},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Second)))

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
		fx := newFixture(t, reed, engine, withConfig(fastConfig))
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Minute)))

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
		fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig))
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Minute)))

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
		fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig))
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, AwaitOperator: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Minute)))

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

// TestRun_Wait_StartupWindow drives Wait over a live pane that never reaches StartupReady, with the
// run deadline (10 minutes) an order of magnitude beyond the startup deadline (1 second), so a run
// that only ever classifies OutcomeTimeout is the pre-fix behaviour and one that classifies
// OutcomeDied or OutcomeDone inside a minute of virtual time proves the startup window bound the
// path under test.
//
// R2-F9: the startup deadline used to be consulted from ONE arm of checkLivenessTick's switch,
// StartupPending, so the two other ways a run can sit in the startup window — a trust prompt whose
// dismissal never takes, and a pane that fails every capture — escaped it and ran on to the full run
// deadline. A run whose persisted RunState.Started is false still runs the startup probe and
// classifies OutcomeDied at the window's end, whatever an attach's own reed reads said of its pane's
// liveness: the attached-but-never-actually-started shape a driver killed before its first liveness
// tick, or a launch against a nonexistent binary, both leave behind.
//
// F1 (crucible round opus5-high-r4): the window expiring is a NEGATIVE answer, and a satisfied file
// contract outranks every negative answer — but classifyStartupWindow used to return a bare
// OutcomeDied on the clock alone. With every declared output file ALREADY WRITTEN and events.jsonl
// never created, the file contract is the only evidence the run finished and the startup deadline
// is the only thing that ever classifies it.
//
// A dismissed trust prompt is recorded: the engine must be handed the SAME capture Startup
// classified, not an empty or stale one, because a provider whose gate is a selection list can only
// tell which key confirms the ACCEPTING option by reading the caret out of that capture.
//
//testtiming:keep pins that the startup window, not the run deadline, classifies every not-ready path, and that a satisfied file contract outranks the expired window
func TestRun_Wait_StartupWindow(t *testing.T) {
	const gateCapture = "❯ No, exit\n  Yes, I trust this folder"
	tests := []struct {
		name string
		// startupScript drains FIFO and its last entry then repeats forever, so a single-entry
		// script pins the pane in that state for the whole run.
		startupScript []StartupState
		captureErr    error
		captureQueue  []string
		output        bool
		wantOutcome   Outcome
		// wantTrustDismissed asserts an Enter was sent and the engine saw gateCapture.
		wantTrustDismissed bool
	}{
		{name: "trust prompt that never clears", startupScript: []StartupState{StartupTrustPrompt}, wantOutcome: OutcomeDied},
		{name: "pane capture fails every probe", captureErr: errors.New("capture pane: no such pane"), wantOutcome: OutcomeDied},
		{name: "still booting", startupScript: []StartupState{StartupPending}, wantOutcome: OutcomeDied},
		{name: "trust prompt that never clears with output files", startupScript: []StartupState{StartupTrustPrompt}, output: true, wantOutcome: OutcomeDone},
		{name: "pane capture fails every probe with output files", captureErr: errors.New("capture pane: no such pane"), output: true, wantOutcome: OutcomeDone},
		{name: "still booting with output files", startupScript: []StartupState{StartupPending}, output: true, wantOutcome: OutcomeDone},
		{
			// The first probe sees the trust prompt (dismissed with Enter); every probe after that
			// sees a still-booting pane, so the run never becomes ready and fast-fails once the
			// startup deadline passes.
			name:          "dismissed trust prompt is recorded before the window expires",
			startupScript: []StartupState{StartupTrustPrompt, StartupPending}, captureQueue: []string{gateCapture},
			wantOutcome: OutcomeDied, wantTrustDismissed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl") // never created
			outputFile := filepath.Join(runDir, "out.md")
			if tt.output {
				touchOutputFile(t, outputFile)
			}

			reed := &fakeReed{
				StatusQueue:  []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
				CaptureErr:   tt.captureErr,
				CaptureQueue: tt.captureQueue,
			}
			engine := &fakeEngine{StartupScript: tt.startupScript}
			fx := newFixture(t, reed, engine, withConfig(shortStartupConfig))
			fc := newFakeClock(time.Now())
			// state.Started is the zero value (false): a persisted run.json whose provider never
			// reached StartupReady.
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: 10 * time.Minute},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath, Started: false}),
				withRunClock(fc, fc.Now().Add(10*time.Minute)))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q — the 1s startup window, not the 10m run deadline, must be what classified this", result.Outcome, tt.wantOutcome)
			}
			// A run that reached the RUN deadline instead would have burned the whole 10 minutes of
			// virtual time; the startup deadline is 1s, so anything past a few seconds means the
			// window did not bind.
			if elapsed := fc.Now().Sub(run.deadline.Add(-10 * time.Minute)); elapsed > time.Minute {
				t.Errorf("virtual time elapsed = %s; want well under a minute (the 1s startup window), not the 10m run window", elapsed)
			}

			if !tt.wantTrustDismissed {
				return
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
			if len(captures) == 0 {
				t.Fatalf("TrustDismissSequence was never called; SendKey calls = %+v", reed.SendKeyCalls)
			}
			if captures[0] != gateCapture {
				t.Errorf("TrustDismissSequence got capture %q; want the capture Startup classified, %q", captures[0], gateCapture)
			}
		})
	}
}

// TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout is F2's regression guard (crucible
// round opus5-high-r4): the run deadline expiring is the second place a clock used to publish
// itself as a verdict on whether the run finished.
//
// The run here has already reached StartupReady (started is seeded true through the persisted
// RunState, so the startup probe never runs and cannot be what classifies this), every declared
// output file is on disk, and events.jsonl is never created — so the file contract is the only
// evidence of completion and the RUN deadline is the only thing that ever fires. Reverting the fix
// (a bare `return run.finalize(OutcomeTimeout, "")`) makes this fail on the outcome.
//
// Reproduced live before being written: a provider that rendered the ready marker, wrote both of
// Discussion-Write's output files, and then stayed alive without appending to events.jsonl was
// recorded as `run.json` outcome "timeout" with started true, and the step as a shed failure.
//
//testtiming:keep pins that a satisfied file contract outranks the run deadline for a started run, with no startup probe run
func TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl") // never created
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)

	reed := &fakeReed{
		StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
	}
	// StartupScript deliberately left empty: with started seeded true the startup probe must never
	// run, so any Startup call at all would mean this test is measuring the wrong deadline.
	engine := &fakeEngine{}
	fx := newFixture(t, reed, engine, withConfig(Config{PollIntervalMS: 600, LivenessEveryNPolls: 1, StartupTimeoutS: 300}))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath, Started: true}),
		withRunClock(fc, fc.Now().Add(time.Minute)))

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q — every declared output file exists, so the run finished whatever the clock says", result.Outcome, OutcomeDone)
	}
	if len(engine.StartupCalls) != 0 {
		t.Errorf("engine.StartupCalls = %v; want none — started was seeded true, so the RUN deadline is what this case measures", engine.StartupCalls)
	}
}

// TestRun_Wait_StartedRun_SkipsStartupProbe pins the seed from the other direction: a run obtained
// from Runner.StartGated over readyStart-scripted fakes has already persisted Started: true through
// Start's own successful probe, so a later Wait over the same handle must never re-run the startup
// probe at all -- neither engine.StartupCalls nor the CapturePane count in reed.CallLog may grow past
// what Start itself already recorded.
//
//testtiming:keep pins that Wait over a started handle never re-runs the startup probe, with no new engine Startup call and no new capture
func TestRun_Wait_StartedRun_SkipsStartupProbe(t *testing.T) {
	reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
	readyStart(reed, engine)
	fc := newFakeClock(time.Now())
	runner := newFixture(t, reed, engine, withConfig(fastConfig), withClock(fc)).Runner

	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, err := runner.StartGated(Spec{Prompt: "x", OutputFiles: []string{outputFile}}, GateSpec{})
	if err != nil {
		t.Fatalf("StartGated() error: %v", err)
	}

	startupCallsAfterStart := len(engine.StartupCalls)
	captureCallsAfterStart := 0
	for _, c := range reed.CallLog {
		if c == "CapturePane" {
			captureCallsAfterStart++
		}
	}

	if err := os.WriteFile(filepath.Join(run.RunDir(), eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	touchOutputFile(t, outputFile)

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q; want %q", result.Outcome, OutcomeDone)
	}
	if len(engine.StartupCalls) != startupCallsAfterStart {
		t.Errorf("engine.StartupCalls grew from %d to %d; want unchanged -- Wait must not re-run the startup probe for a started run", startupCallsAfterStart, len(engine.StartupCalls))
	}
	captureCallsAfterWait := 0
	for _, c := range reed.CallLog {
		if c == "CapturePane" {
			captureCallsAfterWait++
		}
	}
	if captureCallsAfterWait != captureCallsAfterStart {
		t.Errorf("CapturePane calls grew from %d to %d; want unchanged", captureCallsAfterStart, captureCallsAfterWait)
	}
}

// TestRun_Wait_ForkAudit proves finalize's AuditForks wiring: a fork-mode spec's done
// classification calls engine.AuditForks(sessionID, the runner's paneCwd) and attaches its result to
// Result.ForkAudit, while a non-fork spec's done classification never calls AuditForks at all and
// leaves Result.ForkAudit nil. The audit gets the runner's paneCwd rather than its anchorPath; the
// two are the same value in the hub shape, so a detached runner whose pane runs at the worktree
// root while its anchor sits outside it (standalone geometry) is what tells them apart.
func TestRun_Wait_ForkAudit(t *testing.T) {
	tests := []struct {
		name          string
		forkSubagents bool
		// detachPaneCwd points the runner's paneCwd away from its anchorPath.
		detachPaneCwd bool
	}{
		{name: "fork mode on attaches the audit", forkSubagents: true},
		{name: "fork mode off makes no audit call"},
		{name: "the audit runs at paneCwd, not anchorPath", forkSubagents: true, detachPaneCwd: true},
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
			fx := newFixture(t, reed, engine, withConfig(fastConfig))
			if tt.detachPaneCwd {
				fx.Runner.paneCwd = t.TempDir()
			}
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: tt.forkSubagents},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
				withRunClock(fc, fc.Now().Add(time.Minute)))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
			}

			if !tt.forkSubagents {
				if len(engine.AuditForksCalls) != 0 {
					t.Errorf("AuditForksCalls = %v; want none for a non-fork spec", engine.AuditForksCalls)
				}
				if result.ForkAudit != nil {
					t.Errorf("Result.ForkAudit = %+v; want nil for a non-fork spec", result.ForkAudit)
				}
				return
			}
			if len(engine.AuditForksCalls) != 1 {
				t.Fatalf("AuditForksCalls = %v; want exactly one call", engine.AuditForksCalls)
			}
			call := engine.AuditForksCalls[0]
			if call.SessionID != "session-1" || call.Workdir != fx.Runner.paneCwd {
				t.Errorf("AuditForks called with (%q, %q); want (%q, %q)", call.SessionID, call.Workdir, "session-1", fx.Runner.paneCwd)
			}
			if tt.detachPaneCwd && call.Workdir == fx.Runner.anchorPath {
				t.Errorf("AuditForks called with workdir %q == anchorPath; want it to differ, proving the audit moved off anchorPath", call.Workdir)
			}
			if result.ForkAudit == nil || !reflect.DeepEqual(*result.ForkAudit, cannedAudit) {
				t.Errorf("Result.ForkAudit = %+v; want it to carry the fake's canned audit %+v", result.ForkAudit, cannedAudit)
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
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute, ForkSubagents: true},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(fc, fc.Now().Add(time.Minute)))

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

// TestRun_Wait_EventsHandling drives Wait over an events file whose Stop events arrive in awkward
// shapes, with the output file never created so a classified batch is asking: two Stops in one read
// classify as the LAST of them with both consumed; a ParseEvents error must NOT advance run.offset
// past the bytes it failed to parse, or the batch's Stop event would be discarded unread once
// parsing starts succeeding on the NEXT tick's (empty) read, so the same bytes are retried and DO
// classify once the transient failure clears (maxEventsReadRetries is 3, so two failures stay under
// the budget); and a partial line, with no trailing newline yet, is not consumed until a later tick
// sees it complete.
//
//testtiming:keep pins the offset rules over the events file: the last of several Stops wins, a failed parse's bytes are re-read, and a partial line stays unconsumed
func TestRun_Wait_EventsHandling(t *testing.T) {
	tests := []struct {
		name   string
		events string
		// parseFailCount is the number of leading ParseEvents calls that fail.
		parseFailCount int
		// completeLineOnFirstSleep appends the newline that completes a partial line between tick 1
		// and tick 2.
		completeLineOnFirstSleep bool
		wantMessage              string
		// wantOffset is the offset once the whole file is consumed.
		wantOffset int64
	}{
		{name: "multiple stops classify as the last one", events: "STOP:first\nSTOP:second\n", wantMessage: "second", wantOffset: int64(len("STOP:first\nSTOP:second\n"))},
		{name: "a failed parse leaves its bytes to be re-read on retry", events: "STOP:hello\n", parseFailCount: 2, wantMessage: "hello", wantOffset: int64(len("STOP:hello\n"))},
		{name: "a partial line waits for its newline", events: "STOP:partial", completeLineOnFirstSleep: true, wantMessage: "partial", wantOffset: int64(len("STOP:partial\n"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			if err := os.WriteFile(eventsPath, []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}
			outputFile := filepath.Join(runDir, "out.md") // never created -> asking once classified

			engine := &fakeEngine{ParseEventsFailCount: tt.parseFailCount}
			fx := newFixture(t, &fakeReed{}, engine, withConfig(sparseProbeConfig))
			fc := newFakeClock(time.Now())
			var clk clock = fc
			if tt.completeLineOnFirstSleep {
				clk = &scriptedClock{fakeClock: fc, onSleep: func() {
					appendEventsLine(t, eventsPath, "")
				}}
			}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
				withRunClock(clk, clk.Now().Add(time.Minute)))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeAsking {
				t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
			}
			if result.LastAssistantMessage != tt.wantMessage {
				t.Errorf("LastAssistantMessage = %q, want %q", result.LastAssistantMessage, tt.wantMessage)
			}
			if run.offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d (bytes consumed only after a successful parse of a complete line)", run.offset, tt.wantOffset)
			}
		})
	}
}

// TestRun_Wait_Finalize_PersistsOutcomeForEveryTerminalOutcome pins that finalize overwrites the
// persisted RunState.Outcome with the matching classification string for every terminal outcome, not
// only OutcomeDone — the fact on disk a later Attach relies on. The done outcome with KeepPane also
// pins that the write happens before the done-outcome cleanup: the run dir survives cleanup, so its
// persisted run.json must hold "done" rather than a stale "running".
//
//testtiming:keep pins that finalize persists the matching Outcome in run.json for every terminal outcome, and does so before the done cleanup
func TestRun_Wait_Finalize_PersistsOutcomeForEveryTerminalOutcome(t *testing.T) {
	tests := []struct {
		name        string
		seedEvents  string
		seedOutput  bool
		keepPane    bool
		statusQueue []reedengine.StatusResult
		startup     []StartupState
		timeout     time.Duration
		wantOutcome Outcome
	}{
		{
			name:        "done",
			seedEvents:  "STOP:done\n",
			seedOutput:  true,
			statusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
			startup:     []StartupState{StartupReady},
			timeout:     time.Minute,
			wantOutcome: OutcomeDone,
		},
		{
			name:        "done with KeepPane",
			seedEvents:  "STOP:done\n",
			seedOutput:  true,
			keepPane:    true,
			statusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
			startup:     []StartupState{StartupReady},
			timeout:     time.Minute,
			wantOutcome: OutcomeDone,
		},
		{
			name:        "asking",
			seedEvents:  "STOP:need operator input\n",
			statusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
			startup:     []StartupState{StartupReady},
			timeout:     time.Minute,
			wantOutcome: OutcomeAsking,
		},
		{
			name:        "died",
			statusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%0", Live: false}}}},
			timeout:     time.Minute,
			wantOutcome: OutcomeDied,
		},
		{
			name:        "timeout",
			statusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}},
			startup:     []StartupState{StartupReady},
			timeout:     time.Second,
			wantOutcome: OutcomeTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, "events.jsonl")
			outputFile := filepath.Join(runDir, "out.md")
			if tt.seedEvents != "" {
				if err := os.WriteFile(eventsPath, []byte(tt.seedEvents), 0o644); err != nil {
					t.Fatalf("seed events: %v", err)
				}
			}
			if tt.seedOutput {
				if err := os.WriteFile(outputFile, []byte("result"), 0o644); err != nil {
					t.Fatalf("seed output file: %v", err)
				}
			}

			reed := &fakeReed{StatusQueue: tt.statusQueue}
			engine := &fakeEngine{StartupScript: tt.startup}
			pollMS := 1
			if tt.name == "timeout" {
				pollMS = 600
			}
			fx := newFixture(t, reed, engine, withConfig(Config{PollIntervalMS: pollMS, LivenessEveryNPolls: 1, StartupTimeoutS: 30}))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: tt.timeout, KeepPane: tt.keepPane},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath, Outcome: runOutcomeRunning}),
				withRunClock(fc, fc.Now().Add(tt.timeout)))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, tt.wantOutcome)
			}
			if run.state.Outcome != string(tt.wantOutcome) {
				t.Errorf("run.state.Outcome = %q, want %q", run.state.Outcome, tt.wantOutcome)
			}

			// The done outcome without KeepPane cleans the run dir up entirely, so there is no run.json
			// left to read — the in-memory run.state assertion above is the only observable proof.
			if tt.wantOutcome == OutcomeDone && !tt.keepPane {
				return
			}
			rs, found, err := loadRunState(runDir)
			if err != nil {
				t.Fatalf("loadRunState: %v", err)
			}
			if !found {
				t.Fatal("loadRunState: run.json not found")
			}
			if rs.Outcome != string(tt.wantOutcome) {
				t.Errorf("persisted RunState.Outcome = %q, want %q", rs.Outcome, tt.wantOutcome)
			}
		})
	}
}

// TestRun_Wait_Finalize_OutcomeWriteFailure_StillReturnsClassifiedResult pins the best-effort
// disposition: a saveRunState failure inside finalize must not fail the run — the classified Result
// is returned unchanged, per attach-only-a-run-that-never-terminated's failed-write disposition.
// saveRunState is made to fail by replacing run.json's path with a directory before Wait finalizes,
// following fakeEngine.PrepareHook's precedent for planting on-disk state that breaks a later
// saveRunState call.
func TestRun_Wait_Finalize_OutcomeWriteFailure_StillReturnsClassifiedResult(t *testing.T) {
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, "events.jsonl")
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(eventsPath, []byte("STOP:need operator input\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	// run.json as a directory makes saveRunState's write fail without disturbing anything else
	// finalize touches (the outcome is asking, so no cleanup runs regardless).
	if err := os.MkdirAll(filepath.Join(runDir, runStateFileName), 0o755); err != nil {
		t.Fatalf("plant run.json dir: %v", err)
	}

	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Live: true}}}}}
	engine := &fakeEngine{StartupScript: []StartupState{StartupReady}}
	fx := newFixture(t, reed, engine, withConfig(fastConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath, Outcome: runOutcomeRunning}),
		withRunClock(fc, fc.Now().Add(time.Minute)))

	buf := logcapture.CaptureVerbose(t)
	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v, want the classified Result returned despite the failed Outcome write", err)
	}
	if result.Outcome != OutcomeAsking {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeAsking)
	}
	if !strings.Contains(buf.String(), "persist run outcome failed") {
		t.Errorf("logger output = %q; want the best-effort write failure logged", buf.String())
	}
}
