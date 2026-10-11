// gate_pending_test.go covers the pending gate state: a PassOnCap entry that holds the run at a turn boundary,
// driven through the same fake engine, fake reed and fake clock gate_test.go uses — hermetic, untagged, and never sleeping for real.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const pendingSendText = "A reviewer notice is waiting — read it and end your turn."

// sendCaptures returns the captures one successful Send of text consumes: a probe, a clean baseline, and the delivery.
func sendCaptures(text string) []string {
	return []string{"idle pane", "idle pane", text}
}

// pendingFixture is a gated Run over a first Done already in the events file, whose clock runs steps in order, one per Sleep.
type pendingFixture struct {
	run        *Run
	reed       *fakeReed
	eventsPath string
	fc         *fakeClock
}

// newPendingFixture builds the fixture; steps receive the fixture so a step can append an arrival or push the clock past the deadline.
func newPendingFixture(t *testing.T, spec GateSpec, captures []string, steps ...func(f *pendingFixture)) *pendingFixture {
	t.Helper()
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: captures}
	fx := newFixture(t, reed, readyAgentEngine(), withConfig(gateConfig))
	stubInputSleep(t)

	f := &pendingFixture{reed: reed, eventsPath: eventsPath, fc: newFakeClock(time.Now())}
	var wrapped []func()
	for _, step := range steps {
		wrapped = append(wrapped, func() { step(f) })
	}
	mc := &multiStepClock{fakeClock: f.fc, steps: wrapped}
	f.run = fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(mc, mc.Now().Add(time.Hour)),
		withRunGate(spec))
	return f
}

func noStep(*pendingFixture) {}

func appendArrival(line string) func(*pendingFixture) {
	return func(f *pendingFixture) {
		os.WriteFile(f.eventsPath, append(mustRead(f.eventsPath), []byte(line+"\n")...), 0o644)
	}
}

func mustRead(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}

// scriptedPending answers from script, one element per call, repeating the last once spent, and counts calls.
func scriptedPending(calls *int, script ...GateResult) Gate {
	return func() (GateResult, error) {
		i := *calls
		*calls++
		if i >= len(script) {
			i = len(script) - 1
		}
		return script[i], nil
	}
}

//testtiming:keep pins that a pending gate sends its carried text exactly once, charges no attempt, and settles done
func TestGatePending_HoldsSendsOnceAndPasses(t *testing.T) {
	var calls int
	gate := scriptedPending(&calls,
		GateResult{Pending: true, Send: pendingSendText},
		GateResult{Pending: true},
		GateResult{Passed: true},
	)
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, PassOnCap: true}}, sendCaptures(pendingSendText),
		appendArrival("STOP:turn2"))

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want done", result.Outcome)
	}
	if calls != 3 {
		t.Errorf("gate ran %d times, want 3 (Done, the next Done, one idle poll tick)", calls)
	}
	if len(f.reed.SendTextCalls) != 1 || f.reed.SendTextCalls[0].Text != pendingSendText {
		t.Errorf("SendText calls = %+v, want the carried text exactly once", f.reed.SendTextCalls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
	if result.Gate.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0 (a pending send is not a re-prompt)", result.Gate.Attempts)
	}
}

func TestGatePending_FailedSendWarnsAndStaysPending(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	var calls int
	gate := scriptedPending(&calls,
		GateResult{Pending: true, Send: pendingSendText, SendFailedWayForward: "tell the parent by hand"},
		GateResult{Passed: true},
	)
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, PassOnCap: true}}, sendCaptures(pendingSendText))
	f.reed.SendTextErr = errors.New("pane gone")

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Fatalf("Outcome = %q, want done", result.Outcome)
	}
	if calls != 2 {
		t.Errorf("gate ran %d times, want 2 (the writer stays at the boundary, so the next tick re-evaluates)", calls)
	}
	logged := buf.String()
	for _, want := range []string{"parent-review", "pane gone", "tell the parent by hand"} {
		if !strings.Contains(logged, want) {
			t.Errorf("Warn output lacks %q: %s", want, logged)
		}
	}
	if result.Gate.Attempts != 0 || f.run.gateFails[0] != 0 {
		t.Errorf("a failed pending send changed a count: attempts %d, fails %d", result.Gate.Attempts, f.run.gateFails[0])
	}
}

