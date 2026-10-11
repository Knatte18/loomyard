// fix_test.go tables the fix half (Engine.Fix, ProbeFix, ResumeFix) against the same-package fakeShuttle.
// It covers the one-seat table's shape, a spawn for a review holding only a NIT finding, the empty-review skip, a failing gate spawning the fixer with the gate findings as its work, a gate closure error spawning nothing, ErrReviewChanged before and after the fixer, a failed fixer gate passing through, probe and resume, and what the fixer's files hold.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// nitReview holds one NIT finding, so a fixer is spawned although the review is approved.
	nitReview = "---\nverdict: APPROVED\nfindings:\n" +
		"  - id: N1\n    severity: NIT\n    class: consistency\n    location: target.txt:1\n    summary: naming\n    fix: rename x to y in target.txt:1\n" +
		"---\n### [NIT] naming\n**Location:** target.txt:1\n**Issue:** naming\n"
)

// newFixCase writes review as the profile's review file and returns the engine root, the profile and the digest the review half would have handed over.
func newFixCase(t *testing.T, review string) (root string, p Profile, digest string) {
	t.Helper()
	root, p = newEngineTestProfile(t)
	if err := os.WriteFile(filepath.Join(root, "review.md"), []byte(review), 0o644); err != nil {
		t.Fatalf("WriteFile(review) = %v; want nil", err)
	}
	return root, p, reviewDigest([]byte(review))
}

// gateOf returns a one-entry gate spec named name whose closure returns result, err and counts its calls in calls.
func gateOf(name string, result shuttleengine.GateResult, err error, calls *int) shuttleengine.GateSpec {
	return shuttleengine.GateSpec{{Name: name, Attempts: 1, Gate: func() (shuttleengine.GateResult, error) {
		*calls++
		return result, err
	}}}
}

// fixInstructionFiles returns the content of the fix half's instruction files under the engine root, keyed by file name.
func fixInstructionFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(root, ".lyx", "burler", "fix-*"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("fix instruction dirs = %v, %v; want exactly one", dirs, err)
	}
	entries, err := os.ReadDir(dirs[0])
	if err != nil {
		t.Fatalf("ReadDir(%s) = %v; want nil", dirs[0], err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(dirs[0], entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) = %v; want nil", entry.Name(), err)
		}
		files[entry.Name()] = string(content)
	}
	return files
}

