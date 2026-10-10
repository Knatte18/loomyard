package shedfake

import (
	"context"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func gateOf(result shuttleengine.GateResult, err error, calls *int) shuttleengine.GateSpec {
	return shuttleengine.GateSpec{{
		Name:     "entry",
		Attempts: 1,
		Gate: func() (shuttleengine.GateResult, error) {
			*calls++
			return result, err
		},
	}}
}

func TestShuttle_RunGated(t *testing.T) {
	boom := errors.New("gate closure failed")
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	tests := []struct {
		name       string
		result     shuttleengine.Result
		gate       shuttleengine.GateResult
		gateErr    error
		wantCalls  int
		wantErr    error
		wantGate   bool
		wantPassed bool
		wantReason string
	}{
		{name: "Pass", result: done, gate: shuttleengine.GateResult{Passed: true}, wantCalls: 1, wantGate: true, wantPassed: true},
		{name: "FailCarriesReason", result: done, gate: shuttleengine.GateResult{}, wantCalls: 1, wantGate: true, wantReason: "why"},
		{name: "ClosureError", result: done, gateErr: boom, wantCalls: 1, wantErr: boom},
		{name: "SkippedOnNonDone", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}, gate: shuttleengine.GateResult{Passed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Shuttle{Result: tt.result, GateAttempts: 2, GateReason: "why"}
			calls := 0
			res, err := f.RunGated(shuttleengine.Spec{Prompt: "p"}, gateOf(tt.gate, tt.gateErr, &calls))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RunGated() error = %v; want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("gate closure ran %d times; want %d", calls, tt.wantCalls)
			}
			if (res.Gate != nil) != tt.wantGate {
				t.Fatalf("Result.Gate = %+v; want non-nil %v", res.Gate, tt.wantGate)
			}
			if res.Gate != nil {
				if res.Gate.Passed != tt.wantPassed || res.Gate.Reason != tt.wantReason || res.Gate.Attempts != 2 {
					t.Errorf("Result.Gate = %+v; want Passed %v, Reason %q, Attempts 2", res.Gate, tt.wantPassed, tt.wantReason)
				}
			}
			if len(f.GotGateSpec) != 1 {
				t.Errorf("GotGateSpec has %d entries; want 1", len(f.GotGateSpec))
			}
		})
	}
}

func TestShuttle_AttachGated(t *testing.T) {
	boom := errors.New("gate closure failed")
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	tests := []struct {
		name       string
		found      bool
		result     shuttleengine.Result
		gate       shuttleengine.GateResult
		gateErr    error
		wantCalls  int
		wantErr    error
		wantGate   bool
		wantPassed bool
	}{
		{name: "Pass", found: true, result: done, gate: shuttleengine.GateResult{Passed: true}, wantCalls: 1, wantGate: true, wantPassed: true},
		{name: "Fail", found: true, result: done, wantCalls: 1, wantGate: true},
		{name: "ClosureError", found: true, result: done, gateErr: boom, wantCalls: 1, wantErr: boom},
		{name: "SkippedOnNonDone", found: true, result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, gate: shuttleengine.GateResult{Passed: true}},
		{name: "SkippedWhenNothingAttached", found: false, result: done, gate: shuttleengine.GateResult{Passed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Shuttle{AttachResult: tt.result, AttachFound: tt.found, GateAttempts: 3}
			calls := 0
			res, found, err := f.AttachGated(shuttleengine.Spec{Prompt: "p"}, gateOf(tt.gate, tt.gateErr, &calls))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AttachGated() error = %v; want %v", err, tt.wantErr)
			}
			if found != tt.found {
				t.Errorf("AttachGated() found = %v; want %v", found, tt.found)
			}
			if calls != tt.wantCalls {
				t.Errorf("gate closure ran %d times; want %d", calls, tt.wantCalls)
			}
			if (res.Gate != nil) != tt.wantGate {
				t.Fatalf("Result.Gate = %+v; want non-nil %v", res.Gate, tt.wantGate)
			}
			if res.Gate != nil && (res.Gate.Passed != tt.wantPassed || res.Gate.Attempts != 3) {
				t.Errorf("Result.Gate = %+v; want Passed %v, Attempts 3", res.Gate, tt.wantPassed)
			}
			if len(f.GotAttachGateSpec) != 1 {
				t.Errorf("GotAttachGateSpec has %d entries; want 1", len(f.GotAttachGateSpec))
			}
		})
	}
}

