// bouncer_amendment_test.go covers the Bouncer's new review generation over a changed subject: the archive, the generation record, the amendment allowance and the note that scopes an amendment's review.

package shedadapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// amendmentRubricBody is a rubric that renders the amendment note, so a seed prompt shows it.
const amendmentRubricBody = "# Rubric\n\nAmendment: {{.amendment}}\n"

// amendmentDesignWrong is a ledger entry that keeps the soft-cap rule from accepting a round.
var amendmentDesignWrong = taggedEntry{"alpha", "open", "design", "MEDIUM", "design-wrong"}

// amendmentChangedSubject is a document subject that differs from the reentry harness's default.
var amendmentChangedSubject = Subject{Kind: SubjectDocument, Hash: "hash-2", Base: "base-2"}

// amendmentRow is one TestBouncer_Amendment scenario.
type amendmentRow struct {
	name string
	run  func(t *testing.T)
}

// verdictShuttle returns a shuttle whose judge run writes *verdict, the ledger ledger renders for the round and the next round's focus file, and whose seed run writes nothing.
func verdictShuttle(verdict *string, ledger func(round int) string) *shedfake.Shuttle {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.DuringRun = func() {
		outputs := shuttle.GotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		round, _ := OutputRound(outputs[0])
		_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent(*verdict)), 0o644)
		_ = os.WriteFile(outputs[1], []byte(ledger(round)), 0o644)
		_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
	}
	return shuttle
}

// newAmendment builds a reentry whose rubric renders the amendment note and whose judge writes *verdict over the ledger, bouncerLedgerContent when ledger is nil.
func newAmendment(t *testing.T, verdict *string, ledger func(round int) string, opts ...reentryOption) *reentry {
	t.Helper()
	if ledger == nil {
		ledger = bouncerLedgerContent
	}
	rubric := func(h *reentry, cfg *BouncerConfig) {
		writeBouncerStencils(h.t, cfg.StencilsDir, map[string]string{cfg.RubricStencil: amendmentRubricBody})
	}
	return newReentry(t, verdictShuttle(verdict, ledger), append([]reentryOption{rubric}, opts...)...)
}

// withAmendConfig tells the Bouncer an amendment allowance, a hard cap and a Bounces seam, which nil leaves unchanged.
func withAmendConfig(allowance, hardCap int, bounces func() (int, int, bool, error)) reentryOption {
	return func(h *reentry, cfg *BouncerConfig) {
		cfg.AmendAllowance = allowance
		cfg.HardCap = hardCap
		if bounces != nil {
			cfg.Bounces = bounces
		}
	}
}

// withRejectReview tells the Bouncer a reject review at path that happened offset after the harness's clock when the Call reads it.
func withRejectReview(path string, offset time.Duration) reentryOption {
	return func(h *reentry, cfg *BouncerConfig) {
		cfg.RejectReview = func() (string, time.Time, bool, error) { return path, h.now.Add(offset), true, nil }
	}
}

// layoutSettledRound1 lays out a settled round 1 in the run directory: its converged verdict and ledger, a generation record and a settle record over subject, settled at the harness's clock.
func (h *reentry) layoutSettledRound1(subject Subject, patch []byte) {
	h.t.Helper()
	layoutBouncerRun(h.t, h.cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1),
	}})
	settledAt := h.now.UTC().Format(time.RFC3339)
	if err := writeGenerationRecord(h.cfg.RunDir, generationRecord{Number: 1, JudgeSubject: subject, JudgeBase: subject.Base, JudgeAt: settledAt}); err != nil {
		h.t.Fatalf("writeGenerationRecord() = %v; want nil", err)
	}
	record := settleRecord{Subject: subject, Base: subject.Base, Generation: 1, Round: 1, SettledAt: settledAt, Settled: true}
	if err := writeSettleRecord(h.cfg.RunDir, record, patch); err != nil {
		h.t.Fatalf("writeSettleRecord() = %v; want nil", err)
	}
}

func (h *reentry) generation() generationRecord {
	h.t.Helper()
	generation, found, err := readGenerationRecord(h.cfg.RunDir)
	if err != nil || !found {
		h.t.Fatalf("readGenerationRecord() = %+v, %v, %v; want a record", generation, found, err)
	}
	return generation
}

// seedPrompts returns the prompt of every seed spawn the shuttle ran, in order.
func (h *reentry) seedPrompts() []string {
	var prompts []string
	for _, spec := range h.shuttle.Specs {
		if spec.Role == bouncerSeedRole {
			prompts = append(prompts, spec.Prompt)
		}
	}
	return prompts
}

