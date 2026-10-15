// review_test.go tables the review half (Engine.Review, ProbeReview, ResumeReview) against the same-package fakeShuttle.
// It covers the one-seat table's shape, an accepted review's digest, verdict and findings, the strict-parse and cluster-audit refusals, every non-done outcome, the start and stop failures, and probe and resume.

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

// doneReview scripts a review half whose reviewer writes review and is done.
func doneReview(review string) *fakeShuttle {
	return &fakeShuttle{review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, writes: review}}
}

// TestEngine_Review_TableShape proves Review starts exactly one seat and builds its spec and gate as the review half pins them:
// the review role, segment and skills, the review file as the only output, the opts' review model choice, timeout and round, the review-parse gate that passes on its cap, and ForkSubagents exactly when the profile names a cluster fan.
func TestEngine_Review_TableShape(t *testing.T) {
	t.Parallel()

	opts := RunOpts{
		Review:  ModelChoice{Model: "haiku", Effort: "low", Version: "5"},
		Fix:     ModelChoice{Model: "opus", Effort: "high"},
		Timeout: 5 * time.Minute,
		Round:   "1",
	}
	tests := []struct {
		name           string
		clusterFan     string
		segment        segmentcolor.Segment
		rolePrefix     string
		languageSkills []string
		codeRound      bool
		wantSegment    segmentcolor.Segment
		wantRole       string
		wantForkFlag   bool
		wantSkills     []string
	}{
		{name: "solo reviewer", wantSegment: segmentcolor.Review, wantRole: reviewHalfRole, wantSkills: burlerSkills},
		{name: "cluster reviewer", clusterFan: "standard", wantSegment: segmentcolor.Review, wantRole: reviewHalfRole, wantForkFlag: true, wantSkills: burlerSkills},
		{name: "merge-segment reviewer", segment: segmentcolor.Merge, rolePrefix: "merge", wantSegment: segmentcolor.Merge, wantRole: "merge-" + reviewHalfRole, wantSkills: burlerSkills},
		{
			name:           "code round",
			languageSkills: []string{"scribe:golang-testing", "scribe:testing"},
			codeRound:      true,
			wantSegment:    segmentcolor.Review,
			wantRole:       reviewHalfRole,
			wantSkills:     []string{"scribe:prose", "scribe:code-quality", "scribe:testing", "scribe:golang-testing"},
		},
		{
			name:           "non-code round with a language list",
			languageSkills: []string{"scribe:golang-testing", "scribe:testing"},
			wantSegment:    segmentcolor.Review,
			wantRole:       reviewHalfRole,
			wantSkills:     burlerSkills,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			p.ClusterFan = tt.clusterFan
			p.Segment = tt.segment
			p.LanguageSkills = tt.codeRound
			p.RolePrefix = tt.rolePrefix
			shuttle := doneReview(approvedReview)
			e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug"}, clusterTestConfig, newTestStencilsDir(t), "", tt.languageSkills)

			// A cluster reviewer with no fork audit is refused after the spawn; the spec is what this test reads.
			_, _ = e.Review(p, opts)

			if want := []string{tt.wantRole}; !slices.Equal(shuttle.started, want) {
				t.Fatalf("started seats = %v; want %v, the one chair", shuttle.started, want)
			}
			spec := shuttle.specs[tt.wantRole]
			if spec.Prompt == "" {
				t.Errorf("spec.Prompt = \"\"; want non-empty")
			}
			if want := []string{filepath.Join(root, "review.md")}; !slices.Equal(spec.OutputFiles, want) {
				t.Errorf("spec.OutputFiles = %v; want %v", spec.OutputFiles, want)
			}
			if spec.Segment != tt.wantSegment {
				t.Errorf("spec.Segment = %q; want %q", spec.Segment, tt.wantSegment)
			}
			if !slices.Equal(spec.Skills, tt.wantSkills) {
				t.Errorf("spec.Skills = %v; want %v", spec.Skills, tt.wantSkills)
			}
			if spec.Model != opts.Review.Model || spec.Effort != opts.Review.Effort || spec.Version != opts.Review.Version {
				t.Errorf("spec model = (%q, %q, %q); want %+v", spec.Model, spec.Effort, spec.Version, opts.Review)
			}
			if spec.Timeout != opts.Timeout || spec.Round != opts.Round || spec.Interactive {
				t.Errorf("spec timeout %v, round %q, interactive %v; want %v, %q, false", spec.Timeout, spec.Round, spec.Interactive, opts.Timeout, opts.Round)
			}
			if spec.ForkSubagents != tt.wantForkFlag {
				t.Errorf("spec.ForkSubagents = %v; want %v", spec.ForkSubagents, tt.wantForkFlag)
			}
			gate := shuttle.gates[tt.wantRole]
			if len(gate) != 1 || gate[0].Name != "review" || !gate[0].PassOnCap || gate[0].Attempts != reviewGateAttempts {
				t.Errorf("gate = %+v; want the one review-parse entry that passes on its cap", gate)
			}
		})
	}
}

