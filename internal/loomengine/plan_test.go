// plan_test.go — untagged Tier-1 unit tests for PlanSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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

	spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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
		spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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
		if strings.Contains(prompt, "\n\n\n\n") {
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
		spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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

// TestPlanSpec_PromptStatesCardCriteria verifies the prompt states card criteria.
func TestPlanSpec_PromptStatesCardCriteria(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"What a card is",
		"smallest change",
		"independently committable",
		"Bundles its own test",
		"not a substitute for a bundled test",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the card-granularity contract must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesRootResolution verifies root resolution is documented.
func TestPlanSpec_PromptStatesRootResolution(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"`root:` is optional",
		"`<root>/<path>`",
		"worktree-root-relative",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; root:/// resolution rules must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesContextSemantics verifies Uses: semantics are documented.
func TestPlanSpec_PromptStatesContextSemantics(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"names what the card reads but does not change",
		"is a contradiction: is it being changed, or only read?",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; Uses:/overlap semantics must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesVerifyIsRunnable verifies verify is runnable.
func TestPlanSpec_PromptStatesVerifyIsRunnable(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"runnable shell commands",
		"never prose",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the runnable-verify rule must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesTypeLabelGrammar verifies the type-label grammar is documented.
func TestPlanSpec_PromptStatesTypeLabelGrammar(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"one or more bold type labels from `**Create:**`, `**Edit:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`",
		"sub-bullets are the card's targets for that label",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the type-label grammar must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesImpactSummaryRequirement verifies the ImpactSummary requirement is documented.
func TestPlanSpec_PromptStatesImpactSummaryRequirement(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"`**ImpactSummary:**` on `Edit`/`Delete` cards only",
		"inline on the label line",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the ImpactSummary requirement must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesSkillLoads verifies the prompt's Step 0 loads scribe:prose and
// scribe:testing but not scribe:conversation.
func TestPlanSpec_PromptStatesSkillLoads(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"scribe:prose",
		"scribe:testing",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; Step 0's skill load must reach the agent", want)
		}
	}
	if strings.Contains(prompt, "scribe:conversation") {
		t.Errorf("PlanSpec(...).Prompt contains %q; the Plan producer is autonomous, with no operator for chat-reply discipline to serve", "scribe:conversation")
	}
}

// TestPlanSpec_PromptStatesDegradedQuarryMode verifies the prompt states that no quarry inventory
// is handed to the agent, that its absence is never an error, and that the agent performs the
// mechanical lookups itself.
func TestPlanSpec_PromptStatesDegradedQuarryMode(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"No quarry inventory is handed to you",
		"never an error",
		"go doc <pkg> <Symbol>",
		"grep -rn",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the degraded quarry-inventory mode must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesSelfCheck verifies the prompt's closing Step 5 runs validate-plan and
// instructs a re-run until it exits 0.
func TestPlanSpec_PromptStatesSelfCheck(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"lyx loom validate-plan",
		"re-run it until it exits 0",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the Step 5 self-check must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptStatesVerifyIsExceptional verifies the prompt states that a per-card Verify:
// is exceptional and that the plan-level ## verify: is the single integration check.
// It does not weaken or delete TestPlanSpec_PromptStatesVerifyIsRunnable, which pins the separate
// never-prose rule.
func TestPlanSpec_PromptStatesVerifyIsExceptional(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"exceptional rather than routine",
		"the single integration check for the whole plan",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the Verify-authoring rule must reach the agent", want)
		}
	}
}

// TestPlanSpec_PromptNeverNamesSupportLog proves the composed plan prompt names neither the literal
// support-log.md filename nor the support log's own absolute path.
// manifest/designs/loom.md states that the Plan-never-reads-support-log boundary is asserted once,
// at build/test time, over Plan-Write's producer definition rather than per run, and that the
// assertion lands with the real Plan-Write -- this is that assertion.
// It builds its own layout rather than calling renderedPlanPrompt because it needs the
// *lyxcwd.Location in hand to compute DiscussionSupportLog's absolute path.
func TestPlanSpec_PromptNeverNamesSupportLog(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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

// renderedPlanPrompt returns the prompt PlanSpec renders for template-content assertions.
func renderedPlanPrompt(t *testing.T) string {
	t.Helper()

	layout := &lyxcwd.Location{HubPath: filepath.Dir(filepath.Join("home", "user", "repo")), WorktreeName: filepath.Base(filepath.Join("home", "user", "repo"))}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}
	return spec.Prompt
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

	spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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
		spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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
		spec, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg)
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

	if _, err := PlanSpec(layout, newTestStencilsDir(t), cfg, reg); err == nil {
		t.Fatal("PlanSpec(..., Plan=\"opus[effort\") = _, nil; want non-nil error")
	}
}
