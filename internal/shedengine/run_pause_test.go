// run_pause_test.go covers the clean-stop, resume, and idempotence scenarios: an explicit pause
// request and the resume that must follow it, context cancellation arriving between and during a
// producer call, crash recovery and resuming a halted run across two separate Shed values against
// one status file, and the already-done short-circuit's idempotence rules.

package shedengine

import (
	"context"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// setPauseRequested flips pause_requested to true against statusPath/statusLockPath via
// state.UpdateJSON on a lenient map type, per this batch's rule that any mid-Call mutation of the
// status file must go through the same lock-cooperating write path an external actor is obliged
// to use -- never a bare os.WriteFile or an unlocked struct write. Cancelling a context is not a
// status-file mutation and is unaffected by this rule.
func setPauseRequested(t *testing.T, statusPath, statusLockPath string) {
	t.Helper()
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur map[string]any, found bool) (map[string]any, error) {
		if !found {
			return nil, fmt.Errorf("setPauseRequested: status file %q not found", statusPath)
		}
		cur["pause_requested"] = true
		return cur, nil
	})
	if err != nil {
		t.Fatalf("setPauseRequested: state.UpdateJSON(...) = %v", err)
	}
}

func TestRun_PauseRequestedMidList(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	// The mutation happens from inside the fake producer's own closure -- the only point
	// in the loop guaranteed to be between step 1's read and step 5's persist.
	p1 := &funcProducer{}
	p1.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		setPauseRequested(t, statusPath, statusLockPath)
		return Done, OutputPointer{}, nil
	}
	p2 := fixedOutcomeProducer(Done, "")
	p3 := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: p1},
		{Name: "B", Producer: p2},
		{Name: "C", Producer: p3},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunPaused {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunPaused)
	}
	if p2.calls != 0 {
		t.Errorf("p2.calls = %d; want 0 -- the run must exit before calling the next producer", p2.calls)
	}
	if p3.calls != 0 {
		t.Errorf("p3.calls = %d; want 0", p3.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, StatePaused)
	}
	// current_producer must still name the producer the loop was about to call -- B, the
	// one A's Done routed to -- not A, so the next Run resumes there rather than skipping
	// it.
	if got.CurrentProducer != "B" {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, "B")
	}
}

func TestRun_ResumeAfterPause(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	pausing := &funcProducer{}
	pausing.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		setPauseRequested(t, statusPath, statusLockPath)
		return Done, OutputPointer{}, nil
	}
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: pausing},
		{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunPaused {
		t.Fatalf("first Run(...).Outcome = %q; want %q", result.Outcome, RunPaused)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Fatalf("persisted State = %q; want %q", got.State, StatePaused)
	}
	// The persist that honoured the pause must also write pause_requested back to false,
	// in the same write that recorded StatePaused.
	if got.PauseRequested {
		t.Errorf("persisted PauseRequested = true after the pause; want false")
	}

	// Without this second Run, the permanently-unresumable bug is invisible: every
	// single-Run pause test passes while the task can never restart, because the next
	// Run's own step 3 would re-read a still-true flag and pause again, forever.
	resumed := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Done, "")},
		{Name: "B", Producer: resumed},
	}
	result2, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run(...) = _, %v; want nil error", err)
	}
	if result2.Outcome != RunDone {
		t.Errorf("second Run(...).Outcome = %q; want %q -- resume must proceed to completion, not pause again", result2.Outcome, RunDone)
	}
	if resumed.calls != 1 {
		t.Errorf("resumed producer B calls = %d; want 1", resumed.calls)
	}
}
