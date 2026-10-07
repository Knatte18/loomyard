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
	"slices"
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

	// gateSpec is the GateSpec RunGated last received.
	gateSpec shuttleengine.GateSpec
	// reviewRewrites holds the review file content the reviewer writes at each re-prompt, in order.
	reviewRewrites []string
	// gateFindings collects the findings of every failed gate evaluation.
	gateFindings []string
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

// RunGated delegates to Run's own body, then — only when gate is non-empty and the delegated outcome is OutcomeDone — evaluates the gate spec like the shuttle wait loop:
// each entry's closure runs in list order, skipping off entries.
// A failing entry is re-prompted up to its Attempts, each re-prompt first writing the next reviewRewrites content to the review file as the reviewer would, and a PassOnCap entry that spent its budget is let through.
// Any other entry that spent its budget fails the gate and stops the evaluation.
// A closure's error is returned, and otherwise a *GateOutcome is stamped onto the returned Result.
func (f *fakeShuttle) RunGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, error) {
	f.gateSpec = gate

	result, err := f.Run(spec)
	if err != nil || len(gate) == 0 || result.Outcome != shuttleengine.OutcomeDone {
		return result, err
	}

	outcome := &shuttleengine.GateOutcome{Passed: true}
	for _, entry := range gate {
		if entry.Attempts <= 0 {
			continue
		}
		for reprompts := 0; ; reprompts++ {
			gateResult, gerr := entry.Gate()
			if gerr != nil {
				return result, gerr
			}
			if gateResult.Passed {
				break
			}
			f.gateFindings = append(f.gateFindings, gateResult.Findings)
			if reprompts == entry.Attempts {
				if !entry.PassOnCap {
					outcome.Passed = false
				}
				break
			}
			if reprompts < len(f.reviewRewrites) {
				if err := os.WriteFile(spec.OutputFiles[0], []byte(f.reviewRewrites[reprompts]), 0o644); err != nil {
					return result, err
				}
			}
		}
		if !outcome.Passed {
			break
		}
	}
	result.Gate = outcome
	return result, nil
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
	return New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, newTestStencilsDir(t), "")
}

const (
	approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
	blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: colors do not match\n" +
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
	if got, want := shuttle.spec.Skills, []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}; !slices.Equal(got, want) {
		t.Errorf("spec.Skills = %v; want %v", got, want)
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
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t), "")

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
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t), "")

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
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t), "")

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
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t), "")

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
		e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, cfg, newTestStencilsDir(t), "")

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

