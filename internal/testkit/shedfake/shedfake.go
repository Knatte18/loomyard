// Package shedfake fakes the shed-layer seams: the shuttle, the burler half runner, the seat runner, the merge shuttle and the webster run seams, plus the producer Call helpers.
//
// Shuttle satisfies shedadapters.Shuttle and frictionengine.Shuttle, BurlerHalfRunner satisfies shedadapters.BurlerHalfRunner, SeatRunner satisfies shedadapters.SeatRunner, and MergeShuttle satisfies mergeresolve.Shuttle, all structurally, so no consumer package is imported here and shedadapters' and landingshed's in-package tests can use the kit.
// Every fake exposes fields and optional func overrides, and none asserts anything.
// CallOK and RequireOutcome are the only assertions the kit makes.
//
// internal/burlerengine's in-package tests keep their own Shuttle fake, because this kit imports burlerengine for BurlerHalfRunner's types.
package shedfake

import (
	"context"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/seatengine"
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

// BurlerHalfRunner is a fake burler half runner: ReviewResults[i] and ReviewErrs[i] answer the (i+1)th Review, FixResults[i] and FixErrs[i] the (i+1)th Fix, and once either slice is exhausted its last entry repeats.
// ProbeReview, ResumeReview, ProbeFix and ResumeFix answer their own scripted slices the same way, and unscripted probes answer a half that is gone.
// Every method counts its invocations and records the profile, options and digest it received.
type BurlerHalfRunner struct {
	mu sync.Mutex

	ReviewResults []burlerengine.ReviewResult
	ReviewErrs    []error
	FixResults    []burlerengine.FixResult
	FixErrs       []error

	// LiveReviews and ProbeReviewErrs script ProbeReview; ResumeReviewResults and ResumeReviewErrs script ResumeReview.
	LiveReviews         []burlerengine.LiveHalf
	ProbeReviewErrs     []error
	ResumeReviewResults []burlerengine.ReviewResult
	ResumeReviewErrs    []error
	// LiveFixes and ProbeFixErrs script ProbeFix; ResumeFixResults and ResumeFixErrs script ResumeFix.
	LiveFixes        []burlerengine.LiveHalf
	ProbeFixErrs     []error
	ResumeFixResults []burlerengine.FixResult
	ResumeFixErrs    []error

	// ReviewCalls, ProbeReviewCalls, ResumeReviewCalls, FixCalls, ProbeFixCalls and ResumeFixCalls count the invocations of each method.
	ReviewCalls       int
	ProbeReviewCalls  int
	ResumeReviewCalls int
	FixCalls          int
	ProbeFixCalls     int
	ResumeFixCalls    int

	// GotReviewProfiles and GotReviewOpts hold every Profile and RunOpts handed to Review, in order; GotProbeReviewOpts, GotProbeFixOpts and GotResumeFixOpts hold the RunOpts of the other verbs.
	GotReviewProfiles  []burlerengine.Profile
	GotReviewOpts      []burlerengine.RunOpts
	GotProbeReviewOpts []burlerengine.RunOpts
	GotProbeFixOpts    []burlerengine.RunOpts
	GotResumeFixOpts   []burlerengine.RunOpts
	// GotFixProfiles, GotFixOpts and GotFixDigests hold every Profile, RunOpts and digest handed to Fix, in order; GotResumeFixDigests holds the digests handed to ResumeFix.
	GotFixProfiles      []burlerengine.Profile
	GotFixOpts          []burlerengine.RunOpts
	GotFixDigests       []string
	GotResumeFixDigests []string
	// GotLiveReviews and GotLiveFixes hold every LiveHalf handed to ResumeReview and ResumeFix.
	GotLiveReviews []burlerengine.LiveHalf
	GotLiveFixes   []burlerengine.LiveHalf

	// DuringReview and DuringFix run from inside Review and Fix, before they answer, with the zero-based index of the invocation.
	DuringReview func(callIndex int)
	DuringFix    func(callIndex int)
}

// Review records p and opts, runs DuringReview, then answers the scripted entry for this invocation.
func (f *BurlerHalfRunner) Review(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.ReviewResult, error) {
	f.mu.Lock()
	i := f.ReviewCalls
	f.ReviewCalls++
	f.GotReviewProfiles = append(f.GotReviewProfiles, p)
	f.GotReviewOpts = append(f.GotReviewOpts, opts)
	f.mu.Unlock()
	if f.DuringReview != nil {
		f.DuringReview(i)
	}
	return scripted(f.ReviewResults, i), scripted(f.ReviewErrs, i)
}

// ProbeReview counts the call, records opts and answers the scripted LiveHalf and error for this invocation.
func (f *BurlerHalfRunner) ProbeReview(_ burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveHalf, error) {
	f.mu.Lock()
	i := f.ProbeReviewCalls
	f.ProbeReviewCalls++
	f.GotProbeReviewOpts = append(f.GotProbeReviewOpts, opts)
	f.mu.Unlock()
	return scripted(f.LiveReviews, i), scripted(f.ProbeReviewErrs, i)
}

// ResumeReview counts the call, records live and answers the scripted ReviewResult and error for this invocation.
func (f *BurlerHalfRunner) ResumeReview(_ burlerengine.Profile, _ burlerengine.RunOpts, live burlerengine.LiveHalf) (burlerengine.ReviewResult, error) {
	f.mu.Lock()
	i := f.ResumeReviewCalls
	f.ResumeReviewCalls++
	f.GotLiveReviews = append(f.GotLiveReviews, live)
	f.mu.Unlock()
	return scripted(f.ResumeReviewResults, i), scripted(f.ResumeReviewErrs, i)
}

// Fix records p, opts and digest, runs DuringFix, then answers the scripted entry for this invocation.
func (f *BurlerHalfRunner) Fix(p burlerengine.Profile, opts burlerengine.RunOpts, digest string) (burlerengine.FixResult, error) {
	f.mu.Lock()
	i := f.FixCalls
	f.FixCalls++
	f.GotFixProfiles = append(f.GotFixProfiles, p)
	f.GotFixOpts = append(f.GotFixOpts, opts)
	f.GotFixDigests = append(f.GotFixDigests, digest)
	f.mu.Unlock()
	if f.DuringFix != nil {
		f.DuringFix(i)
	}
	return scripted(f.FixResults, i), scripted(f.FixErrs, i)
}

// ProbeFix counts the call, records opts and answers the scripted LiveHalf and error for this invocation.
func (f *BurlerHalfRunner) ProbeFix(_ burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.LiveHalf, error) {
	f.mu.Lock()
	i := f.ProbeFixCalls
	f.ProbeFixCalls++
	f.GotProbeFixOpts = append(f.GotProbeFixOpts, opts)
	f.mu.Unlock()
	return scripted(f.LiveFixes, i), scripted(f.ProbeFixErrs, i)
}

// ResumeFix counts the call, records opts, digest and live, and answers the scripted FixResult and error for this invocation.
func (f *BurlerHalfRunner) ResumeFix(_ burlerengine.Profile, opts burlerengine.RunOpts, digest string, live burlerengine.LiveHalf) (burlerengine.FixResult, error) {
	f.mu.Lock()
	i := f.ResumeFixCalls
	f.ResumeFixCalls++
	f.GotResumeFixOpts = append(f.GotResumeFixOpts, opts)
	f.GotResumeFixDigests = append(f.GotResumeFixDigests, digest)
	f.GotLiveFixes = append(f.GotLiveFixes, live)
	f.mu.Unlock()
	return scripted(f.ResumeFixResults, i), scripted(f.ResumeFixErrs, i)
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

// SeatRunner is a fake seat runner: Results[i] and Errs[i] answer the (i+1)th Run, and once either slice is exhausted its last entry repeats.
// Probe and Resume answer their own scripted slices the same way, and an unscripted Probe answers a table with no seat live.
// RunFn, ProbeFn and ResumeFn, when set, replace the scripted answer of their verb.
// Run and Resume follow the "every test fake evaluates the gate once" contract: a done chair whose answered result carries no GateOutcome has the table's gate evaluated once and the outcome stamped onto it.
type SeatRunner struct {
	mu sync.Mutex

	Results []seatengine.Result
	Errs    []error

	// LiveTables and ProbeErrs script Probe; ResumeResults and ResumeErrs script Resume.
	LiveTables    []seatengine.LiveTable
	ProbeErrs     []error
	ResumeResults []seatengine.Result
	ResumeErrs    []error

	// RunFn, ProbeFn and ResumeFn replace the scripted answers, for a consumer that dispatches per table.
	RunFn    func(table seatengine.Table) (seatengine.Result, error)
	ProbeFn  func(table seatengine.Table) (seatengine.LiveTable, error)
	ResumeFn func(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error)

	// Calls, ProbeCalls and ResumeCalls count the invocations of each verb.
	Calls       int
	ProbeCalls  int
	ResumeCalls int
	// GotTables, GotProbeTables and GotResumeTables hold every Table handed to each verb, in order, and GotLive every LiveTable handed to Resume.
	GotTables       []seatengine.Table
	GotProbeTables  []seatengine.Table
	GotResumeTables []seatengine.Table
	GotLive         []seatengine.LiveTable

	// GateAttempts is stamped onto the GateOutcome a gated Run or Resume builds, and GateReason onto it as Reason when the gate did not pass.
	GateAttempts int
	GateReason   string
}

// stampGate evaluates table's gate once for a done chair whose result carries no GateOutcome, stamping the outcome onto result.Chair.
// A non-empty err, an empty gate, a chair that is not done and a result already carrying a GateOutcome are returned unchanged; a closure's error is returned with result.
func (f *SeatRunner) stampGate(table seatengine.Table, result seatengine.Result, err error) (seatengine.Result, error) {
	if err != nil || len(table.Gate) == 0 || result.Chair.Outcome != shuttleengine.OutcomeDone || result.Chair.Gate != nil {
		return result, err
	}
	passed, gerr := evalGateList(table.Gate)
	if gerr != nil {
		return result, gerr
	}
	result.Chair.Gate = &shuttleengine.GateOutcome{Passed: passed, Attempts: f.GateAttempts}
	if !passed {
		result.Chair.Gate.Reason = f.GateReason
	}
	return result, nil
}

// Run records table, then answers RunFn, else the scripted entry for this invocation.
func (f *SeatRunner) Run(table seatengine.Table) (seatengine.Result, error) {
	f.mu.Lock()
	i := f.Calls
	f.Calls++
	f.GotTables = append(f.GotTables, table)
	f.mu.Unlock()
	if f.RunFn != nil {
		result, err := f.RunFn(table)
		return f.stampGate(table, result, err)
	}
	return f.stampGate(table, scripted(f.Results, i), scripted(f.Errs, i))
}

// Probe counts the call, records table, then answers ProbeFn, else the scripted LiveTable and error for this invocation.
func (f *SeatRunner) Probe(table seatengine.Table) (seatengine.LiveTable, error) {
	f.mu.Lock()
	i := f.ProbeCalls
	f.ProbeCalls++
	f.GotProbeTables = append(f.GotProbeTables, table)
	f.mu.Unlock()
	if f.ProbeFn != nil {
		return f.ProbeFn(table)
	}
	return scripted(f.LiveTables, i), scripted(f.ProbeErrs, i)
}

// Resume counts the call, records table and live, then answers ResumeFn, else the scripted Result and error for this invocation.
func (f *SeatRunner) Resume(table seatengine.Table, live seatengine.LiveTable) (seatengine.Result, error) {
	f.mu.Lock()
	i := f.ResumeCalls
	f.ResumeCalls++
	f.GotResumeTables = append(f.GotResumeTables, table)
	f.GotLive = append(f.GotLive, live)
	f.mu.Unlock()
	if f.ResumeFn != nil {
		result, err := f.ResumeFn(table, live)
		return f.stampGate(table, result, err)
	}
	return f.stampGate(table, scripted(f.ResumeResults, i), scripted(f.ResumeErrs, i))
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
	shuttleEngine  struct{ shuttleengine.ForkAuditor }
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
		ReadOnly:   func(string) bool { return false },
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
