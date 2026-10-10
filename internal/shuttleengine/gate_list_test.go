// gate_list_test.go covers the ordered gate list: per-entry consecutive-failure counts, PassOnCap, off entries, and the per-entry GateOutcome report, driven through the same fake engine and scripted gate closures gate_test.go uses.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// scriptedGate returns a Gate that answers from script, one element per call, repeating the last element once the script is spent, and counts its calls through calls.
func scriptedGate(calls *int, script ...bool) Gate {
	return func() (GateResult, error) {
		i := *calls
		*calls++
		if i >= len(script) {
			i = len(script) - 1
		}
		if script[i] {
			return GateResult{Passed: true}, nil
		}
		return GateResult{Passed: false, Findings: "bad"}, nil
	}
}

// runGateList drives a gated Wait whose first arrival is already in the events file and which sees one further arrival per expected re-prompt, so a spec that re-prompts reprompts times settles on arrival reprompts+1.
func runGateList(t *testing.T, spec GateSpec, reprompts int) (Result, *fakeReed, string) {
	t.Helper()
	runDir := t.TempDir()
	eventsPath := filepath.Join(runDir, eventsFileName)
	outputFile := filepath.Join(runDir, "out.md")
	touchOutputFile(t, outputFile)
	if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	findingsPath := filepath.Join(runDir, gateFindingsFileName)

	reed := &fakeReed{
		StatusQueue:  liveStrandStatus(true),
		CaptureQueue: repromptCaptureSequence(findingsPath, reprompts),
	}
	fx := newFixture(t, reed, readyAgentEngine(), withConfig(gateConfig))
	stubInputSleep(t)

	fc := newFakeClock(time.Now())
	var steps []func()
	for i := 0; i < reprompts; i++ {
		line := "STOP:turn" + string(rune('2'+i))
		steps = append(steps, func() { appendEventsLine(t, eventsPath, line) })
	}
	mc := &multiStepClock{fakeClock: fc, steps: steps}
	run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
		withRunDir(runDir),
		withRunState(RunState{StrandGUID: "strand-1", SessionID: "session-1", EventsPath: eventsPath}),
		withRunClock(mc, mc.Now().Add(time.Hour)),
		withRunGate(spec))

	result, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait() error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, OutcomeDone)
	}
	return result, reed, findingsPath
}

// wantStates fails the test unless outcome's entries carry exactly want, in list order.
func wantStates(t *testing.T, outcome *GateOutcome, want ...GateEntryState) {
	t.Helper()
	if outcome == nil {
		t.Fatalf("Gate outcome is nil, want entry states %v", want)
	}
	if len(outcome.Entries) != len(want) {
		t.Fatalf("Entries = %+v, want %d entries", outcome.Entries, len(want))
	}
	for i, state := range want {
		if outcome.Entries[i].State != state {
			t.Errorf("Entries[%d] (%s) state = %q, want %q", i, outcome.Entries[i].Name, outcome.Entries[i].State, state)
		}
	}
}

// TestGateList_FailPassFailStartsBudgetAfresh pins the consecutive-failure reset:
// an entry that failed, then passed on an arrival where a later entry failed, then fails again, has its whole budget back — so with Attempts 1 it is run and re-prompted again instead of finalizing (or, with PassOnCap, being let through without running).
//
//testtiming:keep pins the consecutive-failure reset: an entry that fails, passes and fails again gets its whole budget back, with and without pass_on_cap
func TestGateList_FailPassFailStartsBudgetAfresh(t *testing.T) {
	for _, passOnCap := range []bool{false, true} {
		name := "without_pass_on_cap"
		if passOnCap {
			name = "with_pass_on_cap"
		}
		t.Run(name, func(t *testing.T) {
			// A PassOnCap entry at Attempts 1 would already be let through on arrival two, never running to pass;
			// Attempts 2 keeps it running until a missing reset would cap it.
			attempts := 1
			if passOnCap {
				attempts = 2
			}
			var aCalls, bCalls int
			spec := GateSpec{
				{Name: "a", Gate: scriptedGate(&aCalls, false, true, false, true), Attempts: attempts, PassOnCap: passOnCap},
				{Name: "b", Gate: scriptedGate(&bCalls, false, true), Attempts: 5},
			}

			result, reed, _ := runGateList(t, spec, 3)

			if aCalls != 4 {
				t.Errorf("entry a ran %d times, want 4 (never let through or finalized on a spent budget)", aCalls)
			}
			if bCalls != 2 {
				t.Errorf("entry b ran %d times, want 2", bCalls)
			}
			if len(reed.SendTextCalls) != 3 {
				t.Errorf("SendText calls = %d, want 3", len(reed.SendTextCalls))
			}
			if !result.Gate.Passed {
				t.Errorf("Gate = %+v, want passed", result.Gate)
			}
			wantStates(t, result.Gate, GateEntryPassed, GateEntryPassed)
		})
	}
}

