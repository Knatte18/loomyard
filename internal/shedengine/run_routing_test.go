// run_routing_test.go covers Run's outcome-routing scenarios: the happy path, the completion
// terminal values, the unconditional re-call guarantee, Stuck routing and the per-producer,
// episode-scoped bounce budget, and producer errors and unrecognised outcomes. It drives Run end
// to end against a real status file for every branch the loop's routing switch takes.

package shedengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRun_HappyPath(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	p1 := fixedOutcomeProducer(Done, "")
	p2 := fixedOutcomeProducer(Done, "")
	p3 := fixedOutcomeProducer(Done, "")
	wantNames := []string{"Preflight", "Plan-Write", "Finalize"}
	shed.Producers = linearChain(t, wantNames, []ShedProducer{p1, p2, p3})
	seedStatus(t, statusPath, statusLockPath, commonSeed("Preflight"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, StateDone)
	}

	if len(got.History) != len(wantNames) {
		t.Fatalf("len(History) = %d; want %d", len(got.History), len(wantNames))
	}
	for i, name := range wantNames {
		entry := got.History[i]
		if entry.Producer != name {
			t.Errorf("History[%d].Producer = %q; want %q", i, entry.Producer, name)
		}
		if entry.Outcome != Done {
			t.Errorf("History[%d].Outcome = %q; want %q", i, entry.Outcome, Done)
		}
		assertRFC3339UTC(t, entry.At)
	}
	assertHistoryNonDecreasing(t, got.History)

	for i, p := range []*funcProducer{p1, p2, p3} {
		if p.calls != 1 {
			t.Errorf("producer %d calls = %d; want 1", i, p.calls)
		}
	}
}

