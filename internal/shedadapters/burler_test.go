package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/burlermarker"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// Compile-time proof that *shedfake.BurlerRunner satisfies BurlerRunner.
var _ BurlerRunner = (*shedfake.BurlerRunner)(nil)

// simpleBurlerProfile returns a minimal burlerengine.Profile suitable for passing through Call --
// its content fields are irrelevant since BurlerProducer never invokes burlerengine's own validate.
func simpleBurlerProfile() burlerengine.Profile {
	return burlerengine.Profile{
		Rubric:   "rubric text",
		FixScope: burlerengine.FixScopeOverlay,
	}
}

// writeRoundFile writes a placeholder file at path, creating its parent directory.
func writeRoundFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// writeRoundPair writes both of round n's own files under runDir, making it complete -- but not
// judged, which is a separate condition writeJudgedRound adds on top.
func writeRoundPair(t *testing.T, runDir string, n int) {
	t.Helper()
	writeRoundFile(t, roundReviewPath(runDir, n))
	writeRoundFile(t, roundFixerReportPath(runDir, n))
}

// writeJudgedRound writes round n's own two files plus the CONTINUE verdict and ledger the segment's Bouncer writes when it rejects that round -- the complete on-disk state of a round the producer may advance past.
// The verdict is CONTINUE rather than CONVERGED because a CONVERGED round is one the segment left on a Done, never one the round producer is called after.
func writeJudgedRound(t *testing.T, runDir string, n int) {
	t.Helper()
	writeRoundPair(t, runDir, n)
	if err := os.WriteFile(verdictPath(runDir, n), []byte(bouncerVerdictContent("CONTINUE")), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict round %d): %v", n, err)
	}
	if err := os.WriteFile(ledgerPath(runDir, n), []byte(bouncerLedgerContent(n)), 0o644); err != nil {
		t.Fatalf("WriteFile(ledger round %d): %v", n, err)
	}
}

// --- Constructor ---

func TestNewBurlerProducer_Validation(t *testing.T) {
	dir := t.TempDir()
	profile := simpleBurlerProfile()

	tests := []struct {
		name    string
		runner  BurlerRunner
		remover burlerengine.StrandRemover
		pname   string
		runDir  string
		anchor  string
		wantErr bool
	}{
		{"Valid", &shedfake.BurlerRunner{}, &shedfake.StrandRemover{}, "burler", filepath.Join(dir, "runs"), dir, false},
		{"NilRunner", nil, &shedfake.StrandRemover{}, "burler", filepath.Join(dir, "runs"), dir, true},
		{"NilRemover", &shedfake.BurlerRunner{}, nil, "burler", filepath.Join(dir, "runs"), dir, true},
		{"EmptyName", &shedfake.BurlerRunner{}, &shedfake.StrandRemover{}, "", filepath.Join(dir, "runs"), dir, true},
		{"EmptyRunDir", &shedfake.BurlerRunner{}, &shedfake.StrandRemover{}, "burler", "", dir, true},
		{"RelativeRunDir", &shedfake.BurlerRunner{}, &shedfake.StrandRemover{}, "burler", "relative/runs", dir, true},
		{"RelativeAnchorPath", &shedfake.BurlerRunner{}, &shedfake.StrandRemover{}, "burler", filepath.Join(dir, "runs"), "relative/anchor", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewBurlerProducer(tt.pname, BurlerDeps{Runner: tt.runner, Remover: tt.remover, AnchorPath: tt.anchor}, profile, burlerengine.RunOpts{}, tt.runDir, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NewBurlerProducer() error = nil; want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewBurlerProducer() error = %v; want nil", err)
			}
			if p == nil {
				t.Fatal("NewBurlerProducer() producer = nil; want non-nil")
			}
		})
	}
}

// --- Round-scan ---