//testtiming:keep pins that mid-turn ticks never re-evaluate the gate and that a deadline finalize reads Final rather than Gate, leaving the entry waiting
func TestGatePending_MidTurnTicksNeverReevaluate_DeadlineUsesFinal(t *testing.T) {
	var gateCalls, finalCalls int
	gate := scriptedPending(&gateCalls, GateResult{Pending: true, Send: pendingSendText})
	final := scriptedPending(&finalCalls, GateResult{Pending: true})
	idle := func(f *pendingFixture) {
		if gateCalls != 1 {
			t.Errorf("gate re-evaluated mid-turn: %d calls", gateCalls)
		}
	}
	pastDeadline := func(f *pendingFixture) { f.fc.Sleep(2 * time.Hour) }
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Final: final, Attempts: 3, PassOnCap: true}}, sendCaptures(pendingSendText),
		idle, idle, idle, pastDeadline)

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Fatalf("Outcome = %q, want done (the files exist)", result.Outcome)
	}
	if gateCalls != 1 || finalCalls != 1 {
		t.Errorf("gate/final calls = %d/%d, want 1/1 (finalize reads Final, never Gate)", gateCalls, finalCalls)
	}
	wantStates(t, result.Gate, GateEntryWaiting)
	if !result.Gate.Passed {
		t.Errorf("Gate.Passed = false, want true: a waiting PassOnCap entry counts like a let-through one")
	}
}

//testtiming:keep pins that a rejection after a pending hold re-prompts exactly once with the findings and counts one attempt
func TestGatePending_RejectAfterPendingRepromptsOnce(t *testing.T) {
	var calls int
	gate := scriptedPending(&calls,
		GateResult{Pending: true},
		GateResult{Passed: false, Findings: "the parent rejected it"},
		GateResult{Passed: true},
	)
	// The idle tick one poll after the pending evaluation sits inside the throttle window, so it reads nothing;
	// the STOP:turn2 arrival two fake-clock seconds in is still inside the window and evaluates at once.
	idleTickReadsNothing := func(*pendingFixture) {
		if calls != 1 {
			t.Errorf("gate ran %d times by the idle tick, want 1 (the throttle window skips it)", calls)
		}
	}
	arrivalEvaluates := func(*pendingFixture) {
		if calls != 2 {
			t.Errorf("gate ran %d times after the arrival, want 2 (a new arrival evaluates inside the window)", calls)
		}
	}
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 2, PassOnCap: true}}, nil,
		idleTickReadsNothing, appendArrival("STOP:turn2"), arrivalEvaluates)
	findingsPath := filepath.Join(f.run.runDir, gateFindingsFileName)
	f.reed.CaptureQueue = repromptCaptureSequence(findingsPath, 1)

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if len(f.reed.SendTextCalls) != 1 || f.reed.SendTextCalls[0].Text != gateRepromptText(findingsPath) {
		t.Errorf("SendText calls = %+v, want exactly one findings re-prompt", f.reed.SendTextCalls)
	}
	if calls != 3 {
		t.Errorf("gate ran %d times, want 3", calls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
	if result.Gate.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Gate.Attempts)
	}
}

