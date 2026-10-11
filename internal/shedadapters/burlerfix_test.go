// burlerfix_test.go pins BurlerFixProducer over shedfake.BurlerHalfRunner:
// It covers the refusal of a round with no accepted review, the hand-on of an unjudged round, and the fix a finished round carries and records.
// It also covers the gate-failed and changed-review exits and their archive sets, every other failure exit, cancellation, and probe and resume.

package shedadapters

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// newFixProducer builds a BurlerFixProducer over runDir whose review row is named "burler-review", failing the test on constructor error.
func newFixProducer(t *testing.T, runDir string, runner *shedfake.BurlerHalfRunner, spec halfProducerSpec) *BurlerFixProducer {
	t.Helper()
	p, err := NewBurlerFixProducer("burler-fix", "burler-review", BurlerHalfDeps{Runner: runner, Models: spec.models}, spec.profile, spec.opts, runDir, spec.now)
	if err != nil {
		t.Fatalf("NewBurlerFixProducer() error = %v; want nil", err)
	}
	return p
}

// fixDone is a finished fix.
var fixDone = burlerengine.FixResult{Outcome: shuttleengine.OutcomeDone}

// wayForwardToReview is the way forward every refusal that needs the round reviewed again ends with.
const wayForwardToReview = `way forward: run "lyx loom goto --to burler-review", then "lyx loom resume", in the task worktree`

// TestBurlerFixProducer_Call_RefusesARoundWithNoAcceptedReview proves a round with no acceptance record, and one whose record has no review beside it, is a hard error.
// The error names the way forward back to the review row, with nothing spawned and nothing archived.
func TestBurlerFixProducer_Call_RefusesARoundWithNoAcceptedReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, runDir string)
	}{
		{name: "an empty run directory", setup: func(t *testing.T, runDir string) {}},
		{name: "a review with no record", setup: func(t *testing.T, runDir string) { writeRoundFile(t, roundReviewPath(runDir, 1)) }},
		{name: "a record with no review", setup: func(t *testing.T, runDir string) {
			if err := writeAcceptance(runDir, 1, "d1"); err != nil {
				t.Fatalf("writeAcceptance: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			tt.setup(t, runDir)
			runner := &shedfake.BurlerHalfRunner{FixResults: []burlerengine.FixResult{fixDone}}
			p := newFixProducer(t, runDir, runner, newHalfSpec())

			_, _, err := p.Call(context.Background())

			want := "round 1 has no accepted review; " + wayForwardToReview
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("Call() error = %v; want one containing %q", err, want)
			}
			if runner.FixCalls != 0 || runner.ProbeFixCalls != 0 {
				t.Errorf("Fix calls %d, ProbeFix calls %d; want none", runner.FixCalls, runner.ProbeFixCalls)
			}
			requireArchived(t, runDir, "round-1-review.md", 0)
			requireArchived(t, runDir, "round-1-acceptance.md", 0)
		})
	}
}

// TestBurlerFixProducer_Call_HandsOnAnUnjudgedRound proves the highest complete round with no recorded verdict is handed on BudgetExempt with its review as Path.
// It spawns, probes and archives nothing, so a fixer report already on disk is passed through to the Bouncer.
func TestBurlerFixProducer_Call_HandsOnAnUnjudgedRound(t *testing.T) {
	t.Parallel()
	runDir := t.TempDir()
	writeRoundPair(t, runDir, 1)
	runner := &shedfake.BurlerHalfRunner{FixResults: []burlerengine.FixResult{fixDone}}
	p := newFixProducer(t, runDir, runner, newHalfSpec())

	ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

	if ptr.Path != roundReviewPath(runDir, 1) || ptr.Reason != "round 1 is complete but unjudged; handing on for judgment" || !ptr.BudgetExempt {
		t.Errorf("Call() pointer = %+v; want round 1's review, the unjudged reason and BudgetExempt", ptr)
	}
	if runner.FixCalls != 0 || runner.ProbeFixCalls != 0 {
		t.Errorf("Fix calls %d, ProbeFix calls %d; want none", runner.FixCalls, runner.ProbeFixCalls)
	}
	requireArchived(t, runDir, "round-1-review.md", 0)
	requireArchived(t, runDir, "round-1-fixer-report.md", 0)
}