// TestEngine_Review_Outcomes table-drives Review over an accepted review, the strict-parse and cluster-audit refusals, every non-done outcome and the start and stop failures.
// A non-done outcome carries through with no verdict and a nil error; every refusal and failure is an error; the reviewer's identity passes through to the result.
func TestEngine_Review_Outcomes(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	notStarted := fmt.Errorf("shuttle: start run: %w", shuttleengine.ErrNotStarted)
	violatingAudit := &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", WriteCalls: 1}}}
	cleanAudit := &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1"}}}
	noFixReview := "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: colors do not match\n" +
		"---\n### [BLOCKING] colors do not match\n"

	tests := []struct {
		name         string
		script       halfScript
		clusterFan   string
		stopErr      error
		wantOutcome  shuttleengine.Outcome
		wantVerdict  Verdict
		wantFindings int
		wantErrIs    error
		wantErrText  string
		wantNotStart bool
		wantAudit    *shuttleengine.ForkAudit
		wantWarnings int
		// wantGateReason is the Reason of the reviewer's Gate the result must carry, and wantNilGate marks a reviewer that never ran gated.
		wantGateReason string
		wantNilGate    bool
	}{
		{name: "an accepted blocking review", script: halfScript{result: done, writes: blockingReview}, wantOutcome: shuttleengine.OutcomeDone, wantVerdict: VerdictBlocking, wantFindings: 1},
		{name: "an accepted approved review", script: halfScript{result: done, writes: approvedReview}, wantOutcome: shuttleengine.OutcomeDone, wantVerdict: VerdictApproved},
		{name: "a review missing a finding's fix is refused by the strict parse", script: halfScript{result: done, writes: noFixReview}, wantOutcome: shuttleengine.OutcomeDone, wantErrText: "missing a non-empty fix"},
		{name: "an unparseable review is refused", script: halfScript{result: done, writes: malformedReview}, wantOutcome: shuttleengine.OutcomeDone, wantErrText: "round reached done but its review file is invalid"},
		{name: "a failed review gate is an error", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Reason: "spent"}}, writes: blockingReview}, wantOutcome: shuttleengine.OutcomeDone, wantErrText: "review gate failed", wantGateReason: "spent"},
		{name: "a violating cluster audit is refused with the audit kept", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: violatingAudit}, writes: approvedReview}, clusterFan: "standard", wantOutcome: shuttleengine.OutcomeDone, wantErrText: "write/edit tool call", wantAudit: violatingAudit},
		{name: "a clean cluster audit passes its warnings through", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: cleanAudit}, writes: approvedReview}, clusterFan: "standard", wantOutcome: shuttleengine.OutcomeDone, wantVerdict: VerdictApproved, wantAudit: cleanAudit, wantWarnings: 1},
		{name: "a reviewer that died", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}, wantOutcome: shuttleengine.OutcomeDied, wantNilGate: true},
		{name: "a reviewer that timed out", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}, wantOutcome: shuttleengine.OutcomeTimeout, wantNilGate: true},
		{name: "a provider that never came up is a not-started death", script: halfScript{startErr: notStarted}, wantOutcome: shuttleengine.OutcomeDied, wantNotStart: true, wantNilGate: true},
		{name: "any other start error is wrapped", script: halfScript{startErr: errors.New("reed: add strand failed")}, wantErrText: "burler: shuttle run: ", wantNilGate: true},
		{name: "a timed-out reviewer that cannot be stopped is ErrHalfNotStopped", script: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}, stopErr: errors.New("reed unreachable"), wantOutcome: shuttleengine.OutcomeTimeout, wantErrIs: ErrHalfNotStopped, wantNilGate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			p.ClusterFan = tt.clusterFan
			shuttle := &fakeShuttle{review: tt.script, stopErr: tt.stopErr}
			e := newEngineWith(t, Geometry{WorktreeRoot: root, AnchorPath: root}, clusterTestConfig, shuttle)

			got, err := e.Review(p, RunOpts{})

			wantErr := tt.wantErrText != "" || tt.wantErrIs != nil
			if (err != nil) != wantErr {
				t.Fatalf("Review() error = %v; wantErr %v", err, wantErr)
			}
			if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("Review() error = %q; want it to contain %q", err, tt.wantErrText)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Review() error = %v; want it to wrap %v", err, tt.wantErrIs)
			}
			if got.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q", got.Outcome, tt.wantOutcome)
			}
			if got.NotStarted != tt.wantNotStart || (tt.wantNotStart && got.Review.StartError == "") {
				t.Errorf("NotStarted = %v, StartError = %q; want NotStarted %v with the start error", got.NotStarted, got.Review.StartError, tt.wantNotStart)
			}
			if (got.Gate == nil) != tt.wantNilGate || (got.Gate != nil && got.Gate.Reason != tt.wantGateReason) {
				t.Errorf("Gate = %+v; want the reviewer's gate with reason %q, nil %v", got.Gate, tt.wantGateReason, tt.wantNilGate)
			}
			if got.Verdict != tt.wantVerdict || len(got.Findings) != tt.wantFindings {
				t.Errorf("verdict %q with %d findings; want %q with %d", got.Verdict, len(got.Findings), tt.wantVerdict, tt.wantFindings)
			}
			if wantDigest := got.Verdict != ""; (got.Digest != "") != wantDigest {
				t.Errorf("Digest = %q; want it set exactly for an accepted review", got.Digest)
			}
			if got.ForkAudit != tt.wantAudit || len(got.ClusterWarnings) != tt.wantWarnings {
				t.Errorf("ForkAudit %v with %d warnings; want %v with %d", got.ForkAudit, len(got.ClusterWarnings), tt.wantAudit, tt.wantWarnings)
			}
			if tt.clusterFan != "" && !slices.Equal(got.Lenses, []string{"style"}) && err == nil {
				t.Errorf("Lenses = %v; want the resolved fan's [style]", got.Lenses)
			}
		})
	}
}

