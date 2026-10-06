// budgetexempt_test.go covers the budget-exempt Stuck verdict: an exempt Stuck routes like any Stuck but is neither counted toward, nor blocked by, its row's bounce budget.

package shedengine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// scriptedStuckShed wires a single self-bouncing row "Wait" with the given budget, whose producer returns Stuck on every call with BudgetExempt taken in order from exempt (the last value repeats).
// It returns the Shed and its status paths.
func scriptedStuckShed(t *testing.T, maxBounces int, exempt []bool) (shed *Shed, statusPath, statusLockPath string) {
	t.Helper()
	shed, statusPath, _, statusLockPath = newTestShed(t)
	i := 0
	shed.Producers = []ProducerDef{{
		Name:       "Wait",
		OnStuck:    "Wait",
		MaxBounces: maxBounces,
		Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			e := exempt[min(i, len(exempt)-1)]
			i++
			return Stuck, OutputPointer{BudgetExempt: e}, nil
		}},
	}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Wait"))
	return shed, statusPath, statusLockPath
}

func TestStep_ExemptStuckRoutesAndPersistsFlag(t *testing.T) {
	shed, statusPath, statusLockPath := scriptedStuckShed(t, 1, []bool{true})

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step = _, %v; want nil", err)
	}
	if res.State != StateRunning || res.Next != "Wait" {
		t.Errorf("Step State/Next = %q/%q; want running/Wait", res.State, res.Next)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 1 || !got.History[0].BudgetExempt {
		t.Fatalf("History = %+v; want one entry with BudgetExempt true", got.History)
	}
}

func TestStep_ExemptStuckInterleavedWithinBudget(t *testing.T) {
	const n = 2
	// Two counted Stucks stay within budget however many exempt ones interleave; the next counted one (the (N+1)th) blocks.
	script := []bool{true, false, true, true, false, true, true, false}
	shed, statusPath, statusLockPath := scriptedStuckShed(t, n, script)

	for i := 0; i < len(script)-1; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d = _, %v; want nil", i, err)
		}
		if res.State != StateRunning {
			t.Fatalf("Step %d State = %q; want running", i, res.State)
		}
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("final Step = _, %v; want nil", err)
	}
	if res.State != StateBlocked || !strings.HasPrefix(res.Reason, ReasonBounceBudgetExhausted) {
		t.Errorf("final Step State/Reason = %q/%q; want blocked/%q", res.State, res.Reason, ReasonBounceBudgetExhausted)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateBlocked {
		t.Errorf("persisted State = %q; want blocked", got.State)
	}
}

func TestStep_ExemptStuckAfterBudgetFullySpent(t *testing.T) {
	const n = 2
	// N counted Stucks (all allowed), then an exempt one that must still route, then a counted one that blocks.
	shed, _, _ := scriptedStuckShed(t, n, []bool{false, false, true, false})

	for i := 0; i < 3; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d = _, %v; want nil", i, err)
		}
		if res.State != StateRunning || res.Next != "Wait" {
			t.Fatalf("Step %d State/Next = %q/%q; want running/Wait", i, res.State, res.Next)
		}
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step 3 = _, %v; want nil", err)
	}
	if res.State != StateBlocked || !strings.HasPrefix(res.Reason, ReasonBounceBudgetExhausted) {
		t.Errorf("Step 3 State/Reason = %q/%q; want blocked/%q", res.State, res.Reason, ReasonBounceBudgetExhausted)
	}
}

func TestStep_ExemptStuckWithoutOnStuckStillBlocks(t *testing.T) {
	shed, _, _, _ := newTestShed(t)
	shed.Producers = []ProducerDef{{
		Name: "Wait",
		Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{BudgetExempt: true, Reason: "waiting"}, nil
		}},
	}}
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, commonSeed("Wait"))
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step = _, %v; want nil", err)
	}
	if res.State != StateBlocked {
		t.Errorf("State = %q; want blocked", res.State)
	}
}