// archivedSiblings counts the archived siblings of runDir.
func archivedSiblings(t *testing.T, runDir string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(runDir))
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v; want nil", filepath.Dir(runDir), err)
	}
	count := 0
	for _, entry := range entries {
		if entry.Name() != filepath.Base(runDir) && strings.HasPrefix(entry.Name(), filepath.Base(runDir)+"-") {
			count++
		}
	}
	return count
}

// requireSeedPromptHolds fails the test unless the only seed prompt holds each of wants.
func (h *reentry) requireSeedPromptHolds(wants ...string) {
	h.t.Helper()
	prompts := h.seedPrompts()
	if len(prompts) != 1 {
		h.t.Fatalf("seed spawns = %d; want 1", len(prompts))
	}
	for _, want := range wants {
		if !strings.Contains(prompts[0], want) {
			h.t.Errorf("seed prompt = %q; want it to contain %q", prompts[0], want)
		}
	}
}

// amendmentRoundsRow runs an amendment of stuckRounds CONTINUE rounds that return Stuck, each budget-exempt or not as wantExempt says.
// When escalates is set, a further CONTINUE round escalates hard-cap offering accept alone.
func amendmentRoundsRow(name string, allowance, hardCap int, bounces func() (int, int, bool, error), stuckRounds int, wantExempt, escalates bool) amendmentRow {
	return amendmentRow{name, func(t *testing.T) {
		verdict := "CONTINUE"
		ledger := func(round int) string { return taggedLedgerContent(round, amendmentDesignWrong) }
		h := newAmendment(t, &verdict, ledger, withAmendConfig(allowance, hardCap, bounces))
		h.layoutSettledRound1(h.subject, nil)
		h.subject = amendmentChangedSubject
		shedfake.RequireOutcome(t, h.b, shedengine.Stuck)

		for round := 1; round <= stuckRounds; round++ {
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: round, report: bouncerReport(round)}})
			ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			if ptr.BudgetExempt != wantExempt {
				t.Errorf("round %d BudgetExempt = %v; want %v", round, ptr.BudgetExempt, wantExempt)
			}
		}
		if !escalates {
			return
		}
		round := stuckRounds + 1
		layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: round, report: bouncerReport(round)}})
		ptr := shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
		requireReasonContains(t, ptr.Reason, "cause: hard-cap", "lyx loom circling accept")
		if strings.Contains(ptr.Reason, "lyx loom circling continue") {
			t.Errorf("Reason = %q; want no continue verb at the hard cap", ptr.Reason)
		}
	}}
}

// amendmentRejectRow runs a changed subject over a reject that happened offset after the settle, and checks whether the record and the seed prompt name its review file.
func amendmentRejectRow(name string, offset time.Duration, wantNamed bool) amendmentRow {
	const rejectPath = "/review/pr-gate-reject.md"
	return amendmentRow{name, func(t *testing.T) {
		verdict := "CONVERGED"
		h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withRejectReview(rejectPath, offset))
		h.layoutSettledRound1(h.subject, nil)
		h.subject = amendmentChangedSubject
		shedfake.RequireOutcome(t, h.b, shedengine.Stuck)

		recorded := h.generation().RejectReview
		named := strings.Contains(h.seedPrompts()[0], rejectPath)
		if wantNamed != (recorded == rejectPath) || wantNamed != named {
			t.Errorf("reject review recorded = %q, named in the seed prompt = %v; want it named: %v", recorded, named, wantNamed)
		}
	}}
}