func TestShuttle_RunFnReplacesScriptedAnswer(t *testing.T) {
	f := &Shuttle{Result: shuttleengine.Result{SessionID: "scripted"}}
	f.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
		return shuttleengine.Result{SessionID: spec.Prompt}, nil
	}
	for _, prompt := range []string{"a", "b"} {
		res, err := f.Run(shuttleengine.Spec{Prompt: prompt})
		if err != nil || res.SessionID != prompt {
			t.Fatalf("Run(%q) = %+v, %v; want SessionID %q", prompt, res, err, prompt)
		}
	}
	if len(f.Specs) != 2 || !f.Called || f.GotSpec.Prompt != "b" {
		t.Errorf("recorded Specs %d, Called %v, GotSpec %q; want 2, true, %q", len(f.Specs), f.Called, f.GotSpec.Prompt, "b")
	}
}

func TestBurlerRunner_LastEntryRepeats(t *testing.T) {
	first, last := errors.New("first"), errors.New("last")
	f := &BurlerRunner{
		Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}, {Outcome: shuttleengine.OutcomeTimeout}},
		Errs:    []error{first, nil, last},
	}
	var seen []int
	f.DuringRun = func(i int) { seen = append(seen, i) }

	wantOutcomes := []shuttleengine.Outcome{shuttleengine.OutcomeDone, shuttleengine.OutcomeTimeout, shuttleengine.OutcomeTimeout, shuttleengine.OutcomeTimeout}
	wantErrs := []error{first, nil, last, last}
	for i := range wantOutcomes {
		res, err := f.Run(burlerengine.Profile{Rubric: "r"}, burlerengine.RunOpts{})
		if res.Outcome != wantOutcomes[i] || !errors.Is(err, wantErrs[i]) {
			t.Errorf("call %d = %q, %v; want %q, %v", i, res.Outcome, err, wantOutcomes[i], wantErrs[i])
		}
	}
	if f.Calls != 4 || len(f.GotProfiles) != 4 || len(f.GotOpts) != 4 {
		t.Errorf("Calls %d, GotProfiles %d, GotOpts %d; want 4 each", f.Calls, len(f.GotProfiles), len(f.GotOpts))
	}
	if len(seen) != 4 || seen[0] != 0 || seen[3] != 3 {
		t.Errorf("DuringRun indices = %v; want 0..3", seen)
	}

	// ProbeRound and Resume script the same way, and unscripted answer a round with neither half live.
	live := burlerengine.LiveRound{Review: burlerengine.LiveHalf{State: burlerengine.HalfLive}}
	probing := &BurlerRunner{LiveRounds: []burlerengine.LiveRound{live}, ProbeErrs: []error{nil, first}, ResumeResults: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	for i, wantErr := range []error{nil, first, first} {
		got, err := probing.ProbeRound(burlerengine.Profile{}, burlerengine.RunOpts{Round: "1"})
		if got != live || !errors.Is(err, wantErr) {
			t.Errorf("ProbeRound call %d = %+v, %v; want %+v, %v", i, got, err, live, wantErr)
		}
	}
	res, err := probing.Resume(burlerengine.Profile{}, burlerengine.RunOpts{}, live)
	if res.Outcome != shuttleengine.OutcomeDone || err != nil || probing.ProbeCalls != 3 || len(probing.GotProbeOpts) != 3 || probing.ResumeCalls != 1 || probing.GotLive[0] != live {
		t.Errorf("Resume() = %+v, %v with ProbeCalls %d, ResumeCalls %d, GotLive %+v; want done, nil, 3, 1, [%+v]", res, err, probing.ProbeCalls, probing.ResumeCalls, probing.GotLive, live)
	}
	unscripted := &BurlerRunner{}
	if got, err := unscripted.ProbeRound(burlerengine.Profile{}, burlerengine.RunOpts{}); got != (burlerengine.LiveRound{}) || err != nil {
		t.Errorf("unscripted ProbeRound() = %+v, %v; want the zero LiveRound and nil", got, err)
	}

}