// TestBurlerFixProducer_Call_FinishedFixIsRecorded proves a finished fix, a skipped empty review included, hands on as a BudgetExempt Stuck, never Done, with the review as Path and the gate attempts.
// It also proves the fix runs under the recorded digest, the round token, the picked fix model, the friction note id and the template's gate, and merges the fix half's usage beside the review half's.
func TestBurlerFixProducer_Call_FinishedFixIsRecorded(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		result burlerengine.FixResult
	}{
		{name: "a spawned fixer", result: burlerengine.FixResult{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 2}, Fix: burlerengine.Half{StartedAt: started, EndedAt: started.Add(time.Minute)}}},
		{name: "a skipped empty review", result: burlerengine.FixResult{Outcome: shuttleengine.OutcomeDone, Skipped: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := filepath.Join(t.TempDir(), "plan")
			writeJudgedRound(t, runDir, 1)
			writeAcceptedRound(t, runDir, 2, "digest-2")
			if err := writeRoundUsage(runDir, 2, roundUsage{Fan: "fanX", Review: &halfUsage{Model: "reviewer"}}); err != nil {
				t.Fatalf("writeRoundUsage: %v", err)
			}
			spec := newHalfSpec()
			spec.opts = burlerengine.RunOpts{Timeout: 3 * time.Minute, Gate: shuttleengine.GateSpec{{Name: "verify", Attempts: 1}}}
			spec.models = burlerengine.RoundModels{Fix: []burlerengine.ModelChoice{{Model: "f1"}, {Model: "f2"}}}
			runner := &shedfake.BurlerHalfRunner{FixResults: []burlerengine.FixResult{tt.result}}
			p := newFixProducer(t, runDir, runner, spec)

			ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)

			if ptr.Path != roundReviewPath(runDir, 2) || !ptr.BudgetExempt || (ptr.GateAttempts != nil) != (tt.result.Gate != nil) {
				t.Errorf("Call() pointer = %+v; want round 2's review, BudgetExempt and gate attempts iff the fix was gated", ptr)
			}
			if runner.FixCalls != 1 || runner.GotFixDigests[0] != "digest-2" {
				t.Fatalf("Fix calls %d with digests %v; want one call under the recorded digest", runner.FixCalls, runner.GotFixDigests)
			}
			profile, opts := runner.GotFixProfiles[0], runner.GotFixOpts[0]
			if profile.ReviewPath != roundReviewPath(runDir, 2) || profile.FixerReportPath != roundFixerReportPath(runDir, 2) {
				t.Errorf("profile paths = %q, %q; want round 2's review and fixer report", profile.ReviewPath, profile.FixerReportPath)
			}
			if opts.Round != "2" || opts.Fix.Model != "f2" || opts.NoteID != "burler-plan-r2" || opts.Timeout != 3*time.Minute || len(opts.Gate) != 1 {
				t.Errorf("opts = %+v; want round 2, f2, note id burler-plan-r2 and the template's timeout and gate", opts)
			}
			usage, err := readCompletedRoundUsage(runDir, 2)
			if err != nil || usage.Review.Model != "reviewer" || usage.Fix == nil || usage.Fix.Model != "f2" {
				t.Errorf("usage record = %+v, %v; want the fix half merged beside the review half", usage, err)
			}
			requireArchived(t, runDir, "round-2-review.md", 0)
			requireArchived(t, runDir, "round-2-acceptance.md", 0)
		})
	}
}

