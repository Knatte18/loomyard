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
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// TestPlanSpec verifies PlanSpec's field mapping and its composed prompt, at an unanchored location and at a subpath-anchored one.
// The anchored row has a non-"." AnchorRel so the anchor path and the worktree path are distinguishable strings:
// PlanSpec's plan-path call sites must pass layout.AnchorPath() and never layout.WorktreePath().
// The prompt states every told path (the specs directory absolute, a deployed spec sitting outside the agent's own worktree), leaves no marker unrendered, and never names the support log: the Plan-never-reads-support-log boundary is asserted at build/test time over Plan-Write's producer definition rather than per run.
//
//testtiming:keep pins OutputFiles, Interactive, Role, Effort, Timeout and every told path in the prompt, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestPlanSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", ""},
		{"anchored under backend", "backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := &lyxcwd.Location{HubPath: filepath.Join("home", "user"), WorktreeName: "repo", AnchorRel: tt.anchorRel}
			cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}
			reg, err := modelspec.LoadRegistry(t.TempDir())
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}
			specsDir := newTestSpecsDir(t)
			if !filepath.IsAbs(specsDir) {
				t.Fatalf("newTestSpecsDir(t) = %q; want an absolute path -- a deployed spec sits outside the agent's own worktree, so only an absolute spelling is reachable", specsDir)
			}

			spec, err := PlanSpec(layout, newTestStencilsDir(t), specsDir, "", cfg, reg)
			if err != nil {
				t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
			}

			wantOverview := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName, "00-overview.md")
			if len(spec.OutputFiles) != 1 || spec.OutputFiles[0] != wantOverview {
				t.Errorf("PlanSpec(...).OutputFiles = %v; want [%q]", spec.OutputFiles, wantOverview)
			}
			if spec.Interactive {
				t.Error("PlanSpec(...).Interactive = true; want false")
			}
			if spec.Role != "plan" {
				t.Errorf("PlanSpec(...).Role = %q; want %q", spec.Role, "plan")
			}
			if spec.Segment != segmentcolor.Plan {
				t.Errorf("PlanSpec(...).Segment = %q; want %q", spec.Segment, segmentcolor.Plan)
			}
			if spec.Model == "" {
				t.Error("PlanSpec(...).Model = \"\"; want non-empty")
			}
			if spec.Effort != "high" {
				t.Errorf("PlanSpec(...).Effort = %q; want %q", spec.Effort, "high")
			}
			if want := 120 * time.Minute; spec.Timeout != want {
				t.Errorf("PlanSpec(...).Timeout = %s; want %s", spec.Timeout, want)
			}

			wantPlanDir := filepath.Join(layout.AnchorPath(), lyxdirs.LyxDirName, planparser.PlanDirName)
			for _, want := range []string{DiscussionDecisionRecord(layout), planparser.PlanDir(layout.AnchorPath()), planparser.PlanOverview(layout.AnchorPath()), wantPlanDir, specsDir} {
				if !strings.Contains(spec.Prompt, want) {
					t.Errorf("PlanSpec(...).Prompt does not contain %q", want)
				}
			}
			if tt.anchorRel != "" {
				wrongPlanDir := filepath.Join(layout.WorktreePath(), lyxdirs.LyxDirName, planparser.PlanDirName)
				if strings.Contains(spec.Prompt, wrongPlanDir) {
					t.Errorf("PlanSpec(...).Prompt contains the WorktreePath()-rooted plan dir %q; want only the AnchorPath()-rooted one", wrongPlanDir)
				}
				if wrongOverview := filepath.Join(wrongPlanDir, "00-overview.md"); spec.OutputFiles[0] == wrongOverview {
					t.Errorf("PlanSpec(...).OutputFiles[0] = %q; equals the WorktreePath()-rooted path, want the AnchorPath()-rooted one", spec.OutputFiles[0])
				}
			}
			if strings.Contains(spec.Prompt, "{{") {
				t.Error("PlanSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
			}
			if strings.Contains(spec.Prompt, "support-log.md") {
				t.Error("PlanSpec(...).Prompt contains \"support-log.md\"; the Plan producer must never read the support log")
			}
			if supportLogPath := DiscussionSupportLog(layout); strings.Contains(spec.Prompt, supportLogPath) {
				t.Errorf("PlanSpec(...).Prompt contains the support log's own absolute path %q; the Plan producer must never read the support log", supportLogPath)
			}
		})
	}
}

