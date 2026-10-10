package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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
// the parsed bytes, a Stop after it is a held turn end carrying its message and the offset past its line,
// and with the output files present it is done.
//
//testtiming:keep pins that a waiting turn end is still running and advances the offset, becomes a held turn end on a later Stop, and is done when the output files exist
func TestPollEventsTick_Waiting(t *testing.T) {
	const waitLine = "WAIT:background work\n"
	tests := []struct {
		name         string
		touchOutput  bool
		wantFirst    Outcome
		checkOffset  bool
		checkTrace   bool
		stopAfter    string
		wantSecondIn string
	}{
		{name: "waiting is still running", wantFirst: "", checkOffset: true, checkTrace: true},
		{
			name: "a stop after waiting is held", wantFirst: "", checkOffset: true,
			stopAfter: "STOP:what now?", wantSecondIn: "what now?",
		},
		{name: "waiting with output files is done", touchOutput: true, wantFirst: OutcomeDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFile := filepath.Join(t.TempDir(), "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			buf := logcapture.CaptureVerbose(t)
			outstanding := []BackgroundTask{
				{Kind: BackgroundFork, ID: "agent-1", Label: "review the diff", Signal: SignalPayload},
				{Kind: BackgroundShell, ID: "shell-1", Label: "sleep 600", Signal: SignalTranscript},
			}
			fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{outstanding: outstanding}, withConfig(gateConfig))
			fc := newFakeClock(time.Now())
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1"}),
				withRunEvents(waitLine),
				withRunClock(fc, fc.Now().Add(time.Hour)))

			outcome, held, err := run.pollEventsTick()
			if err != nil {
				t.Fatalf("pollEventsTick error: %v", err)
			}
			if held != nil {
				t.Errorf("first tick held = %+v, want nil: a waiting turn end is not a held one", held)
			}
			if outcome != tt.wantFirst {
				t.Errorf("outcome = %q, want %q", outcome, tt.wantFirst)
			}
			if tt.checkOffset {
				if want := int64(len(waitLine)); run.offset != want {
					t.Errorf("offset = %d, want %d (advanced past the parsed bytes)", run.offset, want)
				}
			}
			if tt.checkTrace {
				// Idle ticks re-check the waiting turn end and log nothing more.
				for range 2 {
					if _, _, err := run.pollEventsTick(); err != nil {
						t.Fatalf("idle tick error: %v", err)
					}
				}
				if got := strings.Count(buf.String(), "turn end waiting on background work"); got != 1 {
					t.Errorf("waiting trace lines = %d, want 1 in %q", got, buf.String())
				}
				for _, want := range []string{
					"strand-1",
					"kind=fork id=agent-1", "review the diff", "signal=payload",
					"kind=shell id=shell-1", "sleep 600", "signal=transcript",
				} {
					if !strings.Contains(buf.String(), want) {
						t.Errorf("trace missing %q in %q", want, buf.String())
					}
				}
			}
			if tt.stopAfter == "" {
				return
			}

			appendEventsLine(t, run.state.EventsPath, tt.stopAfter)
			outcome, held, err = run.pollEventsTick()
			if err != nil {
				t.Fatalf("second tick error: %v", err)
			}
			wantOffset := int64(len(waitLine) + len(tt.stopAfter) + 1)
			if outcome != "" || held == nil || held.message != tt.wantSecondIn || held.offset != wantOffset {
				t.Errorf("second tick = (%q, %+v), want a held turn end with message %q and offset %d", outcome, held, tt.wantSecondIn, wantOffset)
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

// payloadShellTask and transcriptShellTask are one background shell as reported by the turn-end payload and by the transcript fallback.
var (
	payloadShellTask    = BackgroundTask{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999", Signal: SignalPayload}
	transcriptShellTask = BackgroundTask{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999", Signal: SignalTranscript}
)

// TestPollEventsTick_OutstandingBackgroundWork covers a waiting turn end whose outstanding list is background work, one rule for a shell of either signal:
// with an output file missing the turn end keeps waiting on a shell, a fork or an awaited shell however long they run, and is never held;
// with every output file present an ungated run finishes done at once whatever is outstanding;
// a gated run finishes done at once only when it was started fresh, no gated arrival has reached the gate yet and every outstanding task is an unawaited shell, and otherwise waits for the next real turn end.
//
//testtiming:keep pins that no shell or fork expires at a waiting turn end, and the files-present rules of an ungated and a gated run
func TestPollEventsTick_OutstandingBackgroundWork(t *testing.T) {
	passingGate := GateSpec{{Gate: func() (GateResult, error) { return GateResult{Passed: true}, nil }, Attempts: 1}}
	fork := BackgroundTask{Kind: BackgroundFork, ID: "fork-1"}
	awaitedShell := BackgroundTask{Kind: BackgroundShell, ID: "sh-2", Label: "await-me 03", Signal: SignalTranscript}
	awaitedSpec := Spec{AwaitedShellPrefixes: []string{"await-me"}}
	tests := []struct {
		name  string
		tasks []BackgroundTask
		spec  Spec
		gated bool
		// fresh marks the gated run as started rather than attached or resumed.
		fresh bool
		// arrived marks the gated run as having taken a gated arrival to the gate already.
		arrived     bool
		touchOutput bool
		// wantDone expects the first tick to finish done, with the tasks counted for the run's record and the waiting list cleared.
		wantDone bool
	}{
		{name: "a transcript-reported shell keeps waiting past the bound while an output file is missing", tasks: []BackgroundTask{transcriptShellTask}},
		{name: "a payload-reported shell keeps waiting past the bound while an output file is missing", tasks: []BackgroundTask{payloadShellTask}},
		{name: "a fork beside a shell keeps waiting past the bound", tasks: []BackgroundTask{transcriptShellTask, fork}},
		{name: "an awaited shell keeps waiting past the bound", tasks: []BackgroundTask{awaitedShell}, spec: awaitedSpec},
		{name: "an ungated run with outputs and a transcript-reported shell finishes done", tasks: []BackgroundTask{transcriptShellTask}, touchOutput: true, wantDone: true},
		{name: "an ungated run with outputs and a fork finishes done", tasks: []BackgroundTask{fork}, touchOutput: true, wantDone: true},
		{name: "a fresh gated run with outputs and a transcript-reported shell finishes done before any gated arrival", tasks: []BackgroundTask{transcriptShellTask}, gated: true, fresh: true, touchOutput: true, wantDone: true},
		{name: "a fresh gated run with outputs and a payload-reported shell finishes done before any gated arrival", tasks: []BackgroundTask{payloadShellTask}, gated: true, fresh: true, touchOutput: true, wantDone: true},
		{name: "a gated run waits once a gated arrival has reached the gate", tasks: []BackgroundTask{transcriptShellTask}, gated: true, fresh: true, arrived: true, touchOutput: true},
		{name: "an attached or resumed gated run waits", tasks: []BackgroundTask{transcriptShellTask}, gated: true, touchOutput: true},
		{name: "a fork beside a shell keeps the gated run waiting", tasks: []BackgroundTask{payloadShellTask, fork}, gated: true, fresh: true, touchOutput: true},
		{name: "an awaited shell beside a shell keeps the gated run waiting", tasks: []BackgroundTask{payloadShellTask, awaitedShell}, spec: awaitedSpec, gated: true, fresh: true, touchOutput: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFile := filepath.Join(t.TempDir(), "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			run, fc := shellWaitFixture(t, outputFile, tt.tasks, tt.spec)
			if tt.gated {
				run.gate = passingGate
			}
			run.startedFresh = tt.fresh
			run.gatedArrival = tt.arrived

			outcome, held, err := run.pollEventsTick()
			if tt.wantDone {
				if err != nil || outcome != OutcomeDone || held != nil || !slices.Equal(run.countedTasks, tt.tasks) || len(run.waitingTasks) != 0 {
					t.Fatalf("first tick = (%q, %+v, %v) with counted %v and waiting %v, want done with the tasks counted and the waiting list cleared", outcome, held, err, run.countedTasks, run.waitingTasks)
				}
				return
			}
			if err != nil || outcome != "" || held != nil {
				t.Fatalf("first tick = (%q, %+v, %v), want still waiting", outcome, held, err)
			}
			fc.Sleep(time.Hour - time.Second)
			if outcome, held, err := run.pollEventsTick(); err != nil || outcome != "" || held != nil {
				t.Errorf("tick past the bound = (%q, %+v, %v), want still waiting and never held", outcome, held, err)
			}
		})
	}
}

// jumpClock is a fake clock whose Sleep advances a fixed jump, so Wait reaches a deadline in a few ticks.
type jumpClock struct {
	*fakeClock
	jump time.Duration
}

func (c *jumpClock) Sleep(time.Duration) { c.fakeClock.Sleep(c.jump) }

// TestWait_GatedFreshRunWithOutstandingShellEvaluatesGateAtOnce pins that a fresh gated run whose waiting turn end has every output file and a shell outstanding is a gated arrival at once:
// the gate runs once, the run finishes done, the shell is recorded in Result.EndedShells, and the cleaned end logs that the strand removal ended it.
// It captures the process-global logger, so it does not run in parallel.
//
//testtiming:keep pins the gated at-once finish end to end: one gate evaluation, the recorded shell and the removal line
func TestWait_GatedFreshRunWithOutstandingShellEvaluatesGateAtOnce(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
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

	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{outstanding: []BackgroundTask{transcriptShellTask}}, withConfig(gateConfig))
	fc := newFakeClock(time.Now())
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(fc, fc.Now().Add(time.Hour)),
		withRunGate(GateSpec{{Gate: gate, Attempts: 3}}))
	run.startedFresh = true

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	if gateCalls != 1 {
		t.Errorf("gate evaluated %d times, want 1 at the at-once arrival", gateCalls)
	}
	if want := []EndedShell{{Label: "sleep 9999", ID: "sh-1", Signal: SignalTranscript}}; !slices.Equal(result.EndedShells, want) {
		t.Errorf("EndedShells = %+v, want %+v", result.EndedShells, want)
	}
	if !run.gatedArrival {
		t.Error("gatedArrival = false after a gated Done arrival, want true")
	}
	if !strings.Contains(buf.String(), "strand removal ended the background shells it could") {
		t.Errorf("log lacks the removal line; log:\n%s", buf.String())
	}
}

// TestWait_RecordsEndedShells pins that every end of a run with a shell outstanding records it in Result.EndedShells with its time outstanding:
// an ungated at-once finish logs that the strand removal ended the shell,
// and a run deadline and a liveness end log that the run ended with the shell outstanding and the strand left to its caller.
// It captures the process-global logger, so it does not run in parallel.
//
//testtiming:keep pins the ended-shell record and its Info line on the ungated, deadline and liveness ends
func TestWait_RecordsEndedShells(t *testing.T) {
	const removed = "strand removal ended the background shells it could"
	const left = "run ended with background shells outstanding"
	livenessConfig := gateConfig
	livenessConfig.LivenessEveryNPolls = 1
	tests := []struct {
		name        string
		cfg         Config
		touchOutput bool
		status      []reedengine.StatusResult
		started     bool
		// jump is how far each tick's sleep advances the clock.
		jump            time.Duration
		wantOutcome     Outcome
		wantOutstanding time.Duration
		wantLog         string
		wantNoLog       string
	}{
		{name: "an ungated at-once finish", cfg: gateConfig, touchOutput: true, status: liveStrandStatus(true), wantOutcome: OutcomeDone, wantLog: removed, wantNoLog: left},
		{name: "the run deadline", cfg: gateConfig, status: liveStrandStatus(true), jump: 40 * time.Minute, wantOutcome: OutcomeTimeout, wantOutstanding: 80 * time.Minute, wantLog: left, wantNoLog: removed},
		{name: "the liveness check", cfg: livenessConfig, status: liveStrandStatus(false), started: true, wantOutcome: OutcomeDied, wantLog: left, wantNoLog: removed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			runDir := t.TempDir()
			eventsPath := filepath.Join(runDir, eventsFileName)
			outputFile := filepath.Join(runDir, "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			if err := os.WriteFile(eventsPath, []byte("WAIT:background work\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}
			fx := newFixture(t, &fakeReed{StatusQueue: tt.status}, &waitingEngine{outstanding: []BackgroundTask{transcriptShellTask}}, withConfig(tt.cfg))
			fc := newFakeClock(time.Now())
			var clk Clock = fc
			if tt.jump > 0 {
				clk = &jumpClock{fakeClock: fc, jump: tt.jump}
			}
			run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
				withRunDir(runDir),
				withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath, Started: tt.started}),
				withRunClock(clk, fc.Now().Add(time.Hour)))

			result, err := run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q, want %q", result.Outcome, tt.wantOutcome)
			}
			want := []EndedShell{{Label: "sleep 9999", ID: "sh-1", Signal: SignalTranscript, Outstanding: tt.wantOutstanding}}
			if !slices.Equal(result.EndedShells, want) {
				t.Errorf("EndedShells = %+v, want %+v", result.EndedShells, want)
			}
			if !strings.Contains(buf.String(), tt.wantLog) || strings.Contains(buf.String(), tt.wantNoLog) {
				t.Errorf("log should hold %q and not %q; log:\n%s", tt.wantLog, tt.wantNoLog, buf.String())
			}
		})
	}
}

