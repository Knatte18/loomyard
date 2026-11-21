// bouncer_amendment_test.go covers the Bouncer's review generation over a changed subject:
// the archive, the generation record, the amendment allowance and the note that scopes an amendment's review.

package shedadapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// amendmentRubricBody is a rubric that renders the amendment note, so a seed prompt shows it.
const amendmentRubricBody = "# Rubric\n\nAmendment: {{.amendment}}\n"

// amendmentDesignWrong is a ledger entry that keeps the soft-cap rule from accepting a round.
var amendmentDesignWrong = taggedEntry{"alpha", "open", "design", "MEDIUM", "design-wrong"}

// amendmentChangedSubject is a document subject with its own hash and base, the subject an amendment reviews.
var amendmentChangedSubject = Subject{Kind: SubjectDocument, Hash: "hash-2", Base: "base-2"}

// amendmentRow is one TestBouncer_Amendment scenario.
type amendmentRow struct {
	name string
	run  func(t *testing.T)
}

// verdictShuttle returns a shuttle whose judge run writes *verdict, the round's ledger and the next focus file.
// Its seed run writes nothing.
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

// newAmendment builds a reentry whose rubric renders the amendment note and whose judge writes *verdict.
// The judge writes the ledger from ledger, or bouncerLedgerContent when ledger is nil.
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

// withAmendConfig sets the amendment allowance and the hard cap, and the Bounces seam unless bounces is nil.
func withAmendConfig(allowance, hardCap int, bounces func() (int, int, bool, error)) reentryOption {
	return func(h *reentry, cfg *BouncerConfig) {
		cfg.AmendAllowance = allowance
		cfg.HardCap = hardCap
		if bounces != nil {
			cfg.Bounces = bounces
		}
	}
}

// withReviewInput makes the Bouncer read path as its one non-full review input, stamped offset after the harness's clock.
func withReviewInput(path string, offset time.Duration) reentryOption {
	return withReviewInputs(offset, ReviewInput{Path: path})
}

// withReviewInputs makes the Bouncer read inputs as its review inputs, each stamped offset after the harness's clock.
func withReviewInputs(offset time.Duration, inputs ...ReviewInput) reentryOption {
	return func(h *reentry, cfg *BouncerConfig) {
		cfg.ReviewInputs = func() ([]ReviewInput, error) {
			stamped := make([]ReviewInput, len(inputs))
			for i, input := range inputs {
				input.WrittenAt = h.now.Add(offset)
				stamped[i] = input
			}
			return stamped, nil
		}
	}
}

// hasInputPaths reports whether generation names exactly the review input paths in want, in order.
func hasInputPaths(generation generationRecord, want ...string) bool {
	return slices.Equal(generation.inputPaths(), want)
}

// requireFullGenerationOver fails the test unless the run directory's generation is generation 2,
// and a full one over paths.
// A full generation carries no Amendment mark and no base, and its review input note names each path.
func requireFullGenerationOver(t *testing.T, h *reentry, paths ...string) {
	t.Helper()
	generation := h.generation()
	if generation.Number != 2 || generation.Amendment || generation.Base != "" || !hasInputPaths(generation, paths...) {
		t.Errorf("generation record = %+v; want generation 2, not an amendment, over review inputs %v", generation, paths)
	}
	note := ReviewInputNote(h.cfg.RunDir)
	if !strings.Contains(note, "reviews the whole range") {
		t.Errorf("ReviewInputNote() = %q; want it to say the generation reviews the whole range", note)
	}
	for _, path := range paths {
		if !strings.Contains(note, path) {
			t.Errorf("ReviewInputNote() = %q; want it to name %q", note, path)
		}
	}
	if amendmentNote := AmendmentNote(h.cfg.RunDir); amendmentNote != noNote {
		t.Errorf("AmendmentNote() = %q; want %q for a full generation", amendmentNote, noNote)
	}
}

// layoutSettledRound1 lays out a settled round 1 in the run directory.
// It writes the converged verdict and ledger, a generation record, and a settle record over subject.
// The records are settled at the harness's clock.
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

// amendmentRoundsRow runs an amendment of stuckRounds CONTINUE rounds that return Stuck.
// Each round is budget-exempt exactly when wantExempt is set.
// When escalates is set, a further CONTINUE round escalates to the hard cap, offering accept alone.
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
		requireReasonContains(t, ptr.Reason, "cause: hard-cap", "lym loom circling accept")
		if strings.Contains(ptr.Reason, "lym loom circling continue") {
			t.Errorf("Reason = %q; want no continue verb at the hard cap", ptr.Reason)
		}
	}}
}