// TestEngine_Fix_TableShapeAndFiles proves Fix spawns exactly one seat for a review holding only a NIT finding, and builds its spec and gate as the fix half pins them.
// It also proves the instruction files hold the review path, the review format and the target paths and none of the rubric, fasit, PATTERN directive, focus file or prior rounds.
func TestEngine_Fix_TableShapeAndFiles(t *testing.T) {
	t.Parallel()

	root, p, digest := newFixCase(t, nitReview)
	writeFixtureFile(t, root, "focus.md", "focus text")
	writeFixtureFile(t, root, "prior-review.md", "prior review")
	p.FixScope = FixScopeOverlay
	p.FocusDirective = "focus.md"
	p.PriorReviews = []string{"prior-review.md"}
	shuttle := &fakeShuttle{fix: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, writes: "nothing fixed"}}
	e := newEngineForTest(t, root, shuttle)

	var gateCalls int
	opts := RunOpts{
		Review:  ModelChoice{Model: "haiku"},
		Fix:     ModelChoice{Model: "opus", Effort: "high", Version: "5"},
		Timeout: 5 * time.Minute,
		Round:   "1",
		Gate:    gateOf("build", shuttleengine.GateResult{Passed: false, Findings: "build broke"}, nil, &gateCalls),
	}
	got, err := e.Fix(p, opts, digest)
	if err != nil {
		t.Fatalf("Fix() = %v; want nil error", err)
	}

	if got.Outcome != shuttleengine.OutcomeDone || got.Skipped {
		t.Errorf("Fix() outcome %q skipped %v; want done and not skipped, since the review holds a finding", got.Outcome, got.Skipped)
	}
	if want := []string{fixHalfRole}; !slices.Equal(shuttle.started, want) {
		t.Fatalf("started seats = %v; want %v, the one chair", shuttle.started, want)
	}
	spec := shuttle.specs[fixHalfRole]
	if want := []string{filepath.Join(root, "fixer-report.md")}; !slices.Equal(spec.OutputFiles, want) {
		t.Errorf("spec.OutputFiles = %v; want %v", spec.OutputFiles, want)
	}
	if spec.Segment != segmentcolor.Review || spec.Interactive || spec.ForkSubagents {
		t.Errorf("spec segment %q interactive %v forks %v; want the review segment, neither interactive nor forking", spec.Segment, spec.Interactive, spec.ForkSubagents)
	}
	if spec.Model != opts.Fix.Model || spec.Effort != opts.Fix.Effort || spec.Version != opts.Fix.Version {
		t.Errorf("spec model = (%q, %q, %q); want %+v", spec.Model, spec.Effort, spec.Version, opts.Fix)
	}
	if spec.Timeout != opts.Timeout || spec.Round != opts.Round {
		t.Errorf("spec timeout %v round %q; want %v and %q", spec.Timeout, spec.Round, opts.Timeout, opts.Round)
	}
	gate := shuttle.gates[fixHalfRole]
	if len(gate) != 1 || gate[0].Name != "build" {
		t.Fatalf("gate = %+v; want the opts' one entry", gate)
	}
	failed, err := gate[0].Gate()
	if err != nil || !strings.Contains(failed.Findings, "build broke") || !strings.Contains(failed.Findings, filepath.Join(root, "fixer-report.md")) {
		t.Errorf("wrapped gate result = %+v, %v; want the gate's findings plus the instruction to rewrite the fixer report", failed, err)
	}

	files := fixInstructionFiles(t, root)
	fixStep, format := files["instruction-fix.md"], files["review-format.md"]
	for _, want := range []string{filepath.Join(root, "review.md"), filepath.Join(root, "target.txt")} {
		if !strings.Contains(fixStep+spec.Prompt, want) {
			t.Errorf("fix files and prompt do not contain %q", want)
		}
	}
	if !strings.Contains(format, "`fix` is the exact change to make and where") {
		t.Errorf("review-format.md = %q; want the review-file format", format)
	}
	all := spec.Prompt + fixStep + format
	for _, absent := range []string{p.Rubric, filepath.Join(root, "fasit.txt"), "focus text", "prior review", filepath.Join(root, "prior-review.md"), "## Constraints"} {
		if strings.Contains(all, absent) {
			t.Errorf("fixer prompt and files contain %q; want none of the rubric, fasit, PATTERN directive, focus file or prior rounds", absent)
		}
	}
}

