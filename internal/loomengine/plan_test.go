// plan_test.go — untagged Tier-1 unit tests for PlanSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestPlanSpec verifies PlanSpec's field mapping.
func TestPlanSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	wantOutputFiles := []string{
		filepath.Join(worktreeRoot, "_lyx", "plan", "00-overview.md"),
	}
	wantTimeout := 120 * time.Minute

	spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	if len(spec.OutputFiles) != len(wantOutputFiles) {
		t.Fatalf("PlanSpec(...).OutputFiles = %v; want %v", spec.OutputFiles, wantOutputFiles)
	}
	for i, want := range wantOutputFiles {
		if spec.OutputFiles[i] != want {
			t.Errorf("PlanSpec(...).OutputFiles[%d] = %q; want %q", i, spec.OutputFiles[i], want)
		}
	}
	if spec.Interactive != false {
		t.Errorf("PlanSpec(...).Interactive = %v; want false", spec.Interactive)
	}
	if spec.Role != "plan" {
		t.Errorf("PlanSpec(...).Role = %q; want %q", spec.Role, "plan")
	}
	if spec.Model == "" {
		t.Error("PlanSpec(...).Model = \"\"; want non-empty")
	}
	if spec.Effort != "high" {
		t.Errorf("PlanSpec(...).Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Timeout != wantTimeout {
		t.Errorf("PlanSpec(...).Timeout = %s; want %s", spec.Timeout, wantTimeout)
	}
	if spec.Prompt == "" {
		t.Error("PlanSpec(...).Prompt = \"\"; want non-empty")
	}
}

// TestPlanSpec_PromptFilled verifies all markers are filled in the prompt.
func TestPlanSpec_PromptFilled(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	decisionRecordPath := DiscussionDecisionRecord(layout)
	planDir := planparser.PlanDir(layout.AnchorPath())
	overviewPath := planparser.PlanOverview(layout.AnchorPath())

	for _, want := range []string{decisionRecordPath, planDir, overviewPath} {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q", want)
		}
	}
	if strings.Contains(spec.Prompt, "{{") {
		t.Error("PlanSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
	}
}

// TestPlanSpec_PatternDirectiveOptional verifies pattern_directive is optional.
func TestPlanSpec_PatternDirectiveOptional(t *testing.T) {
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	t.Run("empty pattern_directive (PATTERN inactive) renders cleanly", func(t *testing.T) {
		layout := &lyxcwd.Location{HubPath: filepath.Dir(filepath.Join("home", "user", "repo")), WorktreeName: filepath.Base(filepath.Join("home", "user", "repo"))}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		prompt := spec.Prompt
		if strings.Contains(prompt, "{{") {
			t.Errorf("PlanSpec(...).Prompt contains a leftover {{: %q", prompt)
		}
		if strings.Contains(prompt, "## Constraints") {
			t.Errorf("PlanSpec(...).Prompt contains an orphan ## Constraints heading: %q", prompt)
		}
		// Two adjacent optional markers (pattern_directive, friction_directive) both rendering
		// empty legitimately produce one more blank line than a single empty marker would --
		// the threshold below is widened by exactly one newline from its pre-Tier-2 value to
		// account for that, while still catching any further unintended blank-line growth.
		if strings.Contains(prompt, "\n\n\n\n\n") {
			t.Errorf("PlanSpec(...).Prompt contains a stray blank-line block: %q", prompt)
		}
	})

	t.Run("non-empty pattern_directive (PATTERN active) precedes Step 1", func(t *testing.T) {
		worktreeRoot := t.TempDir()
		patternDir := filepath.Join(worktreeRoot, lyxdirs.LyxDirName)
		if err := os.MkdirAll(patternDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
		}
		if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
		}
		layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		prompt := spec.Prompt
		directiveIdx := strings.Index(prompt, "## Constraints")
		stepIdx := strings.Index(prompt, "## Step 1")
		if directiveIdx == -1 || stepIdx == -1 || directiveIdx >= stepIdx {
			t.Errorf("pattern_directive (idx %d) does not precede ## Step 1 (idx %d) in prompt: %q", directiveIdx, stepIdx, prompt)
		}
	})
}

// TestPlanSpec_PromptNeverNamesSupportLog proves the composed plan prompt names neither the literal
// support-log.md filename nor the support log's own absolute path.
// The Plan-never-reads-support-log boundary is asserted once,
// at build/test time, over Plan-Write's producer definition rather than per run, and that the
// assertion lands with the real Plan-Write -- this is that assertion.
// It builds its own layout because it needs the *lyxcwd.Location in hand to compute DiscussionSupportLog's absolute path.
func TestPlanSpec_PromptNeverNamesSupportLog(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	if strings.Contains(spec.Prompt, "support-log.md") {
		t.Error("PlanSpec(...).Prompt contains \"support-log.md\"; the Plan producer must never read the support log")
	}
	supportLogPath := DiscussionSupportLog(layout)
	if strings.Contains(spec.Prompt, supportLogPath) {
		t.Errorf("PlanSpec(...).Prompt contains the support log's own absolute path %q; the Plan producer must never read the support log", supportLogPath)
	}
}

