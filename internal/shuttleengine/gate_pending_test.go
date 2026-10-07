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
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 2, PassOnCap: true}}, nil,
		noStep, appendArrival("STOP:turn2"))
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

//testtiming:keep pins that a verdict recorded between idle ticks is read at the next tick and not masked by the pending state
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
	if calls != 3 {
		t.Errorf("gate ran %d times, want 3: the verdict recorded between ticks must be read at the next one", calls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
}

// TestAttachGated_Pending covers a pending gate over a resumed run whose events file already holds a
// Done: the replayed Done is the first gated arrival, and with a carried Send text it sends once and
// is then released by the deadline, whose finalize reads Final; without one, the gate re-evaluates
// on every poll tick until a verdict arrives.
//
//testtiming:keep pins a pending gate over a resumed run: the replayed Done evaluates once and sends the carried text, and a bare pending re-evaluates on every poll tick
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
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir())
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
			var entry GateEntry
			if tt.sendText != "" {
				// The writer is mid-turn after the send, so only the deadline releases the run.
				entry = GateEntry{
					Gate:  scriptedPending(&gateCalls, GateResult{Pending: true, Send: tt.sendText}),
					Final: scriptedPending(&finalCalls, GateResult{Passed: true}),
				}
			} else {
				entry = GateEntry{Gate: scriptedPending(&gateCalls, GateResult{Pending: true}, GateResult{Pending: true}, GateResult{Passed: true})}
			}
			entry.Name, entry.Attempts, entry.PassOnCap = "parent-review", 3, true

			result, found, err := runner.AttachGated(spec, GateSpec{entry})
			if err != nil || !found {
				t.Fatalf("AttachGated() = found %v, err %v; want found and nil", found, err)
			}
			wantStates(t, result.Gate, GateEntryPassed)
			if tt.sendText == "" {
				if gateCalls != 3 {
					t.Errorf("gate ran %d times, want 3 (replayed Done, then one per poll tick with no new arrival)", gateCalls)
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
