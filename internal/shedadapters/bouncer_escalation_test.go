// bouncer_escalation_test.go covers the Bouncer's escalation of a CIRCLING ruling or a spent bounce budget to the run's parent: the record it writes, the Awaiting it returns, and the way it acts on the recorded decision.

package shedadapters

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// spentBounces is a Bounces seam reporting a budget of three with all three spent.
func spentBounces() (int, int, bool, error) { return 3, 3, true, nil }

// escalationBouncer builds a Bouncer over a run directory whose rounds 1 and 2 are judged, round 1 CONTINUE and round 2 verdict,
// with a gating key open in both so a CIRCLING at round 2 is earned.
// The fixture tells the brief render its slug, worktree root and decision record path.
func escalationBouncer(t *testing.T, verdict string, tweak func(*BouncerConfig), opts ...bouncerFixtureOpt) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	opts = append([]bouncerFixtureOpt{withNestedRunDir(), withShuttle(shuttle), withEscalationInputs()}, opts...)
	cfg := newBouncerFixture(t, opts...).Config
	cfg.CirclingCheckpoint = 2
	if tweak != nil {
		tweak(&cfg)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1)},
		{round: 2, report: bouncerReport(2), verdict: bouncerVerdictContent(verdict), ledger: bouncerLedgerContent(2)},
	})
	gating := evidenceEntry{"alpha", "open", "design", "MEDIUM"}
	writeEvidenceLedger(t, cfg.RunDir, 1, gating)
	writeEvidenceLedger(t, cfg.RunDir, 2, gating)
	// RecordCirclingDecision resolves the latest round from the review file, not the fixture's report name.
	for n := 1; n <= 2; n++ {
		if err := os.WriteFile(roundReviewPath(cfg.RunDir, n), []byte("review\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(review round %d) = %v; want nil", n, err)
		}
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, &cfg, shuttle
}

// requireEscalation fails the test unless round 2 carries an escalation record of the wanted cause, and returns its notice.
func requireEscalation(t *testing.T, cfg *BouncerConfig, want EscalationCause) string {
	t.Helper()
	cause, notice, exists, err := readEscalation(cfg.RunDir, 2)
	if err != nil || !exists {
		t.Fatalf("readEscalation(round 2) exists = %v, err = %v; want a record", exists, err)
	}
	if cause != want {
		t.Errorf("escalation cause = %q; want %q", cause, want)
	}
	return notice
}

func requireNoEscalation(t *testing.T, cfg *BouncerConfig) {
	t.Helper()
	if _, err := os.Stat(escalationPath(cfg.RunDir, 2)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(escalation record) = %v; want it absent", err)
	}
}

func TestBouncer_Escalation_SpentBudgetContinueAwaitsWithCauseBudget(t *testing.T) {
	b, cfg, shuttle := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces })

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	requireReasonContains(t, ptr.Reason, "round 2", "cause: budget", escalationPath(cfg.RunDir, 2), "lyx loom circling accept my-task", "lyx loom circling continue my-task", "lyx loom start")
	notice := requireEscalation(t, cfg, EscalationBudget)
	if notice == "" || ptr.ParentNotice != notice {
		t.Errorf("ParentNotice = %q, notice file = %q; want them equal and non-empty", ptr.ParentNotice, notice)
	}
	requireReasonContains(t, ptr.ParentNotice, "my-task", escalationPath(cfg.RunDir, 2))
	brief, err := os.ReadFile(escalationPath(cfg.RunDir, 2))
	if err != nil || !strings.Contains(string(brief), "round 2") {
		t.Errorf("escalation brief = %q, err = %v; want a rendered brief naming round 2", brief, err)
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 3)); err != nil {
		t.Errorf("round-3 focus file = %v; want ensureFocus to have written it", err)
	}
	if shuttle.Called {
		t.Error("Call() spawned a run; want none")
	}
}

func TestBouncer_Escalation_SpentBudgetOverCirclingIsBudget(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CIRCLING", func(c *BouncerConfig) { c.Bounces = spentBounces })

	shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	requireEscalation(t, cfg, EscalationBudget)
}

func TestBouncer_Escalation_BelowBudget(t *testing.T) {
	below := func(c *BouncerConfig) {
		c.Bounces = func() (int, int, bool, error) { return 1, 3, true, nil }
	}

	t.Run("a guarded CIRCLING escalates with cause circling", func(t *testing.T) {
		b, cfg, _ := escalationBouncer(t, "CIRCLING", below)

		ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		requireEscalation(t, cfg, EscalationCircling)
		requireReasonContains(t, ptr.Reason, "cause: circling")
		if ptr.ParentNotice == "" {
			t.Error("ParentNotice is empty; want the notice")
		}
	})

	t.Run("a CONTINUE returns Stuck and writes no record", func(t *testing.T) {
		b, cfg, _ := escalationBouncer(t, "CONTINUE", below)

		ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
		if ptr.BudgetExempt || ptr.ParentNotice != "" {
			t.Errorf("pointer = %+v; want a plain counted Stuck", ptr)
		}
		requireNoEscalation(t, cfg)
	})
}