//testtiming:keep pins that a verdict recorded between idle ticks is read at the first throttled recheck and not masked by the pending state
func TestGatePending_VerdictBetweenTicksIsNotMasked(t *testing.T) {
	var calls int
	recorded := false
	gate := func() (GateResult, error) {
		calls++
		if recorded {
			return GateResult{Passed: true}, nil
		}
		return GateResult{Pending: true}, nil
	}
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, PassOnCap: true}}, nil,
		noStep, func(*pendingFixture) { recorded = true })

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if calls != 2 {
		t.Errorf("gate ran %d times, want 2: the Done evaluation, then the verdict read at the first throttled recheck", calls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
}

// TestAttachGated_Pending covers a pending gate over a resumed run whose events file already holds a
// Done: the replayed Done is the first gated arrival, and with a carried Send text it sends once and
// is then released by the deadline, whose finalize reads Final; without one, the gate re-evaluates
// on the throttled cadence, no sooner than gateRecheckEvery apart, until a verdict arrives.
//
//testtiming:keep pins a pending gate over a resumed run: the replayed Done evaluates once and sends the carried text, and a bare pending re-evaluates on the throttled cadence rather than every poll tick
func TestAttachGated_Pending(t *testing.T) {
	tests := []struct {
		name string
		// sendText is the text the first evaluation carries; empty means a bare pending.
		sendText string
	}{
		{name: "replayed Done sends the carried text then the deadline reads Final", sendText: pendingSendText},
		{name: "pending without text re-evaluates on poll ticks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
			var engine *fakeEngine
			spec := Spec{Timeout: time.Minute}
			if tt.sendText != "" {
				reed.CaptureQueue = sendCaptures(tt.sendText)
				engine = readyAgentEngine()
				spec.Timeout = 300 * time.Millisecond
			} else {
				engine = &fakeEngine{}
			}
			clk := newFakeClock(time.Now())
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir(), withClock(clk))
			runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
			seedPresentReedState(t, dotLyxDir)
			if tt.sendText != "" {
				stubInputSleep(t)
			}

			outputFile := filepath.Join(runRoot, "out.md")
			spec.OutputFiles = []string{outputFile}
			runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1",
				outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true,
			})
			touchOutputFile(t, outputFile)
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			var gateCalls, finalCalls int
			var evalTimes []time.Time
			var entry GateEntry
			if tt.sendText != "" {
				// The writer is mid-turn after the send, so only the deadline releases the run.
				entry = GateEntry{
					Gate:  scriptedPending(&gateCalls, GateResult{Pending: true, Send: tt.sendText}),
					Final: scriptedPending(&finalCalls, GateResult{Passed: true}),
				}
			} else {
				script := scriptedPending(&gateCalls, GateResult{Pending: true}, GateResult{Pending: true}, GateResult{Passed: true})
				entry = GateEntry{Gate: func() (GateResult, error) {
					evalTimes = append(evalTimes, clk.Now())
					return script()
				}}
			}
			entry.Name, entry.Attempts, entry.PassOnCap = "parent-review", 3, true

			result, found, err := runner.AttachGated(spec, GateSpec{entry})
			if err != nil || !found {
				t.Fatalf("AttachGated() = found %v, err %v; want found and nil", found, err)
			}
			wantStates(t, result.Gate, GateEntryPassed)
			if tt.sendText == "" {
				if gateCalls != 3 {
					t.Errorf("gate ran %d times, want 3 (replayed Done, then one per throttled recheck with no new arrival)", gateCalls)
				}
				for i := 1; i < len(evalTimes); i++ {
					if gap := evalTimes[i].Sub(evalTimes[i-1]); gap < gateRecheckEvery {
						t.Errorf("evaluation %d came %v after the previous, want at least %v", i+1, gap, gateRecheckEvery)
					}
				}
				return
			}
			if gateCalls != 1 {
				t.Errorf("gate ran %d times, want 1 (the replayed Done evaluates once; the writer is then mid-turn)", gateCalls)
			}
			if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != tt.sendText {
				t.Errorf("SendText calls = %+v, want the carried text once", reed.SendTextCalls)
			}
			if finalCalls != 1 {
				t.Errorf("final ran %d times, want 1", finalCalls)
			}
		})
	}
}

