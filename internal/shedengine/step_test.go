// step_test.go covers (*Shed).Step's routing arms (one test per arm stepLocked's switch takes),
// its shared preamble (preflight's parent-dir creation and validate-before-lock ordering), and its
// per-call lock window (the ErrShedBusy refusal and the release-between-calls property that makes
// stepping possible at all). It reuses newTestShed, seedStatus, readStatus, commonSeed,
// fixedOutcomeProducer, funcProducer, and assertRFC3339UTC from testsupport_test.go rather than
// redeclaring any of them.

package shedengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestStep_DoneWithNonEmptyOnDone(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Done, "out.md")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: producer, OnDone: "B"},
		{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.Producer != "A" {
		t.Errorf("Producer = %q; want %q", res.Producer, "A")
	}
	if res.Outcome != Done {
		t.Errorf("Outcome = %q; want %q", res.Outcome, Done)
	}
	if res.Next != "B" {
		t.Errorf("Next = %q; want %q", res.Next, "B")
	}
	if res.State != StateRunning {
		t.Errorf("State = %q; want %q", res.State, StateRunning)
	}
	if len(res.History) != 1 {
		t.Fatalf("len(History) = %d; want 1", len(res.History))
	}
	if res.History[0].Producer != "A" || res.History[0].Outcome != Done || res.History[0].Output != "out.md" {
		t.Errorf("History[0] = %+v; want Producer A, Outcome done, Output out.md", res.History[0])
	}
	assertRFC3339UTC(t, res.History[0].At)

	got := readStatus(t, statusPath, statusLockPath)
	if got.CurrentProducer != "B" || got.State != StateRunning {
		t.Errorf("persisted CurrentProducer/State = %q/%q; want B/running", got.CurrentProducer, got.State)
	}
}

func TestStep_DoneWithEmptyOnDone(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "Finalize", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Finalize"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.State != StateDone {
		t.Errorf("State = %q; want %q", res.State, StateDone)
	}
	if res.Next != "Finalize" {
		t.Errorf("Next = %q; want %q -- never empty", res.Next, "Finalize")
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, StateDone)
	}
}

func TestStep_StuckWithinBudget(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "B"},
		{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.Next != "B" {
		t.Errorf("Next = %q; want %q", res.Next, "B")
	}
	if res.State != StateRunning {
		t.Errorf("State = %q; want %q", res.State, StateRunning)
	}
}

func TestStep_StuckWithNoOnStuck(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.State != StateBlocked {
		t.Errorf("State = %q; want %q", res.State, StateBlocked)
	}
	const wantReason = ReasonNoOnStuckTarget
	if res.Reason != wantReason {
		t.Errorf("Reason = %q; want %q", res.Reason, wantReason)
	}
}

func TestStep_StuckAtBudgetBoundary(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A", MaxBounces: 3},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	// Three bounces within budget.
	for i := 0; i < 3; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d (...) = _, %v; want nil error", i, err)
		}
		if res.State != StateRunning {
			t.Fatalf("Step %d State = %q; want %q", i, res.State, StateRunning)
		}
		if res.Next != "A" {
			t.Fatalf("Step %d Next = %q; want %q", i, res.Next, "A")
		}
	}

	// The fourth Stuck exhausts the budget and blocks.
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("fourth Step(...) = _, %v; want nil error", err)
	}
	if res.State != StateBlocked {
		t.Errorf("fourth Step State = %q; want %q", res.State, StateBlocked)
	}
	const wantReason = ReasonBounceBudgetExhausted
	if !strings.HasPrefix(res.Reason, wantReason) {
		t.Errorf("fourth Step Reason = %q; want prefix %q", res.Reason, wantReason)
	}
	if a.calls != 4 {
		t.Errorf("a.calls = %d; want 4", a.calls)
	}
}

func TestStep_PauseRequested(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seed := commonSeed("A")
	seed.PauseRequested = true
	seedStatus(t, statusPath, statusLockPath, seed)

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.Producer != "" {
		t.Errorf("Producer = %q; want empty", res.Producer)
	}
	if res.State != StatePaused {
		t.Errorf("State = %q; want %q", res.State, StatePaused)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.PauseRequested {
		t.Errorf("persisted PauseRequested = true; want false")
	}
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}
}

func TestStep_CancelledContextWithProducerError(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	ctx, cancel := context.WithCancel(context.Background())
	producer := &funcProducer{}
	producer.fn = func(innerCtx context.Context) (Outcome, OutputPointer, error) {
		cancel()
		return "", OutputPointer{}, ctx.Err()
	}
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(ctx)
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.Producer != "A" {
		t.Errorf("Producer = %q; want %q", res.Producer, "A")
	}
	if res.Outcome != "" {
		t.Errorf("Outcome = %q; want empty", res.Outcome)
	}
	if res.State != StatePaused {
		t.Errorf("State = %q; want %q", res.State, StatePaused)
	}
	if len(res.History) != 0 {
		t.Errorf("len(History) = %d; want 0 -- unchanged", len(res.History))
	}
}