// TestPlanSpec_PatternDirective verifies PlanSpec's pattern_directive: PATTERN.md at the worktree root renders the directive before Step 1, and with none the prompt renders cleanly.
// The directive's call site passes layout.WorktreePath() and never layout.AnchorPath(), since PATTERN.md sits at the worktree root, which in a subpath-anchored hub is not the anchor path; the anchored rows use a non-"." AnchorRel and a real t.TempDir() hub because the positive direction must actually create files under WorktreePath() and have PlanSpec read them there.
//
//testtiming:keep pins that PATTERN.md is read from the worktree root and not the anchor path, and the clean render without it, which its covering tests do not assert
func TestPlanSpec_PatternDirective(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		anchorRel string
		// patternAt is "worktree", "anchor" or "" for no PATTERN.md.
		patternAt     string
		wantDirective bool
	}{
		{"PATTERN.md at the worktree root", "", "worktree", true},
		{"PATTERN.md at the worktree root of an anchored location", "backend", "worktree", true},
		{"PATTERN.md under the anchor path alone is not read", "backend", "anchor", false},
		{"no PATTERN.md (PATTERN inactive) renders cleanly", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "repo", AnchorRel: tt.anchorRel}
			if err := os.MkdirAll(layout.AnchorPath(), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", layout.AnchorPath(), err)
			}
			switch tt.patternAt {
			case "worktree":
				writeTestPattern(t, layout.WorktreePath())
			case "anchor":
				writeTestPattern(t, layout.AnchorPath())
			}
			reg, err := modelspec.LoadRegistry(t.TempDir())
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			spec, err := PlanSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), "", Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}, reg)
			if err != nil {
				t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
			}

			prompt := spec.Prompt
			if strings.Contains(prompt, "{{") {
				t.Errorf("PlanSpec(...).Prompt contains a leftover {{: %q", prompt)
			}
			// The edit directive renders unconditionally, with or without PATTERN.md.
			if !strings.Contains(prompt, "Edit or Write") {
				t.Errorf("PlanSpec(...).Prompt lacks the edit directive's \"Edit or Write\" sentence: %q", prompt)
			}
			directiveIdx := strings.Index(prompt, "## Constraints")
			if tt.wantDirective {
				if stepIdx := strings.Index(prompt, "## Step 1"); directiveIdx == -1 || stepIdx == -1 || directiveIdx >= stepIdx {
					t.Errorf("pattern_directive (idx %d) does not precede ## Step 1 (idx %d) in prompt: %q", directiveIdx, stepIdx, prompt)
				}
				return
			}
			if directiveIdx != -1 {
				t.Errorf("PlanSpec(...).Prompt contains a ## Constraints heading with no PATTERN.md at the worktree root: %q", prompt)
			}
			// Two adjacent optional markers (pattern_directive, friction_directive) both rendering
			// empty legitimately produce one more blank line than a single empty marker would --
			// the threshold below is widened by exactly one newline from its pre-Tier-2 value to
			// account for that, while still catching any further unintended blank-line growth.
			if strings.Contains(prompt, "\n\n\n\n\n") {
				t.Errorf("PlanSpec(...).Prompt contains a stray blank-line block: %q", prompt)
			}
		})
	}
}

// writeTestPattern writes a minimal PATTERN.md into dir.
func writeTestPattern(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
	}
}

// TestPlanSpec_Refuses verifies PlanSpec rejects a malformed model-spec, and an empty specs directory:
// specs_dir is a required marker, so a blank render must fail loudly at composition instead of silently producing a prompt with a dead reference.
func TestPlanSpec_Refuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		plan      string
		blankSpec bool
	}{
		{"malformed model-spec", "opus[effort", false},
		{"empty specs directory", "opus[effort=high]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktreeRoot := filepath.Join("home", "user", "repo")
			layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
			reg, err := modelspec.LoadRegistry(t.TempDir())
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}
			specsDir := newTestSpecsDir(t)
			if tt.blankSpec {
				specsDir = ""
			}

			if _, err := PlanSpec(layout, newTestStencilsDir(t), specsDir, "", Config{Plan: tt.plan, PlanTimeoutMin: 120}, reg); err == nil {
				t.Errorf("PlanSpec(Plan=%q, specsDir=%q) = _, nil; want an error", tt.plan, specsDir)
			}
		})
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