// TestPlanSpec_PromptStatesSpecsDir verifies the composed prompt contains the told specs directory
// verbatim and absolute, and that no literal "{{.specs_dir}}" marker survives rendering -- the
// composer-level guarantee that a rewritten normative citation actually resolves to a real,
// openable path rather than a dead reference an agent cannot act on.
func TestPlanSpec_PromptStatesSpecsDir(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	specsDir := newTestSpecsDir(t)
	if !filepath.IsAbs(specsDir) {
		t.Fatalf("newTestSpecsDir(t) = %q; want an absolute path -- a deployed spec sits outside the agent's own worktree, so only an absolute spelling is reachable", specsDir)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), specsDir, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	if !strings.Contains(spec.Prompt, specsDir) {
		t.Errorf("PlanSpec(...).Prompt does not contain the told specs directory %q", specsDir)
	}
	if strings.Contains(spec.Prompt, "{{.specs_dir}}") {
		t.Error("PlanSpec(...).Prompt contains a literal \"{{.specs_dir}}\" marker; want it rendered")
	}
}

// TestPlanSpec_EmptySpecsDirErrors verifies composing the Plan prompt with an empty specs directory
// returns an error rather than a prompt carrying a blank path: specs_dir is a required marker, so a
// blank render must fail loudly at composition instead of silently reproducing the dead reference
// this task removes.
func TestPlanSpec_EmptySpecsDirErrors(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := PlanSpec(layout, newTestStencilsDir(t), "", cfg, reg); err == nil {
		t.Error("PlanSpec(..., specsDir=\"\") = _, nil; want an error")
	}
}

// newTestSpecsDir returns a real, non-empty directory to pass as PlanSpec's/composePlanPrompt's
// specsDir parameter, alongside newTestStencilsDir. A real directory rather than an empty-string
// placeholder is required: specs_dir is a required marker, so an empty value would compile and pass
// today only because the plan stencil carries no marker yet, and would start failing at run time the
// moment a later batch inserts the marker.
func newTestSpecsDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath proves PlanSpec's plan-path call sites pass
// layout.AnchorPath() and never layout.WorktreePath().
// It builds a layout with a non-"." AnchorRel so the two roots are distinguishable strings, which is
// why it does not reuse this file's other tests' default (zero-value) AnchorRel — those are testing
// field mapping, not anchoring, and collapse AnchorPath() to WorktreePath() by construction.
func TestPlanSpec_AnchoredUnderAnchorPathNotWorktreePath(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{
		HubPath:      filepath.Dir(worktreeRoot),
		WorktreeName: filepath.Base(worktreeRoot),
		AnchorRel:    "backend",
	}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	wantOverview := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")
	wrongOverview := filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")

	if len(spec.OutputFiles) != 1 {
		t.Fatalf("PlanSpec(...).OutputFiles = %v; want exactly one entry", spec.OutputFiles)
	}
	if spec.OutputFiles[0] != wantOverview {
		t.Errorf("PlanSpec(...).OutputFiles[0] = %q; want %q", spec.OutputFiles[0], wantOverview)
	}
	if spec.OutputFiles[0] == wrongOverview {
		t.Errorf("PlanSpec(...).OutputFiles[0] = %q; equals the WorktreePath()-rooted path %q, want the AnchorPath()-rooted one", spec.OutputFiles[0], wrongOverview)
	}

	wantPlanDir := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName)
	wrongPlanDir := filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName, planparser.PlanDirName)
	if !strings.Contains(spec.Prompt, wantPlanDir) {
		t.Errorf("PlanSpec(...).Prompt does not contain the AnchorPath()-rooted plan dir %q", wantPlanDir)
	}
	if strings.Contains(spec.Prompt, wrongPlanDir) {
		t.Errorf("PlanSpec(...).Prompt contains the WorktreePath()-rooted plan dir %q; want only the AnchorPath()-rooted one", wrongPlanDir)
	}
}

// TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath proves PlanSpec's pattern.Directive call site
// passes layout.AnchorPath() and never layout.WorktreePath() — the anchoring signal that card 2's
// cmd/lyx pattern.File row rewrite would otherwise silently drop, and the transposition detector for
// the plan.go call site.
// It uses a non-"." AnchorRel, and a real t.TempDir() hub, since a real temp root is mandatory rather
// than a preference: the positive direction must actually create files under AnchorPath() and have
// PlanSpec read them there.
func TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath(t *testing.T) {
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	t.Run("PATTERN.md under AnchorPath is read", func(t *testing.T) {
		hub := t.TempDir()
		layout := &lyxcwd.Location{HubPath: hub, WorktreeName: "repo", AnchorRel: "backend"}

		patternDir := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName)
		if err := os.MkdirAll(patternDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
		}
		if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
		}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		if !strings.Contains(spec.Prompt, "## Constraints") {
			t.Errorf("PlanSpec(...).Prompt does not contain \"## Constraints\"; want the directive read from AnchorPath()")
		}
	})

	t.Run("PATTERN.md under WorktreePath alone is not read", func(t *testing.T) {
		hub := t.TempDir()
		layout := &lyxcwd.Location{HubPath: hub, WorktreeName: "repo", AnchorRel: "backend"}

		patternDir := filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName)
		if err := os.MkdirAll(patternDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
		}
		if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
		}

		reg, err := modelspec.LoadRegistry(t.TempDir())
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg)
		if err != nil {
			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
		}

		if strings.Contains(spec.Prompt, "## Constraints") {
			t.Errorf("PlanSpec(...).Prompt contains \"## Constraints\"; want no directive read from a WorktreePath()-only PATTERN.md")
		}
	})
}

