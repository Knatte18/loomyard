package loomshed

import (
	"context"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// countingProducer is a shedengine.ShedProducer fake that always reports Done and counts every
// call, used to prove crash-recovery's unconditional re-call: a producer whose output already
// exists must still be called again, never skipped.
type countingProducer struct {
	calls int
}

func (p *countingProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.calls++
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// resetCurrentProducer patches the status file at statusPath/statusLockPath to name
// currentProducer, state running, and pauseRequested, via state.UpdateJSON -- never by hand-writing
// JSON -- simulating the on-disk snapshot a crash, a bounce, or a pending pause leaves behind.
func resetCurrentProducer(t *testing.T, statusPath, statusLockPath, currentProducer string, pauseRequested bool) {
	t.Helper()
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = currentProducer
		cur.State = shedengine.StateRunning
		cur.PauseRequested = pauseRequested
		return cur, nil
	})
	if err != nil {
		t.Fatalf("resetCurrentProducer: state.UpdateJSON: %v", err)
	}
}

// TestResume_DoesNotRestartAtRowOne drives a run to shedengine.RunBlocked at Batchifier (whose
// OnStuck is empty, per the real Batchifier gate genuinely failing on a malformed config -- never a
// substituted fake row), fixes the on-disk cause, and asserts that a freshly constructed Shed over
// the same status file re-calls Batchifier directly rather than restarting the whole list at
// Preflight.
func TestResume_DoesNotRestartAtRowOne(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	writeBatcherConfig(t, deps.AnchorPath, "active: [not valid yaml\n")

	shed1, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunBlocked {
		t.Fatalf("first Run() outcome = %q; want %q (reason: %s)", result1.Outcome, shedengine.RunBlocked, result1.Reason)
	}
	if result1.HaltedProducer != NameBatchifier {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, NameBatchifier)
	}

	writeBatcherConfig(t, deps.AnchorPath, `active: "identity"`+"\n")

	shed2, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunDone {
		t.Fatalf("second Run() outcome = %q; want %q (reason: %s)", result2.Outcome, shedengine.RunDone, result2.Reason)
	}

	// result2.History is the full persisted history, including the first run's own appended
	// entries (Result.History docs this explicitly) -- so "did not restart at row 1" is proven by
	// Preflight appearing exactly once across both runs, not by History[0] naming Batchifier.
	preflightCount := 0
	for _, e := range result2.History {
		if e.Producer == NamePreflight {
			preflightCount++
		}
	}
	if preflightCount != 1 {
		t.Errorf("Preflight appears %d time(s) across both runs' History; want 1 -- a restart at row 1 would call it again", preflightCount)
	}
}

// TestResume_CrashRecoveryRecallsUnconditionally proves the re-call is unconditional: after a
// complete run, resetting current_producer back to Preflight -- as if Preflight's output already
// exists from the first pass -- still makes a fresh Shed call it again, rather than skip it.
func TestResume_CrashRecoveryRecallsUnconditionally(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	counting := &countingProducer{}
	deps.Preflight = counting

	shed1, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunDone {
		t.Fatalf("first Run() outcome = %q; want %q", result1.Outcome, shedengine.RunDone)
	}
	if counting.calls != 1 {
		t.Fatalf("counting.calls after first Run() = %d; want 1", counting.calls)
	}

	resetCurrentProducer(t, deps.StatusPath, deps.StatusLockPath, NamePreflight, false)

	shed2, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunDone {
		t.Fatalf("second Run() outcome = %q; want %q", result2.Outcome, shedengine.RunDone)
	}
	if counting.calls != 2 {
		t.Errorf("counting.calls after second Run() = %d; want 2 -- Preflight must be re-called even though its output already exists from the first pass", counting.calls)
	}
}

// TestResume_PauseStopsAtBoundaryAndClearsFlag sets pause_requested on a status file already
// mid-list, asserts the run stops at that boundary with shedengine.RunPaused without calling the
// halted producer, that the flag is cleared in the same persist, and that a subsequent run resumes
// rather than re-pausing on the flag it is resuming from.
func TestResume_PauseStopsAtBoundaryAndClearsFlag(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	resetCurrentProducer(t, deps.StatusPath, deps.StatusLockPath, NameBatchifier, true)

	shed1, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunPaused {
		t.Fatalf("first Run() outcome = %q; want %q", result1.Outcome, shedengine.RunPaused)
	}
	if result1.HaltedProducer != NameBatchifier {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, NameBatchifier)
	}
	if len(result1.History) != 0 {
		t.Errorf("first Run() History = %+v; want empty -- pause is checked before the halted producer is ever called", result1.History)
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](deps.StatusPath, deps.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after pause")
	}
	if got.State != shedengine.StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StatePaused)
	}
	if got.PauseRequested {
		t.Errorf("persisted PauseRequested = true; want false -- the flag must clear in the same persist that records the paused state")
	}
	if got.CurrentProducer != NameBatchifier {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, NameBatchifier)
	}

	shed2, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunDone {
		t.Fatalf("second Run() outcome = %q; want %q -- a subsequent run must resume rather than re-pause on the flag it is resuming from", result2.Outcome, shedengine.RunDone)
	}
}

