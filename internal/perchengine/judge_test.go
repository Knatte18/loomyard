// judge_test.go tables runCircling, runMilestone, and runTriage against a
// same-package fakeJudgeShuttle: the happy path (spec construction —
// Role, Model/Effort passthrough, OutputFiles — plus a valid scripted
// verdict file) and every fail-safe branch (Run error, non-done outcome,
// missing verdict file, unparseable verdict file) for each of the three
// calls, asserting the safe default and an empty rationale — never an
// error, since none of the three functions returns one.

package perchengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// errTestShuttle is the scripted Run error fakeJudgeShuttle returns in the
// "shuttle run error" branch of each of the three functions' tables below.
var errTestShuttle = errors.New("fake shuttle run error")

// fakeJudgeShuttle is a same-package Shuttle double: Run records the Spec
// it received, optionally writes scripted content to the Spec's sole
// OutputFiles entry (the verdict file), and returns a scripted
// Result/error.
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

		in := judgeInputs{
			Round:        3,
			PriorReviews: []string{"/run/round-1-review.md", "/run/round-2-review.md"},
			VerdictPath:  verdictPath,
			Model:        "haiku",
			Effort:       "low",
		}
		verdict, rationale, ok := runCircling(sh, in)

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
		if len(sh.spec.OutputFiles) != 1 || sh.spec.OutputFiles[0] != verdictPath {
			t.Errorf("runCircling() spec.OutputFiles = %v; want [%q]", sh.spec.OutputFiles, verdictPath)
		}
	})

	t.Run("shuttle run error defaults to progressing", func(t *testing.T) {
		sh := &fakeJudgeShuttle{err: errTestShuttle}
		verdict, rationale, ok := runCircling(sh, judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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
		verdict, rationale, ok := runCircling(sh, judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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
		verdict, rationale, ok := runCircling(sh, judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "never-written.md")})
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
		verdict, rationale, ok := runCircling(sh, judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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

		in := judgeInputs{
			Round:        5,
			HardCap:      10,
			PriorReviews: []string{"/run/round-1-review.md"},
			VerdictPath:  verdictPath,
			Model:        "haiku",
			Effort:       "low",
		}
		verdict, rationale, ok := runMilestone(sh, in)

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
		if len(sh.spec.OutputFiles) != 1 || sh.spec.OutputFiles[0] != verdictPath {
			t.Errorf("runMilestone() spec.OutputFiles = %v; want [%q]", sh.spec.OutputFiles, verdictPath)
		}
	})

	t.Run("shuttle run error defaults to continue", func(t *testing.T) {
		sh := &fakeJudgeShuttle{err: errTestShuttle}
		verdict, rationale, ok := runMilestone(sh, judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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
		verdict, rationale, ok := runMilestone(sh, judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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
		verdict, rationale, ok := runMilestone(sh, judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "never-written.md")})
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
		verdict, rationale, ok := runMilestone(sh, judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md")})
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

		verdict, rationale := runTriage(sh, 2, "should I proceed without the fasit file?", verdictPath, "haiku", "low")

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
		verdict, rationale := runTriage(sh, 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})

	t.Run("non-done outcome defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
		verdict, rationale := runTriage(sh, 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})

	t.Run("missing verdict file defaults to retry", func(t *testing.T) {
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		verdict, rationale := runTriage(sh, 1, "a question", filepath.Join(t.TempDir(), "never-written.md"), "", "")
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
		verdict, rationale := runTriage(sh, 1, "a question", filepath.Join(t.TempDir(), "v.md"), "", "")
		if verdict != TriageRetry {
			t.Errorf("verdict = %q; want %q", verdict, TriageRetry)
		}
		if rationale != "" {
			t.Errorf("rationale = %q; want empty", rationale)
		}
	})
}
