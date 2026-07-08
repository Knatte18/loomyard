// engine_test.go tables Engine.Run against a same-package fakeShuttle: spec
// construction, the ClusterN>0 rejection, every shuttleengine.Outcome, and
// the review-file parse path (valid BLOCKING/APPROVED, missing file,
// malformed frontmatter) plus a hard shuttle error.

package burlerengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeShuttle is a same-package Shuttle double: Run records the Spec it
// received, optionally writes scripted content to the Spec's OutputFiles
// entries (review then fixer-report, mirroring the real file contract),
// and returns a scripted Result/error.
type fakeShuttle struct {
	called bool
	spec   shuttleengine.Spec

	reviewContent string // written to OutputFiles[0] when non-empty
	fixerContent  string // written to OutputFiles[1] when non-empty
	result        shuttleengine.Result
	err           error
}

func (f *fakeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.called = true
	f.spec = spec

	if f.err != nil {
		return shuttleengine.Result{}, f.err
	}
	if f.reviewContent != "" {
		if err := os.WriteFile(spec.OutputFiles[0], []byte(f.reviewContent), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	if f.fixerContent != "" {
		if err := os.WriteFile(spec.OutputFiles[1], []byte(f.fixerContent), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	return f.result, nil
}

// newEngineTestProfile builds a minimal valid Profile (relative paths — the
// engine resolves them against root via validate) plus the *Engine wired
// to a *hubgeometry.Layout rooted at root and to shuttle.
func newEngineTestProfile(t *testing.T) (root string, p Profile) {
	t.Helper()
	root = t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) = %v; want nil", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fasit.txt"), []byte("fasit"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasit) = %v; want nil", err)
	}

	p = Profile{
		Target:          FileSet{Paths: []string{"target.txt"}},
		Fasit:           FileSet{Paths: []string{"fasit.txt"}},
		Rubric:          "the widget's color must match the housing's color",
		FixScope:        FixScopeSource,
		ReviewPath:      "review.md",
		FixerReportPath: "fixer-report.md",
	}
	return root, p
}

func newEngineForTest(root string, shuttle Shuttle) *Engine {
	return New(shuttle, &hubgeometry.Layout{WorktreeRoot: root})
}

const (
	approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
	blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    location: target.txt:1\n    summary: colors do not match\n" +
		"---\nfound a mismatch\n"
	malformedReview = "not frontmatter at all\n"
)

// TestEngine_Run_SpecConstruction proves Run builds the shuttle Spec
// exactly as the round driver decision pins it: non-empty prompt,
// OutputFiles = [reviewPath, fixerPath] resolved absolute and in that
// order, Role "burler", RunOpts mapped 1:1, and Interactive left false.
func TestEngine_Run_SpecConstruction(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: approvedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(root, shuttle)

	opts := RunOpts{Model: "opus", Effort: "high", Timeout: 5 * time.Minute, Round: "1"}
	if _, err := e.Run(p, opts); err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}
	if !shuttle.called {
		t.Fatalf("fakeShuttle.Run was never called")
	}

	if shuttle.spec.Prompt == "" {
		t.Errorf("spec.Prompt = \"\"; want non-empty")
	}

	wantReview := filepath.Join(root, "review.md")
	wantFixer := filepath.Join(root, "fixer-report.md")
	if got := shuttle.spec.OutputFiles; len(got) != 2 || got[0] != wantReview || got[1] != wantFixer {
		t.Errorf("spec.OutputFiles = %v; want [%q, %q]", got, wantReview, wantFixer)
	}

	if shuttle.spec.Role != "burler" {
		t.Errorf("spec.Role = %q; want %q", shuttle.spec.Role, "burler")
	}
	if shuttle.spec.Model != opts.Model {
		t.Errorf("spec.Model = %q; want %q", shuttle.spec.Model, opts.Model)
	}
	if shuttle.spec.Effort != opts.Effort {
		t.Errorf("spec.Effort = %q; want %q", shuttle.spec.Effort, opts.Effort)
	}
	if shuttle.spec.Timeout != opts.Timeout {
		t.Errorf("spec.Timeout = %v; want %v", shuttle.spec.Timeout, opts.Timeout)
	}
	if shuttle.spec.Round != opts.Round {
		t.Errorf("spec.Round = %q; want %q", shuttle.spec.Round, opts.Round)
	}
	if shuttle.spec.Interactive {
		t.Errorf("spec.Interactive = true; want false (autonomous default)")
	}
}

