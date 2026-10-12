// escalation_test.go drives a real Discussion-Review segment, and a darn run's Darn-Review perch, through a spent review budget:
// the Bouncer escalates to the parent instead of blocking, a budget continue grants exactly one more round, and an accept settles the segment.

package loomrecipe

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// budgetEscalationBudget is the small review budget the escalation tests run with.
const budgetEscalationBudget = 2

// newEscalationShed builds a Shed over the sequence fixture with a small review budget, a judge that rules CONTINUE every round,
// and the SegmentBounces seam wired over the fixture's status file through Routing, as loomcli wires it.
// It returns the Shed, the fixture env and the Discussion-Review run directory.
func newEscalationShed(t *testing.T) (*shedengine.Shed, string, *shedfake.BurlerHalfRunner, func() shedengine.Status) {
	t.Helper()

	_, env, paths := buildSequenceFixture(t)
	blockBouncerJudge(env)
	env.ReviewMaxBounces = budgetEscalationBudget
	env.SegmentBounces = func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath)
		if err != nil || !found {
			return 0, 0, false, err
		}
		routing, err := Routing(budgetEscalationBudget, nil, false)
		if err != nil {
			return 0, 0, false, err
		}
		count, budget, inSegment := routing.Bounces(row, st.History)
		return count, budget, inSegment, nil
	}

	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

	readStatus := func() shedengine.Status {
		st, found, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath)
		if err != nil || !found {
			t.Fatalf("read status file: found=%v err=%v", found, err)
		}
		return st
	}
	return shed, filepath.Join(env.RunRoot, "discussion"), env.Burler.(*shedfake.BurlerHalfRunner), readStatus
}

// haltingProducer is a fake row that counts its calls and returns Stuck with no on_stuck to bounce to, so a run halts on reaching it.
type haltingProducer struct{ calls int }

func (p *haltingProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.calls++
	return shedengine.Stuck, shedengine.OutputPointer{Reason: "halting fake"}, nil
}

// TestEscalation_DarnPerchBudgetSpentEscalatesThenAcceptReachesDescribe drives a darn run's Darn-Review perch past darn.yaml's default budget of 1, with no decision record on disk:
// the Bouncer escalates to the parent with a rendered brief instead of blocking, writes no decision record, and an accept settles the perch onto Describe.
func TestEscalation_DarnPerchBudgetSpentEscalatesThenAcceptReachesDescribe(t *testing.T) {
	env, paths := testEnv(t)
	// A round's review path must sit under the anchor.
	env.RunRoot = filepath.Join(env.AnchorPath, "reviews")
	if err := os.MkdirAll(env.RunRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := loomshed.Seed(env.StatusPath, env.StatusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}
	blockBouncerJudge(env)
	env.SegmentBounces = func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath)
		if err != nil || !found {
			return 0, 0, false, err
		}
		routing, err := DarnRouting(env.DarnReviewMaxBounces)
		if err != nil {
			return 0, 0, false, err
		}
		count, budget, inSegment := routing.Bounces(row, st.History)
		return count, budget, inSegment, nil
	}

	shed, err := NewDarn(env, paths)
	if err != nil {
		t.Fatalf("NewDarn() error = %v; want nil", err)
	}
	describe := &haltingProducer{}
	for i := range shed.Producers {
		switch shed.Producers[i].Name {
		case loomshed.NamePreflight, loomshed.NameDarnWrite:
			shed.Producers[i].Producer = fakeAlwaysDoneProducer{}
		case loomshed.NameDescribe:
			shed.Producers[i].Producer = describe
		}
	}
	runDir := filepath.Join(env.RunRoot, "darn")

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunAwaiting || result.HaltedProducer != loomshed.NameDarnBouncer {
		t.Fatalf("Run() = %q at %q (reason %q); want %q at %q", result.Outcome, result.HaltedProducer, result.Reason, shedengine.RunAwaiting, loomshed.NameDarnBouncer)
	}
	if !strings.Contains(result.Reason, "(cause: budget)") || !strings.Contains(result.Reason, "brief at ") {
		t.Errorf("Run() reason = %q; want the budget cause and the rendered brief's path", result.Reason)
	}
	if st, _, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath); err != nil || st.ParentNotice == "" {
		t.Errorf("status ParentNotice = %q, %v; want the Bouncer's one-line notice", st.ParentNotice, err)
	}
	if _, err := os.Stat(env.DecisionRecordPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(decision record) = %v; want not-exist, since a darn run has none", err)
	}

	if _, cause, err := shedadapters.RecordCirclingDecision(runDir, shedadapters.CirclingAccept); err != nil || cause != shedadapters.EscalationBudget {
		t.Fatalf("RecordCirclingDecision(accept) = _, %q, %v; want %q, nil", cause, err, shedadapters.EscalationBudget)
	}
	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() after accept error = %v; want nil", err)
	}
	if result.HaltedProducer != loomshed.NameDescribe || describe.calls != 1 {
		t.Errorf("Run() after accept halted at %q with Describe called %d times; want it to settle the perch and run Describe once", result.HaltedProducer, describe.calls)
	}
	if _, err := os.Stat(env.DecisionRecordPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(decision record) after accept = %v; want not-exist", err)
	}
}

