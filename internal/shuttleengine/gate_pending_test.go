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
	runner := newWaitTestRunner(t, reed, readyAgentEngine(), Config{PollIntervalMS: 1, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30})
	stubInputSleep(t)

	f := &pendingFixture{reed: reed, eventsPath: eventsPath, fc: newFakeClock(time.Now())}
	var wrapped []func()
	for _, step := range steps {
		wrapped = append(wrapped, func() { step(f) })
	}
	mc := &multiStepClock{fakeClock: f.fc, steps: wrapped}
	f.run = &Run{
		runner:   runner,
		spec:     Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		runDir:   runDir,
		state:    RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath},
		clock:    mc,
		deadline: mc.Now().Add(time.Hour),
		gate:     spec,
	}
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

func TestGatePending_HoldsSendsOnceAndPassesWithRememberedMessage(t *testing.T) {
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
	if result.LastAssistantMessage != f.run.gateLastDone {
		t.Errorf("LastAssistantMessage = %q, want the remembered Done message %q", result.LastAssistantMessage, f.run.gateLastDone)
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
	buf := captureLoggerOutput(t)
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

func TestGatePending_ContractViolationsAreErrors(t *testing.T) {
	pending := func() (GateResult, error) { return GateResult{Pending: true}, nil }
	passed := func() (GateResult, error) { return GateResult{Passed: true}, nil }
	tests := []struct {
		name  string
		entry GateEntry
		want  string
	}{
		{"pending_without_pass_on_cap", GateEntry{Name: "x", Gate: pending, Attempts: 1}, `entry "x" returned pending but is not pass_on_cap`},
		{"final_without_pass_on_cap", GateEntry{Name: "x", Gate: passed, Final: passed, Attempts: 1}, `entry "x" sets Final but is not pass_on_cap`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPendingFixture(t, GateSpec{tt.entry}, nil)
			_, err := f.run.Wait()
			if err == nil || !strings.Contains(err.Error(), "shuttle: gate: "+tt.want) {
				t.Errorf("Wait() error = %v, want it to contain %q", err, "shuttle: gate: "+tt.want)
			}
		})
	}
}

func TestAttachGated_PendingReplayedDoneThenPollTickReevaluates(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}, CaptureQueue: sendCaptures(pendingSendText)}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, readyAgentEngine(), Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)
	stubInputSleep(t)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", sessionID: "session-1",
		outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true,
	})
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	// The replayed Done is the first gated arrival: it reads pending and sends the text;
	// the writer is then mid-turn, so the run is released by the deadline, whose finalize reads Final.
	var gateCalls, finalCalls int
	gate := scriptedPending(&gateCalls, GateResult{Pending: true, Send: pendingSendText})
	final := scriptedPending(&finalCalls, GateResult{Passed: true})
	result, found, err := runner.AttachGated(Spec{OutputFiles: []string{outputFile}, Timeout: 300 * time.Millisecond},
		GateSpec{{Name: "parent-review", Gate: gate, Final: final, Attempts: 3, PassOnCap: true}})
	if err != nil || !found {
		t.Fatalf("AttachGated() = found %v, err %v; want found and nil", found, err)
	}
	if gateCalls != 1 {
		t.Errorf("gate ran %d times, want 1 (the replayed Done evaluates once; the writer is then mid-turn)", gateCalls)
	}
	if len(reed.SendTextCalls) != 1 || reed.SendTextCalls[0].Text != pendingSendText {
		t.Errorf("SendText calls = %+v, want the carried text once", reed.SendTextCalls)
	}
	if finalCalls != 1 {
		t.Errorf("final ran %d times, want 1", finalCalls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
}

func TestAttachGated_PendingWithoutTextReevaluatesOnPollTicks(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	runner, _, dotLyxDir, runRoot := newAttachTestRunner(t, reed, &fakeEngine{}, Config{StartupTimeoutS: 30, RunTimeoutMin: 5, PollIntervalMS: 1, LivenessEveryNPolls: 1})
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", sessionID: "session-1",
		outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true,
	})
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("STOP:done\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	var calls int
	gate := scriptedPending(&calls, GateResult{Pending: true}, GateResult{Pending: true}, GateResult{Passed: true})
	result, found, err := runner.AttachGated(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute},
		GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, PassOnCap: true}})
	if err != nil || !found {
		t.Fatalf("AttachGated() = found %v, err %v; want found and nil", found, err)
	}
	if calls != 3 {
		t.Errorf("gate ran %d times, want 3 (replayed Done, then one per poll tick with no new arrival)", calls)
	}
	wantStates(t, result.Gate, GateEntryPassed)
}