// TestEngine_Fix_Outcomes table-drives Fix over the empty-review skip, a failing gate, a gate closure error, ErrReviewChanged on either side of the fixer, a fixer gate that failed and every non-done outcome and start and stop failure.
func TestEngine_Fix_Outcomes(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	notStarted := fmt.Errorf("shuttle: start run: %w", shuttleengine.ErrNotStarted)
	passing := shuttleengine.GateResult{Passed: true}
	failing := shuttleengine.GateResult{Findings: "build broke"}

	tests := []struct {
		name        string
		review      string
		staleDigest bool
		script      halfScript
		gateResult  shuttleengine.GateResult
		gateErr     error
		stopErr     error
		// editReview, when set, is written over the review file by the fixer before it reports done.
		editReview   string
		wantOutcome  shuttleengine.Outcome
		wantSkipped  bool
		wantSpawn    bool
		wantGate     bool
		wantNotStart bool
		wantErrIs    error
		wantErrText  string
	}{
		{name: "a review with findings spawns the fixer", review: blockingReview, script: halfScript{result: done, writes: "fixed"}, gateResult: passing, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true},
		{name: "an empty review over passing gates is skipped with no session", review: approvedReview, gateResult: passing, wantOutcome: shuttleengine.OutcomeDone, wantSkipped: true},
		{name: "an empty review over a failing gate spawns the fixer with the gate findings as its work", review: approvedReview, script: halfScript{result: done, writes: "fixed"}, gateResult: failing, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true, wantGate: true},
		{name: "a gate closure error spawns nothing", review: approvedReview, gateResult: passing, gateErr: errors.New("verify unavailable"), wantErrText: "gate \"build\" could not run"},
		{name: "a review changed since it was accepted is refused before the spawn", review: blockingReview, staleDigest: true, gateResult: passing, wantErrIs: ErrReviewChanged},
		{name: "a review changed by the fixer is refused after it", review: blockingReview, script: halfScript{result: done, writes: "fixed"}, gateResult: passing, editReview: approvedReview, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true, wantErrIs: ErrReviewChanged},
		{name: "a failed fixer gate passes through with no error", review: blockingReview, script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Reason: "spent"}}, writes: "fixed"}, gateResult: passing, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true, wantGate: true},
		{name: "a fixer that died", review: blockingReview, script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}, gateResult: passing, wantOutcome: shuttleengine.OutcomeDied, wantSpawn: true},
		{name: "a fixer that timed out", review: blockingReview, script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}, gateResult: passing, wantOutcome: shuttleengine.OutcomeTimeout, wantSpawn: true},
		{name: "a provider that never came up is a not-started death", review: blockingReview, script: halfScript{startErr: notStarted}, gateResult: passing, wantOutcome: shuttleengine.OutcomeDied, wantSpawn: true, wantNotStart: true},
		{name: "any other start error is wrapped", review: blockingReview, script: halfScript{startErr: errors.New("reed: add strand failed")}, gateResult: passing, wantSpawn: true, wantErrText: "burler: shuttle run: "},
		{name: "a timed-out fixer that cannot be stopped is ErrHalfNotStopped", review: blockingReview, script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}, stopErr: errors.New("reed unreachable"), gateResult: passing, wantOutcome: shuttleengine.OutcomeTimeout, wantSpawn: true, wantErrIs: ErrHalfNotStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p, digest := newFixCase(t, tt.review)
			if tt.staleDigest {
				digest = reviewDigest([]byte("an earlier review"))
			}
			script := tt.script
			if tt.editReview != "" {
				script.beforeReturn = func() {
					if err := os.WriteFile(filepath.Join(root, "review.md"), []byte(tt.editReview), 0o644); err != nil {
						t.Errorf("WriteFile(review) = %v; want nil", err)
					}
				}
			}
			shuttle := &fakeShuttle{fix: script, stopErr: tt.stopErr}
			e := newEngineForTest(t, root, shuttle)
			var calls int

			got, err := e.Fix(p, RunOpts{Gate: gateOf("build", tt.gateResult, tt.gateErr, &calls)}, digest)

			wantErr := tt.wantErrText != "" || tt.wantErrIs != nil
			if (err != nil) != wantErr {
				t.Fatalf("Fix() error = %v; wantErr %v", err, wantErr)
			}
			if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("Fix() error = %q; want it to contain %q", err, tt.wantErrText)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Fix() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if got.Outcome != tt.wantOutcome || got.Skipped != tt.wantSkipped || got.NotStarted != tt.wantNotStart {
				t.Errorf("outcome %q skipped %v notStarted %v; want %q, %v, %v", got.Outcome, got.Skipped, got.NotStarted, tt.wantOutcome, tt.wantSkipped, tt.wantNotStart)
			}
			if spawned := len(shuttle.started) == 1; spawned != tt.wantSpawn {
				t.Errorf("started seats = %v; want a spawn: %v", shuttle.started, tt.wantSpawn)
			}
			if gotGate := got.Gate != nil && !got.Gate.Passed; gotGate != tt.wantGate {
				t.Errorf("Gate = %+v; want a failed gate passed through: %v", got.Gate, tt.wantGate)
			}

			report, readErr := os.ReadFile(filepath.Join(root, "fixer-report.md"))
			if tt.wantSkipped {
				if readErr != nil || string(report) != skippedFixerReport {
					t.Errorf("fixer report = %q, %v; want the skipped report", report, readErr)
				}
			}
			if tt.name == "an empty review over a failing gate spawns the fixer with the gate findings as its work" {
				files := fixInstructionFiles(t, root)
				if !strings.Contains(files["gate-findings.md"], "## Gate build") || !strings.Contains(files["gate-findings.md"], "build broke") {
					t.Errorf("gate-findings.md = %q; want the failing entry's findings under its name", files["gate-findings.md"])
				}
				if !strings.Contains(files["instruction-fix.md"], "gate-findings.md") {
					t.Errorf("instruction-fix.md does not name the gate-findings file as the fixer's work")
				}
			}
		})
	}
}

