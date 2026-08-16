// judge_test.go tables runCircling, runMilestone, and runTriage against a same-package
// fakeJudgeShuttle: the happy path (spec construction — Role, Model/Effort passthrough, OutputFiles
// — plus a valid scripted verdict file) and every fail-safe branch (Run error, non-done outcome,
// missing verdict file, unparseable verdict file) for each of the three calls, asserting the safe
// default and an empty rationale — never an error, since none of the three functions returns one.
// It also declares newTestStencilsDir, the package-local test helper every treadleengine test uses
// to seed a hermetic stencils directory from the shipped stencils package defaults.

package treadleengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// newTestStencilsDir builds a t.TempDir() seeded with treadle's four stencils, copied byte-for-byte
// from the stencils package's embedded defaults, and returns the directory to pass as stencilsDir.
func newTestStencilsDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	treadleDir := filepath.Join(dir, "treadle")
	if err := os.MkdirAll(treadleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", treadleDir, err)
	}
	files := map[string][]byte{
		"treadle-template-judge-circling.md":  stencils.TreadleTemplateJudgeCircling,
		"treadle-template-judge-milestone.md": stencils.TreadleTemplateJudgeMilestone,
		"treadle-template-triage.md":          stencils.TreadleTemplateTriage,
		"treadle-template-targeting.md":       stencils.TreadleTemplateTargeting,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(treadleDir, name), content, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
		}
	}
	return dir
}

// errTestShuttle is the scripted Run error fakeJudgeShuttle returns.
var errTestShuttle = errors.New("fake shuttle run error")

// fakeJudgeShuttle is a Shuttle double for testing.
type fakeJudgeShuttle struct {
	called bool
	spec   shuttleengine.Spec

	verdictContent string // written to OutputFiles[0] when non-empty
	result         shuttleengine.Result
	err            error
}

func (f *fakeJudgeShuttle) Run(spec shuttleengine.Spec) (shuttleengine.Result, error) {
	f.called = true
	f.spec = spec

	if f.err != nil {
		return shuttleengine.Result{}, f.err
	}
	if f.verdictContent != "" {
		if err := os.WriteFile(spec.OutputFiles[0], []byte(f.verdictContent), 0o644); err != nil {
			return shuttleengine.Result{}, err
		}
	}
	return f.result, nil
}