func TestBurlerProducer_RoundScan(t *testing.T) {
	t.Run("AbsentRunDirIsCreatedAndStartsAtOne", func(t *testing.T) {
		base := t.TempDir()
		runDir := filepath.Join(base, "runs")
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if _, err := os.Stat(runDir); err != nil {
			t.Errorf("run dir was not created: %v", err)
		}
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.GotOpts[0].Round, "1")
		}
		if runner.GotProfiles[0].ReviewPath != roundReviewPath(runDir, 1) {
			t.Errorf("review path = %q; want %q", runner.GotProfiles[0].ReviewPath, roundReviewPath(runDir, 1))
		}
	})

	t.Run("EmptyRunDirStartsAtOne", func(t *testing.T) {
		runDir := t.TempDir()
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.GotOpts[0].Round, "1")
		}
	})

	t.Run("JudgedRoundNAdvancesToNPlus1", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 2)
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "3" {
			t.Errorf("round token = %q; want %q", runner.GotOpts[0].Round, "3")
		}
		if runner.GotProfiles[0].ReviewPath != roundReviewPath(runDir, 3) {
			t.Errorf("review path = %q; want %q", runner.GotProfiles[0].ReviewPath, roundReviewPath(runDir, 3))
		}
	})

	// The three unjudged cases below are one regression: the segment's Bouncer routes here on every
	// Stuck it returns, degraded judge exits included, so a transient judge fault used to buy a whole
	// extra fixer round over a review nobody had judged -- and left round N+1's judge with no round-N
	// ledger to carry findings forward from.
	t.Run("CompleteButUnjudgedRoundHandsBackWithoutSpawning", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1) // complete, but the Bouncer wrote no verdict for it
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner, withBurlerClock(fixedClock(time.Now())))

		ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if want := roundReviewPath(runDir, 1); ptr.Path != want {
			t.Errorf("Call() pointer = %q; want %q (the review still owed a verdict)", ptr.Path, want)
		}
		if want := "round 1 is complete but unjudged; handing back for judgment"; ptr.Reason != want {
			t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
		}
		if runner.Calls != 0 {
			t.Errorf("runner.Run calls = %d; want 0 -- a fresh round is a real LLM session spent on a review nobody judged", runner.Calls)
		}
		if runner.ProbeCalls != 0 {
			t.Error("ProbeRound was called; want the hand-back to precede the live-round probe, since no round is being started")
		}
		if n := stampedSiblingCount(t, runDir, filepath.Base(roundReviewPath(runDir, 1))); n != 0 {
			t.Errorf("stamped archive siblings = %d; want 0 -- the unjudged round's own artifacts are what the Bouncer must judge", n)
		}
	})

	t.Run("UnparseableVerdictCountsUnjudged", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		if err := os.WriteFile(verdictPath(runDir, 1), []byte("garbage, not frontmatter"), 0o644); err != nil {
			t.Fatalf("WriteFile(unparseable verdict): %v", err)
		}
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if runner.Calls != 0 {
			t.Errorf("runner.Run calls = %d; want 0 -- a verdict the Bouncer will itself re-judge is no verdict", runner.Calls)
		}
	})

	t.Run("VerdictWithoutLedgerCountsUnjudged", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		if err := os.Remove(ledgerPath(runDir, 1)); err != nil {
			t.Fatalf("Remove(ledger): %v", err)
		}
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if runner.Calls != 0 {
			t.Errorf("runner.Run calls = %d; want 0 -- advancing here leaves the next judge with no ledger to carry findings forward from", runner.Calls)
		}
	})

	t.Run("UnjudgedRoundHandsBackHonouringCancellation", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, _, err := p.Call(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
		}
	})

	t.Run("ReviewAbsentFixerReportPresentRerunsSameN", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, roundFixerReportPath(runDir, 1))
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.GotOpts[0].Round, "1")
		}
	})

	t.Run("ReviewOnlyOrphanCountsIncompleteAndIsArchivedAside", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		writeRoundFile(t, roundReviewPath(runDir, 2)) // orphan: no round-2 fixer report

		instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
		wantStamped := filepath.Join(runDir, "round-2-review-20260820T101500Z.md")
		// The orphan must be archived aside before the runner is invoked, never during or after.
		var orphanGoneAtInvoke, stampedExistsAtInvoke bool
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		runner.DuringRun = func(int) {
			_, err := os.Stat(roundReviewPath(runDir, 2))
			orphanGoneAtInvoke = os.IsNotExist(err)
			_, err = os.Stat(wantStamped)
			stampedExistsAtInvoke = err == nil
		}
		p := newBurlerProducer(t, runDir, runner, withBurlerClock(fixedClock(instant)))

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "2" {
			t.Errorf("round token = %q; want %q (re-run, not skipped)", runner.GotOpts[0].Round, "2")
		}
		if _, err := os.Stat(wantStamped); err != nil {
			t.Errorf("orphan was not archived aside to %s: %v", wantStamped, err)
		}
		if !orphanGoneAtInvoke {
			t.Error("orphan review file still present at the round's own path when the runner was invoked")
		}
		if !stampedExistsAtInvoke {
			t.Error("stamped archive sibling did not exist by the time the runner was invoked")
		}
	})

	t.Run("UnrelatedFilesUnaffected", func(t *testing.T) {
		runDir := t.TempDir()
		notes := filepath.Join(runDir, "notes.txt")
		writeRoundFile(t, notes)
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.GotOpts[0].Round, "1")
		}
		if _, err := os.Stat(notes); err != nil {
			t.Errorf("unrelated file was disturbed: %v", err)
		}
	})

	t.Run("StampedArchiveSiblingNeverShiftsRound", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, filepath.Join(runDir, "round-2-review-20260820T101500Z.md"))
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q (stamped sibling never adopted)", runner.GotOpts[0].Round, "1")
		}
	})

	t.Run("NonNumericZeroPaddedAndAttemptSuffixedTokensIgnored", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, filepath.Join(runDir, "round-abc-review.md"))
		writeRoundFile(t, filepath.Join(runDir, "round-abc-fixer-report.md"))
		writeRoundFile(t, filepath.Join(runDir, "round-03-review.md"))
		writeRoundFile(t, filepath.Join(runDir, "round-03-fixer-report.md"))
		writeRoundFile(t, filepath.Join(runDir, "round-3b-review.md"))
		writeRoundFile(t, filepath.Join(runDir, "round-3b-fixer-report.md"))
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if runner.GotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q (none of the malformed tokens adopted)", runner.GotOpts[0].Round, "1")
		}
	})
}

// --- Hydration ---

