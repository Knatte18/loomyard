package shedengine

import (
	"context"
	"testing"
)

// awaitingProducer returns a producer that reports Awaiting with reason.
func awaitingProducer(reason string) *funcProducer {
	return &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return Awaiting, OutputPointer{Reason: reason}, nil
		},
	}
}

func TestRun_AwaitingHaltsWithReason(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	a := awaitingProducer("waiting on PR review")
	b := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A", OnDone: "B"},
		{Name: "B", Producer: b},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunAwaiting {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunAwaiting)
	}
	if result.HaltedProducer != "A" {
		t.Errorf("Result.HaltedProducer = %q; want A", result.HaltedProducer)
	}
	if result.Reason != "waiting on PR review" {
		t.Errorf("Result.Reason = %q; want the producer's reason", result.Reason)
	}
	if b.calls != 0 {
		t.Errorf("b.calls = %d; want 0 (awaiting never routes)", b.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateAwaiting {
		t.Errorf("persisted State = %q; want %q", got.State, StateAwaiting)
	}
	if got.CurrentProducer != "A" {
		t.Errorf("persisted CurrentProducer = %q; want A", got.CurrentProducer)
	}
	if got.Error != "waiting on PR review" {
		t.Errorf("persisted Error = %q; want the reason", got.Error)
	}
	if got.Activity.Wait != "waiting on PR review" {
		t.Errorf("Activity.Wait = %q; want the reason", got.Activity.Wait)
	}
	if len(got.History) != 1 || got.History[0].Outcome != Awaiting {
		t.Errorf("History = %+v; want one awaiting entry", got.History)
	}
}

func TestStep_AwaitingThenStepRecallsSameProducer(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	a := awaitingProducer("hand-off")
	shed.Producers = []ProducerDef{{Name: "A", Producer: a}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step #1 = _, %v", err)
	}
	if res.State != StateAwaiting || res.Reason != "hand-off" {
		t.Errorf("Step #1 State/Reason = %q/%q; want awaiting/hand-off", res.State, res.Reason)
	}

	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step #2 = _, %v", err)
	}
	if a.calls != 2 {
		t.Errorf("a.calls = %d; want 2 (resume re-calls the same producer)", a.calls)
	}
}

func TestRun_AwaitingDoesNotSpendBounceBudget(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 1

	calls := 0
	a := &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			calls++
			if calls%2 == 1 {
				return Awaiting, OutputPointer{Reason: "wait"}, nil
			}
			return Stuck, OutputPointer{}, nil
		},
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: a, OnStuck: "A"}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	// Call 1 awaits, call 2 is the one permitted bounce, call 3 awaits, call 4 is refused: the
	// two Awaiting entries never counted, so the budget of one was fully available.
	wantStates := []RunOutcome{RunAwaiting, RunAwaiting, RunBlocked}
	for i, want := range wantStates {
		result, err := shed.Run(context.Background())
		if err != nil {
			t.Fatalf("Run #%d = _, %v", i+1, err)
		}
		if result.Outcome != want {
			t.Fatalf("Run #%d Outcome = %q; want %q", i+1, result.Outcome, want)
		}
	}
	if calls != 4 {
		t.Errorf("calls = %d; want 4", calls)
	}

	if got := episodeStuckCount([]HistoryEntry{
		{Producer: "A", Outcome: Awaiting},
		{Producer: "A", Outcome: Stuck},
		{Producer: "A", Outcome: Awaiting},
	}, ProducerDef{Name: "A"}, nil); got != 1 {
		t.Errorf("episodeStuckCount = %d; want 1 (Awaiting never counts)", got)
	}
}

func TestStep_AwaitingStatusPassesReadGate(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	seed := commonSeed("A")
	seed.State = StateAwaiting
	seed.Error = "hand-off"
	seedStatus(t, statusPath, statusLockPath, seed)
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}

	if !StateAwaiting.valid() {
		t.Fatal("StateAwaiting.valid() = false; want true")
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil (awaiting file passes the read gate)", err)
	}
	if res.State != StateDone {
		t.Errorf("State = %q; want done after the resumed producer finishes", res.State)
	}
}