func TestRunCircling(t *testing.T) {
	verdictContent := `---
verdict: CIRCLING
rationale: the same nil-check finding recurs in rounds 2 and 4
---
`

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		verdictPath := filepath.Join(dir, "round-3-judge.md")
		sh := &fakeJudgeShuttle{
			verdictContent: verdictContent,
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}

		handoffPath := filepath.Join(dir, "round-3-handoff.md")
		in := judgeInputs{
			Round:               3,
			PriorReviews:        []string{"/run/round-1-review.md", "/run/round-2-review.md"},
			VerdictPath:         verdictPath,
			PreviousHandoffPath: "/run/round-2-handoff.md",
			HandoffPath:         handoffPath,
			Model:               "haiku",
			Effort:              "low",
			StencilsDir:         newTestStencilsDir(t),
		}
		verdict, rationale, ok := runCircling(sh, "perch", in)

		if verdict != JudgeCircling {
			t.Errorf("runCircling() verdict = %q; want %q", verdict, JudgeCircling)
		}
		if rationale == "" {
			t.Error("runCircling() rationale is empty; want the scripted rationale")
		}
		if !ok {
			t.Error("runCircling() ok = false; want true on the success path")
		}
		if !sh.called {
			t.Fatal("runCircling() never called the shuttle")
		}
		if sh.spec.Role != "judge" {
			t.Errorf("runCircling() spec.Role = %q; want %q", sh.spec.Role, "judge")
		}
		if sh.spec.Model != "haiku" {
			t.Errorf("runCircling() spec.Model = %q; want %q", sh.spec.Model, "haiku")
		}
		if sh.spec.Effort != "low" {
			t.Errorf("runCircling() spec.Effort = %q; want %q", sh.spec.Effort, "low")
		}
		if len(sh.spec.OutputFiles) != 2 || sh.spec.OutputFiles[0] != verdictPath || sh.spec.OutputFiles[1] != handoffPath {
			t.Errorf("runCircling() spec.OutputFiles = %v; want [%q, %q]", sh.spec.OutputFiles, verdictPath, handoffPath)
		}
	})

	t.Run("shuttle run error defaults to progressing", func(t *testing.T) {
		sh := &fakeJudgeShuttle{err: errTestShuttle}
		verdict, rationale, ok := runCircling(sh, "perch", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeProgressing {
			t.Errorf("verdict = %q; want %q", verdict, JudgeProgressing)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("non-done outcome defaults to progressing", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}}
		verdict, rationale, ok := runCircling(sh, "perch", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeProgressing {
			t.Errorf("verdict = %q; want %q", verdict, JudgeProgressing)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("missing verdict file defaults to progressing", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		verdict, rationale, ok := runCircling(sh, "perch", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "never-written.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeProgressing {
			t.Errorf("verdict = %q; want %q", verdict, JudgeProgressing)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("unparseable verdict file defaults to progressing", func(t *testing.T) {
		sh := &fakeJudgeShuttle{
			verdictContent: "not a valid verdict file at all",
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}
		verdict, rationale, ok := runCircling(sh, "perch", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeProgressing {
			t.Errorf("verdict = %q; want %q", verdict, JudgeProgressing)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	// The handoff is a REQUIRED second output file, so an agent that writes a
	// perfectly good verdict and no handoff never reaches OutcomeDone —
	// shuttle classifies done only when EVERY OutputFiles entry exists — and
	// this call therefore discards a real, parseable verdict and returns the
	// fail-safe default. That is a deliberate consequence of the
	// handoff-on-disk decision (pre-handoff, the verdict was the sole output
	// file and would have been honoured), pinned here so the coupling stays a
	// recorded choice rather than an accident of the spec's OutputFiles list.
	t.Run("valid verdict but missing handoff discards the verdict", func(t *testing.T) {
		dir := t.TempDir()
		verdictPath := filepath.Join(dir, "round-3-judge.md")
		sh := &fakeJudgeShuttle{
			verdictContent: verdictContent,
			// The file contract is unsatisfied (no handoff written), which is
			// exactly what real shuttle reports as asking rather than done.
			result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking},
		}

		verdict, rationale, ok := runCircling(sh, "perch", judgeInputs{
			Round:        3,
			PriorReviews: []string{"/run/round-2-review.md", "/run/round-3-review.md"},
			VerdictPath:  verdictPath,
			HandoffPath:  filepath.Join(dir, "round-3-handoff.md"),
			StencilsDir:  newTestStencilsDir(t),
		})

		if ok {
			t.Error("ok = true; want false — a non-done outcome is a fail-safe path even with a good verdict on disk")
		}
		if verdict != JudgeProgressing {
			t.Errorf("verdict = %q; want the fail-safe %q", verdict, JudgeProgressing)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}

		// Prove the discarded verdict really was usable: the loss comes from
		// the missing handoff alone, not from a malformed verdict file.
		content, err := os.ReadFile(verdictPath)
		if err != nil {
			t.Fatalf("read verdict file: %v", err)
		}
		if _, _, err := ParseJudgeVerdict(content, framingCircling); err != nil {
			t.Fatalf("scripted verdict file does not parse: %v", err)
		}
	})
}

func TestRunMilestone(t *testing.T) {
	verdictContent := `---
verdict: STOP
rationale: the same two findings oscillate every round
---
`

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		verdictPath := filepath.Join(dir, "round-5-judge.md")
		sh := &fakeJudgeShuttle{
			verdictContent: verdictContent,
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}

		handoffPath := filepath.Join(dir, "round-5-handoff.md")
		in := judgeInputs{
			Round:               5,
			HardCap:             10,
			PriorReviews:        []string{"/run/round-1-review.md"},
			VerdictPath:         verdictPath,
			PreviousHandoffPath: "/run/round-1-handoff.md",
			HandoffPath:         handoffPath,
			StencilsDir:         newTestStencilsDir(t),
			Model:               "haiku",
			Effort:              "low",
		}
		verdict, rationale, ok := runMilestone(sh, "perch", in)

		if verdict != JudgeStop {
			t.Errorf("runMilestone() verdict = %q; want %q", verdict, JudgeStop)
		}
		if rationale == "" {
			t.Error("runMilestone() rationale is empty; want the scripted rationale")
		}
		if !ok {
			t.Error("runMilestone() ok = false; want true on the success path")
		}
		if sh.spec.Role != "judge" {
			t.Errorf("runMilestone() spec.Role = %q; want %q", sh.spec.Role, "judge")
		}
		if sh.spec.Model != "haiku" {
			t.Errorf("runMilestone() spec.Model = %q; want %q", sh.spec.Model, "haiku")
		}
		if sh.spec.Effort != "low" {
			t.Errorf("runMilestone() spec.Effort = %q; want %q", sh.spec.Effort, "low")
		}
		if len(sh.spec.OutputFiles) != 2 || sh.spec.OutputFiles[0] != verdictPath || sh.spec.OutputFiles[1] != handoffPath {
			t.Errorf("runMilestone() spec.OutputFiles = %v; want [%q, %q]", sh.spec.OutputFiles, verdictPath, handoffPath)
		}
	})

	t.Run("shuttle run error defaults to continue", func(t *testing.T) {
		sh := &fakeJudgeShuttle{err: errTestShuttle}
		verdict, rationale, ok := runMilestone(sh, "perch", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeContinue {
			t.Errorf("verdict = %q; want %q", verdict, JudgeContinue)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("non-done outcome defaults to continue", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}}
		verdict, rationale, ok := runMilestone(sh, "perch", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeContinue {
			t.Errorf("verdict = %q; want %q", verdict, JudgeContinue)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("missing verdict file defaults to continue", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		verdict, rationale, ok := runMilestone(sh, "perch", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "never-written.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeContinue {
			t.Errorf("verdict = %q; want %q", verdict, JudgeContinue)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})

	t.Run("unparseable verdict file defaults to continue", func(t *testing.T) {
		sh := &fakeJudgeShuttle{
			verdictContent: "garbled, not a verdict file",
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}
		verdict, rationale, ok := runMilestone(sh, "perch", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
		if verdict != JudgeContinue {
			t.Errorf("verdict = %q; want %q", verdict, JudgeContinue)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
		if ok {
			t.Error("ok = true; want false on a fail-safe path")
		}
	})
}

func TestRunTriage(t *testing.T) {
	verdictContent := `---
verdict: GIVE_UP
rationale: the fasit file referenced does not exist
---
`

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		verdictPath := filepath.Join(dir, "round-2-triage.md")
		sh := &fakeJudgeShuttle{
			verdictContent: verdictContent,
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}

		verdict, rationale := runTriage(newTestStencilsDir(t), sh, "perch", 2, "should I proceed without the fasit file?", verdictPath, "haiku", "low")

		if verdict != TriageGiveUp {
			t.Errorf("runTriage() verdict = %q; want %q", verdict, TriageGiveUp)
		}
		if rationale == "" {
			t.Error("runTriage() rationale is empty; want the scripted rationale")
		}
		if sh.spec.Role != "triage" {
			t.Errorf("runTriage() spec.Role = %q; want %q", sh.spec.Role, "triage")
		}
		if sh.spec.Model != "haiku" {
			t.Errorf("runTriage() spec.Model = %q; want %q", sh.spec.Model, "haiku")
		}
		if sh.spec.Effort != "low" {
			t.Errorf("runTriage() spec.Effort = %q; want %q", sh.spec.Effort, "low")
		}
		if len(sh.spec.OutputFiles) != 1 || sh.spec.OutputFiles[0] != verdictPath {
			t.Errorf("runTriage() spec.OutputFiles = %v; want [%q]", sh.spec.OutputFiles, verdictPath)
		}
	})

	t.Run("shuttle run error defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{err: errTestShuttle}
		verdict, rationale := runTriage(newTestStencilsDir(t), sh, "perch", 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})

	t.Run("non-done outcome defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
		verdict, rationale := runTriage(newTestStencilsDir(t), sh, "perch", 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})

	t.Run("missing verdict file defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		verdict, rationale := runTriage(newTestStencilsDir(t), sh, "perch", 1, "a question", filepath.Join(t.TempDir(), "never-written.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})

	t.Run("unparseable verdict file defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{
			verdictContent: "garbled, not a verdict file",
			result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		}
		verdict, rationale := runTriage(newTestStencilsDir(t), sh, "perch", 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})
}