// TestBurlerFixProducer_Call_GateFailedAndChangedReviewArchiveTheWholeRound proves a finished fixer whose gate failed is a counted Stuck with an empty Path, the gate's reason and attempts.
// It proves a review that changed since it was accepted is a hard error ending with the way forward, and both exits archive the review, fixer report and acceptance record together.
func TestBurlerFixProducer_Call_GateFailedAndChangedReviewArchiveTheWholeRound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		result     burlerengine.FixResult
		err        error
		wantStuck  bool
		wantErrIs  error
		wantErrEnd string
	}{
		{name: "a failed gate", result: burlerengine.FixResult{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Attempts: 3, Reason: "verify is red"}}, wantStuck: true},
		{name: "a changed review", err: burlerengine.ErrReviewChanged, wantErrIs: burlerengine.ErrReviewChanged, wantErrEnd: wayForwardToReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			writeAcceptedRound(t, runDir, 1, "d1")
			runner := &shedfake.BurlerHalfRunner{FixResults: []burlerengine.FixResult{tt.result}, FixErrs: []error{tt.err}}
			runner.DuringFix = func(int) { writeRoundFile(t, roundFixerReportPath(runDir, 1)) }
			p := newFixProducer(t, runDir, runner, newHalfSpec())

			outcome, ptr, err := p.Call(context.Background())

			if tt.wantStuck {
				if err != nil || outcome != shedengine.Stuck || ptr.Path != "" || ptr.BudgetExempt || ptr.Reason != "verify is red" || ptr.GateAttempts == nil || *ptr.GateAttempts != 3 {
					t.Errorf("Call() = %q, %+v, %v; want a counted Stuck with an empty Path, the gate's reason and 3 attempts", outcome, ptr, err)
				}
			} else if !errors.Is(err, tt.wantErrIs) || !strings.HasSuffix(err.Error(), tt.wantErrEnd) {
				t.Errorf("Call() error = %v; want it to wrap %v and end with %q", err, tt.wantErrIs, tt.wantErrEnd)
			}
			for _, base := range []string{"round-1-review.md", "round-1-fixer-report.md", "round-1-acceptance.md"} {
				requireArchived(t, runDir, base, 1)
			}
			if runner.FixCalls != 1 {
				t.Errorf("Fix calls = %d; want 1, since neither exit is retried", runner.FixCalls)
			}
		})
	}
}

