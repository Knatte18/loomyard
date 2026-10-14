// rework_test.go — untagged Tier-1 unit tests for ReworkTable, checked against the ReworkSpec it replaces.
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
// Its lead equals the ReworkSpec it replaces in role, model, effort, version, output, skills, segment, timeout, interactivity and fork flag, and its prompt from seatengine.Prompts under a geometry naming the same parent is byte-equal to the spec's,
// so the seat probe matches a run the spec started and the prompt keeps its bytes.
// The model is the rework role's, falling back to the plan role's, with the plan timeout, and the prompt holds every told path, the first card and the prior plan directory.
// A stencils directory without the rework stencil is refused.
//
//testtiming:keep pins the parity of every lead field and the prompt bytes with ReworkSpec, and every told path and the first card in the prompt, which TestProducerSpecs_SkillsAndParentDirective does not assert
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

			spec, err := ReworkSpec(layout, stencilsDir, specsDir, "ly:orch", cfg, reg, 4, priorPlan)
			if err != nil {
				t.Fatalf("ReworkSpec(...) = _, %v; want nil error", err)
			}
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
			if role := seatengine.SeatRole(table.RolePrefix, lead.Name); lead.Name != "" || role != spec.Role || role != "rework" {
				t.Errorf("lead name %q and role %q; want no name and the role %q the spec carries", lead.Name, role, spec.Role)
			}
			if lead.Model != spec.Model || lead.Effort != spec.Effort || lead.Version != spec.Version || lead.Model != tt.wantModel || lead.Effort != tt.wantEffort {
				t.Errorf("lead model/effort/version = %q/%q/%q; want the spec's %q/%q/%q (%s, %s)", lead.Model, lead.Effort, lead.Version, spec.Model, spec.Effort, spec.Version, tt.wantModel, tt.wantEffort)
			}
			coverage := LoomReworkCoveragePath(layout)
			if want := []string{coverage}; !slices.Equal(lead.Outputs, spec.OutputFiles) || !slices.Equal(lead.Outputs, want) {
				t.Errorf("lead outputs = %v; want the spec's %v, the coverage file %v", lead.Outputs, spec.OutputFiles, want)
			}
			if !slices.Equal(lead.Skills, spec.Skills) || !slices.Equal(lead.Skills, []string{"scribe:prose", "scribe:testing"}) {
				t.Errorf("lead skills = %v; want the spec's %v", lead.Skills, spec.Skills)
			}
			if table.Segment != spec.Segment || table.Segment != segmentcolor.Webster {
				t.Errorf("table segment = %q; want the spec's %q", table.Segment, spec.Segment)
			}
			if table.Timeout != spec.Timeout || table.Timeout != 90*time.Minute {
				t.Errorf("table timeout = %s; want the spec's %s", table.Timeout, spec.Timeout)
			}
			if table.Interactive != spec.Interactive || table.Interactive {
				t.Errorf("table interactive = %v; want the spec's %v", table.Interactive, spec.Interactive)
			}
			if lead.ForkSubagents != spec.ForkSubagents || lead.ForkSubagents {
				t.Errorf("lead ForkSubagents = %v; want the spec's %v", lead.ForkSubagents, spec.ForkSubagents)
			}

			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
			}
			prompt := prompts[""]
			if prompt != spec.Prompt {
				t.Errorf("lead prompt differs from the spec's prompt; want byte-equal prompts\nlead:\n%s\nspec:\n%s", prompt, spec.Prompt)
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
				if !strings.Contains(prompt, want) {
					t.Errorf("lead prompt does not contain %q", want)
				}
			}
			for _, want := range []string{"`first_card: 4`", "numbered from 4 upward", "lyx loom validate-plan --rework"} {
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
