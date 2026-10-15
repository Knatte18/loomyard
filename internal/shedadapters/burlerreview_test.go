// burlerreview_test.go pins BurlerReviewProducer and the pieces both half producers share over shedfake.BurlerHalfRunner:
// It covers the constructors' validation, the acceptance record, the round scan and hand-ons, the profile each round carries, and the accepted exit and its records.
// It also covers every failure exit's archive set, cancellation, and probe and resume.

package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// Compile-time proof that *shedfake.BurlerHalfRunner satisfies BurlerHalfRunner.
var _ BurlerHalfRunner = (*shedfake.BurlerHalfRunner)(nil)

// halfProducerSpec is what a half producer is built from beyond its runner and run directory.
type halfProducerSpec struct {
	profile burlerengine.Profile
	opts    burlerengine.RunOpts
	models  burlerengine.RoundModels
	now     func() time.Time
}

// newHalfSpec returns a spec over the minimal profile and a fixed clock.
func newHalfSpec() halfProducerSpec {
	return halfProducerSpec{profile: simpleBurlerProfile(), now: fixedClock(archiveTestInstant)}
}

// newReviewProducer builds a BurlerReviewProducer over runDir, failing the test on constructor error.
func newReviewProducer(t *testing.T, runDir string, runner *shedfake.BurlerHalfRunner, spec halfProducerSpec) *BurlerReviewProducer {
	t.Helper()
	p, err := NewBurlerReviewProducer("burler-review", BurlerHalfDeps{Runner: runner, Models: spec.models}, spec.profile, spec.opts, runDir, spec.now)
	if err != nil {
		t.Fatalf("NewBurlerReviewProducer() error = %v; want nil", err)
	}
	return p
}

// writeAcceptedRound writes round n's review and its acceptance record, the state a finished review row leaves.
func writeAcceptedRound(t *testing.T, runDir string, n int, digest string) {
	t.Helper()
	writeRoundFile(t, roundReviewPath(runDir, n))
	if err := writeAcceptance(runDir, n, digest); err != nil {
		t.Fatalf("writeAcceptance(round %d): %v", n, err)
	}
}

// reviewDone is an accepted review with the given digest.
func reviewDone(digest string) burlerengine.ReviewResult {
	return burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeDone, Digest: digest}
}

// requireArchived fails the test unless the file base under dir has exactly want stamped siblings.
func requireArchived(t *testing.T, dir, base string, want int) {
	t.Helper()
	if got := stampedSiblingCount(t, dir, base); got != want {
		t.Errorf("stamped siblings of %s = %d; want %d", base, got, want)
	}
}

// TestNewBurlerHalfProducers_Validation proves both constructors refuse a nil runner, an empty name, an empty or relative run directory, and the fix constructor an empty review row.
func TestNewBurlerHalfProducers_Validation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs")
	profile := simpleBurlerProfile()
	fake := &shedfake.BurlerHalfRunner{}

	tests := []struct {
		name      string
		runner    BurlerHalfRunner
		pname     string
		reviewRow string
		runDir    string
		wantErr   string
	}{
		{name: "valid", runner: fake, pname: "p", reviewRow: "row", runDir: runs},
		{name: "nil runner", pname: "p", reviewRow: "row", runDir: runs, wantErr: "runner must not be nil"},
		{name: "empty name", runner: fake, reviewRow: "row", runDir: runs, wantErr: "name must not be empty"},
		{name: "empty run dir", runner: fake, pname: "p", reviewRow: "row", wantErr: "runDir must not be empty"},
		{name: "relative run dir", runner: fake, pname: "p", reviewRow: "row", runDir: "relative/runs", wantErr: "is not absolute"},
		{name: "empty review row", runner: fake, pname: "p", runDir: runs, wantErr: "review row must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := BurlerHalfDeps{Runner: tt.runner}
			_, reviewErr := NewBurlerReviewProducer(tt.pname, deps, profile, burlerengine.RunOpts{}, tt.runDir, nil)
			_, fixErr := NewBurlerFixProducer(tt.pname, tt.reviewRow, deps, profile, burlerengine.RunOpts{}, tt.runDir, nil)
			wantReviewErr := tt.wantErr != "" && tt.wantErr != "review row must not be empty"
			if (reviewErr != nil) != wantReviewErr {
				t.Errorf("NewBurlerReviewProducer() error = %v; want an error: %v", reviewErr, wantReviewErr)
			}
			if (fixErr != nil) != (tt.wantErr != "") || (fixErr != nil && !strings.Contains(fixErr.Error(), tt.wantErr)) {
				t.Errorf("NewBurlerFixProducer() error = %v; want one containing %q", fixErr, tt.wantErr)
			}
		})
	}
}