// TestEngine_Run_ShuttleOutcomes table-drives Run over every shuttle outcome and the review-file parse path.
// A non-done outcome (asking, died, timeout, and a died run that never started) carries through to Result.Outcome with an empty Verdict and a nil error.
// A done outcome parses its review file into VerdictBlocking with its findings or VerdictApproved with none, and fails loud -- never defaulting a verdict -- on a review file that was never written (a fake-shuttle-only scenario; the real shuttle's file-contract polling makes it impossible in production) or whose frontmatter is malformed; a hard shuttle error is wrapped, not swallowed.
// Whatever the outcome, the shuttle's identities, last assistant message, kept RunDir and NotStarted flag pass through to the Result unchanged: the RunDir passthrough is what lets a caller point at the kept shuttle run dir for a died or timed-out round.
func TestEngine_Run_ShuttleOutcomes(t *testing.T) {
	t.Parallel()

	scripted := func(outcome shuttleengine.Outcome, message string, notStarted bool) shuttleengine.Result {
		return shuttleengine.Result{
			Outcome:              outcome,
			LastAssistantMessage: message,
			SessionID:            "sess-1",
			StrandGUID:           "guid-1",
			RunDir:               "/kept/run/dir",
			NotStarted:           notStarted,
		}
	}

	tests := []struct {
		name           string
		shuttle        *fakeShuttle
		wantErr        bool
		errSubstr      string
		wantVerdict    Verdict
		wantFindingIDs []string
	}{
		{
			name:    "asking",
			shuttle: &fakeShuttle{result: scripted(shuttleengine.OutcomeAsking, "which color did you mean?", false)},
		},
		{
			name:    "died",
			shuttle: &fakeShuttle{result: scripted(shuttleengine.OutcomeDied, "", false)},
		},
		{
			name:    "timeout",
			shuttle: &fakeShuttle{result: scripted(shuttleengine.OutcomeTimeout, "", false)},
		},
		{
			name:    "died before starting",
			shuttle: &fakeShuttle{result: scripted(shuttleengine.OutcomeDied, "", true)},
		},
		{
			name: "done with a BLOCKING review",
			shuttle: &fakeShuttle{
				reviewContent: blockingReview,
				fixerContent:  "fixed the mismatch",
				result:        scripted(shuttleengine.OutcomeDone, "", false),
			},
			wantVerdict:    VerdictBlocking,
			wantFindingIDs: []string{"F1"},
		},
		{
			name: "done with an APPROVED review",
			shuttle: &fakeShuttle{
				reviewContent: approvedReview,
				fixerContent:  "nothing fixed",
				result:        scripted(shuttleengine.OutcomeDone, "", false),
			},
			wantVerdict: VerdictApproved,
		},
		{
			// fixerContent set but reviewContent left empty: the fake never writes OutputFiles[0].
			name: "done with a missing review file",
			shuttle: &fakeShuttle{
				fixerContent: "nothing fixed",
				result:       scripted(shuttleengine.OutcomeDone, "", false),
			},
			wantErr: true,
		},
		{
			name: "done with a malformed review file",
			shuttle: &fakeShuttle{
				reviewContent: malformedReview,
				fixerContent:  "nothing fixed",
				result:        scripted(shuttleengine.OutcomeDone, "", false),
			},
			wantErr:   true,
			errSubstr: "frontmatter",
		},
		{
			name:      "hard shuttle error",
			shuttle:   &fakeShuttle{err: errors.New("reed: add strand failed")},
			wantErr:   true,
			errSubstr: "reed: add strand failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			e := newEngineForTest(t, root, tt.shuttle)

			got, err := e.Run(p, RunOpts{})

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Run() error = nil; want an error")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Run() error = %q; want it to carry %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}

			want := tt.shuttle.result
			if got.Outcome != want.Outcome {
				t.Errorf("Result.Outcome = %q; want %q", got.Outcome, want.Outcome)
			}
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Result.Verdict = %q; want %q", got.Verdict, tt.wantVerdict)
			}
			var gotFindingIDs []string
			for _, finding := range got.Findings {
				gotFindingIDs = append(gotFindingIDs, finding.ID)
			}
			if !slices.Equal(gotFindingIDs, tt.wantFindingIDs) {
				t.Errorf("Result.Findings ids = %v; want %v", gotFindingIDs, tt.wantFindingIDs)
			}
			if got.LastAssistantMessage != want.LastAssistantMessage {
				t.Errorf("Result.LastAssistantMessage = %q; want %q", got.LastAssistantMessage, want.LastAssistantMessage)
			}
			if got.SessionID != "sess-1" || got.StrandGUID != "guid-1" {
				t.Errorf("Result identities = (%q, %q); want (\"sess-1\", \"guid-1\")", got.SessionID, got.StrandGUID)
			}
			if got.RunDir != "/kept/run/dir" {
				t.Errorf("Result.RunDir = %q; want %q", got.RunDir, "/kept/run/dir")
			}
			if got.NotStarted != want.NotStarted {
				t.Errorf("Result.NotStarted = %v; want %v", got.NotStarted, want.NotStarted)
			}
		})
	}
}