// TestEngine_Review_DigestIsTheAcceptedBytes proves an accepted review's Digest is the hex SHA-256 of the bytes on disk, and that its finding carries the fix key.
func TestEngine_Review_DigestIsTheAcceptedBytes(t *testing.T) {
	t.Parallel()
	root, p := newEngineTestProfile(t)
	e := newEngineForTest(t, root, doneReview(blockingReview))

	got, err := e.Review(p, RunOpts{})
	if err != nil {
		t.Fatalf("Review() = %v; want nil error", err)
	}

	onDisk, err := os.ReadFile(filepath.Join(root, "review.md"))
	if err != nil {
		t.Fatalf("ReadFile(review) = %v; want nil", err)
	}
	if want := reviewDigest(onDisk); got.Digest != want {
		t.Errorf("Digest = %q; want %q", got.Digest, want)
	}
	if len(got.Findings) != 1 || got.Findings[0].Fix != "change the color in target.txt:1" {
		t.Errorf("Findings = %+v; want one finding carrying its fix", got.Findings)
	}
}

// TestEngine_ProbeReview_And_ResumeReview proves ProbeReview classifies the reviewer as live, done or gone from the fake's runs and the review file, names the review file as the probed output and starts nothing,
// and that ResumeReview attaches to a live reviewer and concludes it as a fresh review is, while a half that is not live, or was not taken from a probe, is refused with nothing started.
func TestEngine_ProbeReview_And_ResumeReview(t *testing.T) {
	t.Parallel()

	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	tests := []struct {
		name      string
		live      bool
		reviewOn  bool
		wantState HalfState
	}{
		{name: "a live reviewer", live: true, wantState: HalfLive},
		{name: "a live reviewer whose review is already on disk", live: true, reviewOn: true, wantState: HalfLive},
		{name: "no live run and the review present", reviewOn: true, wantState: HalfDone},
		{name: "no live run and no review", wantState: HalfGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			shuttle := &fakeShuttle{review: halfScript{result: done, writes: blockingReview}, live: map[string]bool{reviewHalfRole: tt.live}}
			if tt.reviewOn {
				if err := os.WriteFile(filepath.Join(root, "review.md"), []byte("on disk"), 0o644); err != nil {
					t.Fatalf("WriteFile(review) = %v; want nil", err)
				}
			}
			e := newEngineForTest(t, root, shuttle)
			e.languageSkills = []string{"scribe:golang-testing"}
			p.LanguageSkills = true

			half, err := e.ProbeReview(p, RunOpts{Timeout: time.Minute})
			if err != nil {
				t.Fatalf("ProbeReview() = %v; want nil error", err)
			}
			if half.State != tt.wantState {
				t.Fatalf("state = %d; want %d", half.State, tt.wantState)
			}
			if spec := shuttle.probeSpecs[reviewHalfRole]; !slices.Equal(spec.OutputFiles, []string{filepath.Join(root, "review.md")}) || spec.Timeout != time.Minute {
				t.Errorf("probe spec outputs %v timeout %v; want the review file and the opts' timeout", spec.OutputFiles, spec.Timeout)
			}
			if want := append(slices.Clone(burlerSkills), "scribe:golang-testing"); !slices.Equal(shuttle.probeSpecs[reviewHalfRole].Skills, want) {
				t.Errorf("probe spec skills = %v; want %v", shuttle.probeSpecs[reviewHalfRole].Skills, want)
			}
			if len(shuttle.started) != 0 {
				t.Errorf("ProbeReview started %v; want nothing started", shuttle.started)
			}
			if tt.wantState != HalfLive {
				if _, err := e.ResumeReview(p, RunOpts{}, half); err == nil || !strings.Contains(err.Error(), "needs the review half live") {
					t.Errorf("ResumeReview(not live) error = %v; want a refusal", err)
				}
				return
			}

			got, err := e.ResumeReview(p, RunOpts{}, half)
			if err != nil {
				t.Fatalf("ResumeReview() = %v; want nil error", err)
			}
			if got.Outcome != shuttleengine.OutcomeDone || got.Verdict != VerdictBlocking || got.Digest != reviewDigest([]byte(blockingReview)) {
				t.Errorf("ResumeReview() outcome %q verdict %q digest %q; want an accepted blocking review", got.Outcome, got.Verdict, got.Digest)
			}
			if len(shuttle.started) != 0 {
				t.Errorf("ResumeReview started %v; want nothing started", shuttle.started)
			}
			handBuilt := LiveHalf{State: HalfLive, Handle: half.Handle}
			if _, err := e.ResumeReview(p, RunOpts{}, handBuilt); err == nil || !strings.Contains(err.Error(), "needs the table ProbeReview found") {
				t.Errorf("ResumeReview(hand-built) error = %v; want a refusal naming the probe", err)
			}
		})
	}
}
