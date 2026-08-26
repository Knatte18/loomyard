// run_pause_test.go covers the clean-stop, resume, and idempotence scenarios: an explicit pause
// request and the resume that must follow it, context cancellation arriving between and during a
// producer call, crash recovery and resuming a halted run across two separate Shed values against
// one status file, and the already-done short-circuit's idempotence rules.

package shedengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
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
	shed.Producers = linearChain(t, []string{"A", "B", "C"}, []ShedProducer{p1, p2, p3})
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
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{pausing, fixedOutcomeProducer(Done, "")})
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
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{fixedOutcomeProducer(Done, ""), resumed})
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

func TestRun_CancelledBetweenProducers(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	ctx, cancel := context.WithCancel(context.Background())

	p1 := &funcProducer{}
	p1.fn = func(innerCtx context.Context) (Outcome, OutputPointer, error) {
		cancel()
		return Done, OutputPointer{}, nil
	}
	p2 := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{p1, p2})
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(ctx)
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunPaused {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunPaused)
	}
	// Checking the context at the top of every iteration, rather than leaving it to
	// producers, is what stops a cancelled run from launching new producer calls for
	// however long the current one takes to notice.
	if p2.calls != 0 {
		t.Errorf("p2.calls = %d; want 0 -- no producer is called after the cancellation", p2.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, StatePaused)
	}
}

func TestRun_CancelledDuringProducerCall(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	ctx, cancel := context.WithCancel(context.Background())

	// Models a well-behaved producer handed a cancelled context: it cancels and then
	// returns the context's own error from inside the same Call, the common Ctrl-C shape.
	// The returned error's identity is never asserted anywhere in this test -- the branch
	// keys on the context's own state, not a cancellation sentinel.
	p1 := &funcProducer{}
	p1.fn = func(innerCtx context.Context) (Outcome, OutputPointer, error) {
		cancel()
		return "", OutputPointer{}, ctx.Err()
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: p1}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(ctx)
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error, not a failure -- without this scenario the top-of-iteration check passes while every real cancellation still lands in StateFailed", err)
	}
	if result.Outcome != RunPaused {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunPaused)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, StatePaused)
	}
	if len(got.History) != 0 {
		t.Errorf("persisted History = %+v; want no entry appended -- the producer never reached a verdict", got.History)
	}
	if got.CurrentProducer != "A" {
		t.Errorf("persisted CurrentProducer = %q; want %q -- the next Run must re-call this producer", got.CurrentProducer, "A")
	}
}

func TestRun_CrashRecovery(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	wantErr := errors.New("plan-write: disk full")
	p1a := fixedOutcomeProducer(Done, "")
	p2a := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, wantErr
	}}
	p3a := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"A", "B", "C"}, []ShedProducer{p1a, p2a, p3a})
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Run(context.Background()); err == nil {
		t.Fatalf("first Run(...) = _, nil; want a non-nil error")
	}

	halted := readStatus(t, statusPath, statusLockPath)
	if halted.CurrentProducer != "B" {
		t.Fatalf("persisted CurrentProducer after halt = %q; want %q", halted.CurrentProducer, "B")
	}
	if p3a.calls != 0 {
		t.Fatalf("p3a.calls = %d; want 0 -- the run must halt at B, never reaching C", p3a.calls)
	}

	// A fresh Shed value against the same status file and lock paths is the point of this
	// scenario: nothing carries over in memory, so everything the resume relies on must
	// have survived on disk.
	freshShed := &Shed{
		StatusPath:     statusPath,
		LockPath:       shed.LockPath,
		StatusLockPath: statusLockPath,
	}
	p1b := fixedOutcomeProducer(Done, "")
	p2b := fixedOutcomeProducer(Done, "")
	p3b := fixedOutcomeProducer(Done, "")
	freshShed.Producers = linearChain(t, []string{"A", "B", "C"}, []ShedProducer{p1b, p2b, p3b})

	result, err := freshShed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("second Run(...).Outcome = %q; want %q", result.Outcome, RunDone)
	}
	if p1b.calls != 0 {
		t.Errorf("p1b.calls = %d; want 0 -- the resumed run must not re-call the first producer", p1b.calls)
	}
	if p2b.calls != 1 {
		t.Errorf("p2b.calls = %d; want 1", p2b.calls)
	}
	if p3b.calls != 1 {
		t.Errorf("p3b.calls = %d; want 1", p3b.calls)
	}

	// The file's history is cumulative across invocations: Result.History reports the
	// first run's entries as well as its own.
	wantLen := len(halted.History) + 2
	if len(result.History) != wantLen {
		t.Fatalf("len(result.History) = %d; want %d (the first run's entry plus this run's two)", len(result.History), wantLen)
	}
	for i, entry := range halted.History {
		if result.History[i] != entry {
			t.Errorf("result.History[%d] = %+v; want %+v (carried over from the first run)", i, result.History[i], entry)
		}
	}
}