// TestBounceRouting_StuckContinuesAtDeclaredTarget drives Discussion-Validate (a real producer)
// genuinely Stuck by removing its decision record from disk, and asserts the run continues at its
// declared OnStuck target, Discussion-Write, immediately afterward.
func TestBounceRouting_StuckContinuesAtDeclaredTarget(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	if err := os.Remove(deps.DecisionRecordPath); err != nil {
		t.Fatalf("remove decision record: %v", err)
	}

	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	stuckIdx := -1
	for i, e := range result.History {
		if e.Producer == NameDiscussionValidate && e.Outcome == shedengine.Stuck {
			stuckIdx = i
			break
		}
	}
	if stuckIdx == -1 {
		t.Fatalf("Run() History has no %s Stuck entry: %+v", NameDiscussionValidate, result.History)
	}
	if stuckIdx+1 >= len(result.History) {
		t.Fatalf("Run() History ends at the Stuck entry; want a following entry naming the bounce target %q", NameDiscussionWrite)
	}
	if got := result.History[stuckIdx+1].Producer; got != NameDiscussionWrite {
		t.Errorf("History[%d].Producer (following the Stuck entry) = %q; want the declared bounce target %q", stuckIdx+1, got, NameDiscussionWrite)
	}
}

// TestBounceRouting_EmptyTargetBlocksInstead drives Batchifier (a real producer, OnStuck: "")
// genuinely Stuck via a malformed on-disk config, and asserts the run ends shedengine.RunBlocked
// rather than bouncing anywhere.
func TestBounceRouting_EmptyTargetBlocksInstead(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	writeBatcherConfig(t, deps.AnchorPath, "active: [not valid yaml\n")

	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != NameBatchifier {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, NameBatchifier)
	}
	if result.Reason != "stuck with no OnStuck target" {
		t.Errorf("Run() Reason = %q; want %q", result.Reason, "stuck with no OnStuck target")
	}
}

// TestBounceRouting_BudgetExhaustionBlocks drives Discussion-Validate genuinely and repeatedly
// Stuck (its decision record stays absent for the whole run, so Discussion-Write's own Done never
// fixes it), with a small Deps.MaxBounces, and asserts the bounce budget is consumed and exhausting
// it blocks -- MaxBounces+1 Stuck entries, then shedengine.RunBlocked.
func TestBounceRouting_BudgetExhaustionBlocks(t *testing.T) {
	_, deps := buildSequenceFixture(t)
	deps.MaxBounces = 2
	if err := os.Remove(deps.DecisionRecordPath); err != nil {
		t.Fatalf("remove decision record: %v", err)
	}

	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != NameDiscussionValidate {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, NameDiscussionValidate)
	}
	if result.Reason != "bounce budget exhausted" {
		t.Errorf("Run() Reason = %q; want %q", result.Reason, "bounce budget exhausted")
	}

	stuckCount := 0
	for _, e := range result.History {
		if e.Producer == NameDiscussionValidate && e.Outcome == shedengine.Stuck {
			stuckCount++
		}
	}
	if want := deps.MaxBounces + 1; stuckCount != want {
		t.Errorf("Discussion-Validate Stuck count = %d; want %d (MaxBounces+1)", stuckCount, want)
	}
}

// TestCancellation_RealProducersReturnErrorNotStuck asserts the one obligation shedengine cannot
// enforce for itself: every real producer this task builds -- the discussion validator, the plan
// validator, the batch gate, the Webster wrapper, and the Preflight wrapper -- returns a non-nil
// error rather than shedengine.Stuck when called under an already-cancelled context. A Stuck under
// a cancelled context is indistinguishable to Shed from a genuine verdict and would silently
// consume bounce budget for what was actually an operator stop.
func TestCancellation_RealProducersReturnErrorNotStuck(t *testing.T) {
	_, deps := buildSequenceFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producers := []struct {
		name string
		p    interface {
			Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)
		}
	}{
		{NameDiscussionValidate, newDiscussionValidate(NameDiscussionValidate, deps.DecisionRecordPath, deps.SupportLogPath)},
		{NamePlanValidate, newPlanValidate(NamePlanValidate, deps.AnchorPath, deps.WorktreeRoot)},
		{NameBatchifier, newBatchifier(NameBatchifier, deps.AnchorPath)},
		{NameWebster, newWebsterProducer(NameWebster, deps.AnchorPath, deps.WebsterRun, websterengine.RunDeps{})},
		{NamePreflight, NewPreflightProducer(deps.AnchorPath)},
	}

	for _, tt := range producers {
		t.Run(tt.name, func(t *testing.T) {
			outcome, _, err := tt.p.Call(ctx)
			if err == nil {
				t.Fatalf("Call(cancelled) error = nil; want non-nil error")
			}
			if outcome == shedengine.Done || outcome == shedengine.Stuck {
				t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
			}
		})
	}
}
