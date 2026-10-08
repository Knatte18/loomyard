// Package shedfake fakes the shed-layer seams: the shuttle, the burler runner, the merge shuttle and the webster run seams, plus the producer Call helpers.
//
// Shuttle satisfies shedadapters.Shuttle and frictionengine.Shuttle, BurlerRunner satisfies shedadapters.BurlerRunner, and MergeShuttle satisfies mergeresolve.Shuttle, all structurally, so no consumer package is imported here and shedadapters' and landingshed's in-package tests can use the kit.
// Every fake exposes fields and optional func overrides, and none asserts anything.
// CallOK and RequireOutcome are the only assertions the kit makes.
//
// internal/burlerengine's in-package tests keep their own Shuttle fake, because this kit imports burlerengine for BurlerRunner's types.
package shedfake

import (
	"context"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Shuttle is a fake shuttle seam: Run and Attach answer the scripted fields, RunGated and AttachGated follow the "every test fake evaluates the gate once" contract.
// Fields are read after the code under test has returned; the fake locks only its own writes.
type Shuttle struct {
	mu sync.Mutex

	// Result and Err are Run's answer when RunFn is nil.
	Result shuttleengine.Result
	Err    error
	// RunFn replaces the scripted Run answer, for a consumer that dispatches per spec.
	RunFn func(spec shuttleengine.Spec) (shuttleengine.Result, error)
	// DuringRun runs before Run answers, so a test can cancel a context or write files as if mid-run.
	DuringRun func()

	// Called reports whether Run ran, GotSpec is its latest Spec and Specs is every Spec in order.
	Called  bool
	GotSpec shuttleengine.Spec
	Specs   []shuttleengine.Spec

	// AttachResult, AttachFound and AttachErr script Attach's answer.
	AttachResult shuttleengine.Result
	AttachFound  bool
	AttachErr    error
	// DuringAttach is DuringRun's twin for Attach.
	DuringAttach func()
	// AttachCalled reports whether Attach ran and GotAttachSpec is its latest Spec.
	AttachCalled  bool
	GotAttachSpec shuttleengine.Spec

	// GotGateSpec and GotAttachGateSpec record the GateSpec RunGated and AttachGated last received.
	GotGateSpec       shuttleengine.GateSpec
	GotAttachGateSpec shuttleengine.GateSpec

	// GateAttempts is stamped onto the GateOutcome a gated call builds once the gate was consulted.
	GateAttempts int
	// GateReason is stamped onto the GateOutcome as Reason when the gate did not pass in RunGated.
	GateReason string
}

// Run records spec and answers RunFn, else Result and Err.
func (f *Shuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.mu.Lock()
	f.Called = true
	f.GotSpec = spec
	f.Specs = append(f.Specs, spec)
	f.mu.Unlock()
	if f.DuringRun != nil {
		f.DuringRun()
	}
	if f.RunFn != nil {
		return f.RunFn(spec)
	}
	return f.Result, f.Err
}

// Attach records spec and answers AttachResult, AttachFound and AttachErr.
func (f *Shuttle) Attach(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	f.mu.Lock()
	f.AttachCalled = true
	f.GotAttachSpec = spec
	f.mu.Unlock()
	if f.DuringAttach != nil {
		f.DuringAttach()
	}
	return f.AttachResult, f.AttachFound, f.AttachErr
}

// AttachIfLive answers as Attach does and records no removal.
func (f *Shuttle) AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	return f.Attach(spec)
}

// RunGated records gate, delegates to Run, then evaluates the gate once.
// The gate is consulted only when it is non-empty and the delegated outcome is OutcomeDone: a closure's error is returned, otherwise a GateOutcome is stamped onto the Result.
// No re-prompt loop is simulated.
func (f *Shuttle) RunGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, error) {
	f.mu.Lock()
	f.GotGateSpec = gate
	f.mu.Unlock()

	result, err := f.Run(spec)
	if err != nil || len(gate) == 0 || result.Outcome != shuttleengine.OutcomeDone {
		return result, err
	}

	passed, gerr := evalGateList(gate)
	if gerr != nil {
		return result, gerr
	}
	result.Gate = &shuttleengine.GateOutcome{Passed: passed, Attempts: f.GateAttempts}
	if !passed {
		result.Gate.Reason = f.GateReason
	}
	return result, nil
}