func TestRun_ResumeAfterHalt(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"StateBlocked", StateBlocked},
		{"StateFailed", StateFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)

			producer := fixedOutcomeProducer(Done, "")
			shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

			seed := commonSeed("A")
			seed.State = tt.state
			seed.Error = "seeded halt for resume test"
			seedStatus(t, statusPath, statusLockPath, seed)

			// This is deliberately asymmetric with StateDone, so a human can resume after
			// fixing whatever caused the halt without hand-editing the status file.
			result, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run(...) = _, %v; want nil error", err)
			}
			if result.Outcome != RunDone {
				t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
			}
			if producer.calls != 1 {
				t.Errorf("producer.calls = %d; want 1 -- %s must not short-circuit", producer.calls, tt.state)
			}

			got := readStatus(t, statusPath, statusLockPath)
			if got.State != StateDone {
				t.Errorf("persisted State = %q; want %q -- the halted state must be overwritten as the loop advances", got.State, StateDone)
			}
		})
	}
}

// TestRun_ResumeWritesRunningBeforeCallingTheProducer pins the resume write: a run resumed from any
// non-running state must record that it IS running before it spends minutes inside a producer call,
// and must clear the halt's stale error with it. The producer observes the file mid-call, which is
// the only moment the property is observable -- afterwards the verdict's own persist has overwritten
// it either way, which is exactly why the defect was invisible until a live run was watched.
func TestRun_ResumeWritesRunningBeforeCallingTheProducer(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"StatePaused", StatePaused},
		{"StateBlocked", StateBlocked},
		{"StateFailed", StateFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)

			var duringCall Status
			producer := &callbackProducer{
				outcome: Done,
				before: func() {
					duringCall = readStatus(t, statusPath, statusLockPath)
				},
			}
			shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

			seed := commonSeed("A")
			seed.State = tt.state
			seed.Error = "seeded halt for resume test"
			seedStatus(t, statusPath, statusLockPath, seed)

			if _, err := shed.Run(context.Background()); err != nil {
				t.Fatalf("Run(...) = _, %v; want nil error", err)
			}

			if duringCall.State != StateRunning {
				t.Errorf("State observed during the producer call = %q; want %q -- a resumed run must not keep describing itself as %s while it is already spawning", duringCall.State, StateRunning, tt.state)
			}
			if duringCall.Error != "" {
				t.Errorf("Error observed during the producer call = %q; want empty -- the halt's reason is stale the moment the loop retries past it", duringCall.Error)
			}
			if duringCall.Activity.Wait != "" {
				t.Errorf("Activity.Wait observed during the producer call = %q; want empty -- the status strand composes its wait field from Error", duringCall.Activity.Wait)
			}
			if duringCall.CurrentProducer != "A" {
				t.Errorf("CurrentProducer observed during the producer call = %q; want %q -- the resume write must not move the pointer", duringCall.CurrentProducer, "A")
			}
			if len(duringCall.History) != len(seed.History) {
				t.Errorf("History length observed during the producer call = %d; want %d -- the resume write records no verdict", len(duringCall.History), len(seed.History))
			}
		})
	}
}

