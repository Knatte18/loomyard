// fix_test.go tables the fix half (Engine.Fix, ProbeFix, ResumeFix) against the same-package fakeShuttle.
// It covers the one-seat table's shape, a spawn for a review holding only a NIT finding, the empty-review skip, a failing gate spawning the fixer with the gate findings as its work, a gate closure error spawning nothing, ErrReviewChanged before and after the fixer, a failed fixer gate passing through, probe and resume, and what the fixer's files hold.

package burlerengine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
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

// TestEngine_Fix_TableShapeAndFiles proves Fix spawns exactly one seat for a review holding only a NIT finding, and builds its spec and gate as the fix half pins them, for a single fixer and for a twill fixer.
// It also proves the instruction files hold the review path, the review format and the target paths and none of the rubric, fasit, PATTERN directive, focus file or prior rounds.
// The single row's fix step equals the fix stencil with the self-check marker removed and has no lenses;
// the twill row's forks, empty seat model key, self-check part and lenses are the fixer fan's.
func TestEngine_Fix_TableShapeAndFiles(t *testing.T) {
	t.Parallel()

	for _, twill := range []bool{false, true} {
		name := "single fixer"
		if twill {
			name = "twill fixer"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			checkFixTableShapeAndFiles(t, twill)
		})
	}
}

