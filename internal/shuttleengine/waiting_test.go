package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
				// Idle ticks re-check expiry and log nothing more.
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
			if outcome != "" || held == nil || held.message != tt.wantSecondIn || held.offset != wantOffset || len(held.tasks) != 0 {
				t.Errorf("second tick = (%q, %+v), want a held turn end with message %q, no tasks and offset %d", outcome, held, tt.wantSecondIn, wantOffset)
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

// TestPollEventsTick_ShellExpiry covers a waiting turn end whose outstanding list is background shells:
// a shell without a payload signal keeps the turn waiting until the bound, and at the bound it ends done when the output files exist.
// When they do not, it is a held turn end naming the expired shell, with the waiting message and the offset past the waiting line.
// A payload-reported shell never expires, except that a gated run with its output files present finishes done at once on shells that are all payload-reported and unawaited.
//
//testtiming:keep pins the background-shell wait bound: still waiting until the bound, then done with output files or held, naming the shell, without them; and the payload-shell rules
func TestPollEventsTick_ShellExpiry(t *testing.T) {
	passingGate := GateSpec{{Gate: func() (GateResult, error) { return GateResult{Passed: true}, nil }, Attempts: 1}}
	tests := []struct {
		name string
		// tasks is the outstanding list, oneShell when nil.
		tasks []BackgroundTask
		spec  Spec
		// gated keeps the files-exist shortcut out of the way, so the expiry alone ends the turn.
		gated       bool
		touchOutput bool
		// doneFirst expects the first tick to finish done, with wantExpired as the shells waited out.
		doneFirst   bool
		wantExpired []string
		wantOutcome Outcome
		wantHeld    bool
	}{
		{name: "expires after the bound", gated: true, touchOutput: true, wantOutcome: OutcomeDone},
		{name: "expiry with missing output is held", wantHeld: true},
		{name: "a transcript-reported shell expires at the bound", tasks: []BackgroundTask{transcriptShellTask}, gated: true, touchOutput: true, wantOutcome: OutcomeDone},
		{name: "a payload-reported shell is still waiting, not held, past the bound", tasks: []BackgroundTask{payloadShellTask}},
		{
			name: "a gated run with outputs and only payload-reported shells finishes done on the first tick", tasks: []BackgroundTask{payloadShellTask},
			gated: true, touchOutput: true, doneFirst: true, wantExpired: []string{"sleep 9999"},
		},
		{
			name: "a fork beside payload-reported shells keeps the gated run waiting", tasks: []BackgroundTask{payloadShellTask, {Kind: BackgroundFork, ID: "fork-1"}},
			gated: true, touchOutput: true,
		},
		{
			name: "an awaited shell beside payload-reported shells keeps the gated run waiting",
			tasks: []BackgroundTask{payloadShellTask, {Kind: BackgroundShell, ID: "sh-2", Label: "await-me 03", Signal: SignalPayload}},
			spec:  Spec{AwaitedShellPrefixes: []string{"await-me"}}, gated: true, touchOutput: true,
		},
		{name: "an ungated run with outputs and a payload-reported shell finishes done", tasks: []BackgroundTask{payloadShellTask}, touchOutput: true, doneFirst: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputFile := filepath.Join(t.TempDir(), "out.md")
			if tt.touchOutput {
				touchOutputFile(t, outputFile)
			}
			tasks := tt.tasks
			if tasks == nil {
				tasks = oneShell
			}
			run, fc := shellWaitFixture(t, outputFile, tasks, tt.spec)
			if tt.gated {
				run.gate = passingGate
			}

			if tt.doneFirst {
				outcome, held, err := run.pollEventsTick()
				if err != nil || outcome != OutcomeDone || held != nil || !slices.Equal(run.expiredLabels, tt.wantExpired) {
					t.Fatalf("first tick = (%q, %+v, %v) with expired %v, want done with expired %v", outcome, held, err, run.expiredLabels, tt.wantExpired)
				}
				return
			}
			if outcome, held, err := run.pollEventsTick(); err != nil || outcome != "" || held != nil {
				t.Fatalf("first tick = (%q, %v), want still waiting", outcome, err)
			}
			fc.Sleep(10*time.Minute - time.Second)
			if outcome, held, err := run.pollEventsTick(); err != nil || outcome != "" || held != nil {
				t.Fatalf("tick just before the bound = (%q, %v), want still waiting", outcome, err)
			}
			fc.Sleep(time.Second)
			outcome, held, err := run.pollEventsTick()
			if err != nil || outcome != tt.wantOutcome || (held != nil) != tt.wantHeld {
				t.Fatalf("tick at the bound = (%q, %+v, %v), want outcome %q and held=%v", outcome, held, err, tt.wantOutcome, tt.wantHeld)
			}
			if tt.wantHeld {
				wantOffset := int64(len("WAIT:background work\n"))
				if held.message != "background work" || held.offset != wantOffset || !reflect.DeepEqual(held.tasks, oneShell) {
					t.Errorf("held = %+v, want message %q, tasks %+v and offset %d", held, "background work", oneShell, wantOffset)
				}
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
	if outcome, held, err := run.pollEventsTick(); err != nil || outcome != "" || held != nil {
		t.Errorf("tick past the bound with a fork outstanding = (%q, %v), want still waiting", outcome, err)
	}
}

func TestPollEventsTick_AwaitedShellKeepsWaitingPastBound(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	tasks := []BackgroundTask{{Kind: BackgroundShell, ID: "sh-2", Label: "lyx webster recover-batch 03"}}
	run, fc := shellWaitFixture(t, outputFile, tasks, Spec{AwaitedShellPrefixes: []string{"lyx webster recover-batch"}})

	run.pollEventsTick()
	fc.Sleep(time.Hour)
	if outcome, held, err := run.pollEventsTick(); err != nil || outcome != "" || held != nil {
		t.Errorf("tick past the bound with an awaited shell = (%q, %v), want still waiting", outcome, err)
	}
}

func TestPollEventsTick_LaterTurnEndListingExpiredShellHoldsWithoutNewWait(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "out.md")
	run, fc := shellWaitFixture(t, outputFile, oneShell, Spec{})

	run.pollEventsTick()
	fc.Sleep(10 * time.Minute)
	if _, held, _ := run.pollEventsTick(); held == nil {
		t.Fatal("expiry did not report a held turn end")
	}

	appendEventsLine(t, run.state.EventsPath, "WAIT:still there")
	outcome, held, err := run.pollEventsTick()
	if err != nil || outcome != "" || held == nil || held.message != "still there" {
		t.Errorf("later turn end = (%q, %+v, %v), want a held turn end at once with its message", outcome, held, err)
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

// jumpStepClock is a fake clock whose Sleep advances a fixed jump and runs the next scripted step.
type jumpStepClock struct {
	*multiStepClock
	jump time.Duration
}

func (c *jumpStepClock) Sleep(time.Duration) { c.multiStepClock.Sleep(c.jump) }

// TestWait_PayloadShellLogsPastWaitMinOnceAndFinishesAtTheNextTurnEnd drives an autonomous run whose turn end lists a payload-reported shell across several ticks past the bound.
// The run logs the shell once, never holds, and finishes at the agent's next turn end.
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
		t.Errorf("notices = %q, want none: a payload-reported shell is never a held turn end", notices)
	}
	if got := strings.Count(buf.String(), "shuttle: background shell past wait min"); got != 1 {
		t.Errorf("past-wait-min log lines = %d, want exactly 1 across the ticks past the bound; log:\n%s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "sleep 9999") {
		t.Errorf("log does not name the shell's label; log:\n%s", buf.String())
	}
}

// TestShellWaitMark pins the background-shell wait's mark and marker: on while only non-awaited shells are outstanding,
// off once they are waited out or a new event replaces the waiting turn end, and never on with a fork or an awaited shell outstanding.
func TestShellWaitMark(t *testing.T) {
	t.Parallel()

	const bound = 10 * time.Minute
	tests := []struct {
		name  string
		tasks []BackgroundTask
		spec  Spec
		// advance moves the clock past the bound before the second tick.
		advance bool
		// replace appends a Stop event before the second tick.
		replace bool
		wantOn  bool
		// wantOffAfter is whether the second tick leaves the mark off.
		wantOffAfter bool
	}{
		{name: "waited out", tasks: oneShell, advance: true, wantOn: true, wantOffAfter: true},
		{name: "replaced by a later event", tasks: oneShell, replace: true, wantOn: true, wantOffAfter: true},
		{name: "still waiting stays on", tasks: oneShell, wantOn: true},
		{name: "a payload-reported shell stays on past the bound", tasks: []BackgroundTask{payloadShellTask}, advance: true, wantOn: true},
		{name: "a fork outstanding is not a shell wait", tasks: []BackgroundTask{oneShell[0], {Kind: BackgroundFork, ID: "fork-1"}}},
		{
			name:  "an awaited shell is not a shell wait",
			tasks: []BackgroundTask{{Kind: BackgroundShell, ID: "sh-2", Label: "await-me 03"}},
			spec:  Spec{AwaitedShellPrefixes: []string{"await-me"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, fc := shellWaitFixture(t, filepath.Join(t.TempDir(), "out.md"), tt.tasks, tt.spec)
			reed := run.runner.reed.(*fakeReed)
			markerPath := filepath.Join(run.runDir, waitMarkerFileName)

			run.pollEventsTick()
			run.syncShellWait()

			_, statErr := os.Stat(markerPath)
			if on := statErr == nil; on != tt.wantOn {
				t.Fatalf("marker present after the first tick = %v; want %v", on, tt.wantOn)
			}
			if tt.wantOn {
				if want := []waitMarkCall{{"strand-1", "background shells"}}; !slices.Equal(reed.WaitMarkCalls, want) {
					t.Fatalf("mark calls = %+v; want %+v", reed.WaitMarkCalls, want)
				}
			} else if len(reed.WaitMarkCalls) != 0 {
				t.Fatalf("mark calls = %+v; want none", reed.WaitMarkCalls)
			}

			if tt.advance {
				fc.Sleep(bound)
			}
			if tt.replace {
				appendEventsLine(t, run.state.EventsPath, "STOP:done")
			}
			run.pollEventsTick()
			run.syncShellWait()

			_, statErr = os.Stat(markerPath)
			if off := os.IsNotExist(statErr); tt.wantOn && off != tt.wantOffAfter {
				t.Errorf("marker absent after the second tick = %v; want %v", off, tt.wantOffAfter)
			}
			if tt.wantOffAfter {
				if got := reed.WaitMarkCalls; len(got) != 2 || got[1].Label != "" {
					t.Errorf("mark calls = %+v; want a set then a clear", got)
				}
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
