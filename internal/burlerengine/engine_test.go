// engine_test.go tables Engine.Run against a same-package fakeShuttle: spec construction (including
// the ClusterFan -> Spec.ForkSubagents wiring), the cluster audit policy wiring (a violating audit
// fails the round, a clean one passes with warnings copied through, and a non-cluster profile never
// invokes it), every shuttleengine.Outcome, the review-file parse path (valid BLOCKING/APPROVED,
// missing file, malformed frontmatter) plus a hard shuttle error, the per-round
// instruction-file materialization step (happy path writes exactly three files under
// lyxdirs.DotLyxDirName, and a materialization failure returns a hard error before the shuttle
// ever runs), and the PATTERN-directive transposition detector for the pattern.Directive call site
// (active/inactive pair pinning that the directive reaches instruction-1-explore.md).
// Every Geometry built here points WorktreeRoot at a test temp dir, so materialization lands there
// rather than in the real package source tree. TestEngine_Run_MaterializesInstructionFiles is the
// one site that keeps WorktreeRoot and AnchorPath distinct on purpose.

package burlerengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
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
// to a Geometry rooted at root and to shuttle.
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

func newEngineForTest(t *testing.T, root string, shuttle Shuttle) *Engine {
	t.Helper()
	return New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, newTestStencilsDir(t))
}

const (
	approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
	blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    location: target.txt:1\n    summary: colors do not match\n" +
		"---\nfound a mismatch\n"
	malformedReview = "not frontmatter at all\n"
)

// TestEngine_Run_SpecConstruction proves Run builds the shuttle Spec exactly as the round driver
// decision pins it: non-empty prompt, OutputFiles = [reviewPath, fixerPath] resolved absolute and
// in that order, Role "burler", RunOpts mapped 1:1, and Interactive left false.
func TestEngine_Run_SpecConstruction(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: approvedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(t, root, shuttle)

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

// TestEngine_Run_ForkSubagentsSpecWiring proves Spec.ForkSubagents mirrors Profile.ClusterFan
// exactly: true only when a fan actually resolved, false for a plain (non-cluster) profile — the
// sole trigger for a cluster round's fork authorization.
func TestEngine_Run_ForkSubagentsSpecWiring(t *testing.T) {
	cfg := Config{
		Lenses: map[string]string{"style": "style prose"},
		Fans:   map[string][]string{"standard": {"style"}},
	}

	t.Run("cluster profile", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		p.ClusterFan = "standard"
		shuttle := &fakeShuttle{
			reviewContent: approvedReview,
			fixerContent:  "nothing fixed",
			result: shuttleengine.Result{
				Outcome: shuttleengine.OutcomeDone,
				// "standard" resolves to exactly one lens (style) — the
				// audit requires exactly one clean fork report to pass.
				ForkAudit: &shuttleengine.ForkAudit{
					Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true}},
				},
			},
		}
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t))

		if _, err := e.Run(p, RunOpts{}); err != nil {
			t.Fatalf("Run() = %v; want nil error", err)
		}
		if !shuttle.spec.ForkSubagents {
			t.Errorf("spec.ForkSubagents = false; want true for a cluster profile")
		}
	})

	t.Run("plain profile", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		shuttle := &fakeShuttle{
			reviewContent: approvedReview,
			fixerContent:  "nothing fixed",
			result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t))

		if _, err := e.Run(p, RunOpts{}); err != nil {
			t.Fatalf("Run() = %v; want nil error", err)
		}
		if shuttle.spec.ForkSubagents {
			t.Errorf("spec.ForkSubagents = true; want false for a plain profile")
		}
	})
}

