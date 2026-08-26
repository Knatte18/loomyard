package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeBurlerRunner records every Profile/RunOpts it is handed and returns caller-scripted
// burlerengine.Result/error values per invocation, so a test can script attempt 1 and attempt 2
// differently. results[i]/errs[i] answer the (i+1)th invocation; once either slice is exhausted its
// own last entry repeats. duringRun, when set, is invoked from inside Run before it returns --
// letting a test cancel the context or write files as if mid-round, mirroring
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
// The attach seam defaults to a fakeShuttle finding nothing live, which is the ordinary
// no-live-round condition every pre-existing case in this file assumes; a case that wants the probe
// to find something uses newTestBurlerProducerWithAttach instead.
func newTestBurlerProducer(t *testing.T, runDir string, profile burlerengine.Profile, opts burlerengine.RunOpts, runner *fakeBurlerRunner, now func() time.Time) *BurlerProducer {
	t.Helper()
	return newTestBurlerProducerWithAttach(t, runDir, profile, opts, runner, &fakeShuttle{}, now)
}

// newTestBurlerProducerWithAttach is newTestBurlerProducer with the attach seam supplied, for the
// cases that script what the live-round probe finds.
func newTestBurlerProducerWithAttach(t *testing.T, runDir string, profile burlerengine.Profile, opts burlerengine.RunOpts, runner *fakeBurlerRunner, attach Shuttle, now func() time.Time) *BurlerProducer {
	t.Helper()
	p, err := NewBurlerProducer("burler", runner, attach, profile, opts, runDir, now)
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
		attach Shuttle
		pname  string
		runDir string
	}{
		{"NilRunner", nil, &fakeShuttle{}, "burler", filepath.Join(dir, "runs")},
		{"NilAttachSeam", &fakeBurlerRunner{}, nil, "burler", filepath.Join(dir, "runs")},
		{"EmptyName", &fakeBurlerRunner{}, &fakeShuttle{}, "", filepath.Join(dir, "runs")},
		{"EmptyRunDir", &fakeBurlerRunner{}, &fakeShuttle{}, "burler", ""},
		{"RelativeRunDir", &fakeBurlerRunner{}, &fakeShuttle{}, "burler", "relative/runs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBurlerProducer(tt.pname, tt.runner, tt.attach, profile, burlerengine.RunOpts{}, tt.runDir, nil)
			if err == nil {
				t.Fatal("NewBurlerProducer() error = nil; want non-nil")
			}
		})
	}
}

func TestNewBurlerProducer_ValidCallSucceedsWithNoError(t *testing.T) {
	dir := t.TempDir()
	p, err := NewBurlerProducer("burler", &fakeBurlerRunner{}, &fakeShuttle{}, simpleBurlerProfile(), burlerengine.RunOpts{}, filepath.Join(dir, "runs"), nil)
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

	t.Run("FocusFileWithADirectiveIsHydratedAfterDerivedEntries", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		writeFocusFile(t, runDir, 2, focusFile{Round: 2, Focus: []string{"look at the relocation candidate"}})
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		// The hydrated entry is the focus file itself: that is how the judge's directive reaches the
		// fixer round, and it is the delivery that was silently absent while the reader looked for a
		// round-<N>-focus.json the writer never produced.
		wantReviews := []string{roundReviewPath(runDir, 1), focusPath(runDir, 2)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (focus file appended after derived entries)", runner.gotProfiles[0].PriorReviews, wantReviews)
		}
	})

	t.Run("FocusFileWithNoDirectiveIsNotHydrated", func(t *testing.T) {
		runDir := t.TempDir()
		writeRoundPair(t, runDir, 1)
		writeFocusFile(t, runDir, 2, focusFile{Round: 2, ExcludeLenses: []string{}, Focus: []string{}})
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantReviews := []string{roundReviewPath(runDir, 1)}
		if !stringSlicesEqual(runner.gotProfiles[0].PriorReviews, wantReviews) {
			t.Errorf("PriorReviews = %v; want %v (an APPROVED judge's empty focus file asserts nothing and is not handed to the round)", runner.gotProfiles[0].PriorReviews, wantReviews)
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

// --- Call outcomes ---

func TestBurlerProducer_Call_DoneReturnsStuckNeverDone(t *testing.T) {
	tests := []struct {
		name    string
		verdict burlerengine.Verdict
	}{
		{"Approved", burlerengine.VerdictApproved},
		{"Blocking", burlerengine.VerdictBlocking},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDir := t.TempDir()
			runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone, Verdict: tt.verdict}}}
			p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

			outcome, ptr, err := p.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want %q (never shedengine.Done)", outcome, shedengine.Stuck)
			}
			wantPath := roundReviewPath(runDir, 1)
			if ptr.Path != wantPath {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPath)
			}
		})
	}
}