//testtiming:keep pins which prior reviews, fixer reports and focus directive the round's profile carries: the told prefix, orphan and stamped-sibling exclusion, and the focus path only when the file says something
func TestBurlerProducer_Hydration(t *testing.T) {
	t.Run("Round1HydratesNothingBeyondTemplate", func(t *testing.T) {
		runDir := t.TempDir()
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, []string{}) {
			t.Errorf("PriorReviews = %v; want empty", runner.GotProfiles[0].PriorReviews)
		}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorFixerReports, []string{}) {
			t.Errorf("PriorFixerReports = %v; want empty", runner.GotProfiles[0].PriorFixerReports)
		}
	})

	t.Run("Rounds1And2CompleteRound3CarriesBothPriorsInOrder", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		writeJudgedRound(t, runDir, 2)
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		wantReviews := []string{roundReviewPath(runDir, 1), roundReviewPath(runDir, 2)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v", runner.GotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{roundFixerReportPath(runDir, 1), roundFixerReportPath(runDir, 2)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v", runner.GotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("StampedArchiveSiblingNeverHydrated", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		writeRoundFile(t, filepath.Join(runDir, "round-1-review-20260820T101500Z.md"))
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		wantReviews := []string{roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v", runner.GotProfiles[0].PriorReviews, wantReviews)
		}
	})

	t.Run("RoundWithOnlyOneFileSkippedByHydrationEntirely", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, roundReviewPath(runDir, 1)) // no fixer report: incomplete
		writeJudgedRound(t, runDir, 2)
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		wantReviews := []string{roundReviewPath(runDir, 2)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (round 1's orphan never named)", runner.GotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{roundFixerReportPath(runDir, 2)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v", runner.GotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("ToldPriorsPreservedAsPrefix", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		toldReview := filepath.Join(runDir, "told-review.md")
		toldFixer := filepath.Join(runDir, "told-fixer.md")
		writeRoundFile(t, toldReview)
		writeRoundFile(t, toldFixer)
		profile := simpleBurlerProfile()
		profile.PriorReviews = []string{toldReview}
		profile.PriorFixerReports = []string{toldFixer}
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner, withBurlerProfile(profile))

		shedfake.CallOK(t, p)
		wantReviews := []string{toldReview, roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (told entries as prefix)", runner.GotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{toldFixer, roundFixerReportPath(runDir, 1)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v (told entries as prefix)", runner.GotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("FocusFileWithADirectiveReachesFocusDirectiveNotPriorReviews", func(t *testing.T) {
		runDir := t.TempDir()
		writeJudgedRound(t, runDir, 1)
		writeFocusFile(t, runDir, 2, focusFile{Round: 2, Focus: []string{"look at the relocation candidate"}})
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		shedfake.CallOK(t, p)
		wantReviews := []string{roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (derived prior-round entries only)", runner.GotProfiles[0].PriorReviews, wantReviews)
		}
		if got, want := runner.GotProfiles[0].FocusDirective, focusPath(runDir, 2); got != want {
			t.Errorf("FocusDirective = %q; want %q", got, want)
		}
	})

	noDirective := []struct {
		name  string
		setup func(t *testing.T, runDir string)
	}{
		{"FocusFileWithNoDirectiveLeavesFocusDirectiveEmpty", func(t *testing.T, runDir string) {
			writeFocusFile(t, runDir, 2, focusFile{Round: 2, ExcludeLenses: []string{}, Focus: []string{}})
		}},
		{"AbsentFocusFileLeavesFocusDirectiveEmpty", func(t *testing.T, runDir string) {}},
		{"MalformedFocusFileLeavesFocusDirectiveEmpty", func(t *testing.T, runDir string) {
			writeFocusFileRaw(t, runDir, 2, "not a focus file")
		}},
	}
	for _, tt := range noDirective {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			writeJudgedRound(t, runDir, 1)
			tt.setup(t, runDir)
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
			p := newBurlerProducer(t, runDir, runner)

			shedfake.CallOK(t, p)
			if got := runner.GotProfiles[0].FocusDirective; got != "" {
				t.Errorf("FocusDirective = %q; want empty", got)
			}
			wantReviews := []string{roundReviewPath(runDir, 1)}
			if !stringSlicesEqual(runner.GotProfiles[0].PriorReviews, wantReviews) {
				t.Errorf("PriorReviews = %v; want %v (no focus path)", runner.GotProfiles[0].PriorReviews, wantReviews)
			}
		})
	}
}

// --- Call outcomes ---

//testtiming:keep pins that a successful round, approved or blocking and gate-passed or not, is a routine Stuck hand-off with the review as pointer and no reason, never Done
func TestBurlerProducer_Call_DoneReturnsStuckNeverDone(t *testing.T) {
	tests := []struct {
		name    string
		verdict burlerengine.Verdict
		gate    *shuttleengine.GateOutcome
	}{
		{"Approved", burlerengine.VerdictApproved, nil},
		{"Blocking", burlerengine.VerdictBlocking, nil},
		{"PassedGateIsUnchanged", burlerengine.VerdictApproved, &shuttleengine.GateOutcome{Passed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone, Verdict: tt.verdict, Gate: tt.gate}}}
			p := newBurlerProducer(t, runDir, runner)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
			wantPath := roundReviewPath(runDir, 1)
			if ptr.Path != wantPath {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPath)
			}
			if ptr.Reason != "" {
				t.Errorf("Call() Reason = %q; want empty -- a successful round's Stuck is a routine hand-off with no cause", ptr.Reason)
			}
		})
	}
}

