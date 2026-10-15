// escalation_test.go drives a real Discussion-Review segment, and a darn run's Darn-Review perch, through a spent review budget:
// the soft-cap rule continues a design-wrong round one round at a time, the Bouncer escalates at the hard cap instead of blocking, and an accept settles the segment.

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
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// budgetEscalationBudget is the small review budget the escalation tests run with; they run under a hard cap one above it.
const budgetEscalationBudget = 2

// newEscalationShed builds a Shed over the sequence fixture with a small review budget and a hard cap one above it for Discussion-Bouncer.
// The judge rules CONTINUE over one open design-wrong entry every round.
// The SegmentBounces seam is wired over the fixture's status file through Routing, as loomcli wires it.
// It returns the Shed, the fixture env and the Discussion-Review run directory.
func newEscalationShed(t *testing.T) (*shedengine.Shed, string, *shedfake.BurlerHalfRunner, func() shedengine.Status) {
	t.Helper()

	_, env, paths := buildSequenceFixture(t)
	designWrongBouncerJudge(env)
	env.ReviewMaxBounces = budgetEscalationBudget
	env.RowHardMaxBounces = map[string]int{loomshed.NameDiscussionBouncer: budgetEscalationBudget + 1}
	env.SegmentBounces = func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONNoMkdir[shedengine.Status](env.StatusPath, env.StatusLockPath)
		if err != nil || !found {
			return 0, 0, false, err
		}
		routing, err := Routing(budgetEscalationBudget, nil)
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
		st, found, err := state.ReadJSONNoMkdir[shedengine.Status](env.StatusPath, env.StatusLockPath)
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

// TestEscalation_DarnPerchRuleContinuesThenHardCapAcceptReachesDescribe drives a darn run's Darn-Review perch past darn.yaml's default budget of 1 under a hard cap of 2:
// the soft-cap rule continues round 1 automatically, the Bouncer escalates at the hard cap to the parent with a rendered brief instead of blocking, and an accept settles the perch onto Describe and files the open finding under Darn-Review.
func TestEscalation_DarnPerchRuleContinuesThenHardCapAcceptReachesDescribe(t *testing.T) {
	env, paths := testEnv(t)
	// A round's review path must sit under the anchor.
	env.RunRoot = filepath.Join(env.AnchorPath, "reviews")
	if err := os.MkdirAll(env.RunRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := loomshed.Seed(env.StatusPath, env.StatusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}
	designWrongBouncerJudge(env)
	var carried []discussionparser.CarryOver
	env.CarryOver = func(entry discussionparser.CarryOver) error {
		carried = append(carried, entry)
		return nil
	}
	env.RowHardMaxBounces = map[string]int{loomshed.NameDarnBouncer: 2}
	env.SegmentBounces = func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONNoMkdir[shedengine.Status](env.StatusPath, env.StatusLockPath)
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
	if !strings.Contains(result.Reason, "(cause: hard-cap)") || !strings.Contains(result.Reason, "brief at ") {
		t.Errorf("Run() reason = %q; want the hard-cap cause and the rendered brief's path", result.Reason)
	}
	if st, _, err := state.ReadJSONNoMkdir[shedengine.Status](env.StatusPath, env.StatusLockPath); err != nil || st.ParentNotice == "" {
		t.Errorf("status ParentNotice = %q, %v; want the Bouncer's one-line notice", st.ParentNotice, err)
	}

	if _, cause, err := shedadapters.RecordCirclingDecision(runDir, shedadapters.CirclingAccept); err != nil || cause != shedadapters.EscalationHardCap {
		t.Fatalf("RecordCirclingDecision(accept) = _, %q, %v; want %q, nil", cause, err, shedadapters.EscalationHardCap)
	}
	result, err = shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() after accept error = %v; want nil", err)
	}
	if result.HaltedProducer != loomshed.NameDescribe || describe.calls != 1 {
		t.Errorf("Run() after accept halted at %q with Describe called %d times; want it to settle the perch and run Describe once", result.HaltedProducer, describe.calls)
	}
	if len(carried) != 1 || carried[0].Segment != "Darn-Review" || len(carried[0].Findings) != 1 || carried[0].Findings[0].Key != "design-gap" {
		t.Errorf("carry-over entries = %+v; want one for Darn-Review holding the open design-gap finding", carried)
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

// TestEscalation_RuleContinuesOneRoundThenHardCapThenAccept asserts the halts of a spent review budget under a hard cap one above it:
// the soft-cap rule continues the spent budget with one automatic continue, the next round halts awaiting at the hard cap with a parent notice rather than blocked,
// and an accept settles the segment.
func TestEscalation_RuleContinuesOneRoundThenHardCapThenAccept(t *testing.T) {
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
	if !strings.Contains(result.Reason, "(cause: hard-cap)") {
		t.Errorf("Run() reason = %q; want the hard-cap cause", result.Reason)
	}
	st := readStatus()
	if st.State != shedengine.StateAwaiting {
		t.Errorf("status State = %q; want %q", st.State, shedengine.StateAwaiting)
	}
	if st.ParentNotice == "" {
		t.Error("status ParentNotice is empty; want the Bouncer's one-line notice")
	}
	if want := budgetEscalationBudget + 1; burler.ReviewCalls != want {
		t.Errorf("review rounds run = %d; want %d", burler.ReviewCalls, want)
	}
	if burler.FixCalls != burler.ReviewCalls {
		t.Errorf("fix calls = %d, review calls = %d; want one fix per review", burler.FixCalls, burler.ReviewCalls)
	}
	if total, exempt := countStuck(result.History, loomshed.NameDiscussionBouncer); exempt != 1 || total <= exempt {
		t.Errorf("Discussion-Bouncer Stuck entries = %d, budget-exempt = %d; want exactly 1 exempt (the rule's continue) among the counted ones", total, exempt)
	}
	continued, err := os.ReadFile(filepath.Join(runDir, "round-2-circling-decision.md"))
	if err != nil {
		t.Fatalf("ReadFile(round 2 decision) = %v; want nil", err)
	}
	for _, want := range []string{"decision: continue", "cause: budget", "automatic: true"} {
		if !strings.Contains(string(continued), want) {
			t.Errorf("round 2 decision file = %q; want it to hold %q", continued, want)
		}
	}

	round, cause, err := shedadapters.RecordCirclingDecision(runDir, shedadapters.CirclingAccept)
	if err != nil {
		t.Fatalf("RecordCirclingDecision(accept) error = %v; want nil", err)
	}
	if round != budgetEscalationBudget+1 || cause != shedadapters.EscalationHardCap {
		t.Fatalf("RecordCirclingDecision(accept) = %d, %q; want round %d with cause %q", round, cause, budgetEscalationBudget+1, shedadapters.EscalationHardCap)
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
				st, found, err := state.ReadJSONNoMkdir[shedengine.Status](env.StatusPath, env.StatusLockPath)
				if err != nil || !found {
					return 0, 0, false, err
				}
				routing, err := Routing(env.ReviewMaxBounces, rowBudgets)
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