//testtiming:keep pins that an absent budget_exempt decodes as false, counts as a stuck and is omitted on marshal, which its covering test does not
func TestHistoryEntry_MissingBudgetExemptDecodesAndCounts(t *testing.T) {
	var e HistoryEntry
	if err := json.Unmarshal([]byte(`{"producer":"Wait","outcome":"stuck","output":"","at":"2026-01-01T00:00:00Z"}`), &e); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if e.BudgetExempt {
		t.Errorf("BudgetExempt = true; want false when the field is absent")
	}
	if got := episodeStuckCount([]HistoryEntry{e}, ProducerDef{Name: "Wait"}, nil); got != 1 {
		t.Errorf("episodeStuckCount = %d; want 1", got)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "budget_exempt") {
		t.Errorf("marshalled %s carries budget_exempt; want it omitted when false", b)
	}
}

func TestStep_NonStuckOutcomeIgnoresBudgetExempt(t *testing.T) {
	for _, outcome := range []Outcome{Done, Awaiting} {
		shed, statusPath, _, statusLockPath := newTestShed(t)
		shed.Producers = []ProducerDef{{
			Name: "Wait",
			Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
				return outcome, OutputPointer{BudgetExempt: true}, nil
			}},
		}}
		seedStatus(t, statusPath, statusLockPath, commonSeed("Wait"))
		if _, err := shed.Step(context.Background()); err != nil {
			t.Fatalf("%s: Step = _, %v; want nil", outcome, err)
		}
		got := readStatus(t, statusPath, statusLockPath)
		if len(got.History) != 1 || got.History[0].BudgetExempt {
			t.Errorf("%s: History = %+v; want one entry with BudgetExempt false", outcome, got.History)
		}
	}
}

// BenchmarkStep_LongHistory steps a self-bouncing row over a 5000-entry history, measuring the per-step cost of rewriting a long status file on every exempt bounce.
func BenchmarkStep_LongHistory(b *testing.B) {
	shed, statusPath, _, statusLockPath := newTestShed(b)
	shed.Producers = []ProducerDef{{
		Name:    "Wait",
		OnStuck: "Wait",
		Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{BudgetExempt: true}, nil
		}},
	}}
	seed := commonSeed("Wait")
	for i := 0; i < 5000; i++ {
		seed.History = append(seed.History, HistoryEntry{Producer: "Wait", Outcome: Stuck, At: "2026-01-01T00:00:00Z", BudgetExempt: true})
	}
	seedStatus(b, statusPath, statusLockPath, seed)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := shed.Step(context.Background()); err != nil {
			b.Fatalf("Step = _, %v", err)
		}
	}
}

// TestEpisodeStuckCount_SegmentDoneEndsEpisode covers a Burler-round row, which only ever returns Stuck: its segment's Bouncer passing ends its episode, while a standalone row counts on.
//
//testtiming:keep pins that a segment Bouncer's done ends a Burler-round row's episode and segmentEnders of a standalone row is nil, which its covering tests do not
func TestEpisodeStuckCount_SegmentDoneEndsEpisode(t *testing.T) {
	history := []HistoryEntry{
		{Producer: "Bouncer", Outcome: Stuck},
		{Producer: "Burler", Outcome: Stuck},
		{Producer: "Bouncer", Outcome: Done},
		{Producer: "Gate", Outcome: Stuck},
		{Producer: "Bouncer", Outcome: Stuck},
		{Producer: "Burler", Outcome: Stuck},
	}
	producers := []ProducerDef{
		{Name: "Bouncer", Segment: "Review"},
		{Name: "Burler", Segment: "Review"},
		{Name: "Gate"},
	}
	if got := episodeStuckCount(history, producers[1], producers); got != 1 {
		t.Errorf("Burler episode = %d; want 1, the round before the Bouncer passed not counted", got)
	}
	if got := episodeStuckCount(history, ProducerDef{Name: "Burler"}, producers); got != 2 {
		t.Errorf("standalone Burler episode = %d; want 2", got)
	}
	if got := segmentEnders(producers, producers[2]); got != nil {
		t.Errorf("segmentEnders(standalone) = %v; want nil", got)
	}
}