func TestStep_ProducerHardErrorHealthyContext(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	wantMsg := "disk full"
	producer := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, errors.New(wantMsg)
	}}
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err == nil {
		t.Fatalf("Step(...) = %+v, nil; want a non-nil error", res)
	}
	if !reflect.DeepEqual(res, StepResult{}) {
		t.Errorf("StepResult = %+v; want the zero value", res)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateFailed {
		t.Errorf("persisted State = %q; want %q", got.State, StateFailed)
	}
	if got.Error != wantMsg {
		t.Errorf("persisted Error = %q; want %q", got.Error, wantMsg)
	}
}

func TestStep_UnrecognisedOutcome(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return Outcome("approved"), OutputPointer{}, nil
	}}
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err == nil {
		t.Fatalf("Step(...) = %+v, nil; want a non-nil error", res)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateFailed {
		t.Errorf("persisted State = %q; want %q", got.State, StateFailed)
	}
}

func TestStep_AlreadyDone(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seed := commonSeed("A")
	seed.State = StateDone
	seed.History = []HistoryEntry{{Producer: "A", Outcome: Done, At: "2020-01-01T00:00:00Z"}}
	seedStatus(t, statusPath, statusLockPath, seed)

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.Producer != "" {
		t.Errorf("Producer = %q; want empty", res.Producer)
	}
	if res.State != StateDone {
		t.Errorf("State = %q; want %q", res.State, StateDone)
	}
	if len(res.History) != len(seed.History) {
		t.Errorf("len(History) = %d; want %d -- unchanged", len(res.History), len(seed.History))
	}
	if producer.calls != 0 {
		t.Errorf("producer.calls = %d; want 0", producer.calls)
	}
}

func TestStep_ResumeFromHaltedStates(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"StateBlocked", StateBlocked},
		{"StateFailed", StateFailed},
		{"StatePaused", StatePaused},
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

			res, err := shed.Step(context.Background())
			if err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if res.State != StateDone {
				t.Errorf("State = %q; want %q", res.State, StateDone)
			}
			if producer.calls != 1 {
				t.Errorf("producer.calls = %d; want 1 -- the producer must be called and the resume write must fire", producer.calls)
			}
		})
	}
}

func TestStep_EmptyOutcomeWithError(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, errors.New("boom")
	}}
	shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Step(context.Background()); err == nil {
		t.Fatalf("Step(...) = _, nil; want a non-nil error")
	}

	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 0 {
		t.Errorf("persisted History = %+v; want none -- an outcome the producer never reached is no value to record", got.History)
	}
}

func TestStep_ErrShedBusy(t *testing.T) {
	shed, _, lockPath, _ := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}

	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create lock parent dir: %v", err)
	}
	held, locked, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("TryAcquireWriteLock(...) = _, _, %v", err)
	}
	if !locked {
		t.Fatalf("TryAcquireWriteLock(...) locked = false; want true")
	}

	_, err = shed.Step(context.Background())
	if !errors.Is(err, ErrShedBusy) {
		t.Fatalf("Step(...) error = %v; want errors.Is(err, ErrShedBusy)", err)
	}
	if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx shed pause") || !strings.Contains(err.Error(), "lyx shed status") {
		t.Errorf("Step(...) error = %q; want a way forward naming lyx shed pause and lyx shed status", err.Error())
	}
	if _, err := shed.Run(context.Background()); !errors.Is(err, ErrShedBusy) || !strings.Contains(err.Error(), "way forward: \"lyx shed pause\"") {
		t.Errorf("Run(...) error = %v; want ErrShedBusy carrying the same way forward", err)
	}
	shed.RunID = "some-slug"
	if _, err := shed.Step(context.Background()); err == nil || !strings.Contains(err.Error(), `"lyx shed pause some-slug"`) || !strings.Contains(err.Error(), `"lyx shed status some-slug"`) {
		t.Errorf("Step(...) on a Shed told RunID error = %v; want the pause and status verbs addressing some-slug", err)
	}
	shed.RunID = ""

	// The way forward taken: the holder releases, and the refused call succeeds.
	statusPath := shed.StatusPath
	held.Release()
	seedStatus(t, statusPath, shed.StatusLockPath, commonSeed("A"))
	if _, err := shed.Step(context.Background()); err != nil {
		t.Errorf("Step(...) after the lock was released = %v; want nil", err)
	}
}

func TestStep_MissingStatusFileNamesSeeding(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}

	_, err := shed.Step(context.Background())
	if err == nil {
		t.Fatal("Step(...) with no status file = nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lyx shed seed") {
		t.Errorf("Step(...) error = %q; want a way forward naming the bootstrap verb and lyx shed seed", err.Error())
	}

	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))
	if _, err := shed.Step(context.Background()); err != nil {
		t.Errorf("Step(...) after seeding = %v; want nil", err)
	}
}

func TestStep_LockReleasedBetweenCalls(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B"},
		{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("first Step(...) = _, %v; want nil error", err)
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("second Step(...) = _, %v; want nil error -- the lock must be released between calls", err)
	}
}