// TestEngine_Run_ClusterAuditPolicy proves Run wires shuttleResult.ForkAudit through
// auditClusterRound for a done cluster round: a violating audit fails Run with the populated-so-far
// Result carrying the raw ForkAudit, a clean-but-warning audit passes with Result.ClusterWarnings
// copied through, and a non-cluster profile never invokes the policy at all — no
// ForkAudit/ClusterWarnings on the Result even when the fake shuttle's scripted Result carries one.
func TestEngine_Run_ClusterAuditPolicy(t *testing.T) {
	cfg := Config{
		Lenses: map[string]string{"style": "style prose"},
		Fans:   map[string][]string{"standard": {"style"}},
	}

	t.Run("violating audit fails the round", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		p.ClusterFan = "standard"
		violatingAudit := &shuttleengine.ForkAudit{
			Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: true, WriteCalls: 1}},
		}
		shuttle := &fakeShuttle{
			reviewContent: approvedReview,
			fixerContent:  "nothing fixed",
			result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: violatingAudit},
		}
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t))

		got, err := e.Run(p, RunOpts{})
		if err == nil {
			t.Fatalf("Run() error = nil; want a cluster audit policy failure")
		}
		if !strings.Contains(err.Error(), "write/edit tool call") {
			t.Errorf("Run() error = %q; want it to carry the underlying audit violation", err.Error())
		}
		if got.ForkAudit != violatingAudit {
			t.Errorf("Result.ForkAudit = %v; want the raw audit copied through even on a policy failure", got.ForkAudit)
		}
	})

	t.Run("clean audit with a warning passes and copies warnings through", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		p.ClusterFan = "standard"
		cleanAudit := &shuttleengine.ForkAudit{
			Forks: []shuttleengine.ForkReport{{TranscriptPath: "fork-1", ReportReturned: false}},
		}
		shuttle := &fakeShuttle{
			reviewContent: approvedReview,
			fixerContent:  "nothing fixed",
			result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, ForkAudit: cleanAudit},
		}
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t))

		got, err := e.Run(p, RunOpts{})
		if err != nil {
			t.Fatalf("Run() = %v; want nil error", err)
		}
		if len(got.ClusterWarnings) != 1 {
			t.Fatalf("Result.ClusterWarnings = %v; want exactly one warning", got.ClusterWarnings)
		}
	})

	t.Run("non-cluster profile never invokes the policy", func(t *testing.T) {
		root, p := newEngineTestProfile(t)
		// A non-cluster profile carries no ClusterFan; a scripted ForkAudit
		// on the fake shuttle's Result must be ignored entirely — the
		// policy is not even consulted, so it must never surface on
		// Result even though the shuttle "returned" one.
		shuttle := &fakeShuttle{
			reviewContent: approvedReview,
			fixerContent:  "nothing fixed",
			result: shuttleengine.Result{
				Outcome:   shuttleengine.OutcomeDone,
				ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{WriteCalls: 99}}},
			},
		}
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t))

		got, err := e.Run(p, RunOpts{})
		if err != nil {
			t.Fatalf("Run() = %v; want nil error (policy never invoked for a non-cluster profile)", err)
		}
		if got.ForkAudit != nil {
			t.Errorf("Result.ForkAudit = %v; want nil for a non-cluster profile", got.ForkAudit)
		}
		if got.ClusterWarnings != nil {
			t.Errorf("Result.ClusterWarnings = %v; want nil for a non-cluster profile", got.ClusterWarnings)
		}
	})
}

// TestEngine_Run_NonDoneOutcomes proves every non-done shuttleengine outcome carries through to
// Result.Outcome with an empty Verdict and a nil error,
// and that LastAssistantMessage and the kept shuttle RunDir are carried for a non-done outcome —
// the RunDir passthrough is what lets a caller point at the kept shuttle run dir for a died/timeout
// round.
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
			e := newEngineForTest(t, root, shuttle)

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

// TestEngine_Run_DoneBlockingVerdict proves a done run whose review file carries a valid BLOCKING
// verdict parses into VerdictBlocking with its findings,
// and that the shuttle RunDir passes through even on a done outcome.
func TestEngine_Run_DoneBlockingVerdict(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: blockingReview,
		fixerContent:  "fixed the mismatch",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, RunDir: "/kept/run/dir"},
	}
	e := newEngineForTest(t, root, shuttle)

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