// AttachGated is RunGated's Attach twin, with the same gate-once contract.
func (f *Shuttle) AttachGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, bool, error) {
	f.mu.Lock()
	f.GotAttachGateSpec = gate
	f.mu.Unlock()

	result, found, err := f.Attach(spec)
	if err != nil || !found || len(gate) == 0 || result.Outcome != shuttleengine.OutcomeDone {
		return result, found, err
	}

	passed, gerr := evalGateList(gate)
	if gerr != nil {
		return result, found, gerr
	}
	result.Gate = &shuttleengine.GateOutcome{Passed: passed, Attempts: f.GateAttempts}
	return result, found, nil
}

// evalGateList runs gate's entries once each in list order, skipping off entries (Attempts 0) and stopping at the first failure.
func evalGateList(gate shuttleengine.GateSpec) (bool, error) {
	for _, entry := range gate {
		if entry.Attempts <= 0 {
			continue
		}
		result, err := entry.Gate()
		if err != nil {
			return false, err
		}
		if !result.Passed {
			return false, nil
		}
	}
	return true, nil
}

// BurlerRunner is a fake burler runner: Results[i] and Errs[i] answer the (i+1)th Run, and once either slice is exhausted its last entry repeats.
// ProbeRound and Resume answer their own scripted slices the same way, and unscripted answer a round with neither half live.
type BurlerRunner struct {
	mu sync.Mutex

	Results []burlerengine.Result
	Errs    []error

	// LiveRounds and ProbeErrs script ProbeRound; ResumeResults and ResumeErrs script Resume.
	LiveRounds    []burlerengine.LiveRound
	ProbeErrs     []error
	ResumeResults []burlerengine.Result
	ResumeErrs    []error
	// ProbeCalls and ResumeCalls count the invocations, GotProbeOpts holds every RunOpts handed to ProbeRound and GotLive every LiveRound handed to Resume.
	ProbeCalls   int
	ResumeCalls  int
	GotProbeOpts []burlerengine.RunOpts
	GotLive      []burlerengine.LiveRound

	// GotProfiles and GotOpts hold every Profile and RunOpts handed to Run, in order, and Calls counts the invocations.
	GotProfiles []burlerengine.Profile
	GotOpts     []burlerengine.RunOpts
	Calls       int

	// DuringRun runs from inside Run, before it answers, with the zero-based index of the invocation.
	DuringRun func(callIndex int)
}

// Run records p and opts, runs DuringRun, then answers the scripted entry for this invocation.
func (f *BurlerRunner) Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error) {
	f.mu.Lock()
	i := f.Calls
	f.Calls++
	f.GotProfiles = append(f.GotProfiles, p)
	f.GotOpts = append(f.GotOpts, opts)
	f.mu.Unlock()
	if f.DuringRun != nil {
		f.DuringRun(i)
	}

	var result burlerengine.Result
	if i < len(f.Results) {
		result = f.Results[i]
	} else if len(f.Results) > 0 {
		result = f.Results[len(f.Results)-1]
	}
	var err error
	if i < len(f.Errs) {
		err = f.Errs[i]
	} else if len(f.Errs) > 0 {
		err = f.Errs[len(f.Errs)-1]
	}
	return result, err
}

// ProbeRound counts the call, records opts and answers the scripted LiveRound and error for this invocation.
func (f *BurlerRunner) ProbeRound(_ burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveRound, error) {
	f.mu.Lock()
	i := f.ProbeCalls
	f.ProbeCalls++
	f.GotProbeOpts = append(f.GotProbeOpts, opts)
	f.mu.Unlock()
	return scripted(f.LiveRounds, i), scripted(f.ProbeErrs, i)
}