func TestBurlerProducer_Call_WritesRoundUsageRecord(t *testing.T) {
	started := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	ended := started.Add(90 * time.Second)
	models := burlerengine.RoundModels{
		Review: []burlerengine.ModelChoice{{Model: "reviewer-model", Effort: "high"}},
		Fix:    []burlerengine.ModelChoice{{Model: "fixer-model", Effort: "low"}},
	}
	knownReviewer := burlerengine.Half{
		StartedAt: started,
		EndedAt:   ended,
		Usage:     shuttleengine.SessionUsage{Known: true, Fresh: 1000, CacheRead: 5000, Forks: 3, ForkFresh: 600, ForkCacheRead: 4000},
	}
	knownFixer := burlerengine.Half{
		StartedAt: started,
		EndedAt:   ended.Add(time.Minute),
		Usage:     shuttleengine.SessionUsage{Known: true, Fresh: 200, CacheRead: 300},
	}
	tests := []struct {
		name   string
		fan    string
		result burlerengine.Result
		check  func(t *testing.T, got roundUsage)
		wantNo bool
	}{
		{
			name:   "fanned round records fan, models and lenses",
			fan:    "discussion",
			result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Lenses: []string{"a", "b"}, Review: knownReviewer, Fix: knownFixer},
			check: func(t *testing.T, got roundUsage) {
				if got.Fan != "discussion" || !stringSlicesEqual(got.Lenses, []string{"a", "b"}) {
					t.Errorf("fan, lenses = %q, %v; want discussion, [a b]", got.Fan, got.Lenses)
				}
				if got.Review.Model != "reviewer-model" || got.Review.Effort != "high" || got.Fix.Model != "fixer-model" || got.Fix.Effort != "low" {
					t.Errorf("models = %+v / %+v; want the round's picks", got.Review, got.Fix)
				}
				if got.Review.WallSeconds == nil || *got.Review.WallSeconds != 90 {
					t.Errorf("review wall = %v; want 90", got.Review.WallSeconds)
				}
				if !got.Review.TokensKnown || *got.Review.FreshTokens != 1000 || *got.Review.CacheReadTokens != 5000 {
					t.Errorf("review tokens = %+v; want 1000 fresh, 5000 cache-read", got.Review)
				}
				if *got.Review.Forks != 3 || *got.Review.ForkFreshTokens != 600 || *got.Review.ForkCacheReadTokens != 4000 || *got.Review.ForkFreshShare != 0.6 {
					t.Errorf("review forks = %+v; want 3 forks, 600 fresh, 4000 cache-read, share 0.6", got.Review)
				}
				if got.Fix.Forks != nil {
					t.Errorf("fix forks = %v; want none recorded", got.Fix.Forks)
				}
			},
		},
		{
			name:   "solo round records an empty fan and nil lenses",
			result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Review: knownReviewer, Fix: knownFixer},
			check: func(t *testing.T, got roundUsage) {
				if got.Fan != "" || got.Lenses != nil {
					t.Errorf("fan, lenses = %q, %v; want empty, nil", got.Fan, got.Lenses)
				}
			},
		},
		{
			name:   "unknown reading and zero times render as unknown, not zero",
			result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone},
			check: func(t *testing.T, got roundUsage) {
				for label, half := range map[string]halfUsage{"review": got.Review, "fix": got.Fix} {
					if half.TokensKnown || half.FreshTokens != nil || half.CacheReadTokens != nil || half.WallSeconds != nil || half.Forks != nil {
						t.Errorf("%s half = %+v; want tokens, wall time and forks unknown", label, half)
					}
				}
			},
		},
		{
			name:   "gate-failed round leaves no record",
			result: burlerengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: false}},
			wantNo: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{tt.result}}
			profile := simpleBurlerProfile()
			profile.ClusterFan = tt.fan
			p := newBurlerProducer(t, runDir, runner, withBurlerProfile(profile), withBurlerModels(models))

			shedfake.RequireOutcome(t, p, shedengine.Stuck)
			got, err := readRoundUsage(runDir, 1)
			if tt.wantNo {
				if _, statErr := os.Stat(roundUsagePath(runDir, 1)); !os.IsNotExist(statErr) {
					t.Errorf("usage record exists after a gate-failed round (stat err %v); want none", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readRoundUsage() error = %v; want nil", err)
			}
			if got.Round != 1 {
				t.Errorf("record round = %d; want 1", got.Round)
			}
			tt.check(t, got)
		})
	}

	t.Run("unwritable record leaves the Stuck hand-off unchanged", func(t *testing.T) {
		runDir := t.TempDir()
		if err := os.MkdirAll(roundUsagePath(runDir, 1), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newBurlerProducer(t, runDir, runner)

		ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		if want := roundReviewPath(runDir, 1); ptr.Path != want || ptr.Reason != "" {
			t.Errorf("pointer = %+v; want path %q and no reason", ptr, want)
		}
	})
}

func TestBurlerProducer_Call_BudgetExemptAfterBudgetContinue(t *testing.T) {
	tests := []struct {
		name       string
		decision   CirclingDecision
		cause      EscalationCause
		raw        string
		wantExempt bool
	}{
		{name: "budget continue", decision: CirclingContinue, cause: EscalationBudget, wantExempt: true},
		{name: "circling continue", decision: CirclingContinue, cause: EscalationCircling, wantExempt: false},
		{name: "budget accept", decision: CirclingAccept, cause: EscalationBudget, wantExempt: false},
		{name: "no decision", wantExempt: false},
		{name: "malformed decision", raw: "not a decision file", wantExempt: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			writeJudgedRound(t, runDir, 1)
			switch {
			case tt.raw != "":
				if err := os.WriteFile(circlingDecisionPath(runDir, 1), []byte(tt.raw), 0o644); err != nil {
					t.Fatalf("WriteFile(decision): %v", err)
				}
			case tt.decision != "":
				content, err := renderCirclingDecision(1, tt.decision, tt.cause, false)
				if err != nil {
					t.Fatalf("renderCirclingDecision: %v", err)
				}
				if err := os.WriteFile(circlingDecisionPath(runDir, 1), content, 0o644); err != nil {
					t.Fatalf("WriteFile(decision): %v", err)
				}
			}
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone, Verdict: burlerengine.VerdictBlocking}}}
			p := newBurlerProducer(t, runDir, runner)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
			if ptr.BudgetExempt != tt.wantExempt {
				t.Errorf("round 2 Stuck BudgetExempt = %v; want %v", ptr.BudgetExempt, tt.wantExempt)
			}
		})
	}
}

func TestBurlerProducer_Call_ProfileCarriesDerivedFields(t *testing.T) {
	runDir := t.TempDir()
	writeJudgedRound(t, runDir, 1)
	writeFocusFile(t, runDir, 2, focusFile{Round: 2, ExcludeLenses: []string{"lensA"}})
	profile := simpleBurlerProfile()
	profile.ClusterFan = "fanX"
	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newBurlerProducer(t, runDir, runner, withBurlerProfile(profile))

	shedfake.CallOK(t, p)
	got := runner.GotProfiles[0]
	if got.ReviewPath != roundReviewPath(runDir, 2) {
		t.Errorf("ReviewPath = %q; want %q", got.ReviewPath, roundReviewPath(runDir, 2))
	}
	if got.FixerReportPath != roundFixerReportPath(runDir, 2) {
		t.Errorf("FixerReportPath = %q; want %q", got.FixerReportPath, roundFixerReportPath(runDir, 2))
	}
	wantReviews := []string{roundReviewPath(runDir, 1)}
	if !stringSlicesEqual(got.PriorReviews, wantReviews) {
		t.Errorf("PriorReviews = %v; want %v", got.PriorReviews, wantReviews)
	}
	if !stringSlicesEqual(got.ClusterExclude, []string{"lensA"}) {
		t.Errorf("ClusterExclude = %v; want %v", got.ClusterExclude, []string{"lensA"})
	}
	wantMarker, err := burlermarker.Path(filepath.Dir(runDir), filepath.Dir(runDir), roundReviewPath(runDir, 2))
	if err != nil {
		t.Fatalf("burlermarker.Path() = %v; want nil", err)
	}
	if got.ReadyMarkerPath != wantMarker {
		t.Errorf("ReadyMarkerPath = %q; want the round's marker %q", got.ReadyMarkerPath, wantMarker)
	}
}