// TestAcceptanceRecord proves the record round-trips a digest.
// It proves an absent record, one that does not parse, one naming another round and one with no digest all read as absent with a nil error, while an unreadable one is an error.
func TestAcceptanceRecord(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		content    string
		asDir      bool
		wantDigest string
		wantFound  bool
		wantErr    bool
	}{
		{name: "round trip", wantDigest: "abc123", wantFound: true},
		{name: "absent"},
		{name: "not frontmatter", content: "garbage"},
		{name: "another round", content: "---\nround: 2\ndigest: abc\n---\n"},
		{name: "no digest", content: "---\nround: 1\n---\n"},
		{name: "unreadable", asDir: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			switch {
			case tt.wantFound:
				if err := writeAcceptance(runDir, 1, tt.wantDigest); err != nil {
					t.Fatalf("writeAcceptance() = %v; want nil", err)
				}
			case tt.asDir:
				if err := os.Mkdir(roundAcceptancePath(runDir, 1), 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			case tt.content != "":
				writeRoundFileWith(t, roundAcceptancePath(runDir, 1), tt.content)
			}

			digest, found, err := readAcceptance(runDir, 1)

			if (err != nil) != tt.wantErr || found != tt.wantFound || digest != tt.wantDigest {
				t.Errorf("readAcceptance() = %q, %v, %v; want %q, %v, error %v", digest, found, err, tt.wantDigest, tt.wantFound, tt.wantErr)
			}
		})
	}
}