// TestEngine_Run_GateOutcomes table-drives Run over a round's gate list.
// Whatever the caller's list, the gate spec the shuttle receives ends with the review-parse entry, unwrapped:
// its findings name the review file but not the fixer report, which only repairReportBeforeGate adds.
// A round carrying the zero GateSpec therefore receives exactly that one entry, and its approved review passes it.
// A round whose gate fails returns a Result with Gate populated and Passed false, Verdict and Findings left empty, Outcome still OutcomeDone and a nil error -- with the review file never read:
// no review file is left on disk, so a "missing review file" error would mean the gate-failure short-circuit did not fire before the parse step.
// Engine.Run wraps every gate entry's closure in repairReportBeforeGate before handing it to RunGated, so the failing entry's closure the shuttle received is the wrapped one the round actually ran; re-invoking it (the told closure is pure) recovers the findings text the failing attempt produced, which must name this round's own review path and fixer-report path.
// The caller's gate list is left as it was.
func TestEngine_Run_GateOutcomes(t *testing.T) {
	t.Parallel()

	failing := func(name, findings string) shuttleengine.GateEntry {
		return shuttleengine.GateEntry{Name: name, Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
			return shuttleengine.GateResult{Passed: false, Findings: findings}, nil
		}}
	}
	passing := shuttleengine.GateEntry{Name: "first", Attempts: 3, Gate: func() (shuttleengine.GateResult, error) {
		return shuttleengine.GateResult{Passed: true}, nil
	}}

	tests := []struct {
		name string
		gate shuttleengine.GateSpec
		// failingIndex is the position of the entry that fails, -1 when the list is empty.
		failingIndex int
	}{
		{name: "zero gate spec", gate: nil, failingIndex: -1},
		{name: "single failing entry", gate: shuttleengine.GateSpec{failing("", "the widget is the wrong color")}, failingIndex: 0},
		{name: "failing second entry", gate: shuttleengine.GateSpec{passing, failing("second", "the housing is the wrong color")}, failingIndex: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			if tt.failingIndex < 0 {
				shuttle.reviewContent = approvedReview
				shuttle.fixerContent = "nothing fixed"
			}
			e := newEngineForTest(t, root, shuttle)
			var namesBefore []string
			for _, entry := range tt.gate {
				namesBefore = append(namesBefore, entry.Name)
			}

			got, err := e.Run(p, RunOpts{Gate: tt.gate})
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			var namesAfter []string
			for _, entry := range tt.gate {
				namesAfter = append(namesAfter, entry.Name)
			}
			if !slices.Equal(namesBefore, namesAfter) {
				t.Fatalf("gate entry names = %v; want the caller's list left as it was, %v", namesAfter, namesBefore)
			}

			if len(shuttle.gateSpec) != len(tt.gate)+1 {
				t.Fatalf("gate spec has %d entries; want the %d told entries plus the review entry", len(shuttle.gateSpec), len(tt.gate))
			}
			reviewEntry := shuttle.gateSpec[len(tt.gate)]
			if reviewEntry.Name != "review" || reviewEntry.Attempts != reviewGateAttempts || !reviewEntry.PassOnCap {
				t.Errorf("last gate entry = {Name: %q, Attempts: %d, PassOnCap: %v}; want the review entry with %d attempts that passes on cap", reviewEntry.Name, reviewEntry.Attempts, reviewEntry.PassOnCap, reviewGateAttempts)
			}
			if err := os.Remove(filepath.Join(root, "review.md")); err != nil && !os.IsNotExist(err) {
				t.Fatalf("Remove(review) = %v; want nil", err)
			}
			reviewResult, gerr := reviewEntry.Gate()
			if gerr != nil || reviewResult.Passed {
				t.Fatalf("review entry on a missing file = (%+v, %v); want a failed result and nil error", reviewResult, gerr)
			}
			if !strings.Contains(reviewResult.Findings, filepath.Join(root, "review.md")) || strings.Contains(reviewResult.Findings, filepath.Join(root, "fixer-report.md")) {
				t.Errorf("review entry findings = %q; want them to name the review file only", reviewResult.Findings)
			}

			if tt.failingIndex < 0 {
				if got.Gate == nil || !got.Gate.Passed {
					t.Errorf("Result.Gate = %+v; want populated and passed", got.Gate)
				}
				return
			}
			if got.Gate == nil || got.Gate.Passed {
				t.Fatalf("Result.Gate = %+v; want populated with Passed false", got.Gate)
			}
			if got.Verdict != "" {
				t.Errorf("Result.Verdict = %q; want empty", got.Verdict)
			}
			if len(got.Findings) != 0 {
				t.Errorf("Result.Findings = %+v; want none", got.Findings)
			}
			if got.Outcome != shuttleengine.OutcomeDone {
				t.Errorf("Result.Outcome = %q; want %q", got.Outcome, shuttleengine.OutcomeDone)
			}

			gateResult, gerr := shuttle.gateSpec[tt.failingIndex].Gate()
			if gerr != nil {
				t.Fatalf("shuttle.gateSpec[%d].Gate() = %v; want nil error", tt.failingIndex, gerr)
			}
			for _, want := range []string{filepath.Join(root, "review.md"), filepath.Join(root, "fixer-report.md")} {
				if !strings.Contains(gateResult.Findings, want) {
					t.Errorf("gate findings = %q; want them to name %q", gateResult.Findings, want)
				}
			}
		})
	}
}

