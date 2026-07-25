// plan_test.go — untagged Tier-1 unit tests for PlanSpec. Pure Go over an
// in-memory Config and a temp-dir modelspec registry; no live hub, mux, or
// network involved.

package loomengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestPlanSpec verifies PlanSpec's field mapping against a hand-built
// Layout, an in-memory Config, and the built-in modelspec registry (no
// models.yaml present).
func TestPlanSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	wantOutputFiles := []string{
		filepath.Join(worktreeRoot, "_lyx", "plan", "00-overview.md"),
	}
	wantTimeout := 120 * time.Minute

	spec, err := PlanSpec(layout, cfg, reg)
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

// TestPlanSpec_PromptFilled verifies the rendered prompt contains every
// resolved marker value and no leftover "{{", proving stencil.Fill filled
// every marker in plan-template.md rather than silently leaving one blank.
func TestPlanSpec_PromptFilled(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}

	decisionRecordPath := layout.DiscussionDecisionRecord()
	planDir := layout.PlanDir()
	overviewPath := layout.PlanOverview()

	for _, want := range []string{decisionRecordPath, planDir, overviewPath} {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q", want)
		}
	}
	if strings.Contains(spec.Prompt, "{{") {
		t.Error("PlanSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
	}
}

// TestPlanSpec_PromptStatesCardCriteria verifies the rendered prompt
// carries plan-format-v3's card-granularity contract ("What a card is"),
// not just the field format: a live run against a template without these
// criteria produced a card introducing new behavior with no bundled test
// (proven live, round fable-r1), so their presence is pinned here.
func TestPlanSpec_PromptStatesCardCriteria(t *testing.T) {
	prompt := renderedPlanPrompt(t)

	for _, want := range []string{
		"What a card is",
		"smallest change",
		"independently committable",
		"Bundles its own test",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("PlanSpec(...).Prompt does not contain %q; the card-granularity contract must reach the agent", want)
		}
	}
}

// renderedPlanPrompt returns the prompt PlanSpec renders for a hand-built
// Layout and the default in-memory Config, for template-content assertions.
func renderedPlanPrompt(t *testing.T) string {
	t.Helper()

	layout := &hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	spec, err := PlanSpec(layout, cfg, reg)
	if err != nil {
		t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
	}
	return spec.Prompt
}

// TestPlanSpec_MalformedModelSpec verifies a Config with an ungrammatical
// plan model-spec yields a non-nil error rather than propagating the bad
// spec into a Spec.
func TestPlanSpec_MalformedModelSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
	cfg := Config{Plan: "opus[effort", PlanTimeoutMin: 120}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := PlanSpec(layout, cfg, reg); err == nil {
		t.Fatal("PlanSpec(..., Plan=\"opus[effort\") = _, nil; want non-nil error")
	}
}