// writeRoundFileWith writes content at path, creating its parent directory.
func writeRoundFileWith(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// TestBurlerReviewProducer_Call_RoundScan table-drives the run directory's state against the round the review row works on and what it does there:
// a fresh round, the successor of a judged round, and stray names that are never adopted.
// An unjudged highest complete round or an already accepted review is handed on without spawning, and the orphans (a review with no record, a record with no review) are archived and reviewed again.
func TestBurlerReviewProducer_Call_RoundScan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, runDir string)
		// wantRound is the round the runner is asked to review, 0 for a hand-on that spawns nothing.
		wantRound int
		// wantHandOn is the review path round the hand-on points at, with its reason.
		wantHandOn    int
		wantReason    string
		wantArchived  map[string]int
		wantSurviving []string
	}{
		{name: "an absent run directory starts at round 1", wantRound: 1},
		{name: "the successor of a judged round", setup: func(t *testing.T, runDir string) { writeJudgedRound(t, runDir, 2) }, wantRound: 3},
		{
			name: "the successor of a converged round, as a Webster reroute after an approved episode, is reviewed",
			setup: func(t *testing.T, runDir string) {
				writeRoundPair(t, runDir, 2)
				if err := os.WriteFile(verdictPath(runDir, 2), []byte(bouncerVerdictContent("CONVERGED")), 0o644); err != nil {
					t.Fatalf("WriteFile(verdict round 2): %v", err)
				}
				if err := os.WriteFile(ledgerPath(runDir, 2), []byte(bouncerLedgerContent(2)), 0o644); err != nil {
					t.Fatalf("WriteFile(ledger round 2): %v", err)
				}
			},
			wantRound: 3,
		},
		{
			name: "stamped, attempt-suffixed and zero-padded names are never adopted",
			setup: func(t *testing.T, runDir string) {
				writeJudgedRound(t, runDir, 1)
				for _, stray := range []string{"round-2-review-20260820T101500Z.md", "round-3b-review.md", "round-02-review.md", "round-2-fixer-report.md"} {
					writeRoundFile(t, filepath.Join(runDir, stray))
				}
			},
			wantRound: 2,
		},
		{
			name:       "an unjudged highest complete round is handed on",
			setup:      func(t *testing.T, runDir string) { writeRoundPair(t, runDir, 1) },
			wantHandOn: 1,
			wantReason: "round 1 is complete but unjudged; handing on for judgment",
		},
		{
			name: "an accepted review is handed on to the fixer",
			setup: func(t *testing.T, runDir string) {
				writeJudgedRound(t, runDir, 1)
				writeAcceptedRound(t, runDir, 2, "d2")
			},
			wantHandOn:    2,
			wantReason:    "round 2 review is accepted; handing on to the fixer",
			wantSurviving: []string{"round-2-review.md", "round-2-acceptance.md"},
		},
		{
			name:         "a review with no acceptance record is reviewed again",
			setup:        func(t *testing.T, runDir string) { writeRoundFile(t, roundReviewPath(runDir, 1)) },
			wantRound:    1,
			wantArchived: map[string]int{"round-1-review.md": 1},
		},
		{
			name: "a review and fixer report with no acceptance record are archived and reviewed again",
			setup: func(t *testing.T, runDir string) {
				writeRoundFile(t, roundReviewPath(runDir, 1))
				writeRoundFile(t, roundFixerReportPath(runDir, 1))
			},
			wantRound:    1,
			wantArchived: map[string]int{"round-1-review.md": 1, "round-1-fixer-report.md": 1},
		},
		{
			name: "an acceptance record with no review is archived",
			setup: func(t *testing.T, runDir string) {
				writeAcceptedRound(t, runDir, 1, "d1")
				_ = os.Remove(roundReviewPath(runDir, 1))
			},
			wantRound:    1,
			wantArchived: map[string]int{"round-1-acceptance.md": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := filepath.Join(t.TempDir(), "runs")
			if tt.setup != nil {
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				tt.setup(t, runDir)
			}
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
			p := newReviewProducer(t, runDir, runner, newHalfSpec())

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

			if !ptr.BudgetExempt {
				t.Error("Call() BudgetExempt = false; want true, since the review row has no counted exit")
			}
			if tt.wantRound > 0 {
				if runner.ReviewCalls != 1 || runner.GotReviewProfiles[0].ReviewPath != roundReviewPath(runDir, tt.wantRound) {
					t.Errorf("Review calls %d with profiles %+v; want one call for round %d", runner.ReviewCalls, runner.GotReviewProfiles, tt.wantRound)
				}
				if runner.GotReviewOpts[0].Round != strconv.Itoa(tt.wantRound) {
					t.Errorf("round token = %q; want %d", runner.GotReviewOpts[0].Round, tt.wantRound)
				}
				if ptr.Path != roundReviewPath(runDir, tt.wantRound) {
					t.Errorf("Call() Path = %q; want the reviewed round's review", ptr.Path)
				}
			}
			if tt.wantHandOn > 0 {
				if runner.ReviewCalls != 0 || runner.ProbeReviewCalls != 0 {
					t.Errorf("Review calls %d, ProbeReview calls %d; want a hand-on that spawns and probes nothing", runner.ReviewCalls, runner.ProbeReviewCalls)
				}
				if ptr.Path != roundReviewPath(runDir, tt.wantHandOn) || ptr.Reason != tt.wantReason {
					t.Errorf("Call() pointer = %+v; want round %d's review with reason %q", ptr, tt.wantHandOn, tt.wantReason)
				}
			}
			for base, want := range tt.wantArchived {
				requireArchived(t, runDir, base, want)
			}
			for _, name := range tt.wantSurviving {
				if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
					t.Errorf("%s did not survive: %v", name, err)
				}
			}
		})
	}
}

