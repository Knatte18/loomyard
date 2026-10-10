// escalation_test.go drives a real Discussion-Review segment through a spent review budget:
// the Bouncer escalates to the parent instead of blocking, a budget continue grants exactly one more round, and an accept settles the segment.

package loomrecipe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// budgetEscalationBudget is the small review budget the escalation tests run with.
const budgetEscalationBudget = 2

// newEscalationShed builds a Shed over the sequence fixture with a small review budget, a judge that rules CONTINUE every round,
// and the SegmentBounces seam wired over the fixture's status file through Routing, as loomcli wires it.
// It returns the Shed, the fixture env and the Discussion-Review run directory.
func newEscalationShed(t *testing.T) (*shedengine.Shed, string, *shedfake.BurlerRunner, func() shedengine.Status) {
	t.Helper()

	_, env, paths := buildSequenceFixture(t)
	blockBouncerJudge(env)
	env.ReviewMaxBounces = budgetEscalationBudget
	env.SegmentBounces = func(row string) (int, int, bool, error) {
		st, found, err := state.ReadJSONStrict[shedengine.Status](env.StatusPath, env.StatusLockPath)
		if err != nil || !found {
			return 0, 0, false, err
		}
		routing, err := Routing(budgetEscalationBudget)
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
	return shed, filepath.Join(env.RunRoot, "discussion"), env.Burler.(*shedfake.BurlerRunner), readStatus
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
	roundsBefore := burler.Calls

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
	if got := burler.Calls - roundsBefore; got != 1 {
		t.Errorf("rounds run after the budget continue = %d; want exactly 1", got)
	}
	if total, exempt := countStuck(result.History, loomshed.NameDiscussionBurler); total != burler.Calls || exempt != total {
		t.Errorf("Discussion-Burler Stuck entries = %d, budget-exempt = %d; want both %d (every round's hand-back for judgment is exempt)", total, exempt, burler.Calls)
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