// TestEngine_Run_DoneApprovedVerdict proves a done run whose review file carries a valid APPROVED
// verdict parses into VerdictApproved.
func TestEngine_Run_DoneApprovedVerdict(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: approvedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(t, root, shuttle)

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

// TestEngine_Run_DoneMissingReviewFile proves a done outcome whose review file was never actually
// written (a fake-shuttle-only scenario; the real shuttle Spec.validate + file-contract polling
// makes this impossible in production) fails loud with an error rather than defaulting a verdict.
func TestEngine_Run_DoneMissingReviewFile(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		// fixerContent set but reviewContent left empty: the fake never
		// writes OutputFiles[0].
		fixerContent: "nothing fixed",
		result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(t, root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want an error for a missing review file")
	}
}

// TestEngine_Run_DoneMalformedReviewFile proves a done outcome whose review file fails ParseReview
// returns an error that carries the parse failure.
func TestEngine_Run_DoneMalformedReviewFile(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: malformedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	e := newEngineForTest(t, root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want an error for a malformed review file")
	}
	if !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("Run() error = %q; want it to carry the underlying parse failure", err.Error())
	}
}

// TestEngine_Run_ShuttleError proves a hard shuttle failure is wrapped rather than swallowed.
func TestEngine_Run_ShuttleError(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{err: errors.New("reed: add strand failed")}
	e := newEngineForTest(t, root, shuttle)

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a wrapped shuttle error")
	}
	if !strings.Contains(err.Error(), "reed: add strand failed") {
		t.Errorf("Run() error = %q; want it to carry the underlying shuttle error", err.Error())
	}
}

// TestEngine_Run_MaterializesInstructionFiles proves Run writes exactly three instruction files to
// a fresh per-round directory under lyxdirs.DotLyxDirName, bakes their absolute
// paths into the orchestrator prompt it hands the shuttle, and that a rendered file's content
// reflects a filled marker from the profile.
func TestEngine_Run_MaterializesInstructionFiles(t *testing.T) {
	root, p := newEngineTestProfile(t)
	shuttle := &fakeShuttle{
		reviewContent: approvedReview,
		fixerContent:  "nothing fixed",
		result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	// worktreeRoot and anchorPath are two distinct directories here — anchorPath sits under
	// worktreeRoot at the same "sub/dir" subpath a real AnchorRel would produce — so the instruction
	// dir's AnchorPath anchoring (as opposed to WorktreeRoot) is actually observable, and a swapped
	// constructor is caught directly by the two assertions below rather than surfacing downstream as
	// an unrelated file-not-found.
	worktreeRoot := root
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	e := New(shuttle, Geometry{WorktreeRoot: worktreeRoot, AnchorPath: anchorPath}, Config{}, newTestStencilsDir(t))

	if _, err := e.Run(p, RunOpts{}); err != nil {
		t.Fatalf("Run() = %v; want nil error", err)
	}

	burlerDir := filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "burler")
	matches, err := filepath.Glob(filepath.Join(burlerDir, "round-*", "instruction-*.md"))
	if err != nil {
		t.Fatalf("filepath.Glob() = %v; want nil", err)
	}
	if len(matches) != 3 {
		t.Fatalf("materialized instruction files = %v; want exactly 3", matches)
	}

	// The round directory must land under AnchorPath, never under WorktreeRoot — a swapped
	// constructor (WorktreeRoot/AnchorPath transposed) must fail here, at the construction boundary,
	// rather than surfacing downstream as a file-not-found.
	worktreeBurlerDir := filepath.Join(worktreeRoot, lyxdirs.DotLyxDirName, "burler")
	if _, err := os.Stat(worktreeBurlerDir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) = %v; want it NOT to exist (the round dir must land under AnchorPath, not WorktreeRoot)", worktreeBurlerDir, err)
	}

	for _, path := range matches {
		if !strings.Contains(shuttle.spec.Prompt, path) {
			t.Errorf("spec.Prompt does not contain materialized instruction path %q", path)
		}
	}

	// target.txt's content ("target") is the profile's Target — it should
	// show up in the instruction-1 (explore) file, proving the file
	// actually carries a filled marker rather than empty/leftover template
	// syntax.
	instruction1Path := filepath.Join(filepath.Dir(matches[0]), "instruction-1-explore.md")
	content, err := os.ReadFile(instruction1Path)
	if err != nil {
		t.Fatalf("ReadFile(instruction-1-explore.md) = %v; want nil", err)
	}
	if !strings.Contains(string(content), p.Rubric) {
		t.Errorf("instruction-1-explore.md content = %q; want it to contain the profile's rubric %q", content, p.Rubric)
	}
}