// countStuck reports how many of history's entries are producer's Stuck entries, and how many of those are budget-exempt.
func countStuck(history []shedengine.HistoryEntry, producer string) (total, exempt int) {
	for _, e := range history {
		if e.Producer == producer && e.Outcome == shedengine.Stuck {
			total++
			if e.BudgetExempt {
				exempt++
			}
		}
	}
	return total, exempt
}

// TestEscalation_BudgetContinueGrantsOneRoundThenAccept asserts the three halts of a spent review budget:
// the budget halts awaiting with a parent notice rather than blocked, a continue runs exactly one more round before the Bouncer escalates again,
// and an accept settles the segment.
func TestEscalation_BudgetContinueGrantsOneRoundThenAccept(t *testing.T) {
	shed, runDir, burler, readStatus := newEscalationShed(t)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunAwaiting {
		t.Fatalf("Run() outcome = %q (reason %q); want %q", result.Outcome, result.Reason, shedengine.RunAwaiting)
	}
	if result.HaltedProducer != loomshed.NameDiscussionBouncer {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NameDiscussionBouncer)
	}
	st := readStatus()
	if st.State != shedengine.StateAwaiting {
		t.Errorf("status State = %q; want %q", st.State, shedengine.StateAwaiting)
	}
	if st.ParentNotice == "" {
		t.Error("status ParentNotice is empty; want the Bouncer's one-line notice")
	}
	roundsBefore := burler.ReviewCalls

	round, cause, err := shedadapters.RecordCirclingDecision(runDir, shedadapters.CirclingContinue)
	if err != nil {
		t.Fatalf("RecordCirclingDecision(continue) error = %v; want nil", err)
	}
	if cause != shedadapters.EscalationBudget {
		t.Fatalf("RecordCirclingDecision(continue) cause = %q; want %q", cause, shedadapters.EscalationBudget)
	}
	if want := roundsBefore; round != want {
		t.Fatalf("RecordCirclingDecision(continue) round = %d; want the latest judged round %d", round, want)
	}

	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() after continue error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunAwaiting {
		t.Fatalf("Run() after continue outcome = %q (reason %q); want %q -- one more round, then the next judge escalates again", result.Outcome, result.Reason, shedengine.RunAwaiting)
	}
	if got := burler.ReviewCalls - roundsBefore; got != 1 {
		t.Errorf("rounds run after the budget continue = %d; want exactly 1", got)
	}
	if burler.FixCalls != burler.ReviewCalls {
		t.Errorf("fix calls = %d, review calls = %d; want one fix per review", burler.FixCalls, burler.ReviewCalls)
	}
	for _, row := range []string{loomshed.NameDiscussionBurlerReview, loomshed.NameDiscussionBurlerFix} {
		if total, exempt := countStuck(result.History, row); total != burler.ReviewCalls || exempt != total {
			t.Errorf("%s Stuck entries = %d, budget-exempt = %d; want both %d (every round's hand-back is exempt)", row, total, exempt, burler.ReviewCalls)
		}
	}
	if _, exempt := countStuck(result.History, loomshed.NameDiscussionBouncer); exempt != 1 {
		t.Errorf("Discussion-Bouncer budget-exempt Stuck entries = %d; want 1 (the continue)", exempt)
	}

	if _, _, err := shedadapters.RecordCirclingDecision(runDir, shedadapters.CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) error = %v; want nil", err)
	}
	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() after accept error = %v; want nil", err)
	}
	settled := false
	for _, e := range result.History {
		if e.Producer == loomshed.NameDiscussionBouncer && e.Outcome == shedengine.Done {
			settled = true
		}
	}
	if !settled {
		t.Errorf("History has no %s Done entry after the accept: %+v", loomshed.NameDiscussionBouncer, result.History)
	}
}

