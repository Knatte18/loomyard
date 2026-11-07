// judge_test.go tests runCircling and runMilestone against fakeJudgeShuttle, a same-package Shuttle double.
// The happy path checks the spawn spec (Role, Model/Effort passthrough, OutputFiles) and a valid verdict file.
// Each fail-safe branch is checked for both calls, asserting the safe default and an empty rationale:
// a Run error, a non-done outcome, a missing verdict file, and an unparseable verdict file.
// Neither function returns an error, so the tests never expect one.
// The file also declares newTestStencilsDir, a package-local helper.
// It seeds a hermetic stencils directory through `stencilkit` for every treadleengine test.

package treadleengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func newTestStencilsDir(t *testing.T) string {
	t.Helper()
	return stencilkit.Seed(t)
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

// TestSpawnedRoles_SkillAndParentDirective tables every spawned prompt (circling and milestone judge, targeting)
// against a told parent name and the no-parent variant, asserting the directive and the role's skill on the spec.
func TestSpawnedRoles_SkillAndParentDirective(t *testing.T) {
	tiergate.Require(t, 0)
	const parentName = "mill:parent"
	const noParentLine = "No parent is recorded for this run."
	dir := t.TempDir()

	calls := map[string]func(sh *fakeJudgeShuttle, stencilsDir, parent string){
		"circling judge": func(sh *fakeJudgeShuttle, stencilsDir, parent string) {
			runCircling(sh, "gate", judgeInputs{Round: 2, PriorReviews: []string{"/run/round-1-review.md"}, VerdictPath: filepath.Join(dir, "cv.md"), HandoffPath: filepath.Join(dir, "ch.md"), StencilsDir: stencilsDir, ParentName: parent})
		},
		"milestone judge": func(sh *fakeJudgeShuttle, stencilsDir, parent string) {
			runMilestone(sh, "gate", judgeInputs{Round: 5, HardCap: 10, PriorReviews: []string{"/run/round-4-review.md"}, VerdictPath: filepath.Join(dir, "mv.md"), HandoffPath: filepath.Join(dir, "mh.md"), StencilsDir: stencilsDir, ParentName: parent})
		},
		"targeting": func(sh *fakeJudgeShuttle, stencilsDir, parent string) {
			runTargeting(stencilsDir, parent, sh, "gate", 3, "", filepath.Join(dir, "seed.md"), shuttleengine.ModelChoice{})
		},
	}

	for name, call := range calls {
		for _, tc := range []struct {
			label, parent string
			wantParent    bool
		}{
			{"with a parent", parentName, true},
			{"without a parent", "", false},
		} {
			t.Run(name+" "+tc.label, func(t *testing.T) {
				sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
				call(sh, newTestStencilsDir(t), tc.parent)

				if !sh.called {
					t.Fatal("the call never reached the shuttle")
				}
				if sh.spec.Segment != segmentcolor.Review {
					t.Errorf("spec.Segment = %q; want %q", sh.spec.Segment, segmentcolor.Review)
				}
				if got := sh.spec.Skills; len(got) != 1 || got[0] != "scribe:prose" {
					t.Errorf("spec.Skills = %v; want [scribe:prose]", got)
				}
				if len(sh.spec.Requires) != 0 {
					t.Errorf("spec.Requires = %v; want none", sh.spec.Requires)
				}
				if !strings.Contains(sh.spec.Prompt, "Edit or Write") {
					t.Error("prompt lacks the edit directive's \"Edit or Write\" sentence")
				}
				if strings.Contains(sh.spec.Prompt, parentName) != tc.wantParent {
					t.Errorf("prompt names the parent = %v; want %v", !tc.wantParent, tc.wantParent)
				}
				if strings.Contains(sh.spec.Prompt, noParentLine) == tc.wantParent {
					t.Errorf("prompt carries the no-parent line = %v; want %v", tc.wantParent, !tc.wantParent)
				}
			})
		}
	}
}

func TestRunCircling(t *testing.T) {
	tiergate.Require(t, 0)
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
			Choice:              shuttleengine.ModelChoice{Engine: "b", Model: "haiku", Effort: "low"},
			StencilsDir:         newTestStencilsDir(t),
		}
		verdict, rationale, ok := runCircling(sh, "gate", in)

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
		if sh.spec.Engine != "b" {
			t.Errorf("runCircling() spec.Engine = %q; want %q", sh.spec.Engine, "b")
		}
		if len(sh.spec.Requires) != 0 {
			t.Errorf("runCircling() spec.Requires = %v; want none", sh.spec.Requires)
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
		verdict, rationale, ok := runCircling(sh, "gate", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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
		sh := &fakeJudgeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}}
		verdict, rationale, ok := runCircling(sh, "gate", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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
		verdict, rationale, ok := runCircling(sh, "gate", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "never-written.md"), StencilsDir: newTestStencilsDir(t)})
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
		verdict, rationale, ok := runCircling(sh, "gate", judgeInputs{Round: 1, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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

	// The handoff is a required second output file.
	// An agent that writes a valid verdict but no handoff never reaches OutcomeDone:
	// shuttle classifies a run as done only when every OutputFiles entry exists.
	// This call therefore discards a parseable verdict and returns the fail-safe default.
	// The coupling is deliberate: this test pins the handoff as a recorded choice, not an accident of OutputFiles.
	t.Run("valid verdict but missing handoff discards the verdict", func(t *testing.T) {
		dir := t.TempDir()
		verdictPath := filepath.Join(dir, "round-3-judge.md")
		sh := &fakeJudgeShuttle{
			verdictContent: verdictContent,
			// The file contract is unsatisfied, since no handoff was written.
			// Real shuttle reports that as a non-done outcome.
			result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied},
		}

		verdict, rationale, ok := runCircling(sh, "gate", judgeInputs{
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

		// The discarded verdict is still usable.
		// The loss comes from the missing handoff alone, not from a malformed verdict file.
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
	tiergate.Require(t, 0)
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
			Choice:              shuttleengine.ModelChoice{Engine: "b", Model: "haiku", Effort: "low"},
		}
		verdict, rationale, ok := runMilestone(sh, "gate", in)

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
		if sh.spec.Engine != "b" {
			t.Errorf("runMilestone() spec.Engine = %q; want %q", sh.spec.Engine, "b")
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
		verdict, rationale, ok := runMilestone(sh, "gate", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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
		verdict, rationale, ok := runMilestone(sh, "gate", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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
		verdict, rationale, ok := runMilestone(sh, "gate", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "never-written.md"), StencilsDir: newTestStencilsDir(t)})
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
		verdict, rationale, ok := runMilestone(sh, "gate", judgeInputs{Round: 5, HardCap: 10, VerdictPath: filepath.Join(t.TempDir(), "v.md"), StencilsDir: newTestStencilsDir(t)})
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
