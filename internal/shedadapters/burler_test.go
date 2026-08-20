package shedadapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeBurlerRunner records every Profile/RunOpts it is handed and returns caller-scripted
// burlerengine.Result/error values per invocation, so a test can script attempt 1 and attempt 2
// differently. results[i]/errs[i] answer the (i+1)th invocation; once either slice is exhausted its
// own last entry repeats. duringRun, when set, is invoked from inside Run before it returns --
// letting a test cancel the context or write files as if mid-round, mirroring fakePerchRunner's and
// fakeShuttle's own hook.
type fakeBurlerRunner struct {
	results []burlerengine.Result
	errs    []error

	gotProfiles []burlerengine.Profile
	gotOpts     []burlerengine.RunOpts
	calls       int

	duringRun func(callIndex int)
}

func (f *fakeBurlerRunner) Run(p burlerengine.Profile, opts burlerengine.RunOpts) (burlerengine.Result, error) {
	i := f.calls
	f.calls++
	f.gotProfiles = append(f.gotProfiles, p)
	f.gotOpts = append(f.gotOpts, opts)
	if f.duringRun != nil {
		f.duringRun(i)
	}

	var result burlerengine.Result
	if i < len(f.results) {
		result = f.results[i]
	} else if len(f.results) > 0 {
		result = f.results[len(f.results)-1]
	}
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	} else if len(f.errs) > 0 {
		err = f.errs[len(f.errs)-1]
	}
	return result, err
}

// Compile-time proof that *fakeBurlerRunner satisfies BurlerRunner.
var _ BurlerRunner = (*fakeBurlerRunner)(nil)

// simpleBurlerProfile returns a minimal burlerengine.Profile suitable for passing through Call --
// its content fields are irrelevant since BurlerProducer never invokes burlerengine's own validate.
func simpleBurlerProfile() burlerengine.Profile {
	return burlerengine.Profile{
		Rubric:   "rubric text",
		FixScope: burlerengine.FixScopeOverlay,
	}
}

// newTestBurlerProducer builds a BurlerProducer over runDir with runner, failing the test on
// constructor error.
func newTestBurlerProducer(t *testing.T, runDir string, profile burlerengine.Profile, opts burlerengine.RunOpts, runner *fakeBurlerRunner, now func() time.Time) *BurlerProducer {
	t.Helper()
	p, err := NewBurlerProducer("burler", runner, profile, opts, runDir, now)
	if err != nil {
		t.Fatalf("NewBurlerProducer() error = %v; want nil", err)
	}
	return p
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

// writeRoundPair writes both of round n's own files under runDir, making it complete.
func writeRoundPair(t *testing.T, runDir string, n int) {
	t.Helper()
	writeRoundFile(t, roundReviewPath(runDir, n))
	writeRoundFile(t, roundFixerReportPath(runDir, n))
}

// --- Constructor ---

func TestNewBurlerProducer_Validation(t *testing.T) {
	dir := t.TempDir()
	profile := simpleBurlerProfile()

	tests := []struct {
		name   string
		runner BurlerRunner
		pname  string
		runDir string
	}{
		{"NilRunner", nil, "burler", filepath.Join(dir, "runs")},
		{"EmptyName", &fakeBurlerRunner{}, "", filepath.Join(dir, "runs")},
		{"EmptyRunDir", &fakeBurlerRunner{}, "burler", ""},
		{"RelativeRunDir", &fakeBurlerRunner{}, "burler", "relative/runs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBurlerProducer(tt.pname, tt.runner, profile, burlerengine.RunOpts{}, tt.runDir, nil)
			if err == nil {
				t.Fatal("NewBurlerProducer() error = nil; want non-nil")
			}
		})
	}
}

func TestNewBurlerProducer_ValidCallSucceedsWithNoError(t *testing.T) {
	dir := t.TempDir()
	p, err := NewBurlerProducer("burler", &fakeBurlerRunner{}, simpleBurlerProfile(), burlerengine.RunOpts{}, filepath.Join(dir, "runs"), nil)
	if err != nil {
		t.Fatalf("NewBurlerProducer() error = %v; want nil", err)
	}
	if p == nil {
		t.Fatal("NewBurlerProducer() producer = nil; want non-nil")
	}
}

// --- Round-scan ---