// jumpStepClock is a fake clock whose Sleep advances a fixed jump and runs the next scripted step.
type jumpStepClock struct {
	*multiStepClock
	jump time.Duration
}

func (c *jumpStepClock) Sleep(time.Duration) { c.multiStepClock.Sleep(c.jump) }

// TestWait_PayloadShellLogsPastWaitMinOnceAndFinishesAtTheNextTurnEnd drives an autonomous run whose turn end lists a payload-reported shell across several ticks past the bound.
// The run logs the shell once at Warn, never holds, and finishes at the agent's next turn end.
// It captures the process-global logger, so it does not run in parallel.
func TestWait_PayloadShellLogsPastWaitMinOnceAndFinishesAtTheNextTurnEnd(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	if err := os.WriteFile(eventsPath, []byte("WAIT:background work\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	fx := newFixture(t, &fakeReed{StatusQueue: liveStrandStatus(true)}, &waitingEngine{outstanding: []BackgroundTask{payloadShellTask}}, withConfig(gateConfig))
	var notices []string
	fx.Runner.SetNotifier(func(line string) error {
		notices = append(notices, line)
		return nil
	})
	fc := newFakeClock(time.Now())
	idle := func() {}
	clk := &jumpStepClock{
		multiStepClock: &multiStepClock{fakeClock: fc, steps: []func(){idle, idle, idle, func() {
			touchOutputFile(t, outputFile)
			appendEventsLine(t, eventsPath, "STOP:done")
		}}},
		jump: 6 * time.Minute,
	}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(clk, fc.Now().Add(time.Hour)))

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want %q at the agent's next turn end", result.Outcome, OutcomeDone)
	}
	if len(notices) != 0 {
		t.Errorf("notices = %q, want none: a shell outstanding is never a held turn end", notices)
	}
	if got := strings.Count(buf.String(), "shuttle: background shell past wait min"); got != 1 {
		t.Errorf("past-wait-min log lines = %d, want exactly 1 across the ticks past the bound; log:\n%s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "sleep 9999") {
		t.Errorf("log does not name the shell's label; log:\n%s", buf.String())
	}
}

// TestShellWaitMark pins the background-shell wait's mark and marker, which is display only:
// it shows from the bound on, by first-seen stamp, while only non-awaited shells of either signal are outstanding.
// It turns off when a new event replaces the waiting turn end, and never shows with a fork or an awaited shell outstanding.
func TestShellWaitMark(t *testing.T) {
	t.Parallel()

	const bound = 10 * time.Minute
	type step struct {
		// advance moves the clock before the tick.
		advance time.Duration
		// event is appended to the events file before the tick when non-empty.
		event  string
		wantOn bool
	}
	tests := []struct {
		name  string
		tasks []BackgroundTask
		spec  Spec
		steps []step
		// wantCalls are the pane mark calls over all steps.
		wantCalls []waitMarkCall
	}{
		{
			name:      "a transcript-reported shell shows from the bound",
			tasks:     []BackgroundTask{transcriptShellTask},
			steps:     []step{{wantOn: false}, {advance: bound, wantOn: true}},
			wantCalls: []waitMarkCall{{"strand-1", "background shells"}},
		},
		{
			name:      "a payload-reported shell shows from the bound",
			tasks:     []BackgroundTask{payloadShellTask},
			steps:     []step{{wantOn: false}, {advance: bound, wantOn: true}},
			wantCalls: []waitMarkCall{{"strand-1", "background shells"}},
		},
		{
			name:      "a later event turns it off",
			tasks:     oneShell,
			steps:     []step{{wantOn: false}, {advance: bound, wantOn: true}, {event: "STOP:done", wantOn: false}},
			wantCalls: []waitMarkCall{{"strand-1", "background shells"}, {"strand-1", ""}},
		},
		{
			name:  "a fork outstanding is not a shell wait",
			tasks: []BackgroundTask{oneShell[0], {Kind: BackgroundFork, ID: "fork-1"}},
			steps: []step{{wantOn: false}, {advance: bound, wantOn: false}},
		},
		{
			name:  "an awaited shell is not a shell wait",
			tasks: []BackgroundTask{{Kind: BackgroundShell, ID: "sh-2", Label: "await-me 03"}},
			spec:  Spec{AwaitedShellPrefixes: []string{"await-me"}},
			steps: []step{{wantOn: false}, {advance: bound, wantOn: false}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, fc := shellWaitFixture(t, filepath.Join(t.TempDir(), "out.md"), tt.tasks, tt.spec)
			reed := run.runner.reed.(*fakeReed)
			markerPath := filepath.Join(run.runDir, waitMarkerFileName)

			for i, s := range tt.steps {
				if s.advance > 0 {
					fc.Sleep(s.advance)
				}
				if s.event != "" {
					appendEventsLine(t, run.state.EventsPath, s.event)
				}
				run.pollEventsTick()
				run.syncShellWait()

				_, statErr := os.Stat(markerPath)
				if on := statErr == nil; on != s.wantOn {
					t.Fatalf("step %d: marker present = %v; want %v", i, on, s.wantOn)
				}
			}
			if !slices.Equal(reed.WaitMarkCalls, tt.wantCalls) {
				t.Errorf("mark calls = %+v; want %+v", reed.WaitMarkCalls, tt.wantCalls)
			}
		})
	}
}

// TestWait_ClearsAStaleWaitMarkOnEntry pins that Wait clears the strand's pane mark and removes a marker file a crashed step left behind, before it polls.
func TestWait_ClearsAStaleWaitMarkOnEntry(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(runDir, waitMarkerFileName)
	if err := os.WriteFile(stale, []byte("kind: background shells\npid: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reed := &fakeReed{StatusQueue: liveStrandStatus(true)}
	fc := newFakeClock(time.Now())
	run := newFixture(t, reed, &fakeEngine{}, withConfig(gateConfig)).newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(fc, fc.Now().Add(time.Hour)))

	if _, err := run.Wait(); err != nil {
		t.Fatalf("Wait() error: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale wait marker survives Wait: %v", err)
	}
	if got := reed.WaitMarkCalls; len(got) == 0 || got[0] != (waitMarkCall{"strand-1", ""}) {
		t.Errorf("mark calls = %+v; want a clear first", got)
	}
}
