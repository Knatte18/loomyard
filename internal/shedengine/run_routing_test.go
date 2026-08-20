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
	shed.MaxBounces = 1 // deliberately tiny, so a budget-exhausted reason would already fire here.

	producer := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{{Name: "Plan-Write", Producer: producer}}
	// Pre-seed history with a Stuck count already at (and past) any conceivable budget, so
	// this test pins the ordering: an empty OnStuck blocks with its own reason ahead of the
	// budget check, never falling through to "bounce budget exhausted" just because the
	// history already looks exhausted.
	seed := commonSeed("Plan-Write")
	seed.History = []HistoryEntry{
		{Producer: "Plan-Write", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "Plan-Write", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

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

func TestRun_EpisodeBudgetIndependenceAcrossProducers(t *testing.T) {
	// The property the whole task exists for: one producer's own history does not shrink a
	// different producer's remaining budget. A already has a history of Stuck entries far
	// past any plausible budget; B, seeded to run now, must still get its own full budget.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 2

	b := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Stuck, ""), OnStuck: "A"},
		{Name: "B", Producer: b, OnStuck: "B"},
	}
	seed := commonSeed("B")
	seed.History = []HistoryEntry{
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:02Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:03Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:04Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	wantCalls := shed.MaxBounces + 1
	if b.calls != wantCalls {
		t.Errorf("b.calls = %d; want %d -- B's own budget, unaffected by A's already-exhausted history", b.calls, wantCalls)
	}
}

func TestRun_MaxBouncesInheritance(t *testing.T) {
	tests := []struct {
		name           string
		defMaxBounces  int
		shedMaxBounces int
		wantEffective  int
	}{
		{"def zero inherits Shed.MaxBounces", 0, 5, 5},
		{"both zero inherit defaultMaxBounces", 0, 0, defaultMaxBounces},
		{"non-zero def overrides a different non-zero Shed.MaxBounces", 3, 5, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.MaxBounces = tt.shedMaxBounces

			a := fixedOutcomeProducer(Stuck, "")
			shed.Producers = []ProducerDef{
				{Name: "A", Producer: a, OnStuck: "A", MaxBounces: tt.defMaxBounces},
			}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			result, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run(...) = _, %v; want nil error", err)
			}
			if result.Outcome != RunBlocked {
				t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
			}

			wantCalls := tt.wantEffective + 1
			if a.calls != wantCalls {
				t.Errorf("a.calls = %d; want %d (effective MaxBounces = %d)", a.calls, wantCalls, tt.wantEffective)
			}
		})
	}
}

func TestRun_BudgetDerivedFromPreExistingHistoryAcrossInvocations(t *testing.T) {
	// This is the direct test of the persisted-count decision: the budget accounts for Stuck
	// entries that already existed in history before this Run call, not only ones this
	// invocation appends -- the property that fails under a per-Run-call count.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 3

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seed := commonSeed("A")
	seed.History = []HistoryEntry{
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	// Two Stuck entries already exist; only shed.MaxBounces-2 = 1 further bounce is
	// available before the next Stuck blocks, so this invocation calls A twice: once that
	// bounces, once that blocks.
	wantCalls := shed.MaxBounces - len(seed.History) + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d", a.calls, wantCalls)
	}
}

