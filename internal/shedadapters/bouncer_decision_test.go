//go:build tier0

// bouncer_decision_test.go covers the Bouncer's escalation of a judged round whose review holds a decision-class finding to the run's parent:
// the cause-decision record, the parent's recorded accept and continue, the refusal check they meet, and the carry-over a decision accept writes.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// reviewFixtureFinding is one finding of a decision-test review: its id, severity and class.
type reviewFixtureFinding struct{ id, severity, class string }

// decisionReviewContent renders a review holding findings, BLOCKING when any finding is BLOCKING.
func decisionReviewContent(findings ...reviewFixtureFinding) string {
	verdict := "APPROVED"
	var list strings.Builder
	for _, finding := range findings {
		if finding.severity == "BLOCKING" {
			verdict = "BLOCKING"
		}
		fmt.Fprintf(&list, "  - id: %s\n    severity: %s\n    class: %s\n    location: plan.md:4\n    summary: question %s is open\n    fix: settle %s\n", finding.id, finding.severity, finding.class, finding.id, finding.id)
	}
	body := "findings: []\n"
	if list.Len() > 0 {
		body = "findings:\n" + list.String()
	}
	return fmt.Sprintf("---\nverdict: %s\n%s---\nreview prose\n", verdict, body)
}

// judgeAnswer is the verdict file and ledger file one judge run writes.
type judgeAnswer struct{ verdict, ledger string }

// sequencedJudge returns a shuttle whose nth judge run for round writes the nth answer, the last answer repeating, and the next round's focus file.
func sequencedJudge(round int, answers ...judgeAnswer) *shedfake.Shuttle {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	runs := 0
	shuttle.DuringRun = func() {
		answer := answers[min(runs, len(answers)-1)]
		runs++
		outputs := shuttle.GotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		_ = os.WriteFile(outputs[0], []byte(answer.verdict), 0o644)
		_ = os.WriteFile(outputs[1], []byte(answer.ledger), 0o644)
		_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
	}
	return shuttle
}

// decisionBouncer builds a Bouncer over a two-round run directory, round 1 CONTINUE over an empty ledger and round 2 laid out from review, verdict and ledger, with EscalateDecisions set.
// An empty verdict or ledger leaves round 2 unjudged.
// The fixture tells the brief render its slug, worktree root and decision record path; the decision record itself is not written.
func decisionBouncer(t *testing.T, verdict, review, ledger string, tweak func(*BouncerConfig)) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle), withEscalationInputs()).Config
	cfg.CirclingCheckpoint = 2
	cfg.EscalateDecisions = true
	if tweak != nil {
		tweak(&cfg)
	}
	round2 := bouncerJudgeFixture{round: 2, report: review, ledger: ledger}
	if verdict != "" {
		round2.verdict = bouncerVerdictContent(verdict)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1)},
		round2,
	})
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
	return b, &cfg, cfg.Shuttle.(*shedfake.Shuttle)
}

// judgedDecisionBouncer is decisionBouncer over an unjudged round 2 that the Bouncer's own judge spawns judge, the nth answering with answers[n].
// A CONVERGED verdict has to come from such a spawn: one already on disk at the first Call reads as an earlier settle.
func judgedDecisionBouncer(t *testing.T, review string, tweak func(*BouncerConfig), answers ...judgeAnswer) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	shuttle := sequencedJudge(2, answers...)
	b, cfg, _ := decisionBouncer(t, "", review, "", func(c *BouncerConfig) {
		c.Shuttle = shuttle
		if tweak != nil {
			tweak(c)
		}
	})
	return b, cfg, shuttle
}

// decisionBouncerFor builds a decisionBouncer whose round 2 is judged verdict over ledger: laid out on disk, or judged by a spawn when verdict is CONVERGED.
func decisionBouncerFor(t *testing.T, verdict, review, ledger string, tweak func(*BouncerConfig)) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	if verdict == "CONVERGED" {
		return judgedDecisionBouncer(t, review, tweak, judgeAnswer{bouncerVerdictContent(verdict), ledger})
	}
	return decisionBouncer(t, verdict, review, ledger, tweak)
}