// TestPlanSpec_MalformedModelSpec verifies malformed specs are rejected.
func TestPlanSpec_MalformedModelSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg); err == nil {
		t.Fatal("PlanSpec(..., Plan=\"opus[effort\") = _, nil; want non-nil error")
	}
}

// TestComposePlanPrompt_FrictionEnabled verifies that, given a real friction directive resolved via
// friction.NotePath/friction.Directive exactly as PlanSpec resolves one, composePlanPrompt's rendered
// prompt contains the resolved absolute note path verbatim -- the property that catches a composer
// wiring the wrong path.
func TestComposePlanPrompt_FrictionEnabled(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	frictionDir := filepath.Join(t.TempDir(), "friction")
	notePath := friction.NotePath(frictionDir, "Plan-Write")
	if notePath == "" {
		t.Fatal("friction.NotePath(frictionDir, \"Plan-Write\") = \"\"; want a resolved path")
	}
	directive, err := friction.Directive(notePath, stencilsDir, friction.RoleImplementer)
	if err != nil {
		t.Fatalf("friction.Directive(...) = _, %v; want nil error", err)
	}

	got, err := composePlanPrompt(stencilsDir, newTestSpecsDir(t), "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/plan", "/hub/repo/_lyx/plan/00-overview.md", "", directive)
	if err != nil {
		t.Fatalf("composePlanPrompt(..., directive) = _, %v; want nil error", err)
	}

	if !strings.Contains(string(got), notePath) {
		t.Errorf("composePlanPrompt(...) output does not contain the resolved friction note path %q verbatim", notePath)
	}
}

// TestComposePlanPrompt_FrictionDisabled verifies that, with an empty frictionDirective (Tier 2 off)
// and a stencilsDir carrying no friction-directive stencil at all, composePlanPrompt still renders
// successfully with no friction content -- proving composePlanPrompt reads no friction stencil of its
// own, the same no-read guarantee internal/friction's own tests pin by pointing at a stencilsDir a
// read would fail against.
func TestComposePlanPrompt_FrictionDisabled(t *testing.T) {
	stencilsDir := newMinimalStencilsDir(t)

	got, err := composePlanPrompt(stencilsDir, newTestSpecsDir(t), "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/plan", "/hub/repo/_lyx/plan/00-overview.md", "", "")
	if err != nil {
		t.Fatalf("composePlanPrompt(..., frictionDirective=\"\") = _, %v; want nil error", err)
	}

	rendered := string(got)
	if strings.Contains(rendered, "{{") {
		t.Errorf("composePlanPrompt(..., frictionDirective=\"\") output contains an unrendered marker token:\n%s", rendered)
	}
	if strings.Contains(rendered, "Friction note") {
		t.Error("composePlanPrompt(..., frictionDirective=\"\") output contains friction directive content; want none when Tier 2 is off")
	}
}

// TestComposePlanPrompt_FrictionMarkerFreeTemplate verifies that a seeded loom-template-plan stencil
// whose bytes carry no {{.friction_directive}} literal still composes successfully -- never an error
// -- while Tier 2 is enabled (a non-empty frictionDirective). This is the operator-edited-stencil and
// dev-build case: internal/stencilstore/reconcile.go never refreshes a StateEdited stencil, so it is
// reachable in a real worktree and must degrade to a warning rather than a failed run.
func TestComposePlanPrompt_FrictionMarkerFreeTemplate(t *testing.T) {
	stencilsDir := newTestStencilsDir(t)
	markerFree := bytes.ReplaceAll(stencils.LoomTemplatePlan, []byte("{{.friction_directive}}"), nil)
	planTemplatePath := filepath.Join(stencilsDir, "loom", "loom-template-plan.md")
	if err := os.WriteFile(planTemplatePath, markerFree, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", planTemplatePath, err)
	}

	_, err := composePlanPrompt(stencilsDir, newTestSpecsDir(t), "/hub/repo/_lyx/discussion/decision-record.md", "/hub/repo/_lyx/plan", "/hub/repo/_lyx/plan/00-overview.md", "", "some friction directive text")
	if err != nil {
		t.Fatalf("composePlanPrompt(..., frictionDirective=<non-empty>) with a marker-free template = _, %v; want nil error", err)
	}
}