func TestBouncer_Escalation_UnknownBudgetNeverEscalatesOnBudget(t *testing.T) {
	tests := []struct {
		name    string
		bounces func() (int, int, bool, error)
	}{
		{"unwired seam", nil},
		{"ok false", func() (int, int, bool, error) { return 3, 3, false, nil }},
		{"erroring seam", func() (int, int, bool, error) { return 3, 3, true, errors.New("status unreadable") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = tt.bounces })

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if ptr.BudgetExempt {
				t.Error("Stuck is BudgetExempt; want a counted Stuck")
			}
			requireNoEscalation(t, cfg)
		})
	}
}

func TestBouncer_Escalation_RecordedContinueExemptsOnlyABudgetEscalation(t *testing.T) {
	tests := []struct {
		name       string
		verdict    string
		bounces    func() (int, int, bool, error)
		wantCause  EscalationCause
		wantExempt bool
	}{
		{"budget escalation", "CONTINUE", spentBounces, EscalationBudget, true},
		{"circling escalation", "CIRCLING", func() (int, int, bool, error) { return 1, 3, true, nil }, EscalationCircling, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, _ := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { c.Bounces = tt.bounces })
			shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if round, cause, err := RecordCirclingDecision(cfg.RunDir, CirclingContinue); err != nil || round != 2 || cause != tt.wantCause {
				t.Fatalf("RecordCirclingDecision(continue) = (%d, %q, %v); want (2, %q, nil)", round, cause, err, tt.wantCause)
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			if ptr.BudgetExempt != tt.wantExempt {
				t.Errorf("BudgetExempt = %v; want %v", ptr.BudgetExempt, tt.wantExempt)
			}
		})
	}
}

func TestBouncer_Escalation_RecordedAcceptSettlesEitherCause(t *testing.T) {
	tests := []struct {
		name    string
		verdict string
		bounces func() (int, int, bool, error)
	}{
		{"budget escalation", "CONTINUE", spentBounces},
		{"circling escalation", "CIRCLING", func() (int, int, bool, error) { return 1, 3, true, nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seams := &circlingSeams{}
			b, cfg, _ := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { seams.install(c); c.Bounces = tt.bounces })
			shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if _, _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
				t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			if got := strings.Join(seams.order, ","); got != "approve,commit" {
				t.Errorf("seam order = %q; want approve,commit", got)
			}
			if _, _, settled, _, err := readCirclingDecision(cfg.RunDir, 2); err != nil || !settled {
				t.Errorf("readCirclingDecision settled = %v, err = %v; want settled true", settled, err)
			}

			// A re-entry after the accept clears and re-seeds, whatever the settled round's verdict.
			shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
				t.Errorf("expected the next Call to archive the settled generation: %v", err)
			}
		})
	}
}

func TestBouncer_Escalation_ReCallLeavesTheRecordUntouched(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces })

	first := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	record, err := os.ReadFile(escalationPath(cfg.RunDir, 2))
	if err != nil {
		t.Fatalf("ReadFile(escalation) = %v; want nil", err)
	}
	notice, err := os.ReadFile(parentNoticePath(cfg.RunDir, 2))
	if err != nil {
		t.Fatalf("ReadFile(parent notice) = %v; want nil", err)
	}

	second := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	if second != first {
		t.Errorf("second Call() pointer = %+v; want %+v", second, first)
	}
	if got, _ := os.ReadFile(escalationPath(cfg.RunDir, 2)); !bytes.Equal(got, record) {
		t.Errorf("escalation record changed on the re-call:\n%q\nwant\n%q", got, record)
	}
	if got, _ := os.ReadFile(parentNoticePath(cfg.RunDir, 2)); !bytes.Equal(got, notice) {
		t.Errorf("parent notice changed on the re-call:\n%q\nwant\n%q", got, notice)
	}
}

func TestBouncer_Escalation_MissingStencilStillAwaitsWithThePlainReason(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces }, withoutStencils("bouncer-template-escalation"))

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	requireEscalation(t, cfg, EscalationBudget)
	requireReasonContains(t, ptr.Reason, "round 2", "cause: budget", "lyx loom circling accept my-task", "lyx loom start")
	if strings.Contains(ptr.Reason, "brief at") {
		t.Errorf("Reason = %q; want no brief path when the render failed", ptr.Reason)
	}
	if ptr.ParentNotice != "" {
		t.Errorf("ParentNotice = %q; want none", ptr.ParentNotice)
	}
	if _, err := os.Stat(parentNoticePath(cfg.RunDir, 2)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(parent notice) = %v; want it absent", err)
	}
}

func TestBouncer_Escalation_MalformedRecordHaltsStuckNamingTheWayForward(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces })
	if err := os.WriteFile(escalationPath(cfg.RunDir, 2), []byte("not frontmatter\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(escalation record) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	requireReasonContains(t, ptr.Reason, "round 2", "unreadable", "way forward:", escalationPath(cfg.RunDir, 2), "lyx loom start")
	if ptr.ParentNotice != "" {
		t.Errorf("ParentNotice = %q; want none", ptr.ParentNotice)
	}
}