// TestEngine_ProbeFix_And_ResumeFix proves ProbeFix classifies the fixer as live, done or gone from the fake's runs and the fixer report, names the report as the probed output and starts nothing,
// and that ResumeFix attaches to a live fixer and concludes it against the digest, refusing a half that is not live and one not taken from a probe.
func TestEngine_ProbeFix_And_ResumeFix(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	tests := []struct {
		name      string
		live      bool
		reportOn  bool
		wantState HalfState
	}{
		{name: "a live fixer", live: true, wantState: HalfLive},
		{name: "no live run and the report present", reportOn: true, wantState: HalfDone},
		{name: "no live run and no report", wantState: HalfGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p, digest := newFixCase(t, blockingReview)
			shuttle := &fakeShuttle{fix: halfScript{result: done, writes: "fixed"}, live: map[string]bool{fixHalfRole: tt.live}}
			if tt.reportOn {
				if err := os.WriteFile(filepath.Join(root, "fixer-report.md"), []byte("on disk"), 0o644); err != nil {
					t.Fatalf("WriteFile(report) = %v; want nil", err)
				}
			}
			e := newEngineForTest(t, root, shuttle)

			half, err := e.ProbeFix(p, RunOpts{Timeout: time.Minute})
			if err != nil {
				t.Fatalf("ProbeFix() = %v; want nil error", err)
			}
			if half.State != tt.wantState {
				t.Fatalf("state = %d; want %d", half.State, tt.wantState)
			}
			if spec := shuttle.probeSpecs[fixHalfRole]; !slices.Equal(spec.OutputFiles, []string{filepath.Join(root, "fixer-report.md")}) || spec.Timeout != time.Minute {
				t.Errorf("probe spec outputs %v timeout %v; want the fixer report and the opts' timeout", spec.OutputFiles, spec.Timeout)
			}
			if tt.wantState != HalfLive {
				if _, err := e.ResumeFix(p, RunOpts{}, digest, half); err == nil || !strings.Contains(err.Error(), "needs the fix half live") {
					t.Errorf("ResumeFix(not live) error = %v; want a refusal", err)
				}
				return
			}

			got, err := e.ResumeFix(p, RunOpts{}, digest, half)
			if err != nil || got.Outcome != shuttleengine.OutcomeDone {
				t.Fatalf("ResumeFix() = %+v, %v; want a done fix", got, err)
			}
			if _, err := e.ResumeFix(p, RunOpts{}, reviewDigest([]byte("an earlier review")), half); !errors.Is(err, ErrReviewChanged) {
				t.Errorf("ResumeFix(stale digest) error = %v; want it to wrap ErrReviewChanged", err)
			}
			if _, err := e.ResumeFix(p, RunOpts{}, digest, LiveHalf{State: HalfLive, Handle: half.Handle}); err == nil || !strings.Contains(err.Error(), "needs the table ProbeFix found") {
				t.Errorf("ResumeFix(hand-built) error = %v; want a refusal naming the probe", err)
			}
			if len(shuttle.started) != 0 {
				t.Errorf("ResumeFix started %v; want nothing started", shuttle.started)
			}
		})
	}
}