func TestBurlerProducer_Call_ClusterExcludeDropWarning(t *testing.T) {
	const dropWarning = "shedadapters: focus file names cluster excludes but this round's profile has no cluster fan; dropping them"

	// Raw frontmatter rather than writeFocusFile: renderFocus always writes both list keys, and the
	// first case needs the file a judge not told ClusterExcludes writes, with focus alone.
	// wantHydrated proves that file parsed: a rejected file would also produce no drop WARN.
	tests := []struct {
		name         string
		focus        string
		wantWarning  bool
		wantHydrated bool
	}{
		{"NoExcludeLensesKey", "---\nround: 2\nfocus:\n  - look at the seam\n---\n", false, true},
		{"ExcludesOnFanlessProfile", "---\nround: 2\nexclude_lenses:\n  - lensA\nfocus: []\n---\n", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			runDir := t.TempDir()
			writeJudgedRound(t, runDir, 1)
			writeFocusFileRaw(t, runDir, 2, tt.focus)
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
			p := newBurlerProducer(t, runDir, runner)

			shedfake.CallOK(t, p)
			if got := runner.GotProfiles[0].ClusterExclude; got != nil {
				t.Errorf("ClusterExclude = %v; want nil on a fan-less profile", got)
			}
			if delivered := runner.GotProfiles[0].FocusDirective == focusPath(runDir, 2); delivered != tt.wantHydrated {
				t.Errorf("focus file delivered = %v; want %v; FocusDirective = %q", delivered, tt.wantHydrated, runner.GotProfiles[0].FocusDirective)
			}
			if has := strings.Contains(buf.String(), dropWarning); has != tt.wantWarning {
				t.Errorf("log contains drop warning = %v; want %v; log:\n%s", has, tt.wantWarning, buf.String())
			}
		})
	}
}

// TestBurlerProducer_Call_RunOptsCarriesRoundTokenAndNoteID covers the RunOpts each call hands the
// runner: the round token, both halves' model picks for the round, the note id derived from the run directory's name and the round, and
// the opts template carried through.
//
//testtiming:keep pins the round token, the note id and the opts template the runner receives, which the round-scan tests do not read off the recorded opts
func TestBurlerProducer_Call_RunOptsCarriesRoundTokenAndNoteID(t *testing.T) {
	models := burlerengine.RoundModels{
		Review: []burlerengine.ModelChoice{{Model: "m1", Effort: "e1"}, {Model: "m2", Effort: "e2"}},
		Fix:    []burlerengine.ModelChoice{{Model: "f1", Effort: "g1"}},
	}
	tests := []struct {
		name            string
		runDirName      string
		judgedRound     int
		wantRound       string
		wantNoteID      string
		wantReview      burlerengine.ModelChoice
		wantFix         burlerengine.ModelChoice
		wantTimeoutOpts time.Duration
	}{
		{name: "round 1 runs the first entry of each list", runDirName: "runs", wantRound: "1", wantReview: models.Review[0], wantFix: models.Fix[0], wantTimeoutOpts: time.Minute},
		{name: "a round past a list runs its last entry", runDirName: "runs", judgedRound: 4, wantRound: "5", wantReview: models.Review[1], wantFix: models.Fix[0], wantTimeoutOpts: time.Minute},
		{name: "note id", runDirName: "webster", judgedRound: 2, wantRound: "3", wantNoteID: "burler-webster-r3", wantReview: models.Review[1], wantFix: models.Fix[0], wantTimeoutOpts: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := filepath.Join(t.TempDir(), tt.runDirName)
			for n := 1; n <= tt.judgedRound; n++ {
				writeJudgedRound(t, runDir, n)
			}
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
			p := newBurlerProducer(t, runDir, runner, withBurlerRunOpts(burlerengine.RunOpts{Timeout: time.Minute}), withBurlerModels(models))

			shedfake.CallOK(t, p)
			got := runner.GotOpts[0]
			if got.Round != tt.wantRound {
				t.Errorf("Round = %q; want %q", got.Round, tt.wantRound)
			}
			if got.Review != tt.wantReview || got.Fix != tt.wantFix {
				t.Errorf("Review, Fix = %+v, %+v; want %+v, %+v (the picks for the round)", got.Review, got.Fix, tt.wantReview, tt.wantFix)
			}
			if got.Timeout != tt.wantTimeoutOpts {
				t.Errorf("Timeout = %v; want %v (opts template carried through)", got.Timeout, tt.wantTimeoutOpts)
			}
			if tt.wantNoteID != "" && got.NoteID != tt.wantNoteID {
				t.Errorf("NoteID = %q; want %q", got.NoteID, tt.wantNoteID)
			}
		})
	}
}