// TestBurlerReviewProducer_Call_ProfileAndOpts proves the profile and options each review carries:
// the round's own review and fixer-report paths, the focus file, and the template priors followed by the earlier complete rounds in order.
// It also proves the cluster exclusions of an eligible focus file, the round token, the picked models and the opts template.
func TestBurlerReviewProducer_Call_ProfileAndOpts(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	writeJudgedRound(t, runDir, 1)
	writeJudgedRound(t, runDir, 2)
	if err := writeRoundUsage(runDir, 2, roundUsage{Fan: "fanX", Lenses: []string{"a", "b", "c"}, Review: &halfUsage{}}); err != nil {
		t.Fatalf("writeRoundUsage: %v", err)
	}
	focus := writeFocusFile(t, runDir, 3, focusFile{Round: 3, ExcludeLenses: []string{"a"}, Focus: []string{"look at the seam"}})

	spec := newHalfSpec()
	spec.profile.ClusterFan = "fanX"
	spec.profile.PriorReviews = []string{"template-review.md"}
	spec.opts = burlerengine.RunOpts{Timeout: 7 * time.Minute}
	spec.models = burlerengine.RoundModels{
		Review: []burlerengine.ModelChoice{{Model: "r1"}, {Model: "r2"}, {Model: "r3"}},
		Fix:    []burlerengine.ModelChoice{{Model: "f1"}},
	}
	runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
	p := newReviewProducer(t, runDir, runner, spec)

	shedfake.CallOK(t, p)

	got, opts := runner.GotReviewProfiles[0], runner.GotReviewOpts[0]
	if got.ReviewPath != roundReviewPath(runDir, 3) || got.FixerReportPath != roundFixerReportPath(runDir, 3) || got.FocusDirective != focus {
		t.Errorf("profile paths = %q, %q, %q; want round 3's review, fixer report and focus file", got.ReviewPath, got.FixerReportPath, got.FocusDirective)
	}
	wantReviews := []string{"template-review.md", roundReviewPath(runDir, 1), roundReviewPath(runDir, 2)}
	if !stringSlicesEqual(got.PriorReviews, wantReviews) {
		t.Errorf("PriorReviews = %v; want %v", got.PriorReviews, wantReviews)
	}
	if want := []string{roundFixerReportPath(runDir, 1), roundFixerReportPath(runDir, 2)}; !stringSlicesEqual(got.PriorFixerReports, want) {
		t.Errorf("PriorFixerReports = %v; want %v", got.PriorFixerReports, want)
	}
	if !stringSlicesEqual(got.ClusterExclude, []string{"a"}) {
		t.Errorf("ClusterExclude = %v; want [a]", got.ClusterExclude)
	}
	if opts.Round != "3" || opts.Timeout != 7*time.Minute || opts.Review.Model != "r3" || opts.Fix.Model != "f1" {
		t.Errorf("opts = %+v; want round 3, the template timeout and round 3's model picks", opts)
	}
	if len(spec.profile.PriorReviews) != 1 {
		t.Errorf("the stored template's PriorReviews = %v; want it untouched", spec.profile.PriorReviews)
	}
}