// TestBurlerFixProducer_Call_FailureExits table-drives every other way a fix ends without finishing:
// a died/timeout attempt is retried under the token <n>b.
// Two in a row, a runner error and an unrecognized outcome are hard errors that archive the fixer report alone and keep the accepted review and its record.
// A fixer that could not be stopped returns bare with nothing archived and no retry.
func TestBurlerFixProducer_Call_FailureExits(t *testing.T) {
	t.Parallel()
	died := burlerengine.FixResult{Outcome: shuttleengine.OutcomeDied}
	timeout := burlerengine.FixResult{Outcome: shuttleengine.OutcomeTimeout}
	notStarted := burlerengine.FixResult{Outcome: shuttleengine.OutcomeDied, NotStarted: true}

	tests := []struct {
		name         string
		results      []burlerengine.FixResult
		errs         []error
		wantErr      string
		wantErrIs    error
		wantCalls    int
		wantArchived int
	}{
		{name: "died then done is retried", results: []burlerengine.FixResult{died, fixDone}, wantCalls: 2, wantArchived: 1},
		// Each attempt leaves a report: the first is archived before the second, the second on the failure exit.
		{name: "two died/timeout outcomes in a row", results: []burlerengine.FixResult{died, timeout}, wantErr: "two consecutive died/timeout outcomes", wantCalls: 2, wantArchived: 2},
		{name: "two not-started deaths wrap ErrNotStarted", results: []burlerengine.FixResult{notStarted, notStarted}, wantErr: "two consecutive", wantErrIs: shuttleengine.ErrNotStarted, wantCalls: 2, wantArchived: 2},
		{name: "a runner error is not retried", errs: []error{errors.New("gate closure failed")}, wantErr: "gate closure failed", wantCalls: 1, wantArchived: 1},
		{name: "an unrecognized outcome", results: []burlerengine.FixResult{{Outcome: "weird"}}, wantErr: `unrecognized burler outcome "weird"`, wantCalls: 1, wantArchived: 1},
		{name: "a fixer that could not be stopped", errs: []error{burlerengine.ErrHalfNotStopped}, wantErrIs: burlerengine.ErrHalfNotStopped, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			writeAcceptedRound(t, runDir, 1, "d1")
			runner := &shedfake.BurlerHalfRunner{FixResults: tt.results, FixErrs: tt.errs}
			runner.DuringFix = func(int) { writeRoundFile(t, roundFixerReportPath(runDir, 1)) }
			p := newFixProducer(t, runDir, runner, newHalfSpec())

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
				t.Error("Call() outcome = Done; the fix row never returns Done")
			}
			if runner.FixCalls != tt.wantCalls {
				t.Errorf("Fix calls = %d; want %d", runner.FixCalls, tt.wantCalls)
			}
			if tt.name == "died then done is retried" {
				if runner.GotFixOpts[0].Round != "1" || runner.GotFixOpts[1].Round != "1b" {
					t.Errorf("round tokens = %q, %q; want 1 then 1b", runner.GotFixOpts[0].Round, runner.GotFixOpts[1].Round)
				}
			}
			requireArchived(t, runDir, "round-1-fixer-report.md", tt.wantArchived)
			requireArchived(t, runDir, "round-1-review.md", 0)
			requireArchived(t, runDir, "round-1-acceptance.md", 0)
		})
	}
}

// TestBurlerFixProducer_Call_Cancellation proves a context cancelled before the call and one cancelled during a failing attempt return the cancellation error.
// It proves one cancelled after the fix finished still returns it while the finished round's artifacts survive.
func TestBurlerFixProducer_Call_Cancellation(t *testing.T) {
	t.Parallel()

	t.Run("before the call nothing runs", func(t *testing.T) {
		t.Parallel()
		runner := &shedfake.BurlerHalfRunner{}
		p := newFixProducer(t, t.TempDir(), runner, newHalfSpec())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) || runner.FixCalls != 0 {
			t.Errorf("Call() error = %v with %d Fix calls; want context.Canceled and none", err, runner.FixCalls)
		}
	})

	t.Run("during a failing attempt the report is archived", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		writeAcceptedRound(t, runDir, 1, "d1")
		ctx, cancel := context.WithCancel(context.Background())
		runner := &shedfake.BurlerHalfRunner{FixErrs: []error{errors.New("boom")}}
		runner.DuringFix = func(int) { writeRoundFile(t, roundFixerReportPath(runDir, 1)); cancel() }
		p := newFixProducer(t, runDir, runner, newHalfSpec())

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want context.Canceled over the runner's error", err)
		}
		requireArchived(t, runDir, "round-1-fixer-report.md", 1)
	})

	t.Run("after a finished fix its artifacts survive", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		writeAcceptedRound(t, runDir, 1, "d1")
		ctx, cancel := context.WithCancel(context.Background())
		runner := &shedfake.BurlerHalfRunner{FixResults: []burlerengine.FixResult{fixDone}}
		runner.DuringFix = func(int) { writeRoundFile(t, roundFixerReportPath(runDir, 1)); cancel() }
		p := newFixProducer(t, runDir, runner, newHalfSpec())

		if _, _, err := p.Call(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want context.Canceled", err)
		}
		for _, base := range []string{"round-1-review.md", "round-1-fixer-report.md", "round-1-acceptance.md"} {
			requireArchived(t, runDir, base, 0)
		}
	})
}

