// rework_test.go — untagged Tier-1 unit tests for ReworkTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and a seeded stencils directory;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// TestReworkTable verifies ReworkTable builds the lead the PR-Rework row needs.
// Its lead carries the rework role, model, effort, version, output, skills, segment, timeout, interactivity and fork flag.
// The model is the rework role's, falling back to the plan role's, with the plan timeout, and the prompt holds every told path, the first card and the prior plan directory.
// A stencils directory without the rework stencil is refused.
//
//testtiming:keep pins every lead field, and every told path and the first card in the prompt, which TestProducerSpecs_SkillsAndParentDirective does not assert
func TestReworkTable(t *testing.T) {
	t.Parallel()
	reg := discussionTableRegistry(t)

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
			t.Parallel()
			layout := planTableLayout(t, "backend")
			stencilsDir := newTestStencilsDir(t)
			specsDir := newTestSpecsDir(t)
			priorPlan := filepath.Join("prior", "plan")
			cfg := Config{Plan: "opus[effort=high]", Rework: tt.rework, PlanTimeoutMin: 90}

			table, err := ReworkTable(layout, stencilsDir, specsDir, cfg, reg, 4, priorPlan)
			if err != nil {
				t.Fatalf("ReworkTable(...) = _, %v; want nil error", err)
			}
			if err := table.Validate(stencilsDir); err != nil {
				t.Fatalf("table.Validate() = %v; want nil", err)
			}
			if len(table.Seats) != 1 {
				t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
			}

			lead := table.Chair()
			if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != "rework" {
				t.Errorf("lead name %q and role %q; want no name and the role %q", lead.Name, role, "rework")
			}
			if lead.Model != tt.wantModel || lead.Effort != tt.wantEffort || lead.Version != "" {
				t.Errorf("lead model/effort/version = %q/%q/%q; want %s/%s/none", lead.Model, lead.Effort, lead.Version, tt.wantModel, tt.wantEffort)
			}
			coverage := LoomReworkCoveragePath(layout)
			if want := []string{coverage}; !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want the coverage file %v", lead.Outputs, want)
			}
			if want := []string{"scribe:prose", "scribe:testing"}; !slices.Equal(lead.Skills, want) {
				t.Errorf("lead skills = %v; want %v", lead.Skills, want)
			}
			if table.Segment != segmentcolor.Webster {
				t.Errorf("table segment = %q; want %q", table.Segment, segmentcolor.Webster)
			}
			if table.Timeout != 90*time.Minute {
				t.Errorf("table timeout = %s; want 1h30m0s", table.Timeout)
			}
			if table.Interactive {
				t.Error("table interactive = true; want false")
			}
			if lead.ForkSubagents {
				t.Error("lead ForkSubagents = true; want false")
			}

			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
			}
			prompt := prompts[""]

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
				if !strings.Contains(prompt, want) {
					t.Errorf("lead prompt does not contain %q", want)
				}
			}
			for _, want := range []string{"`first_card: 4`"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lead prompt does not contain %q", want)
				}
			}
			if strings.Contains(prompt, "{{") {
				t.Error("lead prompt contains a leftover \"{{\" marker; want every marker filled")
			}
		})
	}

	t.Run("missing stencil is refused", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "")
		stencilsDir := newTestStencilsDir(t)
		stencilkit.Remove(t, stencilsDir, reworkStencilName)

		// A missing stencil is the seat engine's refusal: the table builder reads none, and Validate reads every seat's.
		table, err := ReworkTable(layout, stencilsDir, newTestSpecsDir(t), Config{Plan: "opus[effort=high]", PlanTimeoutMin: 90}, reg, 4, "prior")
		if err == nil {
			err = table.Validate(stencilsDir)
		}
		if err == nil || !strings.Contains(err.Error(), reworkStencilName) {
			t.Errorf("ReworkTable and its Validate with no rework stencil = %v; want an error naming %q", err, reworkStencilName)
		}
	})
}