//testtiming:keep pins the new generation a changed subject starts: its archive and records, the allowance and hard cap, the note a seed reads, and the paths that degrade or return an error
func TestBouncer_Amendment(t *testing.T) {
	t.Parallel()
	codeSubject := Subject{Kind: SubjectCode, MergeBase: "merge-base-1", Base: "merge-base-1", Head: "head-1", FullHash: "full-1", PatchID: "patch-1", Patch: []byte("settled diff")}

	tests := []amendmentRow{
		{"ChangedSubjectArchivesStartsAnAmendmentAndJudgesWithoutASecondArchive", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			if _, err := os.Stat(archivedRunDirPath(h.cfg.RunDir, h.now, "")); err != nil {
				t.Errorf("archived run directory missing: %v; want the generation archived", err)
			}
			if generation := h.generation(); generation.Number != 2 || !generation.Amendment || generation.Base != "base-1" || generation.RejectReview != "" {
				t.Errorf("generation record = %+v; want generation 2, an amendment on base base-1 with no reject", generation)
			}
			h.requireSeedPromptHolds("since commit base-1")

			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if got := archivedSiblings(t, h.cfg.RunDir); got != 1 {
				t.Errorf("archived siblings = %d; want 1, the second Call archives nothing", got)
			}
			if record := h.settleRecord(); record.Generation != 2 {
				t.Errorf("settle record generation = %d; want 2", record.Generation)
			}
		}},
		amendmentRoundsRow("AllowanceOneWithCountOneRunsTheSoftCapRuleAndEscalatesHardCapAtRoundTwo", 1, 0, func() (int, int, bool, error) { return 1, 3, true, nil }, 1, true, true),
		amendmentRoundsRow("AllowanceAboveTheBounceBudgetRunsTheSoftCapRuleAtThePerchBudget", 3, 0, func() (int, int, bool, error) { return 2, 2, true, nil }, 1, true, false),
		amendmentRoundsRow("HardCapClampsTheAllowanceHardCap", 3, 3, func() (int, int, bool, error) { return 0, 5, true, nil }, 2, false, true),
		{"AllowanceZeroSettlesWithoutAReviewAndAnUnchangedReentryIsDone", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(0, 0, nil))
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject

			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if record := h.settleRecord(); record.Generation != 2 || record.Round != 0 || !record.Settled {
				t.Errorf("settle record = generation %d round %d settled %t; want a settled round-0 record of generation 2", record.Generation, record.Round, record.Settled)
			}
			if generation := h.generation(); generation.Number != 2 || !generation.Amendment {
				t.Errorf("generation record = %+v; want generation 2 marked amendment", generation)
			}

			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if len(h.shuttle.Specs) != 0 {
				t.Errorf("shuttle runs = %d; want none", len(h.shuttle.Specs))
			}
			if got := archivedSiblings(t, h.cfg.RunDir); got != 1 {
				t.Errorf("archived siblings = %d; want 1, the re-entry archives nothing", got)
			}
		}},
		{"FullGenerationCarriesTheNumberAloneAndKeepsThePerchBudget", func(t *testing.T) {
			verdict := "CONTINUE"
			ledger := func(round int) string { return taggedLedgerContent(round, amendmentDesignWrong) }
			h := newAmendment(t, &verdict, ledger, withAmendConfig(0, 0, func() (int, int, bool, error) { return 1, 3, true, nil }), func(h *reentry, cfg *BouncerConfig) {
				cfg.FullGeneration = true
			})
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			if generation := h.generation(); generation.Number != 2 || generation.Amendment || generation.Base != "" || generation.RejectReview != "" {
				t.Errorf("generation record = %+v; want generation 2 alone", generation)
			}
			h.requireSeedPromptHolds("Amendment: none")

			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); ptr.BudgetExempt {
				t.Error("round 1 BudgetExempt = true; want the perch's budget, not reached at count 1 of 3")
			}
			if _, _, _, exists, err := readCirclingDecision(h.cfg.RunDir, 1); err != nil || exists {
				t.Errorf("round 1 decision exists = %v, error %v; want none", exists, err)
			}
			if got := archivedSiblings(t, h.cfg.RunDir); got != 1 {
				t.Errorf("archived siblings = %d; want 1, the second Call archives nothing", got)
			}
		}},
		{"CodeAmendmentNamesTheStoredPatchAndTheWorkingTreeDiff", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			h.layoutSettledRound1(codeSubject, codeSubject.Patch)
			h.subject = Subject{Kind: SubjectCode, MergeBase: "merge-base-1", Base: "merge-base-1", Head: "head-2", FullHash: "full-2", PatchID: "patch-2"}

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			h.requireSeedPromptHolds(filepath.Join(h.cfg.RunDir, settlePatchFile), "working-tree diff against the current merge base")
			if patch, err := os.ReadFile(filepath.Join(h.cfg.RunDir, settlePatchFile)); err != nil || string(patch) != "settled diff" {
				t.Errorf("stored patch = %q, %v; want it carried into the new generation", patch, err)
			}
		}},
		{"RejectReviewErrorIsReturnedWithNothingArchived", func(t *testing.T) {
			sentinel := errors.New("reject unreadable")
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), func(h *reentry, cfg *BouncerConfig) {
				cfg.RejectReview = func() (string, time.Time, bool, error) { return "", time.Time{}, false, sentinel }
			})
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject

			if _, _, err := h.call(); !errors.Is(err, sentinel) {
				t.Errorf("Call() error = %v; want it to wrap %v", err, sentinel)
			}
			if got := archivedSiblings(t, h.cfg.RunDir); got != 0 {
				t.Errorf("archived siblings = %d; want none", got)
			}
			if len(h.shuttle.Specs) != 0 {
				t.Errorf("shuttle runs = %d; want none", len(h.shuttle.Specs))
			}
		}},
		{"FailedArchiveDegradesToAStuckWithTheRunDirectoryUntouched", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject
			parent := filepath.Dir(h.cfg.RunDir)
			if err := os.Chmod(parent, 0o555); err != nil {
				t.Fatalf("Chmod(parent) = %v; want nil", err)
			}
			t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); ptr.GenerationStart {
				t.Error("degraded Stuck GenerationStart = true; want false")
			}
			if len(h.shuttle.Specs) != 0 {
				t.Errorf("shuttle runs = %d; want none", len(h.shuttle.Specs))
			}
			if _, err := os.Stat(verdictPath(h.cfg.RunDir, 1)); err != nil {
				t.Errorf("round 1's verdict is gone: %v; want the run directory untouched", err)
			}
			if generation := h.generation(); generation.Number != 1 {
				t.Errorf("generation record number = %d; want 1", generation.Number)
			}
		}},
		{"SeedAfterAFailedCallCarriesTheMarkAndAReBounceDoesNot", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			// The state a Call leaves when it fails between its archive and its seed.
			settledAt := h.now.UTC().Format(time.RFC3339)
			if err := writeGenerationRecord(h.cfg.RunDir, generationRecord{Number: 2, Amendment: true, Base: "base-1"}); err != nil {
				t.Fatalf("writeGenerationRecord() = %v; want nil", err)
			}
			if err := writeSettleRecord(h.cfg.RunDir, settleRecord{Subject: h.subject, Base: "base-1", Generation: 1, Round: 1, SettledAt: settledAt, Settled: true}, nil); err != nil {
				t.Fatalf("writeSettleRecord() = %v; want nil", err)
			}
			h.subject = amendmentChangedSubject

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); ptr.GenerationStart {
				t.Error("re-bounce Stuck GenerationStart = true; want false")
			}
			if got := len(h.seedPrompts()); got != 1 {
				t.Errorf("seed spawns = %d; want 1", got)
			}
		}},
		{"InterruptedSettleAtASpentBudgetThenAChangedSubjectReturnsTheSeedStuck", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, func() (int, int, bool, error) { return 3, 3, true, nil }))
			if err := h.settleRoundOne(errReentryCommit); !errors.Is(err, errReentryCommit) {
				t.Fatalf("first Call() error = %v; want the commit failure", err)
			}
			h.commitErr = nil
			h.subject = amendmentChangedSubject

			ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			if !ptr.GenerationStart || ptr.BudgetExempt {
				t.Errorf("pointer = %+v; want the seed's Stuck marked GenerationStart", ptr)
			}
		}},
		{"JudgeSubjectChangeStartsTheAmendmentFromTheJudgeBase", func(t *testing.T) {
			verdict := "CONVERGED"
			const rejectPath = "/review/pr-gate-reject.md"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withRejectReview(rejectPath, -time.Hour))
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1),
			}})
			judged := generationRecord{Number: 1, JudgeSubject: h.subject, JudgeBase: "judge-base", JudgeAt: h.now.UTC().Format(time.RFC3339)}
			if err := writeGenerationRecord(h.cfg.RunDir, judged); err != nil {
				t.Fatalf("writeGenerationRecord() = %v; want nil", err)
			}
			h.subject = amendmentChangedSubject

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			if generation := h.generation(); generation.Number != 2 || !generation.Amendment || generation.Base != "judge-base" || generation.RejectReview != "" {
				t.Errorf("generation record = %+v; want generation 2, an amendment on judge-base with no reject older than the judge", generation)
			}
			h.requireSeedPromptHolds("since commit judge-base")
			if strings.Contains(h.seedPrompts()[0], rejectPath) {
				t.Errorf("seed prompt names %q; want a reject older than the judge left out", rejectPath)
			}
		}},
		amendmentRejectRow("RejectLaterThanTheSettleIsNamed", time.Hour, true),
		amendmentRejectRow("RejectEarlierThanTheSettleIsNotNamed", -time.Hour, false),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