// Resume counts the call, records live and answers the scripted Result and error for this invocation.
func (f *BurlerRunner) Resume(_ burlerengine.Profile, _ burlerengine.RunOpts, live burlerengine.LiveRound) (burlerengine.Result, error) {
	f.mu.Lock()
	i := f.ResumeCalls
	f.ResumeCalls++
	f.GotLive = append(f.GotLive, live)
	f.mu.Unlock()
	return scripted(f.ResumeResults, i), scripted(f.ResumeErrs, i)
}

// scripted returns entries[i], the last entry once i runs past them, and the zero value for no entries.
func scripted[T any](entries []T, i int) T {
	var zero T
	switch {
	case i < len(entries):
		return entries[i]
	case len(entries) > 0:
		return entries[len(entries)-1]
	}
	return zero
}

// MergeShuttle is a Run-only fake shuttle: Results[i] and Errs[i] answer the (i+1)th call, and a call past the scripted entries answers a zero Result and a nil error, so an unscripted MergeShuttle is a no-op.
type MergeShuttle struct {
	mu sync.Mutex

	Results []shuttleengine.Result
	Errs    []error
	// RunFn replaces the scripted answer.
	RunFn func(spec shuttleengine.Spec) (shuttleengine.Result, error)
	// DuringRun is called just before the answer, with the 1-based call number and the spec.
	DuringRun func(callNumber int, spec shuttleengine.Spec)

	// Specs holds every Spec handed to Run, in order.
	Specs []shuttleengine.Spec
}

// Run records spec, runs DuringRun, then answers RunFn, else the scripted entry for this call.
func (f *MergeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.mu.Lock()
	f.Specs = append(f.Specs, spec)
	callNumber := len(f.Specs)
	f.mu.Unlock()

	if f.DuringRun != nil {
		f.DuringRun(callNumber, spec)
	}
	if f.RunFn != nil {
		return f.RunFn(spec)
	}

	var res shuttleengine.Result
	if callNumber-1 < len(f.Results) {
		res = f.Results[callNumber-1]
	}
	var err error
	if callNumber-1 < len(f.Errs) {
		err = f.Errs[callNumber-1]
	}
	return res, err
}

type (
	masterStarter  struct{ websterengine.MasterStarter }
	strandStopper  struct{ websterengine.StrandStopper }
	shuttleEngine  struct{ shuttleengine.Engine }
	refMatcherSeam struct{ websterengine.RefMatcher }
	indexSeam      struct{ planindex.Index }
)

// WebsterSeams returns a RunDeps whose Starter, Stopper, Engine and RefMatcher, and the Index of its Geom, are placeholders.
// Each embeds its interface in an empty struct, so it is non-nil for a constructor check and panics if any method is called.
func WebsterSeams() websterengine.RunDeps {
	return websterengine.RunDeps{
		Starter:    masterStarter{},
		Stopper:    strandStopper{},
		Engine:     shuttleEngine{},
		RefMatcher: refMatcherSeam{},
		Geom:       websterengine.Geometry{Index: indexSeam{}},
	}
}

// CallOK calls p once and fails the test on a non-nil error.
func CallOK(t testing.TB, p shedengine.ShedProducer) (shedengine.Outcome, shedengine.OutputPointer) {
	t.Helper()
	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	return outcome, ptr
}

// RequireOutcome is CallOK followed by a non-fatal check of the outcome, and returns the pointer.
func RequireOutcome(t testing.TB, p shedengine.ShedProducer, want shedengine.Outcome) shedengine.OutputPointer {
	t.Helper()
	outcome, ptr := CallOK(t, p)
	if outcome != want {
		t.Errorf("Call() outcome = %q; want %q", outcome, want)
	}
	return ptr
}