func TestRun_CompletionTerminalValues(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = linearChain(t, []string{"Preflight", "Plan-Write", "Finalize"}, []ShedProducer{
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
		fixedOutcomeProducer(Done, ""),
	})
	seedStatus(t, statusPath, statusLockPath, commonSeed("Preflight"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	got := readStatus(t, statusPath, statusLockPath)
	const lastName = "Finalize"
	if got.CurrentProducer != lastName {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, lastName)
	}
	if got.Activity.Now != lastName {
		t.Errorf("persisted Activity.Now = %q; want %q", got.Activity.Now, lastName)
	}
	if result.HaltedProducer != lastName {
		t.Errorf("Result.HaltedProducer = %q; want %q", result.HaltedProducer, lastName)
	}

	if !reflect.DeepEqual(result.History, got.History) {
		t.Errorf("Result.History = %+v; want (persisted) %+v", result.History, got.History)
	}
}

func TestRun_UnconditionalRecall(t *testing.T) {
	// Shed cannot tell a stale output file from a fresh one by existence alone, notably after a
	// bounce-back, so it never skips a call because OutputPointer.Path already exists on disk --
	// that three-case respawn discipline is delegated whole to each engine adapter instead.
	shed, statusPath, _, statusLockPath := newTestShed(t)

	artifactPath := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(artifactPath, []byte("stale from a prior attempt"), 0o644); err != nil {
		t.Fatalf("seed artifact file: %v", err)
	}

	producer := fixedOutcomeProducer(Done, artifactPath)
	shed.Producers = []ProducerDef{{Name: "Plan-Write", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Plan-Write"))

	if _, err := shed.Run(context.Background()); err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	if producer.calls != 1 {
		t.Errorf("producer.calls = %d; want 1 -- Run must never skip a call because its output already exists", producer.calls)
	}
}

func TestRun_StuckWithOnStuckTarget(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := fixedOutcomeProducer(Done, "")
	b := &funcProducer{}
	b.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		if b.calls == 1 {
			return Stuck, OutputPointer{}, nil
		}
		return Done, OutputPointer{}, nil
	}
	// A's OnDone names B, so control still reaches B, and B's OnDone stays empty, so its
	// second call finishes the run.
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: "B"},
		{Name: "B", Producer: b, OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	if a.calls != 2 {
		t.Errorf("a.calls = %d; want 2 -- the bounce-back target must run again", a.calls)
	}
	if b.calls != 2 {
		t.Errorf("b.calls = %d; want 2", b.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	foundBounce := false
	for _, entry := range got.History {
		if entry.Producer == "B" && entry.Outcome == Stuck {
			foundBounce = true
		}
	}
	if !foundBounce {
		t.Errorf("persisted History = %+v; want an entry recording B's stuck outcome", got.History)
	}
}

func TestRun_StuckWithNoTarget(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	producer := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{{Name: "Plan-Write", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Plan-Write"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	if result.HaltedProducer != "Plan-Write" {
		t.Errorf("Result.HaltedProducer = %q; want %q", result.HaltedProducer, "Plan-Write")
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateBlocked {
		t.Errorf("persisted State = %q; want %q", got.State, StateBlocked)
	}

	const wantReason = "stuck with no OnStuck target"
	if result.Reason != wantReason {
		t.Errorf("Result.Reason = %q; want %q", result.Reason, wantReason)
	}
	if got.Error != wantReason {
		t.Errorf("persisted Error = %q; want %q", got.Error, wantReason)
	}
	if result.Reason != got.Error {
		t.Errorf("Result.Reason (%q) and persisted Error (%q) must be equal", result.Reason, got.Error)
	}
}

func TestRun_BounceBudgetExhaustion(t *testing.T) {
	// The boundary this test pins is now one producer's own episode budget, not a run-wide
	// total: a single self-bouncing producer keeps the assertion pinning the boundary
	// directly. A two-producer A<->B cycle would still terminate, but its aggregate call
	// count would be 2*budget+1 -- a deliberate design consequence of the per-producer
	// budget, not the property this test exists to guard.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 3

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	// MaxBounces bounces are permitted -- the (MaxBounces+1)-th Stuck is the one refused. The
	// exact call count is the whole point of the assertion, because this is the classic
	// off-by-one seam.
	wantCalls := shed.MaxBounces + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d (MaxBounces=%d bounces, then the next Stuck blocks)", a.calls, wantCalls, shed.MaxBounces)
	}

	got := readStatus(t, statusPath, statusLockPath)
	const wantReason = "bounce budget exhausted"
	if result.Reason != wantReason {
		t.Errorf("Result.Reason = %q; want %q", result.Reason, wantReason)
	}
	if got.Error != wantReason {
		t.Errorf("persisted Error = %q; want %q", got.Error, wantReason)
	}
}

func TestRun_MaxBouncesZeroResolvesToDefault(t *testing.T) {
	// Same single self-bouncing producer as TestRun_BounceBudgetExhaustion, pinning the
	// default-resolution boundary directly rather than a two-producer aggregate.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 0 // zero means "use the default", never "no bounces allowed".

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	wantCalls := defaultMaxBounces + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d (defaultMaxBounces=%d bounces, then the next Stuck blocks)", a.calls, wantCalls, defaultMaxBounces)
	}
}

func TestRun_ProducerError(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	wantMsg := "plan-write: disk full"
	p1 := fixedOutcomeProducer(Done, "")
	p2 := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, errors.New(wantMsg)
	}}
	p3 := fixedOutcomeProducer(Done, "")
	shed.Producers = linearChain(t, []string{"Preflight", "Plan-Write", "Finalize"}, []ShedProducer{p1, p2, p3})
	seedStatus(t, statusPath, statusLockPath, commonSeed("Preflight"))

	// Explicitly confirmed healthy and never cancelled -- this is what the cancellation-aware
	// branch in step 6's error case must key on, so a genuine producer error is never swallowed
	// as a pause.
	ctx := context.Background()
	if ctx.Err() != nil {
		t.Fatalf("context.Background().Err() = %v; want nil (context must be healthy for this scenario)", ctx.Err())
	}

	result, err := shed.Run(ctx)
	if err == nil {
		t.Fatalf("Run(...) = %+v, nil; want a non-nil error", result)
	}
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("Run(...) error = %q; want it to contain %q", err.Error(), wantMsg)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateFailed {
		t.Errorf("persisted State = %q; want %q", got.State, StateFailed)
	}
	if !strings.Contains(got.Error, wantMsg) {
		t.Errorf("persisted Error = %q; want it to contain %q", got.Error, wantMsg)
	}
	if p3.calls != 0 {
		t.Errorf("p3.calls = %d; want 0 -- no further producer is called after an engine-level failure", p3.calls)
	}

	foundFailure := false
	for _, entry := range got.History {
		if entry.Producer == "Plan-Write" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("persisted History = %+v; want an entry recording the failing call", got.History)
	}
}

func TestRun_UnrecognisedOutcome(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
	}{
		{"plausible-looking wrong value", Outcome("approved")},
		{"empty string", Outcome("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)

			p1 := &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
				return tt.outcome, OutputPointer{}, nil
			}}
			p2 := fixedOutcomeProducer(Done, "")
			shed.Producers = linearChain(t, []string{"Plan-Write", "Finalize"}, []ShedProducer{p1, p2})
			seedStatus(t, statusPath, statusLockPath, commonSeed("Plan-Write"))

			result, err := shed.Run(context.Background())
			if err == nil {
				t.Fatalf("Run(...) = %+v, nil; want a non-nil error", result)
			}
			wantQuoted := fmt.Sprintf("%q", tt.outcome)
			if !strings.Contains(err.Error(), wantQuoted) {
				t.Errorf("Run(...) error = %q; want it to name the offending value %s", err.Error(), wantQuoted)
			}
			if !strings.Contains(err.Error(), "Plan-Write") {
				t.Errorf("Run(...) error = %q; want it to name the producer %q", err.Error(), "Plan-Write")
			}

			got := readStatus(t, statusPath, statusLockPath)
			if got.State != StateFailed {
				t.Errorf("persisted State = %q; want %q", got.State, StateFailed)
			}
			if p2.calls != 0 {
				t.Errorf("p2.calls = %d; want 0 -- no further producer is called after an unrecognised outcome", p2.calls)
			}

			if len(got.History) == 0 {
				t.Fatalf("persisted History is empty; want an entry recording the literal offending value")
			}
			last := got.History[len(got.History)-1]
			if last.Producer != "Plan-Write" || last.Outcome != tt.outcome {
				t.Errorf("last History entry = %+v; want Producer %q, Outcome %q (the literal value received)", last, "Plan-Write", tt.outcome)
			}
		})
	}
}