// TestEscalation_PerchBudgetOneWithAutoAccept drives the sequence fixture with Webster-Bouncer's own budget at 1 and its auto-accept flag set, and a Webster judge that rules CONTINUE.
// With the fix half passing, the Webster-Review segment runs one review and one fix and Webster-Bouncer settles Done with an automatic accept on disk, and the run reaches Describe.
// With the fix half's gate failing in round 1 no round is left to judge, so the run halts blocked, with no circling decision and no Webster-Bouncer Done.
func TestEscalation_PerchBudgetOneWithAutoAccept(t *testing.T) {
	tests := []struct {
		name         string
		fixResults   []burlerengine.FixResult
		wantOutcome  shedengine.RunOutcome
		wantAccepted bool
	}{
		{
			name:         "the fix half passes",
			fixResults:   []burlerengine.FixResult{{Outcome: shuttleengine.OutcomeDone}},
			wantOutcome:  shedengine.RunBlocked,
			wantAccepted: true,
		},
		{
			name: "the fix half's gate fails in round 1",
			fixResults: []burlerengine.FixResult{
				{Outcome: shuttleengine.OutcomeDone},
				{Outcome: shuttleengine.OutcomeDone},
				{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: false, Attempts: 1}},
			},
			wantOutcome: shedengine.RunBlocked,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, env, paths := buildSequenceFixture(t)
			rowBudgets := map[string]int{loomshed.NameWebsterBouncer: 1}
			env.RowMaxBounces = rowBudgets
			env.RowAutoAccept = map[string]bool{loomshed.NameWebsterBouncer: true}
			env.Burler.(*shedfake.BurlerHalfRunner).FixResults = tt.fixResults

			shuttle := env.Shuttle.(*shedfake.Shuttle)
			dispatch := shuttle.RunFn
			websterRunDir := string(filepath.Separator) + "webster" + string(filepath.Separator)
			shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
				if spec.Role == "bouncer-judge" && len(spec.OutputFiles) > 0 && strings.Contains(spec.OutputFiles[0], websterRunDir) {
					return writeBouncerJudge(spec, "CONTINUE")
				}
				return dispatch(spec)
			}
			env.SegmentBounces = func(row string) (int, int, bool, error) {
				st, found, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath)
				if err != nil || !found {
					return 0, 0, false, err
				}
				routing, err := Routing(env.ReviewMaxBounces, rowBudgets, false)
				if err != nil {
					return 0, 0, false, err
				}
				count, budget, inSegment := routing.Bounces(row, st.History)
				return count, budget, inSegment, nil
			}

			shed, err := New(env, paths)
			if err != nil {
				t.Fatalf("New() error = %v; want nil", err)
			}
			shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

			result, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Fatalf("Run() outcome = %q at %q (reason %q); want %q", result.Outcome, result.HaltedProducer, result.Reason, tt.wantOutcome)
			}

			bouncerDone, reviews, fixes, describeDone := 0, 0, 0, 0
			for _, e := range result.History {
				switch {
				case e.Producer == loomshed.NameWebsterBouncer && e.Outcome == shedengine.Done:
					bouncerDone++
				case e.Producer == loomshed.NameWebsterBurlerReview:
					reviews++
				case e.Producer == loomshed.NameWebsterBurlerFix:
					fixes++
				case e.Producer == loomshed.NameDescribe && e.Outcome == shedengine.Done:
					describeDone++
				}
			}
			decision, err := os.ReadFile(filepath.Join(env.RunRoot, "webster", "round-1-circling-decision.md"))

			if tt.wantAccepted {
				if reviews != 1 || fixes != 1 {
					t.Errorf("Webster-Review review/fix calls = %d/%d; want exactly 1/1", reviews, fixes)
				}
				if bouncerDone != 1 || describeDone != 1 {
					t.Errorf("Webster-Bouncer Done = %d, Describe Done = %d; want 1 and 1, the accept reaching Describe", bouncerDone, describeDone)
				}
				if err != nil || !strings.Contains(string(decision), "automatic: true") || !strings.Contains(string(decision), "settled: true") {
					t.Errorf("round-1 decision = %q, %v; want a settled automatic accept", decision, err)
				}
				return
			}
			if bouncerDone != 0 || describeDone != 0 {
				t.Errorf("Webster-Bouncer Done = %d, Describe Done = %d; want neither", bouncerDone, describeDone)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("round-1 decision = %q, %v; want none recorded", decision, err)
			}
		})
	}
}