func TestSeatRunner_LastEntryRepeatsAndFnsOverride(t *testing.T) {
	first, last := errors.New("first"), errors.New("last")
	table := seatengine.Table{RolePrefix: "multi"}
	f := &SeatRunner{
		Results: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}, {Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}},
		Errs:    []error{first, nil, last},
	}
	wantOutcomes := []shuttleengine.Outcome{shuttleengine.OutcomeDone, shuttleengine.OutcomeTimeout, shuttleengine.OutcomeTimeout, shuttleengine.OutcomeTimeout}
	wantErrs := []error{first, nil, last, last}
	for i := range wantOutcomes {
		res, err := f.Run(table)
		if res.Chair.Outcome != wantOutcomes[i] || !errors.Is(err, wantErrs[i]) {
			t.Errorf("Run call %d = %q, %v; want %q, %v", i, res.Chair.Outcome, err, wantOutcomes[i], wantErrs[i])
		}
	}
	if f.Calls != 4 || len(f.GotTables) != 4 || f.GotTables[0].RolePrefix != "multi" {
		t.Errorf("Calls %d, GotTables %d; want 4 each, holding the table", f.Calls, len(f.GotTables))
	}

	// Probe and Resume script the same way, and an unscripted Probe answers a table with no seat live.
	live := seatengine.LiveTable{Advisors: map[string]seatengine.Handle{"advisor-1": nil}}
	probing := &SeatRunner{LiveTables: []seatengine.LiveTable{live}, ProbeErrs: []error{nil, first}, ResumeResults: []seatengine.Result{{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}}}
	for i, wantErr := range []error{nil, first, first} {
		got, err := probing.Probe(table)
		if len(got.Advisors) != 1 || !errors.Is(err, wantErr) {
			t.Errorf("Probe call %d = %+v, %v; want the scripted LiveTable and %v", i, got, err, wantErr)
		}
	}
	res, err := probing.Resume(table, live)
	if res.Chair.Outcome != shuttleengine.OutcomeDone || err != nil || probing.ProbeCalls != 3 || len(probing.GotProbeTables) != 3 || probing.ResumeCalls != 1 || len(probing.GotLive) != 1 || len(probing.GotResumeTables) != 1 {
		t.Errorf("Resume() = %+v, %v with ProbeCalls %d, ResumeCalls %d, GotLive %d; want done, nil, 3, 1, 1", res, err, probing.ProbeCalls, probing.ResumeCalls, len(probing.GotLive))
	}
	if got, err := (&SeatRunner{}).Probe(table); got.Chair != nil || len(got.Advisors) != 0 || err != nil {
		t.Errorf("unscripted Probe() = %+v, %v; want the zero LiveTable and nil", got, err)
	}

	// An override replaces its verb's scripted answer, and the call is still counted.
	overriding := &SeatRunner{
		Results: []seatengine.Result{{ChairOutputs: []string{"scripted"}}},
		RunFn: func(seatengine.Table) (seatengine.Result, error) {
			return seatengine.Result{ChairOutputs: []string{"fn"}}, nil
		},
		ProbeFn: func(seatengine.Table) (seatengine.LiveTable, error) { return live, last },
		ResumeFn: func(seatengine.Table, seatengine.LiveTable) (seatengine.Result, error) {
			return seatengine.Result{}, last
		},
	}
	if got, _ := overriding.Run(table); got.ChairOutputs[0] != "fn" || overriding.Calls != 1 {
		t.Errorf("Run() with RunFn = %+v, Calls %d; want the override's answer, counted", got, overriding.Calls)
	}
	if got, err := overriding.Probe(table); len(got.Advisors) != 1 || !errors.Is(err, last) || overriding.ProbeCalls != 1 {
		t.Errorf("Probe() with ProbeFn = %+v, %v; want the override's answer, counted", got, err)
	}
	if _, err := overriding.Resume(table, live); !errors.Is(err, last) || overriding.ResumeCalls != 1 {
		t.Errorf("Resume() with ResumeFn = %v; want the override's error, counted", err)
	}
}

