// bouncer_circling_test.go covers the Bouncer's handling of a CIRCLING round: the Awaiting Reason it writes for the operator and the way it acts on the operator's recorded decision.

package shedadapters

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// circlingSeams counts the Approve and Commit calls a test's Bouncer makes.
type circlingSeams struct {
	order     []string
	commitErr error
}

func (s *circlingSeams) install(cfg *BouncerConfig) {
	cfg.Approve = func() error { s.order = append(s.order, "approve"); return nil }
	cfg.Commit = func() error { s.order = append(s.order, "commit"); return s.commitErr }
}

// circlingBouncer builds a Bouncer over a run directory whose round 1 is judged CIRCLING.
// The shuttle is a fake that fails the test if a Run is attempted.
func circlingBouncer(t *testing.T, tweak func(*BouncerConfig)) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle)).Config
	if tweak != nil {
		tweak(&cfg)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CIRCLING"), ledger: bouncerLedgerContent(1),
	}})
	// RecordCirclingDecision resolves the latest round from the review file, not the fixture's report name.
	if err := os.WriteFile(roundReviewPath(cfg.RunDir, 1), []byte("review\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(review) = %v; want nil", err)
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, &cfg, shuttle
}

func requireReasonContains(t *testing.T, reason string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(reason, want) {
			t.Errorf("Reason = %q; want it to contain %q", reason, want)
		}
	}
}

func TestBouncer_Circling_NoDecisionAwaitsNamingBothVerbs(t *testing.T) {
	b, cfg, shuttle := circlingBouncer(t, func(c *BouncerConfig) { c.Slug = "my-task" })

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	requireReasonContains(t, ptr.Reason, "round 1", "lyx loom circling accept my-task", "lyx loom circling continue my-task", "lyx loom start")
	if shuttle.Called {
		t.Error("Call() spawned a run; want none")
	}
}

func TestBouncer_Circling_EmptySlugOmitsTheArgument(t *testing.T) {
	b, _, _ := circlingBouncer(t, nil)

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	requireReasonContains(t, ptr.Reason, "`lyx loom circling accept`", "`lyx loom circling continue`")
}

func TestBouncer_Circling_BudgetSentence(t *testing.T) {
	tests := []struct {
		name      string
		bounces   func() (int, int, bool, error)
		wantBlock bool
	}{
		{"budget spent", func() (int, int, bool, error) { return 3, 3, true, nil }, true},
		{"budget left", func() (int, int, bool, error) { return 1, 3, true, nil }, false},
		{"unknown", func() (int, int, bool, error) { return 0, 0, false, nil }, false},
		{"seam error", func() (int, int, bool, error) { return 0, 0, false, errors.New("status unreadable") }, false},
		{"nil seam", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := circlingBouncer(t, func(c *BouncerConfig) { c.Slug = "my-task"; c.Bounces = tt.bounces })

			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			requireReasonContains(t, ptr.Reason, "lyx loom circling accept my-task")
			hasBlock := strings.Contains(ptr.Reason, "bounce budget") && strings.Contains(ptr.Reason, "lyx loom goto --to gate")
			if hasBlock != tt.wantBlock {
				t.Errorf("Reason = %q; budget-block sentence present = %v, want %v", ptr.Reason, hasBlock, tt.wantBlock)
			}
		})
	}
}

func TestBouncer_Circling_RecordedContinueReturnsStuckAndRunsNoSeam(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, shuttle := circlingBouncer(t, seams.install)
	if _, err := RecordCirclingDecision(cfg.RunDir, CirclingContinue); err != nil {
		t.Fatalf("RecordCirclingDecision(continue) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if len(seams.order) != 0 {
		t.Errorf("seams ran %v; want none", seams.order)
	}
	if shuttle.Called {
		t.Error("Call() spawned a run; want none")
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 2)); err != nil {
		t.Errorf("round-2 focus file = %v; want ensureFocus to have written it", err)
	}
}

func TestBouncer_Circling_RecordedAcceptSettlesApprovesCommitsThenClears(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if got := strings.Join(seams.order, ","); got != "approve,commit" {
		t.Errorf("seam order = %q; want approve,commit", got)
	}
	if _, settled, _, err := readCirclingDecision(cfg.RunDir, 1); err != nil || !settled {
		t.Errorf("readCirclingDecision settled = %v, err = %v; want settled true", settled, err)
	}

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
		t.Errorf("expected the next Call to archive the settled generation: %v", err)
	}
	if len(seams.order) != 2 {
		t.Errorf("seams ran %v; want approve and commit once each", seams.order)
	}
}

func TestBouncer_Circling_AcceptWithFailedCommitResumesByClearingWithoutApprove(t *testing.T) {
	seams := &circlingSeams{commitErr: errors.New("commit refused")}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
	}

	if _, _, err := b.Call(t.Context()); err == nil || !strings.Contains(err.Error(), "commit refused") {
		t.Fatalf("Call() error = %v; want the commit failure", err)
	}
	if _, settled, _, err := readCirclingDecision(cfg.RunDir, 1); err != nil || !settled {
		t.Errorf("readCirclingDecision settled = %v, err = %v; want the accept already settled", settled, err)
	}

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if got := strings.Join(seams.order, ","); got != "approve,commit" {
		t.Errorf("seam order = %q; want no second Approve on the resume", got)
	}
	if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
		t.Errorf("expected the resumed Call to archive the settled generation: %v", err)
	}
}

func TestBouncer_Circling_SettledAcceptReachedInSettleDegrades(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
	}
	if err := settleCirclingAccept(cfg.RunDir, 1); err != nil {
		t.Fatalf("settleCirclingAccept = %v; want nil", err)
	}

	outcome, ptr, err := b.settle(t.Context(), 1, true)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("settle(...) = (%q, %v); want Stuck and nil", outcome, err)
	}
	requireReasonContains(t, ptr.Reason, "already settled")
	if len(seams.order) != 0 {
		t.Errorf("seams ran %v; want none", seams.order)
	}
}

func TestBouncer_Circling_MalformedDecisionReturnsStuckAndRunsNoSeam(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if err := os.WriteFile(circlingDecisionPath(cfg.RunDir, 1), []byte("not a decision"), 0o644); err != nil {
		t.Fatalf("WriteFile(decision) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	requireReasonContains(t, ptr.Reason, "circling decision")
	if len(seams.order) != 0 {
		t.Errorf("seams ran %v; want none", seams.order)
	}
}