// requireDecisionEscalation fails the test unless round 2 carries an escalation record of cause decision, and returns its brief.
func requireDecisionEscalation(t *testing.T, cfg *BouncerConfig) string {
	t.Helper()
	requireEscalation(t, cfg, EscalationDecision)
	brief, err := os.ReadFile(escalationPath(cfg.RunDir, 2))
	if err != nil {
		t.Fatalf("ReadFile(escalation brief) = %v; want nil", err)
	}
	return string(brief)
}

// recordDecision records d for round 2 and fails the test unless it records the cause decision.
func recordDecision(t *testing.T, cfg *BouncerConfig, d CirclingDecision) {
	t.Helper()
	if round, cause, err := RecordCirclingDecision(cfg.RunDir, d); err != nil || round != 2 || cause != EscalationDecision {
		t.Fatalf("RecordCirclingDecision(%s) = (%d, %q, %v); want (2, decision, nil)", d, round, cause, err)
	}
}

// writeAddedDecisions writes the decision record at cfg.DecisionRecordPath holding one `Added after Discussion` entry per title.
func writeAddedDecisions(t *testing.T, cfg *BouncerConfig, titles ...string) {
	t.Helper()
	var record strings.Builder
	record.WriteString("# Decision record\n\n")
	for _, title := range titles {
		fmt.Fprintf(&record, "### Added after Discussion (parent, 2026-10-01): %s\n\nthe answer\n\n", title)
	}
	if err := os.WriteFile(cfg.DecisionRecordPath, []byte(record.String()), 0o644); err != nil {
		t.Fatalf("WriteFile(decision record) = %v; want nil", err)
	}
}

// requireFileAbsent fails the test when path exists.
func requireFileAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("Stat(%s) = nil; want the file absent", path)
	}
}

// requireMatches fails the test unless exactly want files match pattern in dir.
func requireMatches(t *testing.T, dir, pattern string, want int) {
	t.Helper()
	if matches, err := filepath.Glob(filepath.Join(dir, pattern)); err != nil || len(matches) != want {
		t.Errorf("Glob(%s) = %v, %v; want %d file(s)", pattern, matches, err, want)
	}
}

// requireSpawns fails the test unless the shuttle ran want times.
func requireSpawns(t *testing.T, shuttle *shedfake.Shuttle, want int) {
	t.Helper()
	if got := len(shuttle.Specs); got != want {
		t.Errorf("shuttle runs = %d; want %d", got, want)
	}
}

// carryOverFindingKeys returns the keys of an entry's findings, in order.
func carryOverFindingKeys(entry discussionparser.CarryOver) []string {
	var keys []string
	for _, finding := range entry.Findings {
		keys = append(keys, finding.Key)
	}
	return keys
}

var (
	lowDecision      = reviewFixtureFinding{"D1", "LOW", "decision"}
	blockingDecision = reviewFixtureFinding{"D1", "BLOCKING", "decision"}
	blockingDesign   = reviewFixtureFinding{"F1", "BLOCKING", "design"}

	resolvedF1Ledger = taggedLedgerContent(2, taggedEntry{key: "F1", status: "resolved", class: "design", severity: "BLOCKING"})
)