func TestMergeShuttle_ScriptsByCallOrder(t *testing.T) {
	boom := errors.New("second call fails")
	f := &MergeShuttle{
		Results: []shuttleengine.Result{{SessionID: "one"}},
		Errs:    []error{nil, boom},
	}
	var numbers []int
	f.DuringRun = func(n int, spec shuttleengine.Spec) { numbers = append(numbers, n) }

	res, err := f.Run(shuttleengine.Spec{Prompt: "a"})
	if err != nil || res.SessionID != "one" {
		t.Errorf("call 1 = %+v, %v; want SessionID one, nil", res, err)
	}
	res, err = f.Run(shuttleengine.Spec{Prompt: "b"})
	if !errors.Is(err, boom) || res.SessionID != "" {
		t.Errorf("call 2 = %+v, %v; want zero Result, %v", res, err, boom)
	}
	res, err = f.Run(shuttleengine.Spec{Prompt: "c"})
	if err != nil || res.SessionID != "" || res.Outcome != "" {
		t.Errorf("exhausted call = %+v, %v; want zero Result and nil error", res, err)
	}
	if len(f.Specs) != 3 || len(numbers) != 3 || numbers[0] != 1 || numbers[2] != 3 {
		t.Errorf("Specs %d, DuringRun call numbers %v; want 3 specs and 1..3", len(f.Specs), numbers)
	}
}

func TestMergeShuttle_RunFnReplacesScriptedAnswer(t *testing.T) {
	f := &MergeShuttle{Results: []shuttleengine.Result{{SessionID: "scripted"}}}
	f.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
		return shuttleengine.Result{SessionID: spec.Prompt}, nil
	}
	res, err := f.Run(shuttleengine.Spec{Prompt: "fn"})
	if err != nil || res.SessionID != "fn" {
		t.Errorf("Run() = %+v, %v; want SessionID fn", res, err)
	}
}

func TestWebsterSeams_FieldsNonNil(t *testing.T) {
	deps := WebsterSeams()
	if deps.Starter == nil || deps.Stopper == nil || deps.Engine == nil || deps.RefMatcher == nil {
		t.Errorf("WebsterSeams() = %+v; want Starter, Stopper, Engine and RefMatcher non-nil", deps)
	}
}

type stubProducer struct {
	outcome shedengine.Outcome
	ptr     shedengine.OutputPointer
}

func (s stubProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return s.outcome, s.ptr, nil
}

func TestCallOK_AndRequireOutcome_PassingPath(t *testing.T) {
	p := stubProducer{outcome: shedengine.Done, ptr: shedengine.OutputPointer{Path: "out.md"}}

	outcome, ptr := CallOK(t, p)
	if outcome != shedengine.Done || ptr.Path != "out.md" {
		t.Errorf("CallOK() = %q, %+v; want %q, out.md", outcome, ptr, shedengine.Done)
	}
	if got := RequireOutcome(t, p, shedengine.Done); got.Path != "out.md" {
		t.Errorf("RequireOutcome() pointer = %+v; want Path out.md", got)
	}
}
