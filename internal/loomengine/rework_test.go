// rework_test.go — untagged Tier-1 unit tests for ReworkSpec.
// Pure Go over an in-memory Config and a temp-dir modelspec registry;
// no live hub, reed, or network involved.

package loomengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// TestReworkSpec verifies the field mapping, that every marker renders its told path or value, and that the model is the rework role's, falling back to the plan role's, with the plan timeout.
//
//testtiming:keep pins OutputFiles, Role, Effort, Model, the reused plan Timeout and every told path and first card in the prompt, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestReworkSpec(t *testing.T) {
	tests := []struct {
		name       string
		rework     string
		wantModel  string
		wantEffort string
	}{
		{"rework empty spawns on plan", "", "opus", "high"},
		{"rework set spawns on its own model", "sonnet[effort=low]", "sonnet", "low"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktreeRoot := filepath.Join("home", "user", "repo")
			layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
			cfg := Config{Plan: "opus[effort=high]", Rework: tt.rework, PlanTimeoutMin: 90}

			reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			stencilsDir := newTestStencilsDir(t)
			specsDir := newTestSpecsDir(t)
			priorPlan := filepath.Join("prior", "plan")
			spec, err := ReworkSpec(layout, stencilsDir, specsDir, "", cfg, reg, 4, priorPlan)
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
			if spec.Segment != segmentcolor.Webster {
				t.Errorf("ReworkSpec(...).Segment = %q; want %q", spec.Segment, segmentcolor.Webster)
			}
			if spec.Interactive {
				t.Error("ReworkSpec(...).Interactive = true; want false")
			}
			if spec.Effort != tt.wantEffort {
				t.Errorf("ReworkSpec(...).Effort = %q; want %q", spec.Effort, tt.wantEffort)
			}
			if spec.Model != tt.wantModel {
				t.Errorf("ReworkSpec(...).Model = %q; want %q", spec.Model, tt.wantModel)
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
				priorPlan,
			}
			for _, want := range wantPaths {
				if !strings.Contains(spec.Prompt, want) {
					t.Errorf("ReworkSpec(...).Prompt does not contain %q", want)
				}
			}
			if !strings.Contains(spec.Prompt, "`first_card: 4`") {
				t.Error("ReworkSpec(...).Prompt does not carry the told first card 4")
			}
			if !strings.Contains(spec.Prompt, "numbered from 4 upward") {
				t.Error("ReworkSpec(...).Prompt does not number the new cards from the told first card 4")
			}
			if !strings.Contains(spec.Prompt, "lyx loom validate-plan --rework") {
				t.Error("ReworkSpec(...).Prompt does not name the rework-scoped self-check")
			}
			if strings.Contains(spec.Prompt, "{{") {
				t.Error("ReworkSpec(...).Prompt contains a leftover \"{{\" marker; want every marker filled")
			}
		})
	}
}

// TestReworkSpec_MissingStencil verifies a stencils directory without the rework stencil fails composition rather than producing an empty prompt.
func TestReworkSpec_MissingStencil(t *testing.T) {
	worktreeRoot := filepath.Join("home", "user", "repo")
	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktreeRoot), WorktreeName: filepath.Base(worktreeRoot)}
	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 90}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	stencilsDir := newTestStencilsDir(t)
	stencilkit.Remove(t, stencilsDir, "loom-template-rework")

	if _, err := ReworkSpec(layout, stencilsDir, newTestSpecsDir(t), "", cfg, reg, 4, "prior"); err == nil {
		t.Error("ReworkSpec(...) with no rework stencil = nil error; want an error")
	}
}