func TestBurlerProducer_RoundScan(t *testing.T) {
	t.Run("AbsentRunDirIsCreatedAndStartsAtOne", func(t *testing.T) {
		base := t.TempDir()
		runDir := filepath.Join(base, "runs")
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if _, err := os.Stat(runDir); err != nil {
			t.Errorf("run dir was not created: %v", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.gotOpts[0].Round, "1")
		}
		if runner.gotProfiles[0].ReviewPath != roundReviewPath(runDir, 1) {
			t.Errorf("review path = %q; want %q", runner.gotProfiles[0].ReviewPath, roundReviewPath(runDir, 1))
		}
	})

	t.Run("EmptyRunDirStartsAtOne", func(t *testing.T) {
		runDir := t.TempDir()
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.gotOpts[0].Round, "1")
		}
	})

	t.Run("CompleteRoundNAdvancesToNPlus1", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 2)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "3" {
			t.Errorf("round token = %q; want %q", runner.gotOpts[0].Round, "3")
		}
		if runner.gotProfiles[0].ReviewPath != roundReviewPath(runDir, 3) {
			t.Errorf("review path = %q; want %q", runner.gotProfiles[0].ReviewPath, roundReviewPath(runDir, 3))
		}
	})

	t.Run("ReviewAbsentFixerReportPresentRerunsSameN", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, roundFixerReportPath(runDir, 1))
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.gotOpts[0].Round, "1")
		}
	})

	t.Run("ReviewOnlyOrphanCountsIncompleteAndIsArchivedAside", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		writeRoundFile(t, roundReviewPath(runDir, 2)) // orphan: no round-2 fixer report

		instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, fixedClock(instant))

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "2" {
			t.Errorf("round token = %q; want %q (re-run, not skipped)", runner.gotOpts[0].Round, "2")
		}
		wantStamped := filepath.Join(runDir, "round-2-review-20260820T101500Z.md")
		if _, err := os.Stat(wantStamped); err != nil {
			t.Errorf("orphan was not archived aside to %s: %v", wantStamped, err)
		}
	})

	t.Run("UnrelatedFilesUnaffected", func(t *testing.T) {
		runDir := t.TempDir()
		notes := filepath.Join(runDir, "notes.txt")
		writeRoundFile(t, notes)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q", runner.gotOpts[0].Round, "1")
		}
		if _, err := os.Stat(notes); err != nil {
			t.Errorf("unrelated file was disturbed: %v", err)
		}
	})

	t.Run("StampedArchiveSiblingNeverShiftsRound", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, filepath.Join(runDir, "round-2-review-20260820T101500Z.md"))
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q (stamped sibling never adopted)", runner.gotOpts[0].Round, "1")
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
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if runner.gotOpts[0].Round != "1" {
			t.Errorf("round token = %q; want %q (none of the malformed tokens adopted)", runner.gotOpts[0].Round, "1")
		}
	})
}

// --- Hydration ---

func TestBurlerProducer_Hydration(t *testing.T) {
	t.Run("Round1HydratesNothingBeyondTemplate", func(t *testing.T) {
		runDir := t.TempDir()
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, []string{}) {
			t.Errorf("PriorReviews = %v; want empty", runner.gotProfiles[0].PriorReviews)
		}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorFixerReports, []string{}) {
			t.Errorf("PriorFixerReports = %v; want empty", runner.gotProfiles[0].PriorFixerReports)
		}
	})

	t.Run("Rounds1And2CompleteRound3CarriesBothPriorsInOrder", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		writeRoundPair(t, runDir, 2)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{roundReviewPath(runDir, 1), roundReviewPath(runDir, 2)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{roundFixerReportPath(runDir, 1), roundFixerReportPath(runDir, 2)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v", runner.gotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("StampedArchiveSiblingNeverHydrated", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		writeRoundFile(t, filepath.Join(runDir, "round-1-review-20260820T101500Z.md"))
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
	})

	t.Run("RoundWithOnlyOneFileSkippedByHydrationEntirely", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundFile(t, roundReviewPath(runDir, 1)) // no fixer report: incomplete
		writeRoundPair(t, runDir, 2)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{roundReviewPath(runDir, 2)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (round 1's orphan never named)", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{roundFixerReportPath(runDir, 2)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v", runner.gotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("ToldPriorsPreservedAsPrefix", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		toldReview := filepath.Join(runDir, "told-review.md")
		toldFixer := filepath.Join(runDir, "told-fixer.md")
		writeRoundFile(t, toldReview)
		writeRoundFile(t, toldFixer)
		profile := simpleBurlerProfile()
		profile.PriorReviews = []string{toldReview}
		profile.PriorFixerReports = []string{toldFixer}
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, profile, burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{toldReview, roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (told entries as prefix)", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
		wantFixerReports := []string{toldFixer, roundFixerReportPath(runDir, 1)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorFixerReports, wantFixerReports) {
			t.Errorf("PriorFixerReports = %v; want %v (told entries as prefix)", runner.gotProfiles[0].PriorFixerReports, wantFixerReports)
		}
	})

	t.Run("FocusFileSurvivingHydrateEntriesAppendedToPriorReviews", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		hydrateEntry := filepath.Join(runDir, "extra-context.md")
		writeRoundFile(t, hydrateEntry)
		writeFocusFile(t, runDir, 2, `{"hydrate":["`+hydrateEntry+`"]}`)
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{roundReviewPath(runDir, 1), hydrateEntry}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (focus hydrate appended after derived entries)", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
	})
}

// --- Stale pre-existing round files ---

func TestBurlerProducer_StalePreexistingRoundFileArchivedBeforeInvocation(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	writeRoundFile(t, reviewPath)

	instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
	var reviewGoneAtInvoke, stampedExistsAtInvoke bool
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	runner.duringRun = func(int) {
		_, err := os.Stat(reviewPath)
		reviewGoneAtInvoke = os.IsNotExist(err)
		_, err = os.Stat(filepath.Join(runDir, "round-1-review-20260820T101500Z.md"))
		stampedExistsAtInvoke = err == nil
	}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, fixedClock(instant))

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if !reviewGoneAtInvoke {
		t.Error("stale review file still present at the round's own path when the runner was invoked")
	}
	if !stampedExistsAtInvoke {
		t.Error("stamped archive sibling did not exist by the time the runner was invoked")
	}
}