func TestBurlerProducer_Call_ProfileCarriesDerivedFields(t *testing.T) {
	runDir := t.TempDir()
	writeRoundPair(t, runDir, 1)
	writeFocusFile(t, runDir, 2, focusFile{Round: 2, ExcludeLenses: []string{"lensA"}})
	profile := simpleBurlerProfile()
	profile.ClusterFan = "fanX"
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newTestBurlerProducer(t, runDir, profile, burlerengine.RunOpts{}, runner, nil)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	got := runner.gotProfiles[0]
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
}

func TestBurlerProducer_Call_RunOptsCarriesRoundToken(t *testing.T) {
	runDir := t.TempDir()
	writeRoundPair(t, runDir, 4)
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{Model: "m"}, runner, nil)

	if _, _, err := p.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if runner.gotOpts[0].Round != "5" {
		t.Errorf("Round = %q; want %q", runner.gotOpts[0].Round, "5")
	}
	if runner.gotOpts[0].Model != "m" {
		t.Errorf("Model = %q; want %q (opts template carried through)", runner.gotOpts[0].Model, "m")
	}
}

func TestBurlerProducer_Call_DiedThenDoneSucceedsWithRetry(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	fixerPath := roundFixerReportPath(runDir, 1)
	runner := &fakeBurlerRunner{
		results: []burlerengine.Result{
			{Outcome: shuttleengine.OutcomeDied, SessionID: "s1"},
			{Outcome: shuttleengine.OutcomeDone},
		},
	}
	runner.duringRun = func(i int) {
		if i == 1 {
			// Attempt 2 (the second invocation) is the one that succeeds and writes the review.
			writeRoundFile(t, reviewPath)
			writeRoundFile(t, fixerPath)
		}
	}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	outcome, ptr, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if runner.calls != 2 {
		t.Fatalf("runner.calls = %d; want 2", runner.calls)
	}
	if runner.gotOpts[0].Round != "1" {
		t.Errorf("attempt 1 round token = %q; want %q", runner.gotOpts[0].Round, "1")
	}
	if runner.gotOpts[1].Round != "1b" {
		t.Errorf("attempt 2 round token = %q; want %q", runner.gotOpts[1].Round, "1b")
	}
	if runner.gotProfiles[1].ReviewPath != runner.gotProfiles[0].ReviewPath {
		t.Errorf("attempt 2 review path = %q; want unchanged from attempt 1 (%q)", runner.gotProfiles[1].ReviewPath, runner.gotProfiles[0].ReviewPath)
	}
	if ptr.Path != reviewPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, reviewPath)
	}
	if _, statErr := os.Stat(reviewPath); statErr != nil {
		t.Errorf("review written by the successful attempt did not survive: %v", statErr)
	}
}

func TestBurlerProducer_Call_TimeoutTwiceIsHardErrorNamingBothSessions(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{
		results: []burlerengine.Result{
			{Outcome: shuttleengine.OutcomeTimeout, SessionID: "s1", RunDir: "/kept/1"},
			{Outcome: shuttleengine.OutcomeTimeout, SessionID: "s2", RunDir: "/kept/2"},
		},
	}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "s1") || !strings.Contains(err.Error(), "s2") {
		t.Errorf("Call() error %q does not name both attempts' session ids", err.Error())
	}
	if runner.calls != 2 {
		t.Errorf("runner.calls = %d; want 2", runner.calls)
	}
}

func TestBurlerProducer_Call_AskingIsHardErrorOnFirstOccurrence(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeAsking, LastAssistantMessage: "what next?"}}}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "what next?") {
		t.Errorf("Call() error %q does not carry LastAssistantMessage", err.Error())
	}
	if runner.calls != 1 {
		t.Errorf("runner.calls = %d; want 1 (asking is never retried)", runner.calls)
	}
}

func TestBurlerProducer_Call_RunnerErrorWrapped(t *testing.T) {
	runDir := t.TempDir()
	seamErr := errors.New("seam exploded")
	runner := &fakeBurlerRunner{errs: []error{seamErr}}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	_, _, err := p.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, seamErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, seamErr)
	}
}

func TestBurlerProducer_Call_FocusClusterExcludeReachesRunnerAsRunnableProfile(t *testing.T) {
	runDir := t.TempDir()
	writeFocusFile(t, runDir, 1, focusFile{Round: 1, ExcludeLenses: []string{"not-in-fan"}})
	profile := simpleBurlerProfile()
	profile.ClusterFan = "fanX"
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newTestBurlerProducer(t, runDir, profile, burlerengine.RunOpts{}, runner, nil)

	outcome, _, err := p.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil (a directive naming a lens the fan lacks reaches the runner rather than erroring)", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if !stringSlicesEqual(runner.gotProfiles[0].ClusterExclude, []string{"not-in-fan"}) {
		t.Errorf("ClusterExclude = %v; want %v (the producer never validates fan membership itself)", runner.gotProfiles[0].ClusterExclude, []string{"not-in-fan"})
	}
}