func TestStep_PreamblePreparesLockDirs(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	// newTestShed deliberately leaves both lock paths' parent directories uncreated, so this
	// exercises preflight's own MkdirAll calls.
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
}

func TestStep_ValidateFailsBeforeTouchingLock(t *testing.T) {
	shed, _, lockPath, _ := newTestShed(t)
	// An empty Producers list is invalid, per validate's own rule.
	shed.Producers = nil

	if _, err := shed.Step(context.Background()); err == nil {
		t.Fatalf("Step(...) = _, nil; want a non-nil error")
	}

	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Errorf("os.Stat(lockPath) = _, %v; want a not-exist error -- validate must fail before the lock is ever touched", statErr)
	}
}

func TestStep_StuckReasonPersistence(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		want   string
	}{
		{"supplied", "x", "x"},
		{"empty", "", ReasonNoOnStuckTarget},
		{"whitespace only", " \t\r\n ", ReasonNoOnStuckTarget},
		{"multi-line", "line one\r\n\nline two", "line one line two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{{Name: "A", Producer: &funcProducer{
				fn: func(context.Context) (Outcome, OutputPointer, error) {
					return Stuck, OutputPointer{Reason: tc.reason}, nil
				},
			}}}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			res, err := shed.Step(context.Background())
			if err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if res.State != StateBlocked || res.Reason != tc.want {
				t.Errorf("State, Reason = %q, %q; want %q, %q", res.State, res.Reason, StateBlocked, tc.want)
			}
			got := readStatus(t, statusPath, statusLockPath)
			if got.State != StateBlocked || got.Error != tc.want || got.Activity.Wait != tc.want {
				t.Errorf("persisted state/error/wait = %q/%q/%q; want %q/%q/%q", got.State, got.Error, got.Activity.Wait, StateBlocked, tc.want, tc.want)
			}
		})
	}
}

func TestStep_StuckReasonIgnoredWhenBouncing(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", OnStuck: "A", Producer: &funcProducer{
		fn: func(context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{Reason: "ignored"}, nil
		},
	}}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if res.State != StateRunning || res.Reason != "" {
		t.Errorf("State, Reason = %q, %q; want running, empty", res.State, res.Reason)
	}
	if got := readStatus(t, statusPath, statusLockPath); got.Error != "" {
		t.Errorf("persisted Error = %q; want empty", got.Error)
	}
}

func TestStep_BudgetArmIgnoresProducerReason(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", OnStuck: "A", MaxBounces: 1, Producer: &funcProducer{
		fn: func(context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{Reason: "producer reason"}, nil
		},
	}}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	var res StepResult
	for i := 0; i < 2; i++ {
		var err error
		if res, err = shed.Step(context.Background()); err != nil {
			t.Fatalf("Step %d = _, %v; want nil error", i, err)
		}
	}
	if res.State != StateBlocked || !strings.HasPrefix(res.Reason, ReasonBounceBudgetExhausted) {
		t.Errorf("State, Reason = %q, %q; want blocked, prefix %q", res.State, res.Reason, ReasonBounceBudgetExhausted)
	}
	if got := readStatus(t, statusPath, statusLockPath); !strings.HasPrefix(got.Error, ReasonBounceBudgetExhausted) {
		t.Errorf("persisted Error = %q; want prefix %q", got.Error, ReasonBounceBudgetExhausted)
	}
}

// TestStep_ToldRunIDNamesGotoWithRunID pins that an exhausted budget and a missing producer address the told run-id in their goto way forward.
func TestStep_ToldRunIDNamesGotoWithRunID(t *testing.T) {
	const want = "lyx shed goto some-slug --to"

	shed, statusPath, statusLockPath := scriptedStuckShed(t, 1, []bool{false})
	shed.RunID = "some-slug"
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("first Step = _, %v; want nil", err)
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("second Step = _, %v; want nil", err)
	}
	if got := readStatus(t, statusPath, statusLockPath); !strings.Contains(got.Error, want) {
		t.Errorf("exhausted-budget reason = %q; want it to contain %q", got.Error, want)
	}

	shed, _, _ = gotoShed(t)
	shed.RunID = "some-slug"
	seed := commonSeed("Renamed")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	_, err := shed.Step(context.Background())
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("missing-producer error = %v; want it to contain %q", err, want)
	}
}

// TestStep_ToldMissingStatusWayForward pins that a told clause replaces the generic seed advice.
func TestStep_ToldMissingStatusWayForward(t *testing.T) {
	shed, _, _, _ := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, "")}}
	shed.MissingStatusWayForward = "way forward: told clause"

	_, err := shed.Step(context.Background())
	if err == nil {
		t.Fatal("Step(...) with no status file = nil; want a refusal")
	}
	if !strings.Contains(err.Error(), "way forward: told clause") || strings.Contains(err.Error(), "lyx shed seed") {
		t.Errorf("Step(...) error = %q; want the told clause and not lyx shed seed", err.Error())
	}
}
