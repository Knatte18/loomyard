// gate_terminal_test.go covers the MayHold entry and the Terminal gate result, driven through gate_pending_test.go's fixture and fakes — hermetic, untagged, and never sleeping for real.

package shuttleengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGateTerminal_MayHoldMustPassEntryMayPendAndSetFinal(t *testing.T) {
	var gateCalls, finalCalls int
	gate := scriptedPending(&gateCalls, GateResult{Pending: true})
	final := scriptedPending(&finalCalls, GateResult{Pending: true})
	pastDeadline := func(f *pendingFixture) { f.fc.Sleep(2 * time.Hour) }
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Final: final, Attempts: 3, MayHold: true}}, nil,
		noStep, pastDeadline)

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Fatalf("Outcome = %q, want done (the files exist)", result.Outcome)
	}
	if gateCalls == 0 || finalCalls != 1 {
		t.Errorf("gate/final calls = %d/%d, want Gate at the boundary and Final exactly once at finalize", gateCalls, finalCalls)
	}
	wantStates(t, result.Gate, GateEntryWaiting)
	if result.Gate.Passed {
		t.Errorf("Gate.Passed = true, want false: a held MayHold must-pass entry is never a pass")
	}
}

func TestGateTerminal_NonMayHoldEntryStillCannotPendOrSetFinal(t *testing.T) {
	passed := func() (GateResult, error) { return GateResult{Passed: true}, nil }
	pending := func() (GateResult, error) { return GateResult{Pending: true}, nil }
	for _, tc := range []struct {
		name  string
		entry GateEntry
		want  string
	}{
		{"pending", GateEntry{Name: "x", Gate: pending, Attempts: 1}, `entry "x" returned pending but is not pass_on_cap`},
		{"final", GateEntry{Name: "x", Gate: passed, Final: passed, Attempts: 1}, `entry "x" sets Final but is not pass_on_cap`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPendingFixture(t, GateSpec{tc.entry}, nil)
			_, err := f.run.Wait()
			if err == nil || !strings.Contains(err.Error(), "shuttle: gate: "+tc.want) {
				t.Fatalf("Wait() error = %v, want it to contain %q", err, "shuttle: gate: "+tc.want)
			}
		})
	}
}

func TestGateTerminal_TerminalFailureFinalizesWithoutReprompt(t *testing.T) {
	var calls int
	gate := scriptedPending(&calls, GateResult{Passed: false, Findings: "the cap round was rejected", Terminal: true})
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, MayHold: true}}, nil)

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Errorf("Outcome = %q, want done", result.Outcome)
	}
	if len(f.reed.SendTextCalls) != 0 {
		t.Errorf("SendText calls = %+v, want none: a terminal failure never re-prompts", f.reed.SendTextCalls)
	}
	if calls != 1 {
		t.Errorf("gate ran %d times, want 1", calls)
	}
	if f.run.gateFails[0] != 0 {
		t.Errorf("gateFails[0] = %d, want 0: a terminal failure does not count", f.run.gateFails[0])
	}
	wantStates(t, result.Gate, GateEntryFailed)
	if result.Gate.Passed {
		t.Errorf("Gate.Passed = true, want false")
	}
	if result.Gate.Reason != "the cap round was rejected" {
		t.Errorf("Gate.Reason = %q, want the terminal result's Findings", result.Gate.Reason)
	}
}

//testtiming:keep pins that a non-terminal failure of a MayHold entry below its budget still re-prompts once and leaves Reason empty, the contrast to a terminal failure
func TestGateTerminal_NonTerminalFailureBelowBudgetStillReprompts(t *testing.T) {
	var calls int
	gate := scriptedPending(&calls,
		GateResult{Passed: false, Findings: "fix it"},
		GateResult{Passed: true},
	)
	f := newPendingFixture(t, GateSpec{{Name: "parent-review", Gate: gate, Attempts: 3, MayHold: true}}, nil, appendArrival("STOP:turn2"))
	f.reed.CaptureQueue = repromptCaptureSequence(gateFindingsPath(f), 1)

	result, err := f.run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if len(f.reed.SendTextCalls) != 1 {
		t.Errorf("SendText calls = %+v, want exactly one re-prompt", f.reed.SendTextCalls)
	}
	if result.Gate.Attempts != 1 || result.Gate.Reason != "" {
		t.Errorf("Gate = %+v, want 1 attempt and no Reason", result.Gate)
	}
	wantStates(t, result.Gate, GateEntryPassed)
}

func TestGateTerminal_TerminalOnPassedOrPendingOrPassOnCapIsGateError(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    GateResult
		passOnCap bool
	}{
		{"passed", GateResult{Passed: true, Terminal: true}, false},
		{"pending", GateResult{Pending: true, Terminal: true}, false},
		{"failed on a pass_on_cap entry", GateResult{Findings: "stop", Terminal: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			gate := scriptedPending(&calls, tc.result)
			f := newPendingFixture(t, GateSpec{{Name: "x", Gate: gate, Attempts: 1, MayHold: !tc.passOnCap, PassOnCap: tc.passOnCap}}, nil)
			_, err := f.run.Wait()
			if err == nil || !strings.Contains(err.Error(), `entry "x" returned a terminal`) {
				t.Fatalf("Wait() error = %v, want a terminal gate error", err)
			}
		})
	}
}

// gateFindingsPath is the findings file the fixture's run writes a failing entry's text to.
func gateFindingsPath(f *pendingFixture) string {
	return filepath.Join(f.run.runDir, gateFindingsFileName)
}