// TestBurlerReviewProducer_Call_AcceptedReviewIsRecorded proves an accepted review hands on as a BudgetExempt Stuck, never Done, with the review as Path.
// It proves the acceptance record carries the result's digest and the review half's usage, fan and lenses are merged into the round's usage record.
func TestBurlerReviewProducer_Call_AcceptedReviewIsRecorded(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	spec := newHalfSpec()
	spec.profile.ClusterFan = "fanX"
	spec.models = burlerengine.RoundModels{Review: []burlerengine.ModelChoice{{Model: "reviewer", Effort: "low"}}}
	started := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	result := burlerengine.ReviewResult{
		Outcome: shuttleengine.OutcomeDone,
		Digest:  "digest-1",
		Lenses:  []string{"style"},
		Review:  burlerengine.Half{StartedAt: started, EndedAt: started.Add(time.Minute), Usage: shuttleengine.SessionUsage{Known: true, Fresh: 100, ForkFresh: 25, Forks: 2}},
	}
	runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{result}}
	p := newReviewProducer(t, runDir, runner, spec)

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

	if ptr.Path != roundReviewPath(runDir, 1) || !ptr.BudgetExempt || ptr.GateAttempts != nil {
		t.Errorf("Call() pointer = %+v; want round 1's review, BudgetExempt and no gate attempts", ptr)
	}
	if digest, found, err := readAcceptance(runDir, 1); digest != "digest-1" || !found || err != nil {
		t.Errorf("acceptance record = %q, %v, %v; want digest-1 recorded", digest, found, err)
	}
	usage, err := readCompletedRoundUsage(runDir, 1)
	if err != nil {
		t.Fatalf("readCompletedRoundUsage() = %v; want nil", err)
	}
	if usage.Fan != "fanX" || !stringSlicesEqual(usage.Lenses, []string{"style"}) || usage.Review.Model != "reviewer" || usage.Review.Effort != "low" || usage.Fix != nil {
		t.Errorf("usage record = %+v; want the review half only, with its fan, lenses and model", usage)
	}
	if usage.Review.Forks == nil || *usage.Review.Forks != 2 || usage.Review.ForkFreshShare == nil || *usage.Review.ForkFreshShare != 0.25 {
		t.Errorf("review usage forks = %v share = %v; want 2 forks and a quarter fresh share", usage.Review.Forks, usage.Review.ForkFreshShare)
	}
}