// TestGateList_FinalArrivalAtFailingPassOnCapEntry covers both final arrivals that cannot re-prompt — a failed send and an expired deadline — stopping at a failing PassOnCap entry with a required entry after it:
// the outcome is failed and the later entry is reported not reached.
//
//testtiming:keep pins both final arrivals that cannot re-prompt, a failed send and an expired deadline, ending at a failing pass_on_cap entry with the later required entry reported not reached
func TestGateList_FinalArrivalAtFailingPassOnCapEntry(t *testing.T) {
	newSpec := func() (GateSpec, *int) {
		var pCalls, rCalls int
		return GateSpec{
			{Name: "p", Gate: scriptedGate(&pCalls, false), Attempts: 3, PassOnCap: true},
			{Name: "r", Gate: scriptedGate(&rCalls, true), Attempts: 3},
		}, &rCalls
	}

	t.Run("send_fails", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, eventsFileName)
		outputFile := filepath.Join(runDir, "out.md")
		touchOutputFile(t, outputFile)
		if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}
		reed := &fakeReed{
			StatusQueue:  liveStrandStatus(true),
			CaptureQueue: []string{"idle pane", "idle pane"},
			SendTextErr:  errors.New("pane swallowed input"),
		}
		fx := newFixture(t, reed, readyAgentEngine(), withConfig(gateConfig))
		stubInputSleep(t)
		spec, rCalls := newSpec()
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(time.Hour)),
			withRunGate(spec))

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Gate == nil || result.Gate.Passed {
			t.Fatalf("Gate = %+v, want a failed verdict", result.Gate)
		}
		wantStates(t, result.Gate, GateEntryFailed, GateEntryNotReached)
		if *rCalls != 0 {
			t.Errorf("required entry ran %d times, want 0", *rCalls)
		}
	})

	t.Run("deadline", func(t *testing.T) {
		runDir := t.TempDir()
		eventsPath := filepath.Join(runDir, eventsFileName)
		outputFile := filepath.Join(runDir, "out.md")
		touchOutputFile(t, outputFile)
		if err := os.WriteFile(eventsPath, []byte("STOP:turn1\n"), 0o644); err != nil {
			t.Fatalf("seed events: %v", err)
		}
		findingsPath := filepath.Join(runDir, gateFindingsFileName)
		reed := &fakeReed{
			StatusQueue:  liveStrandStatus(true),
			CaptureQueue: repromptCaptureSequence(findingsPath, 1),
		}
		fx := newFixture(t, reed, readyAgentEngine(), withConfig(Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1_000_000, StartupTimeoutS: 30}))
		stubInputSleep(t)
		spec, rCalls := newSpec()
		fc := newFakeClock(time.Now())
		run := fx.newRun(Spec{OutputFiles: []string{outputFile}, Timeout: time.Hour},
			withRunDir(runDir),
			withRunState(RunState{StrandGUID: "strand-1", EventsPath: eventsPath}),
			withRunClock(fc, fc.Now().Add(2*time.Millisecond)),
			withRunGate(spec))

		result, err := run.Wait()
		if err != nil {
			t.Fatalf("Wait() error: %v", err)
		}
		if result.Gate == nil || result.Gate.Passed {
			t.Fatalf("Gate = %+v, want a failed verdict", result.Gate)
		}
		wantStates(t, result.Gate, GateEntryFailed, GateEntryNotReached)
		if result.Gate.Attempts != 1 {
			t.Errorf("Attempts = %d, want 1", result.Gate.Attempts)
		}
		if *rCalls != 0 {
			t.Errorf("required entry ran %d times, want 0", *rCalls)
		}
	})
}