func TestRun_EpisodeResetsOnProducersOwnDone(t *testing.T) {
	// A producer that bounces, later returns Done, and is then re-entered and bounces again
	// starts from zero on re-entry -- Stuck entries preceding its own last Done do not count.
	// This is the loom Discussion-Validate shape that decided episode scoping over
	// all-time counting, so it gets its own named test rather than a table row.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 2

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seed := commonSeed("A")
	// A's earlier episode: two Stuck entries, then a Done that closed it out. Re-entry into
	// A (e.g. via some other producer's OnDone naming A again) starts a fresh episode.
	seed.History = []HistoryEntry{
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
		{Producer: "A", Outcome: Done, At: "2020-01-01T00:00:02Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	// The pre-Done Stuck entries do not count: this run's episode starts at zero, so
	// MaxBounces+1 fresh calls are made before it blocks -- the same boundary as an
	// entirely empty history, not one narrowed by the earlier episode.
	wantCalls := shed.MaxBounces + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d -- the pre-Done Stuck entries must not count", a.calls, wantCalls)
	}
}

func TestRun_EpisodeIgnoresOtherProducersDoneEntry(t *testing.T) {
	// A Done entry authored by a different producer does not end this producer's episode.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 2

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
		{Name: "B", Producer: fixedOutcomeProducer(Done, ""), OnDone: ""},
	}
	seed := commonSeed("A")
	seed.History = []HistoryEntry{
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "B", Outcome: Done, At: "2020-01-01T00:00:01Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:02Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	// Two A-authored Stuck entries already exist in history, with B's Done sitting between
	// them without resetting anything -- if it had reset A's count, this first call would
	// bounce instead of blocking immediately.
	if a.calls != 1 {
		t.Errorf("a.calls = %d; want 1 -- B's Done must not reset A's episode count", a.calls)
	}
}

func TestRun_NeverPassingGateAccumulatesAllTime(t *testing.T) {
	// A producer with no Done entry anywhere in history accumulates all-time -- the
	// anti-crash-loop property episode scoping must not weaken.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 3

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seed := commonSeed("A")
	seed.History = []HistoryEntry{
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:02Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:03Z"},
		{Producer: "A", Outcome: Stuck, At: "2020-01-01T00:00:04Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	// Five pre-existing Stuck entries already exceed a budget of three, so the very first
	// call this run blocks immediately.
	if a.calls != 1 {
		t.Errorf("a.calls = %d; want 1 -- an all-time-exhausted history blocks on the first call", a.calls)
	}
}

func TestRun_FailurePathDoneEntryTerminatesEpisode(t *testing.T) {
	// A seeded history containing a "done" entry for a producer that had returned an error
	// alongside that verdict still ends that producer's episode: this pins the accepted
	// behavior so a future reader does not rewrite the scan to ignore it.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 1

	firstCall := true
	a := &funcProducer{}
	a.fn = func(ctx context.Context) (Outcome, OutputPointer, error) {
		if firstCall {
			firstCall = false
			// A hard-failure call: a producer returning Done alongside a non-nil error is
			// still recorded with Outcome: Done, because the engine records the verdict
			// the producer actually returned.
			return Done, OutputPointer{}, errors.New("boom")
		}
		return Stuck, OutputPointer{}, nil
	}
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	// First Run: the hard-failure arm persists {Producer: A, Outcome: Done} alongside
	// StateFailed, and returns the error to the caller.
	if _, err := shed.Run(context.Background()); err == nil {
		t.Fatalf("first Run(...) = _, nil; want a non-nil error")
	}
	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateFailed {
		t.Fatalf("persisted State after first Run = %q; want %q", got.State, StateFailed)
	}
	if len(got.History) != 1 || got.History[0].Outcome != Done {
		t.Fatalf("persisted History after first Run = %+v; want one Done entry for A", got.History)
	}

	// Second Run: a human has resolved the failure and re-invokes. The prior Done entry --
	// even though it was written by the hard-failure arm -- terminates the scan exactly as
	// any other Done would, so this episode starts fresh at zero.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("second Run Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	// a.calls is 1 (the failing call) + MaxBounces+1 (a fresh episode budget of 1: one
	// bounce, then the next Stuck blocks).
	wantCalls := 1 + shed.MaxBounces + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d -- the episode must reset at the hard-failure Done entry", a.calls, wantCalls)
	}
}

func TestRun_StuckAttributionIgnoresOtherProducersEntries(t *testing.T) {
	// Stuck entries authored by a different producer do not consume this producer's
	// budget, even when that other producer's OnStuck targets it.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 1

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
		{Name: "D", Producer: fixedOutcomeProducer(Stuck, ""), OnStuck: "A"},
	}
	seed := commonSeed("A")
	seed.History = []HistoryEntry{
		{Producer: "D", Outcome: Stuck, At: "2020-01-01T00:00:00Z"},
		{Producer: "D", Outcome: Stuck, At: "2020-01-01T00:00:01Z"},
		{Producer: "D", Outcome: Stuck, At: "2020-01-01T00:00:02Z"},
	}
	seedStatus(t, statusPath, statusLockPath, seed)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}

	// D's three prior Stuck entries, though D's OnStuck names A, must not count toward A's
	// budget: A gets its own full MaxBounces+1 calls before blocking.
	wantCalls := shed.MaxBounces + 1
	if a.calls != wantCalls {
		t.Errorf("a.calls = %d; want %d -- D's Stuck entries must not count toward A's budget", a.calls, wantCalls)
	}
}

func TestRun_BlockPathArithmetic(t *testing.T) {
	// After a budget-exhausted block, the producer's episode Stuck count is budget+1. A
	// resumed run whose budget was raised by exactly one blocks again immediately. A
	// resumed run whose budget was raised above the current count proceeds. This pins the
	// escape-hatch arithmetic an operator's bug report would otherwise write.
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.MaxBounces = 2

	a := fixedOutcomeProducer(Stuck, "")
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	// Part 1: run to exhaustion. budget+1 calls, then blocked.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 1 (...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Run 1 Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	if a.calls != shed.MaxBounces+1 {
		t.Fatalf("a.calls after Run 1 = %d; want %d (budget+1)", a.calls, shed.MaxBounces+1)
	}
	callsAfterRun1 := a.calls

	// Part 2: raise the budget by exactly one and resume. The episode count already equals
	// the new budget, so the very next call blocks again immediately -- no bounce.
	shed.MaxBounces++
	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 2 (...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Run 2 Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	if a.calls != callsAfterRun1+1 {
		t.Errorf("a.calls after Run 2 = %d; want %d -- a budget raised by exactly one blocks again immediately, with no bounce", a.calls, callsAfterRun1+1)
	}
	callsAfterRun2 := a.calls

	// Part 3: raise the budget above the current episode count and resume. The run must
	// proceed past the immediate-block point: at least one further bounce happens before
	// (or instead of) another immediate block.
	shed.MaxBounces += 5
	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run 3 (...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunBlocked {
		t.Errorf("Run 3 Result.Outcome = %q; want %q", result.Outcome, RunBlocked)
	}
	if a.calls <= callsAfterRun2+1 {
		t.Errorf("a.calls after Run 3 = %d; want more than %d -- a budget raised above the current count must proceed past an immediate block", a.calls, callsAfterRun2+1)
	}
}
