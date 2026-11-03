// parts_test.go — untagged unit tests for PartsTable.
// Pure Go over an in-memory Config, a temp-dir modelspec registry and a seeded stencils directory;
// the lead's prompt is composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// partsTableConfig returns a config whose parts keys are depth, max and the model-spec parts.
func partsTableConfig(depth string, max int, parts string) Config {
	return Config{Parts: parts, PartsDepth: depth, PartsMax: max, PlanTimeoutMin: 120}
}

// fragmentText returns the text of the named depth fragment under stencilsDir as PartsTable fills it into the prompt.
func fragmentText(t *testing.T, stencilsDir, name string) string {
	t.Helper()
	body, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		t.Fatalf("stencilstore.Read(%q) = _, %v; want nil error", name, err)
	}
	return strings.TrimSpace(stencil.StripLeadingComment(string(body)))
}

// TestPartsTable verifies PartsTable builds the lead the Parts-Write row needs: its role, segment, timeout, inputs, output and model key.
// It also verifies the parts_depth fragment and parts_max reach the composed prompt, and the outcomes of a missing fragment and a missing friction stencil.
//
//testtiming:keep pins the lead's fields and the depth fragment choice, which no producer-level test asserts
func TestPartsTable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	reg := discussionTableRegistry(t)

	t.Run("the table carries the role, segment, timeout, inputs, output and model", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "backend")
		stencilsDir := newTestStencilsDir(t)

		table, err := PartsTable(layout, stencilsDir, newTestSpecsDir(t), partsTableConfig("light", 4, "sonnet[low]"), reg)
		if err != nil {
			t.Fatalf("PartsTable(...) = _, %v; want nil error", err)
		}
		if err := table.Validate(stencilsDir, ""); err != nil {
			t.Fatalf("table.Validate() = %v; want nil", err)
		}
		if table.RolePrefix != "parts" || table.Segment != segmentcolor.Plan || table.Timeout != 120*time.Minute || table.Interactive {
			t.Errorf("table role/segment/timeout/interactive = %q/%q/%s/%v; want parts/%q/2h0m0s/false", table.RolePrefix, table.Segment, table.Timeout, table.Interactive, segmentcolor.Plan)
		}
		if len(table.Seats) != 1 {
			t.Fatalf("table has %d seats; want the lead alone", len(table.Seats))
		}
		lead := table.Chair()
		if want := []string{partsparser.File(layout.AnchorPath())}; !slices.Equal(lead.Outputs, want) {
			t.Errorf("lead outputs = %v; want the parts file %v", lead.Outputs, want)
		}
		if want := []string{DiscussionDecisionRecord(layout), DiscussionRelevant(layout)}; !slices.Equal(lead.Inputs, want) {
			t.Errorf("lead inputs = %v; want the decision record and the relevant list %v", lead.Inputs, want)
		}
		if lead.Model != "sonnet" || lead.Effort != "low" {
			t.Errorf("lead model/effort = %q/%q; want sonnet/low from parts: sonnet[low]", lead.Model, lead.Effort)
		}
		if want := (shuttleengine.ModelKey{Module: "tapestry", Key: "parts"}); lead.ModelKey != want {
			t.Errorf("lead ModelKey = %+v; want %+v", lead.ModelKey, want)
		}
		if lead.ForkSubagents {
			t.Error("lead ForkSubagents = true; want false")
		}
	})

	t.Run("the depth fragment and parts_max reach the composed prompt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			depth, wantFragment, otherFragment string
		}{
			{"light", "loom-fragment-parts-light", "loom-fragment-parts-medium"},
			{"medium", "loom-fragment-parts-medium", "loom-fragment-parts-light"},
		}
		for _, tt := range tests {
			t.Run(tt.depth, func(t *testing.T) {
				t.Parallel()
				layout := planTableLayout(t, "")
				stencilsDir := newTestStencilsDir(t)
				table, err := PartsTable(layout, stencilsDir, newTestSpecsDir(t), partsTableConfig(tt.depth, 7, "sonnet[low]"), reg)
				if err != nil {
					t.Fatalf("PartsTable(depth %s) = _, %v; want nil error", tt.depth, err)
				}
				prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
				if err != nil {
					t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
				}
				prompt := prompts[""]
				want := fragmentText(t, stencilsDir, tt.wantFragment)
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt at depth %s lacks the %s fragment text", tt.depth, tt.depth)
				}
				if other := fragmentText(t, stencilsDir, tt.otherFragment); strings.Contains(prompt, other) {
					t.Errorf("prompt at depth %s carries the other depth's fragment text", tt.depth)
				}
				if !strings.Contains(prompt, "Hold at most 7 parts.") {
					t.Errorf("prompt lacks the configured parts_max 7")
				}
			})
		}
	})

	t.Run("a stencils directory without the chosen fragment fails the table naming it", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "")
		stencilsDir := newTestStencilsDir(t)
		stencilkit.Remove(t, stencilsDir, "loom-fragment-parts-medium")

		_, err := PartsTable(layout, stencilsDir, newTestSpecsDir(t), partsTableConfig("medium", 6, "sonnet[low]"), reg)
		if err == nil || !strings.Contains(err.Error(), "loom-fragment-parts-medium") {
			t.Errorf("PartsTable(medium, fragment removed) = _, %v; want an error naming loom-fragment-parts-medium", err)
		}
	})

	t.Run("a stencils directory without the friction stencil leaves the friction directive empty", func(t *testing.T) {
		t.Parallel()
		layout := planTableLayout(t, "")
		stencilsDir := newMinimalStencilsDir(t)
		cfg := partsTableConfig("light", 6, "sonnet[low]")
		cfg.Friction = "sonnet[medium]"

		table, err := PartsTable(layout, stencilsDir, newTestSpecsDir(t), cfg, reg)
		if err != nil {
			t.Fatalf("PartsTable(friction stencil removed) = _, %v; want nil error", err)
		}
		if got := table.Values[friction.MarkerName]; got != "" {
			t.Errorf("friction directive = %q; want empty", got)
		}
	})
}