//testtiming:keep pins the retry after a died attempt: the second attempt's round token is "1b", over the same review path, and the review it writes survives
func TestBurlerProducer_Call_DiedThenDoneSucceedsWithRetry(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	fixerPath := roundFixerReportPath(runDir, 1)
	runner := &shedfake.BurlerRunner{
		Results: []burlerengine.Result{
			{Outcome: shuttleengine.OutcomeDied, Review: burlerengine.Half{SessionID: "s1"}},
			{Outcome: shuttleengine.OutcomeDone},
		},
	}
	runner.DuringRun = func(i int) {
		if i == 1 {
			// Attempt 2 (the second invocation) is the one that succeeds and writes the review.
			writeRoundFile(t, reviewPath)
			writeRoundFile(t, fixerPath)
		}
	}
	p := newBurlerProducer(t, runDir, runner)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if runner.Calls != 2 {
		t.Fatalf("runner.Calls = %d; want 2", runner.Calls)
	}
	if runner.GotOpts[0].Round != "1" {
		t.Errorf("attempt 1 round token = %q; want %q", runner.GotOpts[0].Round, "1")
	}
	if runner.GotOpts[1].Round != "1b" {
		t.Errorf("attempt 2 round token = %q; want %q", runner.GotOpts[1].Round, "1b")
	}
	if runner.GotProfiles[1].ReviewPath != runner.GotProfiles[0].ReviewPath {
		t.Errorf("attempt 2 review path = %q; want unchanged from attempt 1 (%q)", runner.GotProfiles[1].ReviewPath, runner.GotProfiles[0].ReviewPath)
	}
	if ptr.Path != reviewPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, reviewPath)
	}
	if _, statErr := os.Stat(reviewPath); statErr != nil {
		t.Errorf("review written by the successful attempt did not survive: %v", statErr)
	}
}

// TestBurlerProducer_Call_FailedRunIsAnError covers the runs that end in an error rather than a
// hand-off: two timeouts name both sessions, and a runner error is wrapped.
func TestBurlerProducer_Call_FailedRunIsAnError(t *testing.T) {
	seamErr := errors.New("seam exploded")
	tests := []struct {
		name         string
		runner       *shedfake.BurlerRunner
		wantCalls    int
		wantContains []string
		wantIs       error
	}{
		{
			name: "TimeoutTwiceNamesBothHalvesOfBothAttempts",
			runner: &shedfake.BurlerRunner{Results: []burlerengine.Result{
				{Outcome: shuttleengine.OutcomeTimeout, Review: burlerengine.Half{SessionID: "s1", RunDir: "/kept/1"}, Fix: burlerengine.Half{SessionID: "t1", RunDir: "/kept/1f"}},
				{Outcome: shuttleengine.OutcomeTimeout, Review: burlerengine.Half{SessionID: "s2", RunDir: "/kept/2"}, Fix: burlerengine.Half{SessionID: "t2", RunDir: "/kept/2f"}},
			}},
			wantCalls:    2,
			wantContains: []string{"s1", "t1", "/kept/1f", "s2", "t2", "/kept/2f"},
		},
		{
			name: "NeverStartedHalfNamesItsStartError",
			runner: &shedfake.BurlerRunner{Results: []burlerengine.Result{
				{Outcome: shuttleengine.OutcomeDied, NotStarted: true, Review: burlerengine.Half{StartError: "run dir /r1 strand g1 removed"}},
				{Outcome: shuttleengine.OutcomeDied, NotStarted: true, Review: burlerengine.Half{StrandGUID: "g2"}, Fix: burlerengine.Half{StartError: "run dir /r2 strand g3 removed"}},
			}},
			wantCalls:    2,
			wantContains: []string{"review never started (run dir /r1 strand g1 removed)", "fix not started", "fix never started (run dir /r2 strand g3 removed)"},
		},
		{
			name:      "RunnerErrorWrapped",
			runner:    &shedfake.BurlerRunner{Errs: []error{seamErr}},
			wantCalls: 1,
			wantIs:    seamErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newBurlerProducer(t, t.TempDir(), tt.runner)

			_, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Call() error %q does not contain %q", err.Error(), want)
				}
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Call() error = %v; want it to wrap %v", err, tt.wantIs)
			}
			if tt.runner.Calls != tt.wantCalls {
				t.Errorf("runner.Calls = %d; want %d", tt.runner.Calls, tt.wantCalls)
			}
		})
	}
}

// --- Call cancellation ---

// TestBurlerProducer_Call_CancelledBeforeAnAttempt covers a cancellation seen before an attempt is
// spawned: an already-cancelled context never spawns, and one cancelled during attempt 1 never
// spawns attempt 2.
//
//testtiming:keep pins that a cancelled context spawns no attempt and a cancellation during attempt 1 spawns no attempt 2
func TestBurlerProducer_Call_CancelledBeforeAnAttempt(t *testing.T) {
	tests := []struct {
		name      string
		cancelled bool
		wantCalls int
	}{
		{"AlreadyCancelledContext", true, 0},
		{"CancelledBetweenAttempts", false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDied}}}
			if tt.cancelled {
				cancel()
			} else {
				runner.DuringRun = func(int) { cancel() }
			}
			p := newBurlerProducer(t, t.TempDir(), runner)

			_, _, err := p.Call(ctx)
			if err == nil {
				t.Fatal("Call() error = nil; want non-nil")
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
			}
			if runner.Calls != tt.wantCalls {
				t.Errorf("runner.Calls = %d; want %d", runner.Calls, tt.wantCalls)
			}
		})
	}
}

func TestBurlerProducer_Call_CancelledDuringFailedRoundArchives(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDied}}}
	runner.DuringRun = func(int) {
		writeRoundFile(t, reviewPath)
		cancel()
	}
	instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
	p := newBurlerProducer(t, runDir, runner, withBurlerClock(fixedClock(instant)))

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if _, statErr := os.Stat(reviewPath); !os.IsNotExist(statErr) {
		t.Error("failed round's review file was not archived away")
	}
	wantStamped := filepath.Join(runDir, "round-1-review-20260820T101500Z.md")
	if _, statErr := os.Stat(wantStamped); statErr != nil {
		t.Errorf("expected archived sibling %s: %v", wantStamped, statErr)
	}
}

