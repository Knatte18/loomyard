// bouncer_escalation_test.go covers the Bouncer's escalation of a CIRCLING ruling or a spent bounce budget to the run's parent: the record it writes, the Awaiting it returns, and the way it acts on the recorded decision.

package shedadapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// spentBounces is a Bounces seam reporting a budget of three with all three spent.
func spentBounces() (int, int, bool, error) { return 3, 3, true, nil }

// escalationBouncer builds a Bouncer over a run directory whose rounds 1 and 2 are judged, round 1 CONTINUE and round 2 verdict,
// with a gating key open in both so a CIRCLING at round 2 is earned.
// The fixture tells the brief render its slug, worktree root and decision record path.
// It overwrites round 2's report at cfg.ReportName(2) with `not a review`, so its rounds are in the soft-cap rule's window wherever its callers spend the budget, and an unparseable report keeps their cause budget through the rule's first step.
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
	if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(2)), []byte("not a review\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(report round 2) = %v; want nil", err)
	}
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

// taggedEntry is one ledger entry with the judge's fix tag; an empty fix writes no fix key.
type taggedEntry struct {
	key, status, class, severity, fix string
}

// taggedLedgerContent renders round's ledger from entries as writeEvidenceLedger does, writing `fix:` only when set.
func taggedLedgerContent(round int, entries ...taggedEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "  - key: %s\n    status: %s\n    rounds: [%d]\n", e.key, e.status, round)
		if e.class != "" {
			fmt.Fprintf(&b, "    class: %s\n", e.class)
		}
		if e.severity != "" {
			fmt.Fprintf(&b, "    severity: %s\n", e.severity)
		}
		if e.fix != "" {
			fmt.Fprintf(&b, "    fix: %s\n", e.fix)
		}
	}
	list := "ledger: []\n"
	if len(entries) > 0 {
		list = "ledger:\n" + b.String()
	}
	return fmt.Sprintf("---\nround: %d\n%s---\nprose\n", round, list)
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
// Awaiting pointer, the record and notice it writes, the cause it picks, the accept-only offer at the hard cap, and that a second Call returns the same pointer and leaves the record untouched.
// A round in the soft-cap rule's window whose review does not parse escalates budget, even over a CIRCLING verdict.
//
//testtiming:keep pins the escalation's Awaiting pointer and reason, the record and notice it writes, the cause it picks, the accept-only offer at the hard cap and that a re-call leaves the record untouched
func TestBouncer_Escalation_AwaitsWithTheRecordAndLeavesItUntouched(t *testing.T) {
	tests := []struct {
		name    string
		verdict string
		bounces func() (int, int, bool, error)
		hardCap int
		// parseableReport rewrites round 2's report as a review that parses.
		parseableReport bool
		wantCause       EscalationCause
	}{
		{name: "a spent budget over a CONTINUE", verdict: "CONTINUE", bounces: spentBounces, wantCause: EscalationBudget},
		{name: "a spent budget over a CIRCLING is budget", verdict: "CIRCLING", bounces: spentBounces, wantCause: EscalationBudget},
		{name: "a guarded CIRCLING below budget is circling", verdict: "CIRCLING", bounces: belowBounces, wantCause: EscalationCircling},
		{name: "a spent budget at the hard cap offers only accept", verdict: "CONTINUE", bounces: spentBounces, hardCap: 2, wantCause: EscalationBudget},
		{name: "a design-wrong finding at the hard cap escalates hard-cap", verdict: "CONTINUE", bounces: spentBounces, hardCap: 2, parseableReport: true, wantCause: EscalationHardCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, shuttle := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { c.Bounces = tt.bounces; c.HardCap = tt.hardCap })
			if tt.parseableReport {
				if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(2)), []byte(bouncerReport(2)), 0o644); err != nil {
					t.Fatalf("WriteFile(report round 2) = %v; want nil", err)
				}
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			atCap := tt.hardCap != 0 && 2 >= tt.hardCap
			requireReasonContains(t, ptr.Reason, "round 2", "cause: "+string(tt.wantCause), escalationPath(cfg.RunDir, 2), "lyx loom circling accept my-task", "lyx loom resume")
			if strings.Contains(ptr.Reason, "lyx loom circling continue") == atCap {
				t.Errorf("Reason = %q; want it to name the continue verb exactly when the round is below the hard cap", ptr.Reason)
			}
			notice := requireEscalation(t, cfg, tt.wantCause)
			if notice == "" || ptr.ParentNotice != notice {
				t.Errorf("ParentNotice = %q, notice file = %q; want them equal and non-empty", ptr.ParentNotice, notice)
			}
			requireReasonContains(t, ptr.ParentNotice, "my-task", escalationPath(cfg.RunDir, 2))
			brief, err := os.ReadFile(escalationPath(cfg.RunDir, 2))
			if err != nil || !strings.Contains(string(brief), "round 2") {
				t.Errorf("escalation brief = %q, err = %v; want a rendered brief naming round 2", brief, err)
			}
			if atCap {
				if !strings.Contains(string(brief), "can only be accepted:") || strings.Contains(string(brief), "lyx loom circling continue") {
					t.Errorf("escalation brief = %q; want the accept-only offer", brief)
				}
			} else if !strings.Contains(string(brief), "lyx loom circling continue my-task") {
				t.Errorf("escalation brief = %q; want it to offer the continue verb", brief)
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
// escalation of any cause.
// A continue returns Stuck, exempt from the budget for a budget cause or at a spent budget.
// At the hard cap a continue is moved aside and the round re-escalates with cause hard-cap.
// An accept approves, commits and returns Done, and a re-entry afterwards archives the settled generation.
//
//testtiming:keep pins how a recorded decision settles an escalation of any cause: continue is exempt from the budget for a budget cause or at a spent budget, re-escalates at the hard cap, and accept approves, commits and clears
func TestBouncer_Escalation_RecordedDecision(t *testing.T) {
	tests := []struct {
		name           string
		verdict        string
		bounces        func() (int, int, bool, error)
		hardCap        int
		preRecord      EscalationCause
		decision       CirclingDecision
		wantCause      EscalationCause
		wantExempt     bool
		wantReEscalate bool
	}{
		{name: "continue over a budget escalation", verdict: "CONTINUE", bounces: spentBounces, decision: CirclingContinue, wantCause: EscalationBudget, wantExempt: true},
		{name: "continue over a circling escalation", verdict: "CIRCLING", bounces: belowBounces, decision: CirclingContinue, wantCause: EscalationCircling},
		{name: "continue over a circling escalation at a spent budget is exempt", verdict: "CIRCLING", bounces: spentBounces, preRecord: EscalationCircling, decision: CirclingContinue, wantCause: EscalationCircling, wantExempt: true},
		{name: "continue over a budget escalation at the hard cap re-escalates hard-cap", verdict: "CONTINUE", bounces: spentBounces, hardCap: 2, decision: CirclingContinue, wantCause: EscalationBudget, wantReEscalate: true},
		{name: "continue over a circling escalation at the hard cap re-escalates hard-cap", verdict: "CIRCLING", bounces: belowBounces, hardCap: 2, preRecord: EscalationCircling, decision: CirclingContinue, wantCause: EscalationCircling, wantReEscalate: true},
		{name: "accept over a budget escalation", verdict: "CONTINUE", bounces: spentBounces, decision: CirclingAccept, wantCause: EscalationBudget},
		{name: "accept over a circling escalation", verdict: "CIRCLING", bounces: belowBounces, decision: CirclingAccept, wantCause: EscalationCircling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seams := &circlingSeams{}
			b, cfg, shuttle := escalationBouncer(t, tt.verdict, func(c *BouncerConfig) { seams.install(c); c.Bounces = tt.bounces; c.HardCap = tt.hardCap })
			if tt.preRecord != "" {
				if err := writeEscalation(cfg.RunDir, 2, tt.preRecord, "brief\n", "notice"); err != nil {
					t.Fatalf("writeEscalation(%q) = %v; want nil", tt.preRecord, err)
				}
			}
			shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if round, cause, err := RecordCirclingDecision(cfg.RunDir, tt.decision); err != nil || round != 2 || cause != tt.wantCause {
				t.Fatalf("RecordCirclingDecision(%s) = (%d, %q, %v); want (2, %q, nil)", tt.decision, round, cause, err, tt.wantCause)
			}

			if tt.wantReEscalate {
				ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
				requireReasonContains(t, ptr.Reason, "cause: hard-cap", "lyx loom circling accept my-task")
				if strings.Contains(ptr.Reason, "lyx loom circling continue") {
					t.Errorf("Reason = %q; want no continue verb at the hard cap", ptr.Reason)
				}
				if cause, notice, exists, err := readEscalation(cfg.RunDir, 2); err != nil || !exists || cause != EscalationHardCap || notice == "" {
					t.Errorf("readEscalation = (%q, %q, %v, %v); want a hard-cap record with a notice", cause, notice, exists, err)
				}
				if _, err := os.Stat(circlingDecisionPath(cfg.RunDir, 2)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Stat(decision file) = %v; want it moved aside", err)
				}
				for _, pattern := range []string{"round-2-escalation-*.md", "round-2-parent-notice-*.md", "round-2-circling-decision-*.md"} {
					if matches, err := filepath.Glob(filepath.Join(cfg.RunDir, pattern)); err != nil || len(matches) != 1 {
						t.Errorf("Glob(%s) = %v, %v; want one retired sibling", pattern, matches, err)
					}
				}
				if round, cause, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil || round != 2 || cause != EscalationHardCap {
					t.Fatalf("RecordCirclingDecision(accept) = (%d, %q, %v); want (2, hard-cap, nil)", round, cause, err)
				}
				shedfake.RequireOutcome(t, b, shedengine.Done)
				if _, cause, settled, _, err := readCirclingDecision(cfg.RunDir, 2); err != nil || !settled || cause != EscalationHardCap {
					t.Errorf("readCirclingDecision = (cause %q, settled %v, %v); want settled with cause hard-cap", cause, settled, err)
				}
				if shuttle.Called {
					t.Error("Call() spawned a run; want none")
				}
				return
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

// TestBouncer_AutoAccept covers BouncerConfig.AutoAccept: a spent budget over a verdict this call's judge just wrote settles as an automatic accept,
// and a replay and a BlockingEscalates round go to the soft-cap rule.
// The rule's continue rows run over an untagged MEDIUM entry, which pins that a missing `fix` key reads as design-wrong.
func TestBouncer_AutoAccept(t *testing.T) {
	gating := evidenceEntry{"alpha", "open", "design", "MEDIUM"}
	tests := []struct {
		name       string
		verdict    string
		harvest    bool
		autoAccept bool
		// blockingEscalates is BouncerConfig.BlockingEscalates.
		blockingEscalates bool
		// ledger is the judged round's ledger; nil holds the one open gating entry.
		ledger      []evidenceEntry
		bounces     func() (int, int, bool, error)
		escalated   bool
		decide      CirclingDecision
		wantOutcome shedengine.Outcome
		// wantCause is the escalation cause an Awaiting row writes.
		wantCause EscalationCause
		// wantSettled expects a settled accept on disk whose automatic value is wantAutomaticValue.
		wantSettled        bool
		wantAutomaticValue bool
		wantExempt         bool
		// wantContinue expects the rule's pending continue on disk: cause budget, automatic true.
		wantContinue bool
		// wantCarried, when non-nil, is the keys of the findings the accept carries over, sorted.
		wantCarried []string
	}{
		{name: "BlockingEscalates accepts a ledger with no open BLOCKING entry and carries its open findings", verdict: "CONTINUE", harvest: true, autoAccept: true, blockingEscalates: true, bounces: spentBounces,
			ledger:      []evidenceEntry{{"alpha", "resolved", "design", "MEDIUM"}, {"beta", "open", "design", "MEDIUM"}, {"gone", "resolved", "design", "BLOCKING"}},
			wantOutcome: shedengine.Done, wantSettled: true, wantAutomaticValue: true, wantCarried: []string{"beta"}},
		{name: "BlockingEscalates sends an open BLOCKING entry to the rule, which continues", verdict: "CONTINUE", harvest: true, autoAccept: true, blockingEscalates: true, bounces: spentBounces,
			ledger:      []evidenceEntry{{"alpha", "resolved", "design", "MEDIUM"}, {"hard", "open", "scope", "BLOCKING"}},
			wantOutcome: shedengine.Stuck, wantExempt: true, wantContinue: true},
		{name: "a harvested CONTINUE at a spent budget settles automatically", verdict: "CONTINUE", harvest: true, autoAccept: true, bounces: spentBounces,
			wantOutcome: shedengine.Done, wantSettled: true, wantAutomaticValue: true},
		{name: "a harvested CIRCLING at a spent budget settles automatically", verdict: "CIRCLING", harvest: true, autoAccept: true, bounces: spentBounces,
			wantOutcome: shedengine.Done, wantSettled: true, wantAutomaticValue: true},
		{name: "the flag false sends the round to the rule, which continues", verdict: "CONTINUE", harvest: true, bounces: spentBounces,
			wantOutcome: shedengine.Stuck, wantExempt: true, wantContinue: true},
		{name: "an escalation already recorded returns its Awaiting", verdict: "CONTINUE", harvest: true, autoAccept: true, bounces: spentBounces, escalated: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget},
		{name: "a replayed CONTINUE goes to the rule, which continues", verdict: "CONTINUE", autoAccept: true, bounces: spentBounces,
			wantOutcome: shedengine.Stuck, wantExempt: true, wantContinue: true},
		{name: "a harvested CIRCLING below the budget escalates with cause circling", verdict: "CIRCLING", harvest: true, autoAccept: true, bounces: belowBounces,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationCircling},
		{name: "an erroring bounce seam returns a plain Stuck", verdict: "CONTINUE", harvest: true, autoAccept: true,
			bounces:     func() (int, int, bool, error) { return 3, 3, true, errors.New("status unreadable") },
			wantOutcome: shedengine.Stuck},
		{name: "a recorded parent continue returns its budget-exempt Stuck", verdict: "CONTINUE", autoAccept: true, bounces: spentBounces, escalated: true, decide: CirclingContinue,
			wantOutcome: shedengine.Stuck, wantExempt: true},
		{name: "a recorded parent accept settles with automatic false", verdict: "CONTINUE", autoAccept: true, bounces: spentBounces, escalated: true, decide: CirclingAccept,
			wantOutcome: shedengine.Done, wantSettled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seams := &circlingSeams{}
			carry := &carryOverRecorder{}
			ledger := tt.ledger
			if ledger == nil {
				ledger = []evidenceEntry{gating}
			}
			shuttle := judgeFakeShuttle(2, bouncerVerdictContent(tt.verdict), evidenceLedgerContent(t, 2, ledger...), true)
			cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle), withEscalationInputs()).Config
			cfg.CirclingCheckpoint = 2
			cfg.Bounces = tt.bounces
			cfg.AutoAccept = tt.autoAccept
			cfg.BlockingEscalates = tt.blockingEscalates
			seams.install(&cfg)
			carry.install(&cfg)
			rounds := []bouncerJudgeFixture{
				{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1)},
				{round: 2, report: bouncerReport(2)},
			}
			if !tt.harvest {
				rounds[1].verdict = bouncerVerdictContent(tt.verdict)
				rounds[1].ledger = bouncerLedgerContent(2)
			}
			layoutBouncerRun(t, cfg, rounds)
			writeEvidenceLedger(t, cfg.RunDir, 1, gating)
			if !tt.harvest {
				writeEvidenceLedger(t, cfg.RunDir, 2, ledger...)
			}
			for n := 1; n <= 2; n++ {
				if err := os.WriteFile(roundReviewPath(cfg.RunDir, n), []byte("review\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(review round %d) = %v; want nil", n, err)
				}
			}
			if tt.escalated {
				if err := writeEscalation(cfg.RunDir, 2, EscalationBudget, "brief\n", "notice"); err != nil {
					t.Fatalf("writeEscalation = %v; want nil", err)
				}
			}
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}

			firstOutcome := tt.wantOutcome
			if tt.decide != "" {
				firstOutcome = shedengine.Awaiting
			}
			ptr := shedfake.RequireOutcome(t, b, firstOutcome)
			if tt.decide != "" {
				if _, _, err := RecordCirclingDecision(cfg.RunDir, tt.decide); err != nil {
					t.Fatalf("RecordCirclingDecision(%s) = %v; want nil", tt.decide, err)
				}
				seams.order, carry.entries = nil, nil
				ptr = shedfake.RequireOutcome(t, b, tt.wantOutcome)
			}

			if ptr.BudgetExempt != tt.wantExempt {
				t.Errorf("BudgetExempt = %v; want %v", ptr.BudgetExempt, tt.wantExempt)
			}
			if tt.wantCause != "" {
				requireEscalation(t, &cfg, tt.wantCause)
			}
			if tt.wantOutcome != shedengine.Awaiting && tt.decide == "" {
				requireNoEscalation(t, &cfg)
				if ptr.ParentNotice != "" {
					t.Errorf("ParentNotice = %q; want none", ptr.ParentNotice)
				}
			}

			decision, cause, settled, automatic, exists, err := readCirclingFile(cfg.RunDir, 2)
			if err != nil {
				t.Fatalf("readCirclingFile(round 2) = %v; want nil", err)
			}
			if tt.wantSettled {
				if !exists || decision != CirclingAccept || !settled || automatic != tt.wantAutomaticValue {
					t.Errorf("decision file = (%q, %q, settled %v, automatic %v, exists %v); want a settled accept with automatic %v", decision, cause, settled, automatic, exists, tt.wantAutomaticValue)
				}
				if got := strings.Join(seams.order, ","); got != "approve,commit" {
					t.Errorf("seam order = %q; want approve,commit", got)
				}
				if len(carry.entries) != 1 || carry.entries[0].Closing != discussionparser.CarryOverAccepted {
					t.Errorf("carry-over entries = %+v; want one with closing %q", carry.entries, discussionparser.CarryOverAccepted)
				} else if tt.wantCarried != nil {
					var keys []string
					for _, finding := range carry.entries[0].Findings {
						keys = append(keys, finding.Key)
					}
					if got, want := strings.Join(keys, ","), strings.Join(tt.wantCarried, ","); got != want {
						t.Errorf("carried finding keys = %q; want %q", got, want)
					}
				}
			} else if tt.wantContinue {
				if !exists || decision != CirclingContinue || settled || cause != EscalationBudget || !automatic {
					t.Errorf("decision file = (%q, %q, settled %v, automatic %v, exists %v); want a pending automatic continue with cause budget", decision, cause, settled, automatic, exists)
				}
			} else if tt.decide == "" && exists {
				t.Errorf("decision file exists (%q); want none", decision)
			}
		})
	}
}

// softCapPath is one of the three ways a round reaches the soft-cap rule: a harvest with no auto-accept, a replay on an auto-accept perch, and a harvest on an auto-accept perch whose open BLOCKING entry blocking_escalates sends to the rule.
type softCapPath struct {
	name                          string
	harvest, autoAccept, blocking bool
	// severity replaces MEDIUM in the judged ledger, so the BLOCKING path has the open BLOCKING entry it needs.
	severity string
}

// softCapRuleRow is one case of TestBouncer_SoftCapRule, run once on each softCapPath.
type softCapRuleRow struct {
	name string
	// verdict is round 2's verdict; empty is CONTINUE.
	verdict string
	// bounces is the Bounces seam; nil is spentBounces.
	bounces    func() (int, int, bool, error)
	hardCap    int
	prevLedger []taggedEntry
	ledger     []taggedEntry
	// prevDecision writes round 1's decision as an automatic budget-cause continue.
	prevDecision bool
	// badReport overwrites round 2's report, badPrevLedger round 1's ledger, and badPrevDecision writes `accept` as round 1's decision file.
	badReport, badPrevLedger, badPrevDecision bool
	// preRecord writes round 2's escalation record with this cause before the first Call.
	preRecord   EscalationCause
	wantOutcome shedengine.Outcome
	wantCause   EscalationCause
	wantExempt  bool
	// wantDecision is the rule's decision file for round 2: accept, continue, or empty for none.
	wantDecision CirclingDecision
	wantCarried  []string
}

// TestBouncer_SoftCapRule covers the soft-cap rule's five steps over a spent budget, the hard cap and a goto-reset count, on every path that reaches it.
// Every row Calls a second time, the replay of a rule-settled round, and requires the same result with no further judge run.
func TestBouncer_SoftCapRule(t *testing.T) {
	design := taggedEntry{"alpha", "open", "design", "MEDIUM", "design-wrong"}
	mechanical := taggedEntry{"mech", "open", "design", "MEDIUM", "mechanical"}
	atFourOfThree := func() (int, int, bool, error) { return 4, 3, true, nil }
	paths := []softCapPath{
		{name: "harvest", harvest: true, severity: "MEDIUM"},
		{name: "replay with auto-accept", autoAccept: true, severity: "MEDIUM"},
		{name: "blocking_escalates", harvest: true, autoAccept: true, blocking: true, severity: "BLOCKING"},
	}
	rows := []softCapRuleRow{
		{name: "no design-wrong entry accepts and carries the open MEDIUM mechanical key", ledger: []taggedEntry{{"low", "open", "design", "LOW", "design-wrong"}, mechanical},
			wantOutcome: shedengine.Done, wantDecision: CirclingAccept, wantCarried: []string{"mech"}},
		{name: "a design-wrong entry continues", ledger: []taggedEntry{design},
			wantOutcome: shedengine.Stuck, wantExempt: true, wantDecision: CirclingContinue},
		{name: "an unknown fix tag continues", ledger: []taggedEntry{{"alpha", "open", "design", "MEDIUM", "sideways"}},
			wantOutcome: shedengine.Stuck, wantExempt: true, wantDecision: CirclingContinue},
		{name: "an earned CIRCLING escalates circling", verdict: "CIRCLING", prevLedger: []taggedEntry{design}, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationCircling},
		{name: "an earned CIRCLING past the soft cap escalates circling", verdict: "CIRCLING", bounces: atFourOfThree, prevLedger: []taggedEntry{design}, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationCircling},
		{name: "a previous continue and the same design-wrong key escalates circling", prevDecision: true, prevLedger: []taggedEntry{design}, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationCircling},
		{name: "a previous continue and a key that was LOW continues", prevDecision: true, prevLedger: []taggedEntry{{"alpha", "open", "design", "LOW", "design-wrong"}}, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Stuck, wantExempt: true, wantDecision: CirclingContinue},
		{name: "a previous continue and a key that was mechanical continues", prevDecision: true, prevLedger: []taggedEntry{{"alpha", "open", "design", "MEDIUM", "mechanical"}}, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Stuck, wantExempt: true, wantDecision: CirclingContinue},
		{name: "a design-wrong entry at the hard cap escalates hard-cap", hardCap: 2, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationHardCap},
		{name: "no design-wrong entry at the hard cap accepts", hardCap: 2, ledger: []taggedEntry{mechanical},
			wantOutcome: shedengine.Done, wantDecision: CirclingAccept, wantCarried: []string{"mech"}},
		{name: "a recorded circling escalation at the hard cap keeps its record", hardCap: 2, ledger: []taggedEntry{design}, preRecord: EscalationCircling,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationCircling},
		{name: "a goto-reset count at the hard cap escalates hard-cap over a design-wrong entry", bounces: belowBounces, hardCap: 2, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationHardCap},
		{name: "a goto-reset count at the hard cap accepts without one", bounces: belowBounces, hardCap: 2, ledger: []taggedEntry{mechanical},
			wantOutcome: shedengine.Done, wantDecision: CirclingAccept, wantCarried: []string{"mech"}},
		{name: "an unparseable review escalates budget", ledger: []taggedEntry{design}, badReport: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget},
		{name: "an unreadable previous ledger escalates budget", prevDecision: true, prevLedger: []taggedEntry{design}, badPrevLedger: true, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget},
		{name: "an unreadable previous decision escalates budget", prevLedger: []taggedEntry{design}, badPrevDecision: true, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget},
		{name: "below the window a CONTINUE stays a counted plain Stuck", bounces: belowBounces, ledger: []taggedEntry{design},
			wantOutcome: shedengine.Stuck},
	}
	for _, row := range rows {
		for _, path := range paths {
			t.Run(row.name+"/"+path.name, func(t *testing.T) {
				runSoftCapRuleRow(t, row, path)
			})
		}
	}
}

// runSoftCapRuleRow lays out one TestBouncer_SoftCapRule row on one path, Calls twice and checks the pointer, the records and the decision file.
func runSoftCapRuleRow(t *testing.T, row softCapRuleRow, path softCapPath) {
	t.Helper()
	verdict := row.verdict
	if verdict == "" {
		verdict = "CONTINUE"
	}
	bounces := row.bounces
	if bounces == nil {
		bounces = spentBounces
	}
	ledger := make([]taggedEntry, len(row.ledger))
	for i, entry := range row.ledger {
		if entry.severity == "MEDIUM" {
			entry.severity = path.severity
		}
		ledger[i] = entry
	}

	seams := &circlingSeams{}
	carry := &carryOverRecorder{}
	shuttle := judgeFakeShuttle(2, bouncerVerdictContent(verdict), taggedLedgerContent(2, ledger...), true)
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle), withEscalationInputs()).Config
	cfg.CirclingCheckpoint = 2
	cfg.Bounces = bounces
	cfg.HardCap = row.hardCap
	cfg.AutoAccept = path.autoAccept
	cfg.BlockingEscalates = path.blocking
	seams.install(&cfg)
	carry.install(&cfg)
	rounds := []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: taggedLedgerContent(1, row.prevLedger...)},
		{round: 2, report: bouncerReport(2)},
	}
	if !path.harvest {
		rounds[1].verdict = bouncerVerdictContent(verdict)
		rounds[1].ledger = taggedLedgerContent(2, ledger...)
	}
	layoutBouncerRun(t, cfg, rounds)
	for n := 1; n <= 2; n++ {
		if err := os.WriteFile(roundReviewPath(cfg.RunDir, n), []byte("review\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(review round %d) = %v; want nil", n, err)
		}
	}
	if row.prevDecision {
		if err := createCirclingDecision(cfg.RunDir, 1, CirclingContinue, EscalationBudget, true); err != nil {
			t.Fatalf("createCirclingDecision(round 1) = %v; want nil", err)
		}
	}
	if row.badReport {
		if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(2)), []byte("not a review\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(report round 2) = %v; want nil", err)
		}
	}
	if row.badPrevLedger {
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte("not a ledger\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(ledger round 1) = %v; want nil", err)
		}
	}
	if row.badPrevDecision {
		if err := os.WriteFile(circlingDecisionPath(cfg.RunDir, 1), []byte("accept\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(decision round 1) = %v; want nil", err)
		}
	}
	if row.preRecord != "" {
		if err := writeEscalation(cfg.RunDir, 2, row.preRecord, "brief\n", "notice"); err != nil {
			t.Fatalf("writeEscalation(%q) = %v; want nil", row.preRecord, err)
		}
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	judgeRuns := func() int {
		runs := 0
		for _, spec := range shuttle.Specs {
			if spec.Role == bouncerJudgeRole {
				runs++
			}
		}
		return runs
	}

	ptr := shedfake.RequireOutcome(t, b, row.wantOutcome)
	if ptr.BudgetExempt != row.wantExempt {
		t.Errorf("BudgetExempt = %v; want %v", ptr.BudgetExempt, row.wantExempt)
	}
	if row.wantOutcome == shedengine.Awaiting {
		requireReasonContains(t, ptr.Reason, "cause: "+string(row.wantCause))
		if row.wantCause == EscalationHardCap && strings.Contains(ptr.Reason, "lyx loom circling continue") {
			t.Errorf("Reason = %q; want no continue verb at the hard cap", ptr.Reason)
		}
		requireEscalation(t, &cfg, row.wantCause)
	} else {
		requireNoEscalation(t, &cfg)
		if ptr.ParentNotice != "" {
			t.Errorf("ParentNotice = %q; want none", ptr.ParentNotice)
		}
	}

	decision, cause, settled, automatic, exists, err := readCirclingFile(cfg.RunDir, 2)
	if err != nil {
		t.Fatalf("readCirclingFile(round 2) = %v; want nil", err)
	}
	switch row.wantDecision {
	case CirclingAccept:
		if !exists || decision != CirclingAccept || !settled || !automatic || cause != EscalationBudget {
			t.Errorf("decision file = (%q, %q, settled %v, automatic %v, exists %v); want a settled automatic accept with cause budget", decision, cause, settled, automatic, exists)
		}
		if got := strings.Join(seams.order, ","); got != "approve,commit" {
			t.Errorf("seam order = %q; want approve,commit", got)
		}
		var keys []string
		for _, entry := range carry.entries {
			for _, finding := range entry.Findings {
				keys = append(keys, finding.Key)
			}
		}
		if len(carry.entries) != 1 || carry.entries[0].Closing != discussionparser.CarryOverAccepted || strings.Join(keys, ",") != strings.Join(row.wantCarried, ",") {
			t.Errorf("carry-over entries = %+v; want one accepted entry carrying %v", carry.entries, row.wantCarried)
		}
	case CirclingContinue:
		if !exists || decision != CirclingContinue || settled || !automatic || cause != EscalationBudget {
			t.Errorf("decision file = (%q, %q, settled %v, automatic %v, exists %v); want a pending automatic continue with cause budget", decision, cause, settled, automatic, exists)
		}
	default:
		if exists {
			t.Errorf("decision file exists (%q); want none", decision)
		}
	}

	recordBefore, _ := os.ReadFile(escalationPath(cfg.RunDir, 2))
	runsBefore := judgeRuns()
	switch row.wantOutcome {
	case shedengine.Done:
		shedfake.RequireOutcome(t, b, shedengine.Stuck)
		if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
			t.Errorf("expected the replay to clear the settled generation: %v", err)
		}
		if got := strings.Join(seams.order, ","); got != "approve,commit" {
			t.Errorf("seam order after the replay = %q; want the accept settled once", got)
		}
	default:
		if second := shedfake.RequireOutcome(t, b, row.wantOutcome); second != ptr {
			t.Errorf("replay pointer = %+v; want %+v", second, ptr)
		}
		if recordAfter, _ := os.ReadFile(escalationPath(cfg.RunDir, 2)); !bytes.Equal(recordAfter, recordBefore) {
			t.Errorf("escalation record changed on the replay:\n%q\nwant\n%q", recordAfter, recordBefore)
		}
	}
	if runsAfter := judgeRuns(); runsAfter != runsBefore {
		t.Errorf("judge runs after the replay = %d; want %d", runsAfter, runsBefore)
	}
}