// TestGateList_FinalReports drives runGateList over lists whose final per-entry report is the point:
// an exhausted budget without pass_on_cap finalizes failed; a pass_on_cap entry is let through
// without running again; an entry with zero Attempts is reported off and its closure never runs,
// with and without pass_on_cap, and a list of nothing but off entries still yields a non-nil passed
// outcome; and one list reaches all five per-entry states with GateOutcome.Attempts the sum of the
// entries' sent counts.
func TestGateList_FinalReports(t *testing.T) {
	tests := []struct {
		name      string
		reprompts int
		// build returns the list and the assertions over the Result of waiting on it.
		build func() (GateSpec, func(t *testing.T, result Result, reed *fakeReed, findingsPath string))
	}{
		{
			name: "exhaustion without pass_on_cap finalizes failed", reprompts: 2,
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				var aCalls, bCalls int
				spec := GateSpec{
					{Name: "a", Gate: scriptedGate(&aCalls, true), Attempts: 3},
					{Name: "b", Gate: scriptedGate(&bCalls, false), Attempts: 2},
				}
				return spec, func(t *testing.T, result Result, reed *fakeReed, findingsPath string) {
					if result.Gate.Passed {
						t.Errorf("Gate = %+v, want failed", result.Gate)
					}
					if result.Gate.FindingsPath != findingsPath {
						t.Errorf("FindingsPath = %q, want %q", result.Gate.FindingsPath, findingsPath)
					}
					if result.Gate.Attempts != 2 || len(reed.SendTextCalls) != 2 {
						t.Errorf("Attempts = %d, SendText calls = %d, want 2 and 2", result.Gate.Attempts, len(reed.SendTextCalls))
					}
					wantStates(t, result.Gate, GateEntryPassed, GateEntryFailed)
				}
			},
		},
		{
			name: "pass_on_cap lets through without running again", reprompts: 2,
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				var pCalls, qCalls int
				spec := GateSpec{
					{Name: "p", Gate: scriptedGate(&pCalls, false), Attempts: 1, PassOnCap: true},
					{Name: "q", Gate: scriptedGate(&qCalls, false, true), Attempts: 2},
				}
				return spec, func(t *testing.T, result Result, reed *fakeReed, _ string) {
					if pCalls != 1 {
						t.Errorf("pass_on_cap entry ran %d times, want 1 (let through without its closure after the cap)", pCalls)
					}
					if !result.Gate.Passed {
						t.Errorf("Gate = %+v, want passed", result.Gate)
					}
					if len(reed.SendTextCalls) != 2 {
						t.Errorf("SendText calls = %d, want 2 (one per entry)", len(reed.SendTextCalls))
					}
					wantStates(t, result.Gate, GateEntryLetThrough, GateEntryPassed)
				}
			},
		},
		{
			name: "zero attempts entry is off and never called",
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				return offBeforeLive(false)
			},
		},
		{
			name: "pass_on_cap zero attempts entry is off and never called",
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				return offBeforeLive(true)
			},
		},
		{
			name: "only off entries yield a passed outcome",
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				var calls int
				spec := GateSpec{
					{Name: "a", Gate: scriptedGate(&calls, false), Attempts: 0},
					{Name: "b", Gate: scriptedGate(&calls, false), Attempts: 0, PassOnCap: true},
				}
				return spec, func(t *testing.T, result Result, _ *fakeReed, _ string) {
					if result.Gate == nil || !result.Gate.Passed {
						t.Fatalf("Gate = %+v, want a non-nil passed outcome", result.Gate)
					}
					if calls != 0 {
						t.Errorf("off entries ran %d times, want 0", calls)
					}
					wantStates(t, result.Gate, GateEntryOff, GateEntryOff)
				}
			},
		},
		{
			name: "every state and the aggregate attempts", reprompts: 2,
			build: func() (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
				var offCalls, xCalls, pCalls, fCalls, nCalls int
				spec := GateSpec{
					{Name: "off", Gate: scriptedGate(&offCalls, true), Attempts: 0},
					{Name: "x", Gate: scriptedGate(&xCalls, true), Attempts: 3},
					{Name: "p", Gate: scriptedGate(&pCalls, false), Attempts: 1, PassOnCap: true},
					{Name: "f", Gate: scriptedGate(&fCalls, false), Attempts: 1},
					{Name: "n", Gate: scriptedGate(&nCalls, true), Attempts: 3},
				}
				return spec, func(t *testing.T, result Result, _ *fakeReed, _ string) {
					wantStates(t, result.Gate, GateEntryOff, GateEntryPassed, GateEntryLetThrough, GateEntryFailed, GateEntryNotReached)
					sum := 0
					for _, entry := range result.Gate.Entries {
						sum += entry.Attempts
					}
					if result.Gate.Attempts != sum || sum != 2 {
						t.Errorf("aggregate Attempts = %d, per-entry sum = %d, want both 2", result.Gate.Attempts, sum)
					}
					if result.Gate.Entries[2].Attempts != 1 || result.Gate.Entries[3].Attempts != 1 {
						t.Errorf("Entries = %+v, want one re-prompt each for p and f", result.Gate.Entries)
					}
					if nCalls != 0 || offCalls != 0 {
						t.Errorf("not-reached ran %d times and off ran %d times, want 0 and 0", nCalls, offCalls)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, check := tt.build()
			result, reed, findingsPath := runGateList(t, spec, tt.reprompts)
			check(t, result, reed, findingsPath)
		})
	}
}

// offBeforeLive returns a list whose first entry has zero Attempts, with the given pass_on_cap, ahead
// of a live passing entry, and the assertions that the off entry never ran and the list passed.
func offBeforeLive(passOnCap bool) (GateSpec, func(*testing.T, Result, *fakeReed, string)) {
	var offCalls, liveCalls int
	spec := GateSpec{
		{Name: "off", Gate: scriptedGate(&offCalls, false), Attempts: 0, PassOnCap: passOnCap},
		{Name: "live", Gate: scriptedGate(&liveCalls, true), Attempts: 3},
	}
	return spec, func(t *testing.T, result Result, _ *fakeReed, _ string) {
		if offCalls != 0 {
			t.Errorf("off entry ran %d times, want 0", offCalls)
		}
		if result.Gate == nil || !result.Gate.Passed {
			t.Fatalf("Gate = %+v, want a non-nil passed outcome", result.Gate)
		}
		wantStates(t, result.Gate, GateEntryOff, GateEntryPassed)
	}
}