// TestEngine_Run_ClusterUnsupported proves ClusterN: 1 fails validate
// before the shuttle is ever invoked, with an error satisfying
// errors.Is(err, ErrClusterUnsupported).
func TestEngine_Run_ClusterUnsupported(t *testing.T) {
	root, p := newEngineTestProfile(t)
	p.ClusterN = 1
	shuttle := &fakeShuttle{}
	e := newEngineForTest(root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if !errors.Is(err, ErrClusterUnsupported) {
		t.Fatalf("Run() error = %v; want errors.Is(err, ErrClusterUnsupported)", err)
	}
	if shuttle.called {
		t.Errorf("fakeShuttle.Run was called; want it never invoked for a rejected profile")
	}
}

// TestEngine_Run_NonDoneOutcomes proves every non-done shuttleengine
// outcome carries through to Result.Outcome with an empty Verdict and a
// nil error, and that LastAssistantMessage and the kept shuttle RunDir are
// carried for a non-done outcome — the RunDir passthrough is what lets a
// caller point at the kept shuttle run dir for a died/timeout round.
func TestEngine_Run_NonDoneOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
		message string
	}{
		{name: "asking", outcome: shuttleengine.OutcomeAsking, message: "which color did you mean?"},
		{name: "died", outcome: shuttleengine.OutcomeDied},
		{name: "timeout", outcome: shuttleengine.OutcomeTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newEngineTestProfile(t)
			shuttle := &fakeShuttle{
				result: shuttleengine.Result{
					Outcome:              tt.outcome,
					LastAssistantMessage: tt.message,
					SessionID:            "sess-1",
					StrandGUID:           "guid-1",
					RunDir:               "/kept/run/dir",
				},
			}
			e := newEngineForTest(root, shuttle)

			got, err := e.Run(p, RunOpts{})
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			if got.Outcome != tt.outcome {
				t.Errorf("Result.Outcome = %q; want %q", got.Outcome, tt.outcome)
			}
			if got.Verdict != "" {
				t.Errorf("Result.Verdict = %q; want empty", got.Verdict)
			}
			if got.LastAssistantMessage != tt.message {
				t.Errorf("Result.LastAssistantMessage = %q; want %q", got.LastAssistantMessage, tt.message)
			}
			if got.SessionID != "sess-1" || got.StrandGUID != "guid-1" {
				t.Errorf("Result identities = (%q, %q); want (\"sess-1\", \"guid-1\")", got.SessionID, got.StrandGUID)
			}
			if got.RunDir != "/kept/run/dir" {
				t.Errorf("Result.RunDir = %q; want %q", got.RunDir, "/kept/run/dir")
			}
		})
	}
}

// TestEngine_Run_DoneBlockingVerdict proves a done run whose review file
// carries a valid BLOCKING verdict parses into VerdictBlocking with its
// findings, and that the shuttle RunDir passes through even on a done
// outcome.
func TestEngine_Run_DoneBlockingVerdict(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: blockingReview,
		fixerContent:  "fixed the mismatch",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, RunDir: "/kept/run/dir"},
	}
	e := newEngineForTest(root, shuttle)

	got, err := e.Run(p, RunOpts{})
	if err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}
	if got.Verdict != VerdictBlocking {
		t.Errorf("Result.Verdict = %q; want %q", got.Verdict, VerdictBlocking)
	}
	if len(got.Findings) != 1 || got.Findings[0].ID != "F1" {
		t.Errorf("Result.Findings = %+v; want one finding with id F1", got.Findings)
	}
	if got.RunDir != "/kept/run/dir" {
		t.Errorf("Result.RunDir = %q; want %q", got.RunDir, "/kept/run/dir")
	}
}

// TestEngine_Run_DoneApprovedVerdict proves a done run whose review file
// carries a valid APPROVED verdict parses into VerdictApproved.
func TestEngine_Run_DoneApprovedVerdict(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: approvedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(root, shuttle)

	got, err := e.Run(p, RunOpts{})
	if err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}
	if got.Verdict != VerdictApproved {
		t.Errorf("Result.Verdict = %q; want %q", got.Verdict, VerdictApproved)
	}
	if len(got.Findings) != 0 {
		t.Errorf("Result.Findings = %+v; want none", got.Findings)
	}
}

// TestEngine_Run_DoneMissingReviewFile proves a done outcome whose review
// file was never actually written (a fake-shuttle-only scenario; the real
// shuttle Spec.validate + file-contract polling makes this impossible in
// production) fails loud with an error rather than defaulting a verdict.
func TestEngine_Run_DoneMissingReviewFile(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		// fixerContent set but reviewContent left empty: the fake never
		// writes OutputFiles[0].
		fixerContent: "nothing fixed",
		result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want an error for a missing review file")
	}
}

// TestEngine_Run_DoneMalformedReviewFile proves a done outcome whose
// review file fails ParseReview returns an error that carries the parse
// failure.
func TestEngine_Run_DoneMalformedReviewFile(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: malformedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want an error for a malformed review file")
	}
	if !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("Run() error = %q; want it to carry the underlying parse failure", err.Error())
	}
}

// TestEngine_Run_ShuttleError proves a hard shuttle failure is wrapped
// rather than swallowed.
func TestEngine_Run_ShuttleError(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{err: errors.New("mux: add strand failed")}
	e := newEngineForTest(root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a wrapped shuttle error")
	}
	if !strings.Contains(err.Error(), "mux: add strand failed") {
		t.Errorf("Run() error = %q; want it to carry the underlying shuttle error", err.Error())
	}
}
