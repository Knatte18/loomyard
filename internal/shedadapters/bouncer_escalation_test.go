// bouncer_escalation_test.go covers the Bouncer's escalation of a CIRCLING ruling or a spent bounce budget to the run's parent: the record it writes, the Awaiting it returns, and the way it acts on the recorded decision.

package shedadapters

import (
	"bytes"
	"context"
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

// belowBounces is a Bounces seam reporting a budget of three with one spent.
func belowBounces() (int, int, bool, error) { return 1, 3, true, nil }

// TestBouncer_Escalation_AwaitsWithTheRecordAndLeavesItUntouched covers the escalation itself: the
// Awaiting pointer, the record and notice it writes, the cause it picks (a spent budget wins over a
// CIRCLING ruling) and that a second Call returns the same pointer and leaves the record untouched.
//
//testtiming:keep pins the escalation's Awaiting pointer and reason, the record and notice it writes, the cause it picks and that a re-call leaves the record untouched
func TestBouncer_Escalation_AwaitsWithTheRecordAndLeavesItUntouched(t *testing.T) {
	tests := []struct {
		name      string
		verdict   string
		bounces   func() (int, int, bool, error)
		wantCause EscalationCause
	}{
		{"a spent budget over a CONTINUE", "CONTINUE", spentBounces, EscalationBudget},
		{"a spent budget over a CIRCLING is budget", "CIRCLING", spentBounces, EscalationBudget},
		{"a guarded CIRCLING below budget is circling", "CIRCLING", belowBounces, EscalationCircling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, shuttle := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { c.Bounces = tt.bounces })

			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			requireReasonContains(t, ptr.Reason, "round 2", "cause: "+string(tt.wantCause), escalationPath(cfg.RunDir, 2), "lyx loom circling accept my-task", "lyx loom circling continue my-task", "lyx loom resume")
			notice := requireEscalation(t, cfg, tt.wantCause)
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
			noticeBytes, err := os.ReadFile(parentNoticePath(cfg.RunDir, 2))
			if err != nil {
				t.Fatalf("ReadFile(parent notice) = %v; want nil", err)
			}

			second := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if second != ptr {
				t.Errorf("second Call() pointer = %+v; want %+v", second, ptr)
			}
			if got, _ := os.ReadFile(escalationPath(cfg.RunDir, 2)); !bytes.Equal(got, brief) {
				t.Errorf("escalation record changed on the re-call:\n%q\nwant\n%q", got, brief)
			}
			if got, _ := os.ReadFile(parentNoticePath(cfg.RunDir, 2)); !bytes.Equal(got, noticeBytes) {
				t.Errorf("parent notice changed on the re-call:\n%q\nwant\n%q", got, noticeBytes)
			}
		})
	}
}

func TestBouncer_Escalation_NoBudgetEscalationReturnsAPlainStuck(t *testing.T) {
	tests := []struct {
		name    string
		bounces func() (int, int, bool, error)
	}{
		{"below budget", belowBounces},
		{"unwired seam", nil},
		{"ok false", func() (int, int, bool, error) { return 3, 3, false, nil }},
		{"erroring seam", func() (int, int, bool, error) { return 3, 3, true, errors.New("status unreadable") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = tt.bounces })

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if ptr.BudgetExempt || ptr.ParentNotice != "" {
				t.Errorf("pointer = %+v; want a plain counted Stuck", ptr)
			}
			requireNoEscalation(t, cfg)
		})
	}
}

// TestBouncer_Escalation_RecordedDecision covers the way a recorded circling decision settles an
// escalation of either cause: continue returns Stuck, exempt from the budget only for a budget
// escalation; accept approves, commits and returns Done, and a re-entry afterwards archives the
// settled generation.
//
//testtiming:keep pins how a recorded decision settles an escalation of either cause: continue is exempt from the budget only for a budget cause, and accept approves, commits and clears
func TestBouncer_Escalation_RecordedDecision(t *testing.T) {
	tests := []struct {
		name       string
		verdict    string
		bounces    func() (int, int, bool, error)
		decision   CirclingDecision
		wantCause  EscalationCause
		wantExempt bool
	}{
		{"continue over a budget escalation", "CONTINUE", spentBounces, CirclingContinue, EscalationBudget, true},
		{"continue over a circling escalation", "CIRCLING", belowBounces, CirclingContinue, EscalationCircling, false},
		{"accept over a budget escalation", "CONTINUE", spentBounces, CirclingAccept, EscalationBudget, false},
		{"accept over a circling escalation", "CIRCLING", belowBounces, CirclingAccept, EscalationCircling, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seams := &circlingSeams{}
			b, cfg, _ := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { seams.install(c); c.Bounces = tt.bounces })
			shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if round, cause, err := RecordCirclingDecision(cfg.RunDir, tt.decision); err != nil || round != 2 || cause != tt.wantCause {
				t.Fatalf("RecordCirclingDecision(%s) = (%d, %q, %v); want (2, %q, nil)", tt.decision, round, cause, err, tt.wantCause)
			}

			if tt.decision == CirclingContinue {
				ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
				if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
					t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
				}
				if ptr.BudgetExempt != tt.wantExempt {
					t.Errorf("BudgetExempt = %v; want %v", ptr.BudgetExempt, tt.wantExempt)
				}
				return
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

func TestBouncer_Escalation_MissingStencilStillAwaitsWithThePlainReason(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces }, withoutStencils("bouncer-template-escalation"))

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	requireEscalation(t, cfg, EscalationBudget)
	requireReasonContains(t, ptr.Reason, "round 2", "cause: budget", "lyx loom circling accept my-task", "lyx loom resume")
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

func TestBouncer_Escalation_FailedRecordWriteErrorsNamingTheWayForward(t *testing.T) {
	b, cfg, _ := escalationBouncer(t, "CONTINUE", func(c *BouncerConfig) { c.Bounces = spentBounces })
	noticePath := parentNoticePath(cfg.RunDir, 2)
	if err := os.Mkdir(noticePath, 0o755); err != nil {
		t.Fatalf("Mkdir(parent notice path) = %v; want nil", err)
	}

	_, _, err := b.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want the failed notice write")
	}
	for _, want := range []string{"write parent notice", noticePath, "way forward:", "lyx loom start"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Call() error %q lacks %q", err.Error(), want)
		}
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