// TestBouncer_DecisionEscalation covers a Bouncer told EscalateDecisions.
// A judged round whose review holds a decision-class finding escalates to the run's parent with cause decision, whatever the judge ruled.
// The parent's recorded accept passes the round over those findings and carries them into the carry-over, and a recorded continue runs another round.
// It also covers the refusal check an accept or a continue meets, the budget and hard-cap routes of a held refusal, and every path that must behave as before.
//
//testtiming:keep pins the decision cause's routing, snapshot, refusal and carry-over contract across the verdicts, the budget and the hard cap, which no other test reaches
func TestBouncer_DecisionEscalation(t *testing.T) {
	t.Parallel()

	t.Run("EveryVerdictOverADecisionFindingEscalatesWithTheRecordAndNoSettle", func(t *testing.T) {
		t.Parallel()
		for _, verdict := range []string{"CONVERGED", "CONTINUE", "CIRCLING"} {
			for _, withRecord := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s record %v", verdict, withRecord), func(t *testing.T) {
					t.Parallel()
					seams := &circlingSeams{}
					b, cfg, shuttle := decisionBouncerFor(t, verdict, decisionReviewContent(lowDecision), bouncerLedgerContent(2), seams.install)
					if withRecord {
						writeAddedDecisions(t, cfg, "Earlier answer")
					}

					ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
					if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
						t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
					}
					requireReasonContains(t, ptr.Reason, "round 2", "cause: decision", "lym loom decision add my-task", "lym loom circling accept my-task", "lym loom circling continue my-task", "lym loom resume")
					brief := requireDecisionEscalation(t, cfg)
					for _, want := range []string{"D1", "lym loom decision add my-task", "lym loom circling continue my-task", "lym loom resume"} {
						if !strings.Contains(brief, want) {
							t.Errorf("escalation brief = %q; want it to contain %q", brief, want)
						}
					}
					wantSnapshot := "titles: []\n"
					if withRecord {
						wantSnapshot = "titles:\n    - Earlier answer\n"
					}
					if got, err := os.ReadFile(decisionSnapshotPath(cfg.RunDir, 2)); err != nil || string(got) != wantSnapshot {
						t.Errorf("decision snapshot = %q, %v; want %q", got, err, wantSnapshot)
					}
					if len(seams.order) != 0 {
						t.Errorf("seam calls = %v; want no Approve or Commit", seams.order)
					}
					wantSpawns := 0
					if verdict == "CONVERGED" {
						wantSpawns = 1
					}
					requireSpawns(t, shuttle, wantSpawns)
					for _, path := range []string{verdictPath(cfg.RunDir, 2), ledgerPath(cfg.RunDir, 2)} {
						if _, err := os.Stat(path); err != nil {
							t.Errorf("Stat(%s) = %v; want the judge output left in place", path, err)
						}
					}
				})
			}
		}
	})

	t.Run("ABlockingDecisionFindingEscalatesWithoutAReJudgeAndItsAcceptIsNotRefused", func(t *testing.T) {
		t.Parallel()
		seams := &circlingSeams{}
		carry := &carryOverRecorder{}
		ledger := taggedLedgerContent(2, taggedEntry{key: "D1", status: "open", class: "decision", severity: "BLOCKING"})
		b, cfg, shuttle := decisionBouncerFor(t, "CONVERGED", decisionReviewContent(blockingDecision), ledger, func(c *BouncerConfig) { seams.install(c); carry.install(c) })

		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		requireDecisionEscalation(t, cfg)
		requireSpawns(t, shuttle, 1)

		recordDecision(t, cfg, CirclingAccept)
		shedfake.RequireOutcome(t, b, shedengine.Done)
		requireSpawns(t, shuttle, 1)
		if got := strings.Join(seams.order, ","); got != "approve,commit" {
			t.Errorf("seam order = %q; want approve,commit", got)
		}
		if len(carry.entries) != 1 || !slices.Equal(carryOverFindingKeys(carry.entries[0]), []string{"D1"}) {
			t.Errorf("carry-over entries = %+v; want one entry holding D1 once", carry.entries)
		}
	})

	t.Run("AutoAcceptAtASpentBudgetEscalatesDecisionInsteadOfAccepting", func(t *testing.T) {
		t.Parallel()
		seams := &circlingSeams{}
		b, cfg, shuttle := judgedDecisionBouncer(t, decisionReviewContent(lowDecision), func(c *BouncerConfig) {
			c.AutoAccept = true
			c.Bounces = spentBounces
			seams.install(c)
		}, judgeAnswer{bouncerVerdictContent("CONTINUE"), bouncerLedgerContent(2)})

		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		requireDecisionEscalation(t, cfg)
		requireSpawns(t, shuttle, 1)
		requireFileAbsent(t, circlingDecisionPath(cfg.RunDir, 2))
		if len(seams.order) != 0 {
			t.Errorf("seam calls = %v; want none", seams.order)
		}
	})

	t.Run("AReCallReturnsTheSameAwaitingWithOrWithoutAChangedSubject", func(t *testing.T) {
		t.Parallel()
		shuttle := convergingShuttle(nil)
		h := newReentry(t, shuttle, func(h *reentry, cfg *BouncerConfig) {
			cfg.EscalateDecisions = true
			cfg.Slug = "my-task"
			cfg.WorktreeRoot = t.TempDir()
			cfg.DecisionRecordPath = filepath.Join(t.TempDir(), "decision-record.md")
		})
		layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: decisionReviewContent(lowDecision)}})

		first := shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		again := shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		if !reflect.DeepEqual(again, first) {
			t.Errorf("second Call() pointer = %+v; want %+v", again, first)
		}

		h.subject = Subject{Kind: SubjectDocument, Hash: "hash-2", Base: "base-2"}
		changed := shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		if !reflect.DeepEqual(changed, first) {
			t.Errorf("Call() over a changed subject pointer = %+v; want %+v", changed, first)
		}
		requireSpawns(t, shuttle, 1)
		assertNoArchivedRunDirSibling(t, h.cfg.RunDir)
	})

	t.Run("ARecordedContinueRunsAnotherRoundExemptOnlyAtASpentBudget", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name       string
			bounces    func() (int, int, bool, error)
			wantExempt bool
		}{
			{"below the budget", belowBounces, false},
			{"at a spent budget", spentBounces, true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b, cfg, _ := decisionBouncerFor(t, "CONVERGED", decisionReviewContent(lowDecision), bouncerLedgerContent(2), func(c *BouncerConfig) { c.Bounces = tt.bounces })
				shedfake.RequireOutcome(t, b, shedengine.Awaiting)
				recordDecision(t, cfg, CirclingContinue)

				ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
				if ptr.BudgetExempt != tt.wantExempt {
					t.Errorf("BudgetExempt = %v; want %v", ptr.BudgetExempt, tt.wantExempt)
				}
				if _, err := os.Stat(focusPath(cfg.RunDir, 3)); err != nil {
					t.Errorf("round-3 focus file = %v; want it present", err)
				}
				assertNoArchivedRunDirSibling(t, cfg.RunDir)
			})
		}
	})

	t.Run("ARecordedAcceptCarriesEveryDecisionFindingWithTheTitlesRecordedSinceTheEscalation", func(t *testing.T) {
		t.Parallel()
		seams := &circlingSeams{}
		carry := &carryOverRecorder{}
		review := decisionReviewContent(lowDecision, reviewFixtureFinding{"D2", "MEDIUM", "decision"})
		ledger := taggedLedgerContent(2, taggedEntry{key: "D2", status: "open", class: "design", severity: "MEDIUM"})
		b, cfg, _ := decisionBouncerFor(t, "CONVERGED", review, ledger, func(c *BouncerConfig) { seams.install(c); carry.install(c) })
		writeAddedDecisions(t, cfg, "Earlier answer")

		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		writeAddedDecisions(t, cfg, "Earlier answer", "Cache scope", "Second answer")
		recordDecision(t, cfg, CirclingAccept)
		shedfake.RequireOutcome(t, b, shedengine.Done)

		if got := strings.Join(seams.order, ","); got != "approve,commit" {
			t.Errorf("seam order = %q; want approve,commit", got)
		}
		if len(carry.entries) != 1 {
			t.Fatalf("carry-over entries = %d; want 1", len(carry.entries))
		}
		entry := carry.entries[0]
		if entry.Closing != discussionparser.CarryOverAccepted || !slices.Equal(carryOverFindingKeys(entry), []string{"D1", "D2"}) {
			t.Fatalf("carry-over entry = %+v; want an accepted entry holding D1 and D2 once each", entry)
		}
		wantSince := []string{"Cache scope", "Second answer"}
		for _, finding := range entry.Findings {
			if !slices.Equal(finding.AddedSince, wantSince) {
				t.Errorf("finding %s AddedSince = %v; want %v", finding.Key, finding.AddedSince, wantSince)
			}
		}
		if d1 := entry.Findings[0]; d1.Class != "decision" || d1.Severity != "LOW" {
			t.Errorf("D1 = %+v; want class decision and the review's severity LOW", d1)
		}
	})

	t.Run("ABlockingDesignFindingRefusesTheAcceptUntilAReJudgedRoundAdmitsIt", func(t *testing.T) {
		t.Parallel()
		seams := &circlingSeams{}
		carry := &carryOverRecorder{}
		review := decisionReviewContent(lowDecision, blockingDesign)
		b, cfg, shuttle := judgedDecisionBouncer(t, review, func(c *BouncerConfig) {
			seams.install(c)
			carry.install(c)
			c.Bounces = belowBounces
		}, judgeAnswer{bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(2)}, judgeAnswer{bouncerVerdictContent("CONTINUE"), resolvedF1Ledger})

		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		brief := requireDecisionEscalation(t, cfg)
		for _, want := range []string{"F1", `review finding "F1" is BLOCKING`} {
			if !strings.Contains(brief, want) {
				t.Errorf("escalation brief = %q; want it to contain %q", brief, want)
			}
		}

		recordDecision(t, cfg, CirclingAccept)
		ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
		if ptr.BudgetExempt {
			t.Error("the re-judge Stuck is BudgetExempt; want a counted bounce unit")
		}
		requireReasonContains(t, ptr.Reason, `review finding "F1" is BLOCKING`)
		for _, path := range []string{verdictPath(cfg.RunDir, 2), ledgerPath(cfg.RunDir, 2), focusPath(cfg.RunDir, 3)} {
			requireFileAbsent(t, path)
		}
		for _, path := range []string{escalationPath(cfg.RunDir, 2), circlingDecisionPath(cfg.RunDir, 2)} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("Stat(%s) = %v; want the record kept", path, err)
			}
		}
		if len(seams.order) != 0 {
			t.Errorf("seam calls = %v; want none before the accept is admitted", seams.order)
		}
		requireSpawns(t, shuttle, 1)

		shedfake.RequireOutcome(t, b, shedengine.Done)
		requireSpawns(t, shuttle, 2)
		if len(carry.entries) != 1 || !slices.Equal(carryOverFindingKeys(carry.entries[0]), []string{"D1", "F1"}) {
			t.Fatalf("carry-over entries = %+v; want one entry holding D1 and the BLOCKING F1 the ledger closed", carry.entries)
		}
		if f1 := carry.entries[0].Findings[1]; f1.Class != "design" || f1.Severity != "BLOCKING" || len(f1.AddedSince) != 0 {
			t.Errorf("F1 = %+v; want class design, severity BLOCKING and no answered-since titles", f1)
		}
	})

	t.Run("AHeldRefusalAtTheBudgetRetiresTheAcceptAndEscalatesAgainKeepingTheFirstSnapshot", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			prevLedger string
			review     string
			wantBrief  []string
		}{
			{
				name:      "a BLOCKING design finding",
				review:    decisionReviewContent(lowDecision, blockingDesign),
				wantBrief: []string{`review finding "F1" is BLOCKING`, "escalates the round again while the refusal holds", "lym loom circling continue my-task"},
			},
			{
				name:       "a dropped ledger key",
				prevLedger: taggedLedgerContent(1, taggedEntry{key: "K", status: "open", class: "design", severity: "MEDIUM"}),
				review:     decisionReviewContent(lowDecision),
				wantBrief:  []string{`the ledger drops key "K"`, "escalates the round again while the refusal holds", "A `continue` meets the same check, halts the segment at its budget"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b, cfg, _ := decisionBouncerFor(t, "CONVERGED", tt.review, bouncerLedgerContent(2), func(c *BouncerConfig) { c.Bounces = spentBounces })
				if tt.prevLedger != "" {
					if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(tt.prevLedger), 0o644); err != nil {
						t.Fatalf("WriteFile(ledger round 1) = %v; want nil", err)
					}
				}
				writeAddedDecisions(t, cfg, "Earlier answer")
				shedfake.RequireOutcome(t, b, shedengine.Awaiting)
				snapshot, err := os.ReadFile(decisionSnapshotPath(cfg.RunDir, 2))
				if err != nil {
					t.Fatalf("ReadFile(decision snapshot) = %v; want nil", err)
				}

				writeAddedDecisions(t, cfg, "Earlier answer", "Later answer")
				recordDecision(t, cfg, CirclingAccept)
				ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
				requireReasonContains(t, ptr.Reason, "cause: decision", "lym loom decision add my-task")
				brief := requireDecisionEscalation(t, cfg)
				for _, want := range tt.wantBrief {
					if !strings.Contains(brief, want) {
						t.Errorf("escalation brief = %q; want it to contain %q", brief, want)
					}
				}
				if got, err := os.ReadFile(decisionSnapshotPath(cfg.RunDir, 2)); err != nil || string(got) != string(snapshot) {
					t.Errorf("decision snapshot = %q, %v; want the first one, %q", got, err, snapshot)
				}
				requireMatches(t, cfg.RunDir, "round-2-circling-decision-*.md", 1)
				requireFileAbsent(t, circlingDecisionPath(cfg.RunDir, 2))

				recordDecision(t, cfg, CirclingContinue)
				if tt.prevLedger != "" {
					// The same dropped key refuses the continue, which re-judges the round.
					ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
					requireReasonContains(t, ptr.Reason, `the ledger drops key "K"`)
					return
				}
				shedfake.RequireOutcome(t, b, shedengine.Stuck)
				focus, err := os.ReadFile(focusPath(cfg.RunDir, 3))
				if err != nil {
					t.Fatalf("ReadFile(round-3 focus) = %v; want nil", err)
				}
				if parsed, err := parseFocus(focus); err != nil || len(parsed.Focus) != 0 {
					t.Errorf("round-3 focus = %+v, %v; want an empty one", parsed, err)
				}
			})
		}
	})

	t.Run("ARecordedAcceptAtTheHardCapOverAHeldRefusalHaltsAndAResumeReJudges", func(t *testing.T) {
		t.Parallel()
		review := decisionReviewContent(lowDecision, blockingDesign)
		b, cfg, shuttle := judgedDecisionBouncer(t, review, func(c *BouncerConfig) {
			c.HardCap = 2
			c.Bounces = spentBounces
		}, judgeAnswer{bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(2)}, judgeAnswer{bouncerVerdictContent("CONTINUE"), resolvedF1Ledger})
		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		recordDecision(t, cfg, CirclingAccept)
		brief := requireDecisionEscalation(t, cfg)

		ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
		if !ptr.Halt || !strings.HasPrefix(ptr.Reason, DecisionHaltPrefix) {
			t.Fatalf("pointer = %+v; want a Halt whose Reason opens with %q", ptr, DecisionHaltPrefix)
		}
		requireReasonContains(t, ptr.Reason, "gate", "round 2", `review finding "F1" is BLOCKING`, "lym loom resume")
		if got := requireDecisionEscalation(t, cfg); got != brief {
			t.Errorf("escalation brief changed across the halt:\n%q\nwant\n%q", got, brief)
		}
		requireMatches(t, cfg.RunDir, "round-2-escalation-*.md", 0)
		requireFileAbsent(t, verdictPath(cfg.RunDir, 2))
		requireFileAbsent(t, ledgerPath(cfg.RunDir, 2))
		requireSpawns(t, shuttle, 1)

		shedfake.RequireOutcome(t, b, shedengine.Done)
		requireSpawns(t, shuttle, 2)
	})

	t.Run("ARecordedContinueWhoseLedgerDropsAKeyIsReJudged", func(t *testing.T) {
		t.Parallel()
		b, cfg, _ := decisionBouncerFor(t, "CONVERGED", decisionReviewContent(lowDecision), bouncerLedgerContent(2), nil)
		prev := taggedLedgerContent(1, taggedEntry{key: "K", status: "open", class: "design", severity: "MEDIUM"})
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(prev), 0o644); err != nil {
			t.Fatalf("WriteFile(ledger round 1) = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, b, shedengine.Awaiting)
		recordDecision(t, cfg, CirclingContinue)

		ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
		requireReasonContains(t, ptr.Reason, `the ledger drops key "K"`)
		if ptr.BudgetExempt {
			t.Error("the re-judge Stuck is BudgetExempt; want a counted bounce unit")
		}
		requireFileAbsent(t, verdictPath(cfg.RunDir, 2))
	})

	t.Run("AnEscalationAtTheHardCapOffersAcceptOnlyAndALoweredCapReEscalatesAContinue", func(t *testing.T) {
		t.Parallel()
		t.Run("at the cap", func(t *testing.T) {
			t.Parallel()
			b, cfg, _ := decisionBouncerFor(t, "CONVERGED", decisionReviewContent(lowDecision), bouncerLedgerContent(2), func(c *BouncerConfig) { c.HardCap = 2 })
			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			requireReasonContains(t, ptr.Reason, "lym loom decision add my-task", "lym loom circling accept my-task")
			brief := requireDecisionEscalation(t, cfg)
			if strings.Contains(ptr.Reason, "lym loom circling continue") || strings.Contains(brief, "lym loom circling continue") {
				t.Errorf("Reason = %q, brief = %q; want the accept verb alone", ptr.Reason, brief)
			}
			if !strings.Contains(brief, "lym loom circling accept my-task") {
				t.Errorf("escalation brief = %q; want the accept verb", brief)
			}
		})
		t.Run("a continue settled after the cap is lowered", func(t *testing.T) {
			t.Parallel()
			b, cfg, _ := decisionBouncerFor(t, "CONVERGED", decisionReviewContent(lowDecision), bouncerLedgerContent(2), nil)
			writeAddedDecisions(t, cfg, "Earlier answer")
			shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			snapshot, err := os.ReadFile(decisionSnapshotPath(cfg.RunDir, 2))
			if err != nil {
				t.Fatalf("ReadFile(decision snapshot) = %v; want nil", err)
			}
			recordDecision(t, cfg, CirclingContinue)

			lowered := *cfg
			lowered.HardCap = 2
			b, err = NewBouncer(lowered)
			if err != nil {
				t.Fatalf("NewBouncer(lowered) error = %v; want nil", err)
			}
			writeAddedDecisions(t, cfg, "Earlier answer", "Later answer")
			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			requireReasonContains(t, ptr.Reason, "cause: decision", "lym loom circling accept my-task")
			if strings.Contains(ptr.Reason, "lym loom circling continue") {
				t.Errorf("Reason = %q; want the accept verb alone at the lowered cap", ptr.Reason)
			}
			requireDecisionEscalation(t, cfg)
			if got, err := os.ReadFile(decisionSnapshotPath(cfg.RunDir, 2)); err != nil || string(got) != string(snapshot) {
				t.Errorf("decision snapshot = %q, %v; want the first one, %q", got, err, snapshot)
			}
			requireMatches(t, cfg.RunDir, "round-2-circling-decision-*.md", 1)
		})
	})

	t.Run("ASettledDecisionAcceptReSettlesOverAnUnchangedSubjectAndStartsAGenerationOverAChangedOne", func(t *testing.T) {
		t.Parallel()
		shuttle := convergingShuttle(nil)
		h := newReentry(t, shuttle, func(h *reentry, cfg *BouncerConfig) {
			cfg.EscalateDecisions = true
			cfg.FullGeneration = true
			cfg.Slug = "my-task"
			cfg.WorktreeRoot = t.TempDir()
			cfg.DecisionRecordPath = filepath.Join(t.TempDir(), "decision-record.md")
		})
		layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: decisionReviewContent(lowDecision)}})
		shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		if round, cause, err := RecordCirclingDecision(h.cfg.RunDir, CirclingAccept); err != nil || round != 1 || cause != EscalationDecision {
			t.Fatalf("RecordCirclingDecision(accept) = (%d, %q, %v); want (1, decision, nil)", round, cause, err)
		}
		shedfake.RequireOutcome(t, h.b, shedengine.Done)

		h.resume()
		shedfake.RequireOutcome(t, h.b, shedengine.Done)
		h.requireNoSpawn()
		assertNoArchivedRunDirSibling(t, h.cfg.RunDir)

		h.noSpawn = false
		h.subject = Subject{Kind: SubjectDocument, Hash: "hash-2", Base: "base-2"}
		ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
		if !ptr.GenerationStart {
			t.Errorf("pointer = %+v; want the new generation's start mark", ptr)
		}
		if generation := h.generation(); generation.Number != 2 {
			t.Errorf("generation record = %+v; want generation 2", generation)
		}
	})

	t.Run("AReplayOverASettledDecisionAcceptSettlesWithoutAReJudgeOverABlockingDecisionFinding", func(t *testing.T) {
		t.Parallel()
		shuttle := convergingShuttle(nil)
		h := newReentry(t, shuttle, func(h *reentry, cfg *BouncerConfig) {
			cfg.EscalateDecisions = true
			cfg.Slug = "my-task"
			cfg.WorktreeRoot = t.TempDir()
			cfg.DecisionRecordPath = filepath.Join(t.TempDir(), "decision-record.md")
		})
		layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: decisionReviewContent(blockingDecision)}})
		shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		if _, _, err := RecordCirclingDecision(h.cfg.RunDir, CirclingAccept); err != nil {
			t.Fatalf("RecordCirclingDecision(accept) error = %v; want nil", err)
		}
		shedfake.RequireOutcome(t, h.b, shedengine.Done)

		// A settle record of an earlier generation is not live, which sends the Call through the round-by-round replay.
		if err := writeGenerationRecord(h.cfg.RunDir, generationRecord{Number: 2}); err != nil {
			t.Fatalf("writeGenerationRecord() = %v; want nil", err)
		}
		h.resume()
		shedfake.RequireOutcome(t, h.b, shedengine.Done)
		h.requireNoSpawn()
		if _, err := os.Stat(verdictPath(h.cfg.RunDir, 1)); err != nil {
			t.Errorf("Stat(verdict) = %v; want the judgment left in place", err)
		}
	})

	t.Run("APathThatIsNotAnEscalationTakesTodaysRoute", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			verdict string
			review  string
			// tweak edits the config before the Bouncer is built, and prepare lays out state on disk after it.
			tweak     func(cfg *BouncerConfig)
			prepare   func(t *testing.T, cfg *BouncerConfig)
			wantState func(t *testing.T, cfg *BouncerConfig, outcome shedengine.Outcome, ptr shedengine.OutputPointer)
		}{
			{
				name:    "the switch off settles a converged round",
				verdict: "CONVERGED",
				review:  decisionReviewContent(lowDecision),
				tweak:   func(cfg *BouncerConfig) { cfg.EscalateDecisions = false },
				wantState: func(t *testing.T, cfg *BouncerConfig, outcome shedengine.Outcome, ptr shedengine.OutputPointer) {
					if outcome != shedengine.Done {
						t.Errorf("Call() outcome = %q; want Done", outcome)
					}
					requireNoEscalation(t, cfg)
				},
			},
			{
				name:    "a review that does not parse is refused and re-judged",
				verdict: "CONVERGED",
				review:  "not a review\n",
				wantState: func(t *testing.T, cfg *BouncerConfig, outcome shedengine.Outcome, ptr shedengine.OutputPointer) {
					if outcome != shedengine.Stuck {
						t.Errorf("Call() outcome = %q; want Stuck", outcome)
					}
					requireReasonContains(t, ptr.Reason, "the review does not parse")
					requireNoEscalation(t, cfg)
				},
			},
			{
				name:    "a round already escalated circling keeps its cause",
				verdict: "CONTINUE",
				review:  decisionReviewContent(lowDecision),
				tweak:   func(cfg *BouncerConfig) { cfg.Bounces = spentBounces },
				prepare: func(t *testing.T, cfg *BouncerConfig) {
					if err := writeEscalation(cfg.RunDir, 2, EscalationCircling, "brief\n", "notice"); err != nil {
						t.Fatalf("writeEscalation() = %v; want nil", err)
					}
				},
				wantState: func(t *testing.T, cfg *BouncerConfig, outcome shedengine.Outcome, ptr shedengine.OutputPointer) {
					if outcome != shedengine.Awaiting {
						t.Errorf("Call() outcome = %q; want Awaiting", outcome)
					}
					requireReasonContains(t, ptr.Reason, "cause: circling")
					requireEscalation(t, cfg, EscalationCircling)
					requireFileAbsent(t, decisionSnapshotPath(cfg.RunDir, 2))
				},
			},
			{
				name:    "an automatic budget continue settles as a continue",
				verdict: "CONTINUE",
				review:  decisionReviewContent(lowDecision),
				prepare: func(t *testing.T, cfg *BouncerConfig) {
					if err := recordAutoContinue(cfg.RunDir, 2); err != nil {
						t.Fatalf("recordAutoContinue() = %v; want nil", err)
					}
				},
				wantState: func(t *testing.T, cfg *BouncerConfig, outcome shedengine.Outcome, ptr shedengine.OutputPointer) {
					if outcome != shedengine.Stuck || !ptr.BudgetExempt {
						t.Errorf("Call() = %q, %+v; want a budget-exempt Stuck", outcome, ptr)
					}
					requireNoEscalation(t, cfg)
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				b, cfg, _ := decisionBouncerFor(t, tt.verdict, tt.review, bouncerLedgerContent(2), tt.tweak)
				if tt.prepare != nil {
					tt.prepare(t, cfg)
				}
				outcome, ptr := shedfake.CallOK(t, b)
				tt.wantState(t, cfg, outcome, ptr)
			})
		}
	})
}