// TestBurlerReviewProducer_Call_FailureExits table-drives every way a review ends without an accepted review.
// A retried died/timeout attempt gets its stale outputs archived and the token <n>b; two in a row, a runner error, an unrecognized outcome and a failed acceptance write are hard errors with the round's review and acceptance record archived;
// a reviewer that could not be stopped returns bare, with nothing archived and no retry.
func TestBurlerReviewProducer_Call_FailureExits(t *testing.T) {
	t.Parallel()
	died := burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeDied}
	timeout := burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeTimeout}
	notStarted := burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeDied, NotStarted: true}

	tests := []struct {
		name         string
		results      []burlerengine.ReviewResult
		errs         []error
		duringReview func(t *testing.T, runDir string, i int)
		wantErr      string
		wantErrIs    error
		wantCalls    int
		wantArchived int
	}{
		{name: "died then done is retried", results: []burlerengine.ReviewResult{died, reviewDone("d")}, wantCalls: 2},
		// Each attempt leaves a review file: the first is archived before the second, the second on the failure exit.
		{name: "two died/timeout outcomes in a row", results: []burlerengine.ReviewResult{died, timeout}, wantErr: "two consecutive died/timeout outcomes", wantCalls: 2, wantArchived: 2},
		{name: "two not-started deaths wrap ErrNotStarted", results: []burlerengine.ReviewResult{notStarted, notStarted}, wantErr: "two consecutive", wantErrIs: shuttleengine.ErrNotStarted, wantCalls: 2, wantArchived: 2},
		{name: "a runner error is not retried", errs: []error{errors.New("strict parse refused")}, wantErr: "strict parse refused", wantCalls: 1, wantArchived: 1},
		{name: "an unrecognized outcome", results: []burlerengine.ReviewResult{{Outcome: "weird"}}, wantErr: `unrecognized burler outcome "weird"`, wantCalls: 1, wantArchived: 1},
		{
			name:    "a failed acceptance write",
			results: []burlerengine.ReviewResult{reviewDone("d")},
			duringReview: func(t *testing.T, runDir string, i int) {
				writeRoundFile(t, roundReviewPath(runDir, 1))
				if err := os.Mkdir(roundAcceptancePath(runDir, 1), 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			},
			wantErr: "write round 1 acceptance record", wantCalls: 1, wantArchived: 1,
		},
		{name: "a reviewer that could not be stopped", errs: []error{burlerengine.ErrHalfNotStopped}, wantErrIs: burlerengine.ErrHalfNotStopped, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			runner := &shedfake.BurlerHalfRunner{ReviewResults: tt.results, ReviewErrs: tt.errs}
			runner.DuringReview = func(i int) {
				if tt.duringReview != nil {
					tt.duringReview(t, runDir, i)
					return
				}
				writeRoundFile(t, roundReviewPath(runDir, 1))
			}
			p := newReviewProducer(t, runDir, runner, newHalfSpec())

			outcome, _, err := p.Call(context.Background())

			wantErr := tt.wantErr != "" || tt.wantErrIs != nil
			if (err != nil) != wantErr {
				t.Fatalf("Call() = %q, %v; want an error: %v", outcome, err, wantErr)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Call() error = %q; want it to contain %q", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Call() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if outcome == shedengine.Done {
				t.Error("Call() outcome = Done; the review row never returns Done")
			}
			if runner.ReviewCalls != tt.wantCalls {
				t.Errorf("Review calls = %d; want %d", runner.ReviewCalls, tt.wantCalls)
			}
			if wantErr {
				requireArchived(t, runDir, "round-1-review.md", tt.wantArchived)
			}
			if tt.name == "died then done is retried" {
				if runner.GotReviewOpts[0].Round != "1" || runner.GotReviewOpts[1].Round != "1b" {
					t.Errorf("round tokens = %q, %q; want 1 then 1b", runner.GotReviewOpts[0].Round, runner.GotReviewOpts[1].Round)
				}
				requireArchived(t, runDir, "round-1-review.md", 1)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_Cancellation proves a context cancelled before the call, during a failing attempt and after an accepted review all return the cancellation error.
// The second archives the round, and the third keeps the accepted review and its record.
func TestBurlerReviewProducer_Call_Cancellation(t *testing.T) {
	t.Parallel()

	t.Run("before the call nothing runs", func(t *testing.T) {
		t.Parallel()
		runner := &shedfake.BurlerHalfRunner{}
		p := newReviewProducer(t, t.TempDir(), runner, newHalfSpec())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) || runner.ReviewCalls != 0 {
			t.Errorf("Call() error = %v with %d Review calls; want context.Canceled and none", err, runner.ReviewCalls)
		}
	})

	t.Run("during a failing attempt the round is archived", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		runner := &shedfake.BurlerHalfRunner{ReviewErrs: []error{errors.New("boom")}}
		runner.DuringReview = func(int) { writeRoundFile(t, roundReviewPath(runDir, 1)); cancel() }
		p := newReviewProducer(t, runDir, runner, newHalfSpec())

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want context.Canceled over the runner's error", err)
		}
		requireArchived(t, runDir, "round-1-review.md", 1)
	})

	t.Run("after an accepted review its artifacts survive", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d1")}}
		runner.DuringReview = func(int) { writeRoundFile(t, roundReviewPath(runDir, 1)); cancel() }
		p := newReviewProducer(t, runDir, runner, newHalfSpec())

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want context.Canceled", err)
		}
		requireArchived(t, runDir, "round-1-review.md", 0)
		if digest, found, _ := readAcceptance(runDir, 1); digest != "d1" || !found {
			t.Errorf("acceptance record = %q, %v; want d1 recorded before the cancellation was observed", digest, found)
		}
	})
}