// --- Call cancellation ---

func TestBurlerProducer_Call_AlreadyCancelledContext(t *testing.T) {
	runDir := t.TempDir()
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if runner.calls != 0 {
		t.Errorf("runner.calls = %d; want 0", runner.calls)
	}
}

func TestBurlerProducer_Call_CancelledBetweenAttempts(t *testing.T) {
	runDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDied}}}
	runner.duringRun = func(int) { cancel() }
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

	_, _, err := p.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if runner.calls != 1 {
		t.Errorf("runner.calls = %d; want 1 (attempt 2 never spawned)", runner.calls)
	}
}

func TestBurlerProducer_Call_CancelledDuringFailedRoundArchives(t *testing.T) {
	runDir := t.TempDir()
	reviewPath := roundReviewPath(runDir, 1)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeAsking}}}
	runner.duringRun = func(int) {
		writeRoundFile(t, reviewPath)
		cancel()
	}
	instant := time.Date(2026, 8, 20, 10, 15, 0, 0, time.UTC)
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, fixedClock(instant))

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
	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	runner.duringRun = func(int) {
		writeRoundFile(t, reviewPath)
		writeRoundFile(t, fixerPath)
		cancel()
	}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

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

	runner2 := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p2 := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner2, nil)
	if _, _, err := p2.Call(context.Background()); err != nil {
		t.Fatalf("second Call() error = %v; want nil", err)
	}
	if runner2.gotOpts[0].Round != "2" {
		t.Errorf("second Call() round token = %q; want %q (advances past the completed round)", runner2.gotOpts[0].Round, "2")
	}
}

// --- Archive-on-exit ---

func TestBurlerProducer_Call_ArchiveOnExit(t *testing.T) {
	// assertUnchangedRoundOnRerun drives a fresh Call over runDir and asserts the round token it
	// resolves is still "1" -- proof the prior Call's failure archived its round rather than
	// leaving it looking complete.
	assertUnchangedRoundOnRerun := func(t *testing.T, runDir string) {
		t.Helper()
		runner2 := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
		p2 := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner2, nil)
		if _, _, err := p2.Call(context.Background()); err != nil {
			t.Fatalf("second Call() error = %v; want nil", err)
		}
		if runner2.gotOpts[0].Round != "1" {
			t.Errorf("second Call() round token = %q; want %q (unchanged from the failed first Call())", runner2.gotOpts[0].Round, "1")
		}
	}

	t.Run("TwoConsecutiveDiedWithPhaseAOnlyPartial", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		runner := &fakeBurlerRunner{
			results: []burlerengine.Result{
				{Outcome: shuttleengine.OutcomeDied, SessionID: "s1"},
				{Outcome: shuttleengine.OutcomeDied, SessionID: "s2"},
			},
		}
		runner.duringRun = func(i int) {
			if i == 0 {
				// Attempt 1's phase-A-only partial: review written, fixer report never reached.
				writeRoundFile(t, reviewPath)
			}
		}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})

	t.Run("AskingHardError", func(t *testing.T) {
		runDir := t.TempDir()
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeAsking}}}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})

	t.Run("CancellationBetweenAttempts", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		ctx, cancel := context.WithCancel(context.Background())
		runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDied}}}
		runner.duringRun = func(int) {
			writeRoundFile(t, reviewPath)
			cancel()
		}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

		if _, _, err := p.Call(ctx); err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		assertUnchangedRoundOnRerun(t, runDir)
	})

	t.Run("RunnerErrorWithDoneOutcomeAndBothFilesWritten", func(t *testing.T) {
		runDir := t.TempDir()
		reviewPath := roundReviewPath(runDir, 1)
		fixerPath := roundFixerReportPath(runDir, 1)
		seamErr := errors.New("burler: round reached done but its review file is invalid")
		runner := &fakeBurlerRunner{
			results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}},
			errs:    []error{seamErr},
		}
		runner.duringRun = func(int) {
			writeRoundFile(t, reviewPath)
			writeRoundFile(t, fixerPath)
		}
		p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, nil)

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

	runner := &fakeBurlerRunner{results: []burlerengine.Result{{Outcome: shuttleengine.OutcomeDone}}}
	p := newTestBurlerProducer(t, runDir, simpleBurlerProfile(), burlerengine.RunOpts{}, runner, now)

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
	if runner.calls != 0 {
		t.Errorf("runner.calls = %d; want 0", runner.calls)
	}
}