// amendmentInputRow runs a changed subject over a review input written offset after the settle.
// It checks whether the record, the seed prompt and the amendment note name that input.
func amendmentInputRow(name string, offset time.Duration, wantNamed bool) amendmentRow {
	const inputPath = "/review/review-input.md"
	return amendmentRow{name, func(t *testing.T) {
		verdict := "CONVERGED"
		h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInput(inputPath, offset))
		h.layoutSettledRound1(h.subject, nil)
		h.subject = amendmentChangedSubject
		shedfake.RequireOutcome(t, h.b, shedengine.Stuck)

		recorded := h.generation().inputPaths()
		named := strings.Contains(h.seedPrompts()[0], inputPath)
		noted := strings.Contains(AmendmentNote(h.cfg.RunDir), inputPath)
		if wantNamed != hasInputPaths(h.generation(), inputPath) || wantNamed != named || wantNamed != noted {
			t.Errorf("review inputs recorded = %v, named in the seed prompt = %v, named in the amendment note = %v; want it named: %v", recorded, named, noted, wantNamed)
		}
	}}
}

//testtiming:keep pins the new generation a changed subject starts: its archive and records, the allowance and hard cap, the note a seed reads, and the paths that degrade or return an error
func TestBouncer_Amendment(t *testing.T) {
	tiergate.Require(t, 0)
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
			if generation := h.generation(); generation.Number != 2 || !generation.Amendment || generation.Base != "base-1" || !hasInputPaths(generation) {
				t.Errorf("generation record = %+v; want generation 2, an amendment on base base-1 with no review input", generation)
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
			if generation := h.generation(); generation.Number != 2 || generation.Amendment || generation.Base != "" || !hasInputPaths(generation) {
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
		{"ReviewInputErrorIsReturnedWithNothingArchived", func(t *testing.T) {
			sentinel := errors.New("review input unreadable")
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), func(h *reentry, cfg *BouncerConfig) {
				cfg.ReviewInputs = func() ([]ReviewInput, error) { return nil, sentinel }
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
			permkit.Chmod(t, parent, 0o555)

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
			const inputPath = "/review/review-input.md"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInput(inputPath, -time.Hour))
			layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1),
			}})
			judged := generationRecord{Number: 1, JudgeSubject: h.subject, JudgeBase: "judge-base", JudgeAt: h.now.UTC().Format(time.RFC3339)}
			if err := writeGenerationRecord(h.cfg.RunDir, judged); err != nil {
				t.Fatalf("writeGenerationRecord() = %v; want nil", err)
			}
			h.subject = amendmentChangedSubject

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			if generation := h.generation(); generation.Number != 2 || !generation.Amendment || generation.Base != "judge-base" || !hasInputPaths(generation) {
				t.Errorf("generation record = %+v; want generation 2, an amendment on judge-base with no review input older than the judge", generation)
			}
			h.requireSeedPromptHolds("since commit judge-base")
			if strings.Contains(h.seedPrompts()[0], inputPath) {
				t.Errorf("seed prompt names %q; want a review input older than the judge left out", inputPath)
			}
		}},
		{"RetiredRejectReviewKeyRendersANoteNamingNoInput", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			retired := `{"number":2,"amendment":true,"base":"base-1","reject_review":"/review/retired.md"}`
			if err := os.MkdirAll(h.cfg.RunDir, 0o755); err != nil {
				t.Fatalf("create run directory = %v; want nil", err)
			}
			if err := os.WriteFile(filepath.Join(h.cfg.RunDir, generationRecordFile), []byte(retired), 0o644); err != nil {
				t.Fatalf("write generation record = %v; want nil", err)
			}
			note := AmendmentNote(h.cfg.RunDir)
			if !strings.Contains(note, "since commit base-1") || strings.Contains(note, "retired.md") || strings.Contains(note, "review input") {
				t.Errorf("AmendmentNote() = %q; want the amendment scope and no input named", note)
			}
		}},
		{"SendBackInputOverAnUnchangedSubjectStartsAFullGeneration", func(t *testing.T) {
			const sendBack = "/scratch/pr-findings-1.md"
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInputs(time.Hour, ReviewInput{Path: sendBack, Full: true}))
			if err := h.settleRoundOne(nil); err != nil {
				t.Fatalf("first Call() error = %v; want nil", err)
			}

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			requireFullGenerationOver(t, h, sendBack)
		}},
		{"SendBackInputAfterAnAllowanceZeroSettleStartsAFullGeneration", func(t *testing.T) {
			const sendBack = "/scratch/pr-findings-1.md"
			verdict := "CONVERGED"
			var inputs []ReviewInput
			h := newAmendment(t, &verdict, nil, withAmendConfig(0, 0, nil), func(h *reentry, cfg *BouncerConfig) {
				cfg.ReviewInputs = func() ([]ReviewInput, error) { return inputs, nil }
			})
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject
			shedfake.RequireOutcome(t, h.b, shedengine.Done)
			if record := h.settleRecord(); record.Generation != 2 || record.Round != 0 {
				t.Fatalf("settle record = generation %d round %d; want the round-0 record of generation 2", record.Generation, record.Round)
			}

			inputs = []ReviewInput{{Path: sendBack, WrittenAt: h.now.Add(time.Hour), Full: true}}
			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			generation := h.generation()
			if generation.Number != 3 || generation.Amendment || !hasInputPaths(generation, sendBack) {
				t.Errorf("generation record = %+v; want generation 3, not an amendment, over %q", generation, sendBack)
			}
		}},
		{"SendBackInputStartsAFullGenerationWhateverFullGenerationSays", func(t *testing.T) {
			const sendBack = "/scratch/pr-findings-1.md"
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInputs(time.Hour, ReviewInput{Path: sendBack, Full: true}), func(h *reentry, cfg *BouncerConfig) {
				cfg.FullGeneration = true
			})
			if err := h.settleRoundOne(nil); err != nil {
				t.Fatalf("first Call() error = %v; want nil", err)
			}

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			requireFullGenerationOver(t, h, sendBack)
		}},
		{"SendBackInputAndFixerReportBothStartAFullGenerationHoldingBoth", func(t *testing.T) {
			const sendBack = "/scratch/pr-findings-1.md"
			const fixerReport = "/scratch/Parent-Align-fixer-report.md"
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInputs(time.Hour, ReviewInput{Path: fixerReport}, ReviewInput{Path: sendBack, Full: true}))
			if err := h.settleRoundOne(nil); err != nil {
				t.Fatalf("first Call() error = %v; want nil", err)
			}

			shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
			requireFullGenerationOver(t, h, fixerReport, sendBack)
		}},
		{"SendBackInputOverAChangedSubjectStartsAFullGeneration", func(t *testing.T) {
			const sendBack = "/scratch/pr-findings-1.md"
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil), withReviewInputs(time.Hour, ReviewInput{Path: sendBack, Full: true}))
			h.layoutSettledRound1(h.subject, nil)
			h.subject = amendmentChangedSubject

			if ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck); !ptr.GenerationStart {
				t.Error("seed Stuck GenerationStart = false; want true")
			}
			requireFullGenerationOver(t, h, sendBack)
		}},
		{"AmendmentRecordHoldingAReviewInputRendersNoReviewInputNote", func(t *testing.T) {
			verdict := "CONVERGED"
			h := newAmendment(t, &verdict, nil, withAmendConfig(2, 0, nil))
			held := `{"number":2,"amendment":true,"base":"base-1","review_inputs":["/review/held.md"]}`
			if err := os.MkdirAll(h.cfg.RunDir, 0o755); err != nil {
				t.Fatalf("create run directory = %v; want nil", err)
			}
			if err := os.WriteFile(filepath.Join(h.cfg.RunDir, generationRecordFile), []byte(held), 0o644); err != nil {
				t.Fatalf("write generation record = %v; want nil", err)
			}

			if generation := h.generation(); !generation.Amendment || !hasInputPaths(generation, "/review/held.md") {
				t.Fatalf("generation record = %+v; want an amendment holding the review input /review/held.md", generation)
			}
			if note := ReviewInputNote(h.cfg.RunDir); note != noNote {
				t.Errorf("ReviewInputNote() = %q; want %q for an amendment, whatever input it holds", note, noNote)
			}
		}},
		amendmentInputRow("InputWrittenAfterTheSettleIsNamed", time.Hour, true),
		amendmentInputRow("InputWrittenBeforeTheSettleIsNotNamed", -time.Hour, false),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