// TestBurlerReviewProducer_Call_ProbeAndResume proves a live reviewer is probed before anything is archived and resumed instead of respawned.
// A resumed done review is accepted like a spawned one, a resumed death falls through to a fresh attempt, and a half that is not live proceeds to the spawn.
// A probe error or an unstoppable reviewer returns bare without archiving the stale review on disk, and any other resume error archives the round.
func TestBurlerReviewProducer_Call_ProbeAndResume(t *testing.T) {
	t.Parallel()
	live := burlerengine.LiveHalf{State: burlerengine.HalfLive}

	tests := []struct {
		name         string
		probe        []burlerengine.LiveHalf
		probeErr     error
		resume       burlerengine.ReviewResult
		resumeErr    error
		wantErr      string
		wantErrIs    error
		wantReviews  int
		wantResumes  int
		wantArchived int
		wantRecorded string
	}{
		{name: "not live proceeds to the spawn", wantReviews: 1, wantArchived: 1, wantRecorded: "spawned"},
		{name: "a live reviewer is resumed and accepted", probe: []burlerengine.LiveHalf{live}, resume: reviewDone("resumed"), wantResumes: 1, wantRecorded: "resumed"},
		{name: "a resumed death falls through to a fresh attempt", probe: []burlerengine.LiveHalf{live}, resume: burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeDied}, wantResumes: 1, wantReviews: 1, wantArchived: 1, wantRecorded: "spawned"},
		{name: "a resumed unrecognized outcome archives the round", probe: []burlerengine.LiveHalf{live}, resume: burlerengine.ReviewResult{Outcome: "weird"}, wantErr: "unrecognized outcome", wantResumes: 1, wantArchived: 1},
		{name: "a probe error returns bare", probeErr: errors.New("reed unreadable"), wantErr: "probe the reviewer"},
		{name: "an unstoppable reviewer returns bare", probe: []burlerengine.LiveHalf{live}, resumeErr: burlerengine.ErrHalfNotStopped, wantErrIs: burlerengine.ErrHalfNotStopped, wantResumes: 1},
		{name: "any other resume error archives the round", probe: []burlerengine.LiveHalf{live}, resumeErr: errors.New("strict parse refused"), wantErr: "strict parse refused", wantResumes: 1, wantArchived: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			// A stale review of the round: archived by a spawn, untouched by every exit that must not archive.
			writeRoundFile(t, roundReviewPath(runDir, 1))
			runner := &shedfake.BurlerHalfRunner{
				LiveReviews: tt.probe, ProbeReviewErrs: []error{tt.probeErr},
				ResumeReviewResults: []burlerengine.ReviewResult{tt.resume}, ResumeReviewErrs: []error{tt.resumeErr},
				ReviewResults: []burlerengine.ReviewResult{reviewDone("spawned")},
			}
			p := newReviewProducer(t, runDir, runner, newHalfSpec())

			_, ptr, err := p.Call(context.Background())

			wantErr := tt.wantErr != "" || tt.wantErrIs != nil
			if (err != nil) != wantErr {
				t.Fatalf("Call() error = %v; want an error: %v", err, wantErr)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Call() error = %q; want it to contain %q", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Call() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if runner.ReviewCalls != tt.wantReviews || runner.ResumeReviewCalls != tt.wantResumes {
				t.Errorf("Review calls %d, ResumeReview calls %d; want %d and %d", runner.ReviewCalls, runner.ResumeReviewCalls, tt.wantReviews, tt.wantResumes)
			}
			requireArchived(t, runDir, "round-1-review.md", tt.wantArchived)
			if !wantErr {
				if digest, _, _ := readAcceptance(runDir, 1); digest != tt.wantRecorded || ptr.Path != roundReviewPath(runDir, 1) {
					t.Errorf("recorded digest %q and Path %q; want %q and round 1's review", digest, ptr.Path, tt.wantRecorded)
				}
			}
			if len(runner.GotProbeReviewOpts) != 1 || runner.GotProbeReviewOpts[0].Round != "1" {
				t.Errorf("probe opts = %+v; want one probe under round token 1", runner.GotProbeReviewOpts)
			}
		})
	}
}