// checkFixTableShapeAndFiles runs one row of TestEngine_Fix_TableShapeAndFiles: a fix over a NIT review whose profile names a fixer fan exactly when twill is set.
func checkFixTableShapeAndFiles(t *testing.T, twill bool) {
	t.Helper()

	root, p, digest := newFixCase(t, nitReview)
	writeFixtureFile(t, root, "focus.md", "focus text")
	writeFixtureFile(t, root, "prior-review.md", "prior review")
	p.FixScope = FixScopeOverlay
	p.FocusDirective = "focus.md"
	p.PriorReviews = []string{"prior-review.md"}
	p.Segment = segmentcolor.Merge
	p.LanguageSkills = true
	p.RolePrefix = "merge"
	fixRole := "merge-" + fixHalfRole
	cfg := testClusterFanConfig()
	if twill {
		p.FixFan = "standard"
	}
	shuttle := &fakeShuttle{fix: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, writes: "nothing fixed"}}
	stencilsDir := newTestStencilsDir(t)
	e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug"}, cfg, stencilsDir, "", []string{"scribe:golang-testing"})

	var gateCalls int
	opts := RunOpts{
		Review:  shuttleengine.ModelChoice{Model: "haiku"},
		Fix:     shuttleengine.ModelChoice{Engine: "b", Model: "opus", Effort: "high", Version: "5"},
		Timeout: 5 * time.Minute,
		Round:   "1",
		Gate:    gateOf("build", shuttleengine.GateResult{Passed: false, Findings: "build broke"}, nil, &gateCalls),
	}
	if twill {
		opts.FixKey = shuttleengine.ModelKey{Module: "loom", Key: "discussion_fix"}
		opts.FixFanKey = "discussion_fix_fan"
	}
	resolved := p
	if err := resolved.validateFix(root, cfg); err != nil {
		t.Fatalf("validateFix() = %v; want nil", err)
	}
	got, err := e.Fix(p, opts, digest)
	if err != nil {
		t.Fatalf("Fix() = %v; want nil error", err)
	}

	if got.Outcome != shuttleengine.OutcomeDone || got.Skipped {
		t.Errorf("Fix() outcome %q skipped %v; want done and not skipped, since the review holds a finding", got.Outcome, got.Skipped)
	}
	if want := []string{fixRole}; !slices.Equal(shuttle.started, want) {
		t.Fatalf("started seats = %v; want %v, the one chair", shuttle.started, want)
	}
	spec := shuttle.specs[fixRole]
	if want := []string{filepath.Join(root, "fixer-report.md")}; !slices.Equal(spec.OutputFiles, want) {
		t.Errorf("spec.OutputFiles = %v; want %v", spec.OutputFiles, want)
	}
	if spec.Segment != segmentcolor.Merge || spec.Interactive || spec.ForkSubagents != twill {
		t.Errorf("spec segment %q interactive %v forks %v; want the profile's merge segment, not interactive, forking %v", spec.Segment, spec.Interactive, spec.ForkSubagents, twill)
	}
	if seat := resolved.fixTable(opts, nil, nil).Seats[0]; seat.ModelKey != (shuttleengine.ModelKey{}) || seat.ForkSubagents != twill {
		t.Errorf("fixer seat model key %+v forks %v; want the zero key and forks %v", seat.ModelKey, seat.ForkSubagents, twill)
	}
	if spec.ModelChoice != opts.Fix {
		t.Errorf("spec model choice = %+v; want %+v", spec.ModelChoice, opts.Fix)
	}
	if spec.Timeout != opts.Timeout || spec.Round != opts.Round {
		t.Errorf("spec timeout %v round %q; want %v and %q", spec.Timeout, spec.Round, opts.Timeout, opts.Round)
	}
	if want := []string{"scribe:prose", "scribe:code-quality", "scribe:testing", "scribe:golang-testing"}; !slices.Equal(spec.Skills, want) {
		t.Errorf("spec.Skills = %v; want %v", spec.Skills, want)
	}
	gate := shuttle.gates[fixRole]
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
	if format == "" {
		t.Errorf("review-format.md is empty; want the review-file format written")
	}
	singleStep, err := stencil.FillOptional(bytes.ReplaceAll(stencils.BurlerStepFix, []byte("{{.self_check}}"), nil), map[string]string{
		"work":              fixWork(resolved.ReviewPath, ""),
		"fix_scope_rules":   fixScopeRules(&resolved),
		"review_path":       resolved.ReviewPath,
		"fixer_report_path": resolved.FixerReportPath,
	}, []string{friction.MarkerName})
	if err != nil {
		t.Fatalf("stencil.FillOptional(fix step without self_check) = %v; want nil", err)
	}
	if !twill {
		if fixStep != string(singleStep) {
			t.Errorf("single fix step = %q; want the fix stencil filled without the self_check marker, %q", fixStep, singleStep)
		}
		if got.Lenses != nil {
			t.Errorf("FixResult.Lenses = %v; want nil for a single fixer", got.Lenses)
		}
	} else {
		if fixStep == string(singleStep) {
			t.Errorf("twill fix step equals the single render; want the self-check part in it")
		}
		requireContains(t, fixStep, stencilstore.Path(stencilsDir, "burler-fix-check-rubric"))
		styleIdx, securityIdx := strings.Index(fixStep, "- style: style prose"), strings.Index(fixStep, "- security: security prose")
		if styleIdx == -1 || securityIdx == -1 || styleIdx >= securityIdx {
			t.Errorf("fix step lens bullets at idx style=%d security=%d; want both present, style first", styleIdx, securityIdx)
		}
		if want := []string{"style", "security"}; !slices.Equal(got.Lenses, want) {
			t.Errorf("FixResult.Lenses = %v; want %v", got.Lenses, want)
		}
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
		// fixFan makes the profile name a fixer fan, and wantForks and wantWarnings are the audited fork count and warnings the result must carry.
		fixFan       bool
		wantForks    int
		wantWarnings []string
	}{
		{name: "a twill fixer whose fork wrote is the violation error with its gate", review: blockingReview, script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Reason: "spent"}, ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "t1", ReportReturned: true, WriteCalls: 1}, {TranscriptPath: "t2", ReportReturned: true}}}}, writes: "fixed"}, gateResult: passing, fixFan: true, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true, wantGate: true, wantErrText: "burler: twill fixer fork \"t1\" attempted 1 write/edit tool call(s)", wantForks: 2},
		{name: "a twill fixer with no fork audit is a count warning", review: blockingReview, script: halfScript{result: done, writes: "fixed"}, gateResult: passing, fixFan: true, wantOutcome: shuttleengine.OutcomeDone, wantSpawn: true, wantWarnings: []string{"twill fixer spawned 0 fork(s) for 2 lens(es)"}},
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
		{name: "a single fixer's unmet-requirements start error keeps the generic wrap with no fan key", review: blockingReview, script: halfScript{startErr: fmt.Errorf("seatengine: start: %w", shuttleengine.ErrRequirementsUnmet)}, gateResult: passing, wantSpawn: true, wantErrIs: shuttleengine.ErrRequirementsUnmet, wantErrText: "burler: shuttle run: "},
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
			e := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, testClusterFanConfig(), shuttle)
			var calls int
			if tt.fixFan {
				p.FixFan = "standard"
			}

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
			if got.Forks != tt.wantForks || !slices.Equal(got.Warnings, tt.wantWarnings) {
				t.Errorf("forks %d warnings %v; want %d and %v", got.Forks, got.Warnings, tt.wantForks, tt.wantWarnings)
			}
			if tt.fixFan && tt.wantErrText != "" && !strings.Contains(err.Error(), "; way forward: inspect the target's working-tree changes") {
				t.Errorf("Fix() error = %q; want the twill fixer violation's way forward", err)
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

	refusals := []struct {
		name         string
		fixFan       string
		opts         RunOpts
		wantContains []string
	}{
		{
			name:   "a twill fixer under a loom key names the key, the fan key and the resume step",
			fixFan: "standard",
			opts:   RunOpts{FixKey: shuttleengine.ModelKey{Module: "loom", Key: "discussion_fix"}, FixFanKey: "discussion_fix_fan"},
			wantContains: []string{
				"burler: the twill fixer's fan from discussion_fix_fan needs fork subagents, which the engine of discussion_fix in loom.yaml cannot run",
				"move discussion_fix in loom.yaml to an engine with forks, then resume",
			},
		},
		{
			name:   "a twill fixer under the default engine names shuttle.yaml and the profile field",
			fixFan: "standard",
			opts:   RunOpts{FixKey: shuttleengine.ModelKey{DefaultEngine: true}, FixFanKey: "fix-fan"},
			wantContains: []string{
				"which the engine of engine: in shuttle.yaml cannot run",
				"remove fix-fan from the profile, and re-run",
			},
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p, digest := newFixCase(t, blockingReview)
			p.FixFan = tt.fixFan
			reed := &shuttlefake.Reed{}
			runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(&shuttlefake.Engine{}), root, root, shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30})
			e := New(seatengine.RunnerShuttle(runner), Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug"}, testClusterFanConfig(), newTestStencilsDir(t), "", nil)

			_, err := e.Fix(p, tt.opts, digest)

			if !errors.Is(err, shuttleengine.ErrRequirementsUnmet) {
				t.Fatalf("Fix() error = %v; want it to wrap ErrRequirementsUnmet", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Fix() error = %q; want it to contain %q", err, want)
				}
			}
			if n := strings.Count(err.Error(), "way forward:"); n != 1 {
				t.Errorf("Fix() error = %q; want exactly one way forward, found %d", err, n)
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
		// pending makes the live fixer's handle deliver a pending task prompt, and deliverErr makes that delivery fail.
		pending    bool
		deliverErr error
	}{
		{name: "a live fixer", live: true, wantState: HalfLive},
		{name: "a live fixer with a pending task prompt", live: true, pending: true, wantState: HalfLive},
		{name: "a live fixer whose prompt delivery fails", live: true, deliverErr: errors.New("pane gone"), wantState: HalfLive},
		{name: "no live run and the report present", reportOn: true, wantState: HalfDone},
		{name: "no live run and no report", wantState: HalfGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p, digest := newFixCase(t, blockingReview)
			shuttle := &fakeShuttle{fix: halfScript{result: done, writes: "fixed", pendingPrompt: tt.pending, deliverErr: tt.deliverErr}, live: map[string]bool{fixHalfRole: tt.live}}
			if tt.reportOn {
				if err := os.WriteFile(filepath.Join(root, "fixer-report.md"), []byte("on disk"), 0o644); err != nil {
					t.Fatalf("WriteFile(report) = %v; want nil", err)
				}
			}
			e := newEngineForTest(t, root, shuttle)
			e.languageSkills = []string{"scribe:golang-testing"}
			p.LanguageSkills = true

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
			if want := append(slices.Clone(burlerSkills), "scribe:golang-testing"); !slices.Equal(shuttle.probeSpecs[fixHalfRole].Skills, want) {
				t.Errorf("probe spec skills = %v; want %v", shuttle.probeSpecs[fixHalfRole].Skills, want)
			}
			if tt.wantState != HalfLive {
				if _, err := e.ResumeFix(p, RunOpts{}, digest, half); err == nil || !strings.Contains(err.Error(), "needs the fix half live") {
					t.Errorf("ResumeFix(not live) error = %v; want a refusal", err)
				}
				return
			}

			handle := half.Handle.(*fakeHandle)
			got, err := e.ResumeFix(p, RunOpts{}, digest, half)
			if tt.deliverErr != nil {
				if !errors.Is(err, tt.deliverErr) || handle.returned.Load() {
					t.Fatalf("ResumeFix() = %+v, %v; want the delivery error and nothing resumed", got, err)
				}
				return
			}
			if err != nil || got.Outcome != shuttleengine.OutcomeDone {
				t.Fatalf("ResumeFix() = %+v, %v; want a done fix", got, err)
			}
			if deliveries := handle.deliveries.Load(); deliveries != 1 {
				t.Errorf("DeliverPendingPrompt calls = %d; want one, before the seats resume", deliveries)
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