// TestBurlerFixProducer_Call_ProbeAndResume proves a live fixer is probed before anything is archived and resumed, against the recorded digest, instead of respawned.
// A resumed finished fix is mapped like a spawned one, a resumed death falls through to a fresh attempt, and a half that is not live proceeds to the spawn.
// A probe error or an unstoppable fixer returns bare, and a resumed changed review archives the whole round.
func TestBurlerFixProducer_Call_ProbeAndResume(t *testing.T) {
	t.Parallel()
	live := burlerengine.LiveHalf{State: burlerengine.HalfLive}

	tests := []struct {
		name          string
		probe         []burlerengine.LiveHalf
		probeErr      error
		resume        burlerengine.FixResult
		resumeErr     error
		wantErr       string
		wantErrIs     error
		wantFixes     int
		wantResumes   int
		wantReviewArc int
	}{
		{name: "not live proceeds to the spawn", wantFixes: 1},
		{name: "a live fixer is resumed and finishes", probe: []burlerengine.LiveHalf{live}, resume: fixDone, wantResumes: 1},
		{name: "a resumed death falls through to a fresh attempt", probe: []burlerengine.LiveHalf{live}, resume: burlerengine.FixResult{Outcome: shuttleengine.OutcomeTimeout}, wantResumes: 1, wantFixes: 1},
		{name: "a resumed unrecognized outcome is a hard error", probe: []burlerengine.LiveHalf{live}, resume: burlerengine.FixResult{Outcome: "weird"}, wantErr: "unrecognized outcome", wantResumes: 1},
		{name: "a probe error returns bare", probeErr: errors.New("reed unreadable"), wantErr: "probe the fixer"},
		{name: "an unstoppable fixer returns bare", probe: []burlerengine.LiveHalf{live}, resumeErr: burlerengine.ErrHalfNotStopped, wantErrIs: burlerengine.ErrHalfNotStopped, wantResumes: 1},
		{name: "a resumed changed review archives the whole round", probe: []burlerengine.LiveHalf{live}, resumeErr: burlerengine.ErrReviewChanged, wantErrIs: burlerengine.ErrReviewChanged, wantResumes: 1, wantReviewArc: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			writeAcceptedRound(t, runDir, 1, "recorded")
			runner := &shedfake.BurlerHalfRunner{
				LiveFixes: tt.probe, ProbeFixErrs: []error{tt.probeErr},
				ResumeFixResults: []burlerengine.FixResult{tt.resume}, ResumeFixErrs: []error{tt.resumeErr},
				FixResults: []burlerengine.FixResult{fixDone},
			}
			runner.DuringFix = func(int) { writeRoundFile(t, roundFixerReportPath(runDir, 1)) }
			p := newFixProducer(t, runDir, runner, newHalfSpec())

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
			if runner.FixCalls != tt.wantFixes || runner.ResumeFixCalls != tt.wantResumes {
				t.Errorf("Fix calls %d, ResumeFix calls %d; want %d and %d", runner.FixCalls, runner.ResumeFixCalls, tt.wantFixes, tt.wantResumes)
			}
			if tt.wantResumes > 0 && (runner.GotResumeFixDigests[0] != "recorded" || runner.GotLiveFixes[0] != live) {
				t.Errorf("ResumeFix digests %v live %+v; want the recorded digest and the probed half", runner.GotResumeFixDigests, runner.GotLiveFixes)
			}
			requireArchived(t, runDir, "round-1-review.md", tt.wantReviewArc)
			requireArchived(t, runDir, "round-1-acceptance.md", tt.wantReviewArc)
			if !wantErr && ptr.Path != roundReviewPath(runDir, 1) {
				t.Errorf("Call() Path = %q; want round 1's review", ptr.Path)
			}
			if len(runner.GotProbeFixOpts) != 1 || runner.GotProbeFixOpts[0].Round != "1" {
				t.Errorf("probe opts = %+v; want one probe under round token 1", runner.GotProbeFixOpts)
			}
		})
	}
}