func TestBurlerProducer_Call_CancelledDuringCompletedRoundLeavesArtifacts(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	fixerPath := roundFixerReportPath(runDir, 1)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	runner.DuringRun = func(int) {
		writeRoundFile(t, reviewPath)
		writeRoundFile(t, fixerPath)
		cancel()
	}
	p := newBurlerProducer(t, runDir, runner)

	outcome, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil (cancellation observed after the round completed)")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if outcome == shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want the cancellation error, never %q", outcome, shedengine.Stuck)
	}
	if _, statErr := os.Stat(reviewPath); statErr != nil {
		t.Errorf("completed round's review file did not survive: %v", statErr)
	}
	if _, statErr := os.Stat(fixerPath); statErr != nil {
		t.Errorf("completed round's fixer report did not survive: %v", statErr)
	}

	// The surviving round is judged before the resumed call, exactly as the real sequence judges it:
	// this row's Stuck routes to the segment's Bouncer, whose CONTINUE verdict routes back here.
	// Without that verdict the resumed call would hand back rather than advance, which is a different
	// property (covered in the round-scan cases) than the artifact survival this test is about.
	if err := os.WriteFile(verdictPath(runDir, 1), []byte(bouncerVerdictContent("CONTINUE")), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict round 1): %v", err)
	}
	if err := os.WriteFile(ledgerPath(runDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
		t.Fatalf("WriteFile(ledger round 1): %v", err)
	}

	runner2 := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p2 := newBurlerProducer(t, runDir, runner2)
	shedfake.CallOK(t, p2)
	if runner2.GotOpts[0].Round != "2" {
		t.Errorf("second Call() round token = %q; want %q (advances past the completed round)", runner2.GotOpts[0].Round, "2")
	}
}

// --- Gate ---

// TestBurlerProducer_Gate_FailedGateMapsToStuckWithEmptyPointer proves a gate-failed round returns
// Stuck with an EMPTY pointer -- the signal that tells the segment's Bouncer there is no round
// artifact to judge -- and that both round paths are archived (never left for the Bouncer to read).
// It also proves the runner is never invoked a second time: the attempt-1/attempt-2 retry belongs
// to OutcomeDied/OutcomeTimeout alone.
func TestBurlerProducer_Gate_FailedGateMapsToStuckWithEmptyPointer(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	fixerPath := roundFixerReportPath(runDir, 1)
	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{
		Outcome: shuttleengine.OutcomeDone,
		Gate:    &shuttleengine.GateOutcome{Passed: false, FindingsPath: "/kept/gate-findings.md"},
	}}}
	runner.DuringRun = func(int) {
		writeRoundFile(t, reviewPath)
		writeRoundFile(t, fixerPath)
	}
	instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
	p := newBurlerProducer(t, runDir, runner, withBurlerClock(fixedClock(instant)))

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != "" {
		t.Errorf("Call() pointer.Path = %q; want empty -- there is no round artifact for the Bouncer to judge", ptr.Path)
	}
	if ptr.GateAttempts == nil || *ptr.GateAttempts != 0 {
		t.Errorf("Call() pointer.GateAttempts = %v; want pointer to 0", ptr.GateAttempts)
	}
	if want := "gate did not pass after 0 attempts; findings: /kept/gate-findings.md"; ptr.Reason != want {
		t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
	}
	if _, statErr := os.Stat(reviewPath); !os.IsNotExist(statErr) {
		t.Error("gate-failed round's review file was not archived away")
	}
	if _, statErr := os.Stat(fixerPath); !os.IsNotExist(statErr) {
		t.Error("gate-failed round's fixer report was not archived away")
	}
	if runner.Calls != 1 {
		t.Errorf("runner.Run calls = %d; want 1 -- a gate-failed round is a determinate verdict, never retried", runner.Calls)
	}
}

// TestBurlerProducer_Gate_ProbeLiveRoundPassesGateAndMapsFailedGateIdentically is the regression guard for the resume hole:
// the live-round probe must hand the runner the caller's gate list, leaving it as it was, so a resumed fixer is gated exactly as a fresh one is,
// and a resumed round's failed gate must map identically to the spawn path's -- Stuck with an empty pointer, archived, retry untouched.
func TestBurlerProducer_Gate_ProbeLiveRoundPassesGateAndMapsFailedGateIdentically(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	runner := &shedfake.BurlerRunner{
		Results:    []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
		LiveRounds: []burlerengine.LiveRound{roundWith(burlerengine.HalfLive, burlerengine.HalfLive)},
		ResumeResults: []burlerengine.Result{{
			Outcome: shuttleengine.OutcomeDone,
			Gate:    &shuttleengine.GateOutcome{Passed: false},
		}},
	}
	opts := burlerengine.RunOpts{Gate: shuttleengine.GateSpec{{Name: "told", Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{}, nil
	}}}}
	p := newBurlerProducer(t, runDir, runner, withBurlerRunOpts(opts), withBurlerClock(fixedClock(time.Now())))
	// Only the review file exists at Call entry -- a complete pair here would make highestCompleteRound treat round 1 as already finished and hand back before the probe is ever reached.
	writeRoundFile(t, reviewPath)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
	if ptr.Path != "" {
		t.Errorf("Call() pointer.Path = %q; want empty, identically to the spawn path's own gate-failed exit", ptr.Path)
	}
	if ptr.GateAttempts == nil || *ptr.GateAttempts != 0 {
		t.Errorf("Call() pointer.GateAttempts = %v; want pointer to 0", ptr.GateAttempts)
	}
	if want := "gate did not pass after 0 attempts; findings: "; ptr.Reason != want {
		t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
	}
	if got := runner.GotProbeOpts[0].Gate; len(got) != 1 || got[0].Name != "told" {
		t.Errorf("probe gate spec = %+v; want the caller's one told entry", got)
	}
	if len(opts.Gate) != 1 {
		t.Errorf("caller's gate list has %d entries; want it left at 1", len(opts.Gate))
	}
	if runner.Calls != 0 {
		t.Errorf("runner.Run calls = %d; want 0 -- a resumed round is never respawned", runner.Calls)
	}
	if _, statErr := os.Stat(reviewPath); !os.IsNotExist(statErr) {
		t.Error("gate-failed resumed round's review file was not archived away")
	}
}

// --- Archive-on-exit ---

