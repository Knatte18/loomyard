// rework_test.go — untagged Tier-1 unit tests for ReworkSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newReworkStencilsDir seeds the standard test stencils directory plus the rework stencil.
func newReworkStencilsDir(t *testing.T) string {
	t.Helper()

	dir := newTestStencilsDir(t)
	path := filepath.Join(dir, "loom", "loom-template-rework.md")
	if err := os.WriteFile(path, stencils.LoomTemplateRework, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
	return dir
}

// TestReworkSpec verifies the field mapping, that every marker renders its told path or value, and that the plan role's model-spec and timeout are reused.
func TestReworkSpec(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 90}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	stencilsDir := newReworkStencilsDir(t)
	specsDir := newTestSpecsDir(t)
	spec, err := ReworkSpec(layout, stencilsDir, specsDir, cfg, reg, 4)
	if err != nil {
		t.Fatalf("ReworkSpec(...) = _, %v; want nil error", err)
	}

	coverage := LoomReworkCoveragePath(layout)
	if len(spec.OutputFiles) != 1 || spec.OutputFiles[0] != coverage {
		t.Errorf("ReworkSpec(...).OutputFiles = %v; want [%q]", spec.OutputFiles, coverage)
	}
	if spec.Role != "rework" {
		t.Errorf("ReworkSpec(...).Role = %q; want %q", spec.Role, "rework")
	}
	if spec.Interactive {
		t.Error("ReworkSpec(...).Interactive = true; want false")
	}
	if spec.Effort != "high" {
		t.Errorf("ReworkSpec(...).Effort = %q; want %q", spec.Effort, "high")
	}
	if spec.Model == "" {
		t.Error("ReworkSpec(...).Model = \"\"; want non-empty")
	}
	if want := 90 * time.Minute; spec.Timeout != want {
		t.Errorf("ReworkSpec(...).Timeout = %s; want %s", spec.Timeout, want)
	}

	wantPaths := []string{
		LoomRejectionPath(layout),
		planparser.PlanDir(layout.AnchorPath()),
		planparser.PlanOverview(layout.AnchorPath()),
		DiscussionDecisionRecord(layout),
		coverage,
		stencilstore.Path(stencilsDir, "loom-template-plan"),
		specsDir,
	}
	for _, want := range wantPaths {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("ReworkSpec(...).Prompt does not contain %q", want)
		}
	}
	if !strings.Contains(spec.Prompt, "Number the new cards from 4 upward") {
		t.Error("ReworkSpec(...).Prompt does not number the new cards from the told card number 4")
	}
	if !strings.Contains(spec.Prompt, "lyx loom validate-plan --rework") {
		t.Error("ReworkSpec(...).Prompt does not name the rework-scoped self-check")
	}
	if strings.Contains(spec.Prompt, "{{") {
		t.Error("ReworkSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
	}
}

// TestReworkSpec_MissingStencil verifies a stencils directory without the rework stencil fails composition rather than producing an empty prompt.
func TestReworkSpec_MissingStencil(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 90}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	if _, err := ReworkSpec(layout, newTestStencilsDir(t), newTestSpecsDir(t), cfg, reg, 4); err == nil {
		t.Error("ReworkSpec(...) with no rework stencil = nil error; want an error")
	}
}