// TestGatePending_ExpiryProbeEndsMidTurnWait covers a MayHold entry's Expired probe.
// It is called only while the writer is off the boundary.
// It ends the run done within gateRecheckEvery of turning true.
// Finalize then reads the verdict through the entries' Final closures.
func TestGatePending_ExpiryProbeEndsMidTurnWait(t *testing.T) {
	const deadlineStep = 40
	tests := []struct {
		name string
		// noBoundary replaces the seeded turn end with a line the fake engine ignores, so the loop never sees a boundary, as after a driver restart mid-turn.
		noBoundary bool
		// bareHold makes the parent-review entry pend without a Send, so the writer stays at the idle boundary.
		bareHold bool
		// firstFailsAtFinalize makes the must-pass entry fail on every evaluation after the first.
		firstFailsAtFinalize bool
		// turnsTrueAtTick is the tick after whose sleep the probe answers true; 0 means it never does and the deadline ends the run.
		turnsTrueAtTick int
		probeErr        error
		wantProbed      bool
		wantStates      []GateEntryState
		wantPassed      bool
		wantFinalCalls  int
	}{
		{name: "pending send moved the writer off the boundary", turnsTrueAtTick: 12, wantProbed: true,
			wantStates: []GateEntryState{GateEntryPassed, GateEntryPassed}, wantPassed: true, wantFinalCalls: 1},
		{name: "first entry failing at finalize leaves the second not reached", firstFailsAtFinalize: true, turnsTrueAtTick: 12, wantProbed: true,
			wantStates: []GateEntryState{GateEntryFailed, GateEntryNotReached}},
		{name: "no boundary seen", noBoundary: true, turnsTrueAtTick: 12, wantProbed: true,
			wantStates: []GateEntryState{GateEntryPassed, GateEntryPassed}, wantPassed: true, wantFinalCalls: 1},
		{name: "probe that stays false ends at the deadline", wantProbed: true,
			wantStates: []GateEntryState{GateEntryPassed, GateEntryPassed}, wantPassed: true, wantFinalCalls: 1},
		{name: "writer pending at the idle boundary is never probed", bareHold: true,
			wantStates: []GateEntryState{GateEntryPassed, GateEntryPassed}, wantPassed: true, wantFinalCalls: 1},
		{name: "probe error warns and keeps polling", probeErr: errors.New("probe broke"), wantProbed: true,
			wantStates: []GateEntryState{GateEntryPassed, GateEntryPassed}, wantPassed: true, wantFinalCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged interface{ String() string }
			if tt.probeErr != nil {
				logged = logcapture.CaptureVerbose(t)
			}
			var firstCalls, finalCalls, probeCalls int
			expired := false
			var turnedAt time.Time
			first := func() (GateResult, error) {
				firstCalls++
				if tt.firstFailsAtFinalize && firstCalls > 1 {
					return GateResult{Findings: "the discussion broke"}, nil
				}
				return GateResult{Passed: true}, nil
			}
			hold := GateResult{Pending: true, Send: pendingSendText}
			if tt.bareHold {
				hold = GateResult{Pending: true}
			}
			second := func() (GateResult, error) { return hold, nil }
			final := func() (GateResult, error) {
				finalCalls++
				return GateResult{Passed: true}, nil
			}
			probe := func() (bool, error) {
				probeCalls++
				if tt.probeErr != nil {
					return false, tt.probeErr
				}
				return expired, nil
			}

			steps := make([]func(*pendingFixture), deadlineStep)
			for i := range steps {
				steps[i] = noStep
			}
			if tt.turnsTrueAtTick > 0 {
				steps[tt.turnsTrueAtTick-1] = func(f *pendingFixture) {
					expired = true
					turnedAt = f.fc.Now()
				}
			}
			steps[deadlineStep-1] = func(f *pendingFixture) { f.fc.Sleep(2 * time.Hour) }
			spec := GateSpec{
				{Name: "discussion", Gate: first, Attempts: 3},
				{Name: "parent-review", Gate: second, Final: final, Expired: probe, Attempts: 3, MayHold: true},
			}
			f := newPendingFixture(t, spec, sendCaptures(pendingSendText), steps...)
			if tt.noBoundary {
				if err := os.WriteFile(f.eventsPath, []byte("START\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			result, err := f.run.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}
			if result.Outcome != OutcomeDone {
				t.Fatalf("Outcome = %q, want done", result.Outcome)
			}
			wantStates(t, result.Gate, tt.wantStates...)
			if result.Gate.Passed != tt.wantPassed {
				t.Errorf("Gate.Passed = %v, want %v", result.Gate.Passed, tt.wantPassed)
			}
			if finalCalls != tt.wantFinalCalls {
				t.Errorf("Final ran %d times, want %d", finalCalls, tt.wantFinalCalls)
			}
			if (probeCalls > 0) != tt.wantProbed {
				t.Errorf("probe ran %d times, want it called: %v", probeCalls, tt.wantProbed)
			}
			if tt.turnsTrueAtTick > 0 {
				if lag := result.EndedAt.Sub(turnedAt); lag > gateRecheckEvery {
					t.Errorf("run ended %v after the probe turned true, want within %v", lag, gateRecheckEvery)
				}
			} else if f.fc.Now().Before(f.run.deadline) {
				t.Errorf("run ended at %v, before its deadline %v, with a probe that never turned true", f.fc.Now(), f.run.deadline)
			}
			if logged != nil {
				for _, want := range []string{"parent-review", "probe broke"} {
					if !strings.Contains(logged.String(), want) {
						t.Errorf("Warn output lacks %q: %s", want, logged.String())
					}
				}
			}
		})
	}
}