func TestBurlerProducer_Call_ArchiveOnExit(t *testing.T) {
	// assertUnchangedRoundOnRerun drives a fresh Call over runDir and asserts the round token it
	// resolves is still "1" -- proof the prior Call's failure archived its round rather than
	// leaving it looking complete.
	assertUnchangedRoundOnRerun := func(t *testing.T, runDir string) {
		t.Helper()
		runner2 := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p2 := newBurlerProducer(t, runDir, runner2)
		shedfake.CallOK(t, p2)
		if runner2.GotOpts[0].Round != "1" {
			t.Errorf("second Call() round token = %q; want %q (unchanged from the failed first Call())", runner2.GotOpts[0].Round, "1")
		}
	}

	t.Run("TwoConsecutiveDiedWithPhaseAOnlyPartial", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		runner := &shedfake.BurlerRunner{
			Results: []burlerengine.Result{
				{Outcome: shuttleengine.OutcomeDied, Review: burlerengine.Half{SessionID: "s1"}},
				{Outcome: shuttleengine.OutcomeDied, Review: burlerengine.Half{SessionID: "s2"}},
			},
		}
		runner.DuringRun = func(i int) {
			if i == 0 {
				// Attempt 1's phase-A-only partial: review written, fixer report never reached.
				writeRoundFile(t, reviewPath)
			}
		}
		p := newBurlerProducer(t, runDir, runner)

		if _, _, err := p.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})

	t.Run("TwoConsecutiveDiedWrapsErrNotStartedOnlyWhenNotStarted", func(t *testing.T) {
		for _, notStarted := range []bool{true, false} {
			runDir := t.TempDir()
			runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{
				{Outcome: shuttleengine.OutcomeDied, Review: burlerengine.Half{SessionID: "s1"}, NotStarted: notStarted},
				{Outcome: shuttleengine.OutcomeDied, Review: burlerengine.Half{SessionID: "s2"}, NotStarted: notStarted},
			}}
			p := newBurlerProducer(t, runDir, runner)

			_, _, err := p.Call(context.Background())
			if err == nil {
				t.Fatalf("NotStarted=%v: Call() error = nil; want non-nil", notStarted)
			}
			if got := errors.Is(err, shuttleengine.ErrNotStarted); got != notStarted {
				t.Errorf("NotStarted=%v: errors.Is(err, ErrNotStarted) = %v; want %v", notStarted, got, notStarted)
			}
		}
	})

	t.Run("CancellationBetweenAttempts", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		ctx, cancel := context.WithCancel(context.Background())
		runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDied}}}
		runner.DuringRun = func(int) {
			writeRoundFile(t, reviewPath)
			cancel()
		}
		p := newBurlerProducer(t, runDir, runner)

		if _, _, err := p.Call(ctx); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})

	t.Run("HalfNotStoppedReturnsWithoutArchivingOrRetrying", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		fixerPath := roundFixerReportPath(runDir, 1)
		runner := &shedfake.BurlerRunner{
			Errs: []error{fmt.Errorf("%w: strand g", burlerengine.ErrHalfNotStopped)},
		}
		runner.DuringRun = func(int) {
			writeRoundFile(t, reviewPath)
			writeRoundFile(t, fixerPath)
		}
		p := newBurlerProducer(t, runDir, runner)

		_, _, err := p.Call(context.Background())

		if !errors.Is(err, burlerengine.ErrHalfNotStopped) {
			t.Fatalf("Call() error = %v; want it to wrap ErrHalfNotStopped", err)
		}
		if runner.Calls != 1 {
			t.Errorf("runner.Calls = %d; want 1 -- a half that cannot be stopped is never retried", runner.Calls)
		}
		for _, path := range []string{reviewPath, fixerPath} {
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("%s was archived away (%v); a live half may still write it", path, statErr)
			}
		}
	})

	t.Run("RunnerErrorWithDoneOutcomeAndBothFilesWritten", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		fixerPath := roundFixerReportPath(runDir, 1)
		seamErr := errors.New("burler: round reached done but its review file is invalid")
		runner := &shedfake.BurlerRunner{
			Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
			Errs:    []error{seamErr},
		}
		runner.DuringRun = func(int) {
			writeRoundFile(t, reviewPath)
			writeRoundFile(t, fixerPath)
		}
		p := newBurlerProducer(t, runDir, runner)

		if _, _, err := p.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})
}

// TestBurlerProducer_Call_PreAttemptArchiveFailureHonoursCancellation is the guard for this
// package's shared cancellation rule at the one exit that used to skip it: the pre-attempt
// archiveStaleOutputs failure returned bare, so a run an operator had already cancelled was reported
// as an unrelated infrastructure error instead of as the cancellation it was. Every other
// non-success exit in Call already routes through failureExit, which consults cancelErr first.
//
// Reaching that branch needs the archive to fail AND the context to be cancelled at that moment, and
// an already-cancelled context would be caught by Call's entry check long before -- so the injected
// clock is used as the seam. archiveStaleOutputs calls now() after it has stat'd the stale file and
// before it renames it, so cancelling from there lands the cancellation exactly inside the failing
// archive, which a read-only run directory guarantees.
func TestBurlerProducer_Call_PreAttemptArchiveFailureHonoursCancellation(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	if err := os.WriteFile(reviewPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", reviewPath, err)
	}
	if err := os.Chmod(runDir, 0o555); err != nil {
		t.Fatalf("Chmod(%s): %v", runDir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(runDir, 0o755) })

	ctx, cancel := context.WithCancel(context.Background())
	cancelledDuringArchive := false
	now := func() time.Time {
		cancelledDuringArchive = true
		cancel()
		return time.Unix(0, 0).UTC()
	}

	runner := &shedfake.BurlerRunner{Results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newBurlerProducer(t, runDir, runner, withBurlerClock(now))

	_, _, err := p.Call(ctx)

	if !cancelledDuringArchive {
		t.Fatal("the injected clock never ran; the test never reached the archive step it is about")
	}
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want it to wrap context.Canceled, not the archive failure", err)
	}
	if runner.Calls != 0 {
		t.Errorf("runner.Calls = %d; want 0", runner.Calls)
	}
}
