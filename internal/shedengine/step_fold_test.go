// step_fold_test.go covers the fold of consecutive identical budget-exempt Stucks through a real Shed.Step: the fold lands inside the step's single persist, recomposes activity.last, and is ended by any interleaved entry.

package shedengine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

const foldedBounceWording = "Wait → bounced to Wait"

// mustStep runs one Step and fails the test on an error.
func mustStep(t *testing.T, shed *Shed) StepResult {
	t.Helper()
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step = _, %v; want nil", err)
	}
	return res
}

func TestStep_ExemptStuckFoldsIntoOneEntry(t *testing.T) {
	shed, statusPath, statusLockPath := scriptedStuckShed(t, 10, []bool{true})

	first := mustStep(t, shed)
	firstAt := readStatus(t, statusPath, statusLockPath).History[0].At
	var res StepResult
	const further = 3
	for i := 0; i < further; i++ {
		res = mustStep(t, shed)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 1 {
		t.Fatalf("persisted history has %d entries; want 1: %+v", len(got.History), got.History)
	}
	entry := got.History[0]
	if entry.Repeats != further {
		t.Errorf("Repeats = %d; want %d", entry.Repeats, further)
	}
	if entry.At != firstAt {
		t.Errorf("At = %q; want the first step's %q", entry.At, firstAt)
	}
	assertRFC3339UTC(t, entry.LastAt)
	at, _ := time.Parse(time.RFC3339, entry.At)
	lastAt, _ := time.Parse(time.RFC3339, entry.LastAt)
	if lastAt.Before(at) {
		t.Errorf("LastAt %q is earlier than At %q", entry.LastAt, entry.At)
	}
	if first.History[0].Repeats != 0 {
		t.Errorf("first step's entry Repeats = %d; want 0", first.History[0].Repeats)
	}
	if fmt.Sprint(res.History) != fmt.Sprint(got.History) {
		t.Errorf("StepResult.History = %+v; want persisted %+v", res.History, got.History)
	}
}

func TestStep_FoldRecomposesActivityLast(t *testing.T) {
	shed, statusPath, statusLockPath := scriptedStuckShed(t, 10, []bool{true})
	mustStep(t, shed)

	err := state.UpdateJSON(statusPath, statusLockPath, func(cur map[string]any, found bool) (map[string]any, error) {
		cur["activity"] = map[string]any{"now": "x", "last": "sentinel", "wait": ""}
		return cur, nil
	})
	if err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	mustStep(t, shed)

	if last := readStatus(t, statusPath, statusLockPath).Activity.Last; last != foldedBounceWording {
		t.Errorf("activity.last = %q; want %q", last, foldedBounceWording)
	}
}

func TestStep_FoldingStepCommitsOnce(t *testing.T) {
	shed, _, _ := scriptedStuckShed(t, 10, []bool{true})
	mustStep(t, shed)
	var calls []commitStatusCall
	shed.CommitStatus = recordingCommitStatus(&calls)

	mustStep(t, shed)

	if len(calls) != 1 {
		t.Errorf("CommitStatus called %d times on a folding step; want 1", len(calls))
	}
}

func TestStep_NoVerdictWriteCarriesActivityLastAfterFold(t *testing.T) {
	shed, statusPath, statusLockPath := scriptedStuckShed(t, 10, []bool{true})
	mustStep(t, shed)
	mustStep(t, shed)

	setPauseRequested(t, statusPath, statusLockPath)
	res := mustStep(t, shed)
	if res.State != StatePaused {
		t.Fatalf("State = %q; want paused", res.State)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 1 || got.History[0].Repeats != 1 {
		t.Errorf("history after pause = %+v; want one entry with Repeats 1", got.History)
	}
	if got.Activity.Last != foldedBounceWording {
		t.Errorf("activity.last after pause = %q; want %q", got.Activity.Last, foldedBounceWording)
	}

	mustStep(t, shed)
	got = readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 1 || got.History[0].Repeats != 2 {
		t.Errorf("history after resume = %+v; want one entry with Repeats 2", got.History)
	}
	if got.Activity.Last != foldedBounceWording {
		t.Errorf("activity.last after resume = %q; want %q", got.Activity.Last, foldedBounceWording)
	}
}

func TestStep_CountedAndExemptStucksDoNotFold(t *testing.T) {
	cases := []struct {
		name   string
		exempt []bool
	}{
		{"exempt then counted", []bool{true, false}},
		{"counted then exempt", []bool{false, true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shed, statusPath, statusLockPath := scriptedStuckShed(t, 10, tc.exempt)
			mustStep(t, shed)
			mustStep(t, shed)

			got := readStatus(t, statusPath, statusLockPath)
			if len(got.History) != 2 {
				t.Errorf("history has %d entries; want 2: %+v", len(got.History), got.History)
			}
		})
	}
}

func TestStep_AwaitingEndsAFold(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	script := []Outcome{Stuck, Stuck, Awaiting, Stuck}
	i := 0
	shed.Producers = []ProducerDef{{
		Name:       "Wait",
		OnStuck:    "Wait",
		MaxBounces: 10,
		Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			o := script[i]
			i++
			return o, OutputPointer{BudgetExempt: o == Stuck}, nil
		}},
	}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Wait"))

	for range script {
		mustStep(t, shed)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if len(got.History) != 3 {
		t.Fatalf("history has %d entries; want 3: %+v", len(got.History), got.History)
	}
	if got.History[0].Repeats != 1 || got.History[0].Outcome != Stuck {
		t.Errorf("entry 0 = %+v; want a Stuck with Repeats 1", got.History[0])
	}
	if got.History[1].Outcome != Awaiting {
		t.Errorf("entry 1 outcome = %q; want awaiting", got.History[1].Outcome)
	}
	last := got.History[2]
	if last.Outcome != Stuck || last.Repeats != 0 || last.LastAt != "" {
		t.Errorf("entry 2 = %+v; want a fresh Stuck with no Repeats/LastAt", last)
	}
}