func TestRun_EmptyOnDoneFinishesFromNonLastPosition(t *testing.T) {
	// A three-row list whose first row has an empty OnDone finishes the whole run on that
	// row's Done -- the producer list carries no positional meaning once Done routes by
	// OnDone.
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := fixedOutcomeProducer(Done, "")
	b := fixedOutcomeProducer(Done, "")
	c := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: ""},
		{Name: "B", Producer: b, OnDone: "C"},
		{Name: "C", Producer: c, OnDone: ""},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}
	if result.HaltedProducer != "A" {
		t.Errorf("Result.HaltedProducer = %q; want %q", result.HaltedProducer, "A")
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, StateDone)
	}
	if got.CurrentProducer != "A" {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, "A")
	}

	if b.calls != 0 {
		t.Errorf("b.calls = %d; want 0 -- B is never reached", b.calls)
	}
	if c.calls != 0 {
		t.Errorf("c.calls = %d; want 0 -- C is never reached", c.calls)
	}
}

func TestRun_OnDoneSkipsForward(t *testing.T) {
	// A three-row list whose first row's OnDone names the third row skips the middle row
	// entirely.
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := fixedOutcomeProducer(Done, "")
	b := fixedOutcomeProducer(Done, "")
	c := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: "C"},
		{Name: "B", Producer: b, OnDone: ""},
		{Name: "C", Producer: c, OnDone: ""},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	if b.calls != 0 {
		t.Errorf("b.calls = %d; want 0 -- B is skipped entirely", b.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	wantNames := []string{"A", "C"}
	if len(got.History) != len(wantNames) {
		t.Fatalf("len(History) = %d; want %d (%v)", len(got.History), len(wantNames), wantNames)
	}
	for i, name := range wantNames {
		if got.History[i].Producer != name {
			t.Errorf("History[%d].Producer = %q; want %q", i, got.History[i].Producer, name)
		}
	}
}

func TestRun_OnDoneRoutesBackward(t *testing.T) {
	// A later row's OnDone names an earlier row, and the run continues from there. A is the
	// backward re-entry target: counter-driven, it reports Done on its first call so the
	// chain advances to B, then Stuck (with no OnStuck target) on its second call -- the call
	// B's backward OnDone routes back to -- so the run terminates by blocking rather than
	// looping forever.
	shed, statusPath, _, statusLockPath := newTestShed(t)

	a := &funcProducer{}
	a.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		if a.calls == 1 {
			return Done, OutputPointer{}, nil
		}
		return Stuck, OutputPointer{}, nil
	}
	b := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnDone: "B"},
		{Name: "B", Producer: b, OnDone: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	if a.calls != 2 {
		t.Errorf("a.calls = %d; want 2 -- the backward re-entry must run A again", a.calls)
	}

	got := readStatus(t, statusPath, statusLockPath)
	wantNames := []string{"A", "B", "A"}
	if len(got.History) != len(wantNames) {
		t.Fatalf("len(History) = %d; want %d (%v)", len(got.History), len(wantNames), wantNames)
	}
	for i, name := range wantNames {
		if got.History[i].Producer != name {
			t.Errorf("History[%d].Producer = %q; want %q", i, got.History[i].Producer, name)
		}
	}
	if got.History[0].Outcome != Done || got.History[1].Outcome != Done || got.History[2].Outcome != Stuck {
		t.Errorf("History outcomes = %+v; want [done done stuck]", got.History)
	}
}