// TestEngine_Run_ReviewGateRepairsUnparseableReview drives Run with a first review file that fails ParseReview.
// The review entry fails with the parse error and the quoting hint in its findings, and the fake shuttle's reviewer rewrites the file at each re-prompt.
// A rewrite that parses passes the entry, and Run returns the verdict of the repaired file.
// A file still invalid after the entry's budget is let through, and Run fails with the strict post-gate parse error, after exactly one failed evaluation per attempt plus the capping one.
func TestEngine_Run_ReviewGateRepairsUnparseableReview(t *testing.T) {
	t.Parallel()

	const quotedFragmentReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: \"capital\" is misspelled as \"captial\"\n" +
		"---\nfound a mismatch\n"

	tests := []struct {
		name           string
		rewrites       []string
		wantErrSubstr  string
		wantVerdict    Verdict
		wantEvaluation int
	}{
		{name: "repaired on the first re-prompt", rewrites: []string{blockingReview}, wantVerdict: VerdictBlocking, wantEvaluation: 1},
		{name: "still invalid after the budget", rewrites: []string{quotedFragmentReview, quotedFragmentReview}, wantErrSubstr: "round reached done but its review file is invalid", wantEvaluation: reviewGateAttempts + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newEngineTestProfile(t)
			shuttle := &fakeShuttle{
				reviewContent:  quotedFragmentReview,
				fixerContent:   "fixed the mismatch",
				reviewRewrites: tt.rewrites,
				result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			e := newEngineForTest(t, root, shuttle)

			got, err := e.Run(p, RunOpts{})

			if len(shuttle.gateFindings) != tt.wantEvaluation {
				t.Fatalf("failed review gate evaluations = %d; want %d", len(shuttle.gateFindings), tt.wantEvaluation)
			}
			for _, findings := range shuttle.gateFindings {
				for _, want := range []string{"frontmatter is not valid YAML", "ONE double-quoted string", filepath.Join(root, "review.md")} {
					if !strings.Contains(findings, want) {
						t.Errorf("review gate findings = %q; want them to carry %q", findings, want)
					}
				}
			}
			if tt.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("Run() error = %v; want it to carry %q", err, tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Result.Verdict = %q; want %q", got.Verdict, tt.wantVerdict)
			}
		})
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
	e := New(shuttle, Geometry{WorktreeRoot: worktreeRoot, AnchorPath: anchorPath}, Config{}, newTestStencilsDir(t), "")

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
// Each sub-test plants PATTERN.md at the repo root, or somewhere else for the negative cases,
// before Run, and asserts on instruction-1-explore.md's disk content for the inlined overview text.
// The subpath-anchored cases tell WorktreeRoot and AnchorPath a subdirectory of the repo root, as a
// hub with a non-"." AnchorRel does, so a directive read through either of them instead of RepoRoot
// fails here.
// The pair of active and inactive cases is what makes the assertion
// meaningful, since a presence-only assertion cannot distinguish a working read from a template that
// hardcodes the text.
func TestEngine_Run_PatternDirectiveReachesInstruction1(t *testing.T) {
	const overview = "# PATTERN\n\n- PATTERN-demo: the demo rule\n"
	tests := []struct {
		name       string
		patternAt  string // directory under the repo root holding PATTERN.md, "" for none
		anchorRel  string // the told WorktreeRoot and AnchorPath, relative to the repo root
		wantPinned bool
	}{
		{name: "PATTERN active", patternAt: ".", anchorRel: ".", wantPinned: true},
		{name: "PATTERN inactive", patternAt: "", anchorRel: ".", wantPinned: false},
		{name: "PATTERN active, subpath-anchored", patternAt: ".", anchorRel: "sub/dir", wantPinned: true},
		{name: "PATTERN only under the anchor path", patternAt: "sub/dir", anchorRel: "sub/dir", wantPinned: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, p := newEngineTestProfile(t)
			anchorPath := filepath.Join(root, filepath.FromSlash(tt.anchorRel))
			if tt.anchorRel != "." {
				// The profile's relative paths resolve against WorktreeRoot, which is the anchor path here.
				if err := os.MkdirAll(anchorPath, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", anchorPath, err)
				}
				for _, name := range []string{"target.txt", "fasit.txt"} {
					if err := os.WriteFile(filepath.Join(anchorPath, name), []byte(name), 0o644); err != nil {
						t.Fatalf("WriteFile(%s) = %v; want nil", name, err)
					}
				}
			}
			if tt.patternAt != "" {
				patternPath := filepath.Join(root, filepath.FromSlash(tt.patternAt), "PATTERN.md")
				if err := os.WriteFile(patternPath, []byte(overview), 0o644); err != nil {
					t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
				}
			}
			shuttle := &fakeShuttle{
				reviewContent: approvedReview,
				fixerContent:  "nothing fixed",
				result:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			e := New(shuttle, Geometry{WorktreeRoot: anchorPath, AnchorPath: anchorPath, RepoRoot: root}, Config{}, newTestStencilsDir(t), "")

			if _, err := e.Run(p, RunOpts{}); err != nil {
				t.Fatalf("Run() = %v; want nil error", err)
			}

			burlerDir := filepath.Join(anchorPath, lyxdirs.DotLyxDirName, "burler")
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

			gotPinned := strings.Contains(string(content), overview)
			if gotPinned != tt.wantPinned {
				t.Errorf("instruction-1-explore.md contains the PATTERN overview = %v; want %v", gotPinned, tt.wantPinned)
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
	e := New(shuttle, Geometry{WorktreeRoot: root, AnchorPath: root}, Config{}, newTestStencilsDir(t), "")

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