// TestEngine_Run_PatternDirectiveReachesInstruction1 closes the one pattern.Directive call site with
// no behavioural coverage at all before this task: Engine.Run passes the directive into
// composePrompt as patternDirective, and composePrompt fills it into the instruction-1 values map
// only, through stencil.FillOptional with pattern_directive in the optional set, so instructions 2
// and 3 never receive it and the orchestrator prompt never carries it.
// The active sub-test plants root/_lyx/PATTERN.md before Run; the inactive sub-test plants nothing.
// Both assert on instruction-1-explore.md's disk content for the literal relative pointer
// "_lyx/PATTERN.md" that every role variant carries — the pair is what makes the assertion
// meaningful, since a presence-only assertion cannot distinguish a working read from a template that
// hardcodes the text.
func TestEngine_Run_PatternDirectiveReachesInstruction1(t *testing.T) {
	tests := []struct {
		name       string
		writeFile  bool
		wantPinned bool
	}{
		{name: "PATTERN active", writeFile: true, wantPinned: true},
		{name: "PATTERN inactive", writeFile: false, wantPinned: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newEngineTestProfile(t)
			if tt.writeFile {
				patternDir := filepath.Join(root, lyxdirs.LyxDirName)
				if err := os.MkdirAll(patternDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
				}
				if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
				}
			}
			shuttle := &fakeShuttle{
				reviewContent: approvedReview,
				fixerContent:  "nothing fixed",
				result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			e := newEngineForTest(t, root, shuttle)

			if _, err := e.Run(p, RunOpts{}); err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}

			burlerDir := filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName, "burler")
			entries, err := os.ReadDir(burlerDir)
			if err != nil {
				t.Fatalf("ReadDir(%q) = %v; want nil", burlerDir, err)
			}
			if len(entries) != 1 {
				t.Fatalf("ReadDir(%q) = %v; want exactly one round entry", burlerDir, entries)
			}

			instruction1Path := filepath.Join(burlerDir, entries[0].Name(), "instruction-1-explore.md")
			content, err := os.ReadFile(instruction1Path)
			if err != nil {
				t.Fatalf("ReadFile(instruction-1-explore.md) = %v; want nil", err)
			}

			gotPinned := strings.Contains(string(content), "_lyx/PATTERN.md")
			if gotPinned != tt.wantPinned {
				t.Errorf("instruction-1-explore.md contains \"_lyx/PATTERN.md\" = %v; want %v", gotPinned, tt.wantPinned)
			}
		})
	}
}

// TestEngine_Run_MaterializeFailure proves a materialization failure (the .lyx parent directory
// cannot be created) returns a hard error naming the failure, before the shuttle is ever invoked.
func TestEngine_Run_MaterializeFailure(t *testing.T) {
	root, p := newEngineTestProfile(t)

	// A regular file at <root>/.lyx makes os.MkdirAll(<root>/.lyx/burler)
	// fail: ".lyx" cannot be traversed as a directory component. The burler
	// dir join is AnchorPath()-anchored (lyxdirs.DotLyxDirName); this
	// fixture leaves AnchorRel unset (AnchorRel "."), so AnchorPath()
	// equals WorktreePath() and the file sits directly at WorktreePath()/
	// .lyx. validate() resolves target.txt/fasit.txt against the same
	// WorktreePath() root and is unaffected.
	notdir := filepath.Join(root, lyxdirs.DotLyxDirName)
	if err := os.WriteFile(notdir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(notdir) = %v; want nil", err)
	}

	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, newTestStencilsDir(t))

	_, err := e.Run(p, RunOpts{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a materialization failure")
	}
	if !strings.Contains(err.Error(), "materialize instruction files") {
		t.Errorf("Run() error = %q; want it to name the materialization failure", err.Error())
	}
	if shuttle.called {
		t.Errorf("fakeShuttle.Run was called; want it never invoked on a materialization failure")
	}
}