// TestRun_AlreadyRunningStateWritesNothingExtra pins the resume write's conditionality: on the
// ordinary running-to-running path it must not fire, so the loop keeps its one-persist-per-iteration
// shape for every step after a resume.
func TestRun_AlreadyRunningStateWritesNothingExtra(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	var duringCall Status
	producer := &callbackProducer{
		outcome: Done,
		before: func() {
			duringCall = readStatus(t, statusPath, statusLockPath)
		},
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}

	seed := commonSeed("A")
	seed.State = StateRunning
	seed.Error = ""
	seedStatus(t, statusPath, statusLockPath, seed)

	if _, err := shed.Run(context.Background()); err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	// The seeded activity is whatever seedStatus wrote; an unconditional resume write would have
	// recomposed it, so an unchanged value is the observable proof no extra write happened.
	if duringCall.Activity != seed.Activity {
		t.Errorf("Activity observed during the producer call = %+v; want the seeded %+v unchanged -- an already-running run needs no resume write", duringCall.Activity, seed.Activity)
	}
}

// callbackProducer returns a fixed outcome and invokes before (when non-nil) at the start of every
// Call, so a test can observe on-disk state from inside a producer call -- the one window in which
// the loop's own mid-call state is visible.
type callbackProducer struct {
	outcome Outcome
	before  func()
	calls   int
}

func (p *callbackProducer) Call(_ context.Context) (Outcome, OutputPointer, error) {
	p.calls++
	if p.before != nil {
		p.before()
	}
	return p.outcome, OutputPointer{}, nil
}

func TestRun_RerunIdempotence(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{fixedOutcomeProducer(Done, ""), fixedOutcomeProducer(Done, "")})
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Run(context.Background()); err != nil {
		t.Fatalf("first Run(...) = _, %v; want nil error", err)
	}

	before, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after first Run: %v", err)
	}

	// Without the short-circuit a second invocation would re-call the final producer,
	// which for a merge-shaped terminal producer means re-running a merge.
	rerunA := fixedOutcomeProducer(Done, "")
	rerunB := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{rerunA, rerunB})

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("second Run(...).Outcome = %q; want %q", result.Outcome, RunDone)
	}
	if rerunA.calls != 0 {
		t.Errorf("rerunA.calls = %d; want 0", rerunA.calls)
	}
	if rerunB.calls != 0 {
		t.Errorf("rerunB.calls = %d; want 0", rerunB.calls)
	}

	after, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status file after second Run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("status file bytes changed across the short-circuited re-run:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRun_RerunResultEquality(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{fixedOutcomeProducer(Done, ""), fixedOutcomeProducer(Done, "")})
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	first, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run(...) = _, %v; want nil error", err)
	}

	shed.Producers = linearChain(t, []string{"A", "B"}, []ShedProducer{fixedOutcomeProducer(Done, ""), fixedOutcomeProducer(Done, "")})
	second, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run(...) = _, %v; want nil error", err)
	}

	// This is what proves the short-circuit fills both fields from the file rather than
	// returning a bare RunDone, and that idempotence is meant at the API surface, not
	// merely on disk.
	if second.Outcome != first.Outcome {
		t.Errorf("second.Outcome = %q; want %q (equal to first)", second.Outcome, first.Outcome)
	}
	if second.HaltedProducer != first.HaltedProducer {
		t.Errorf("second.HaltedProducer = %q; want %q (equal to first)", second.HaltedProducer, first.HaltedProducer)
	}
	if !reflect.DeepEqual(second.History, first.History) {
		t.Errorf("second.History = %+v; want %+v (equal to first)", second.History, first.History)
	}
}

func TestRun_RerunWithChangedProducerList(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	seed := commonSeed("Retired-Producer")
	seed.State = StateDone
	seedStatus(t, statusPath, statusLockPath, seed)

	// The producer list no longer contains the name the file records. This confirms the
	// short-circuit sits after the read and before the lookup, and that the position is a
	// decision rather than an accident: a finished task must not become un-queryable
	// because someone later edited the producer list, and the never-guess protection
	// exists to stop a live run resuming into the wrong producer, a risk that does not
	// exist once nothing more will be called.
	shed.Producers = []ProducerDef{
		{Name: "Current-Producer", Producer: fixedOutcomeProducer(Done, "")},
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error, not the step-2 lookup error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}
}
