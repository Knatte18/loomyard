// raddletable_test.go — tier-0 unit tests for RaddleTable.
// The tests are pure Go over an in-memory Config, the built-in model registry and a seeded stencils directory.
// The seat prompt is composed through seatengine.Prompts, which starts nothing.

package loomengine

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRaddleTable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	reg := discussionTableRegistry(t)

	const reason = "the board moved: the entry other was revised"
	tests := []struct {
		name         string
		interactive  bool
		returnReason string
	}{
		{name: "FirstSessionAuto"},
		{name: "FirstSessionInteractive", interactive: true},
		{name: "ReturnAuto", returnReason: reason},
		{name: "ReturnInteractive", interactive: true, returnReason: reason},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout := discussionTableLayout(t)
			stencilsDir := newTestStencilsDir(t)
			cfg := Config{Raddle: "opus[effort=high]", DiscussionTimeoutMin: 480, SubagentModel: "haiku"}

			table, err := RaddleTable(layout, stencilsDir, discussionTableSpecsDir, cfg, reg, discussionTableSlug, tt.interactive, tt.returnReason)
			if err != nil {
				t.Fatalf("RaddleTable(...) = _, %v; want nil error", err)
			}

			if table.RolePrefix != raddleRole || table.Segment != segmentcolor.Discussion || table.Timeout != 480*time.Minute || table.Interactive != tt.interactive {
				t.Errorf("table prefix/segment/timeout/interactive = %q/%q/%s/%v; want %q/%q/480m0s/%v", table.RolePrefix, table.Segment, table.Timeout, table.Interactive, raddleRole, segmentcolor.Discussion, tt.interactive)
			}
			if want := []string{"return_note"}; !slices.Equal(table.Optional, want) {
				t.Errorf("table.Optional = %v; want %v", table.Optional, want)
			}
			if len(table.Seats) != 1 {
				t.Fatalf("table has %d seats; want the one lead", len(table.Seats))
			}
			seat := table.Chair()
			wantOutput := RaddleProposal(layout)
			if tt.interactive {
				wantOutput = RaddleApproval(layout)
			}
			if !slices.Equal(seat.Outputs, []string{wantOutput}) {
				t.Errorf("seat outputs = %v; want only %q", seat.Outputs, wantOutput)
			}
			if want := (shuttleengine.ModelKey{Module: "loom", Key: "raddle"}); seat.ModelKey != want {
				t.Errorf("seat ModelKey = %+v; want %+v", seat.ModelKey, want)
			}
			if seat.Name != "" || seat.Stencil != raddleStencil || seat.Engine != "claude" || seat.Model == "" || seat.Effort != "high" || !slices.Equal(seat.Skills, raddleSkills) || seat.ForkSubagents {
				t.Errorf("seat = %+v; want the lead with the raddle stencil, claude, a model, high effort, its skills and no forks", seat)
			}

			if got, want := table.Values["return_note"] != "", tt.returnReason != ""; got != want {
				t.Errorf("return_note set = %v; want %v", got, want)
			}
			if err := table.Validate(stencilsDir, ""); err != nil {
				t.Fatalf("table.Validate() = %v; want nil, a blank return_note included", err)
			}
			prompts, err := seatengine.Prompts(discussionTableGeometry(layout, stencilsDir), table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = _, %v; want nil error", err)
			}

			prompt := prompts[""]
			wants := []string{"`ly:orch`", discussionTableSlug, DiscussionDecisionRecord(layout), RaddleProposal(layout), discussionTableSpecsDir + "/raddle/raddle-proposal-spec.md", `model: "haiku"`, raddleModeRules(tt.interactive, RaddleProposal(layout))}
			if tt.returnReason != "" {
				wants = append(wants, tt.returnReason)
			}
			if tt.returnReason != "" && !tt.interactive {
				wants = append(wants, RaddleReturnedProposal(layout))
			}
			for _, want := range wants {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt does not contain %q", want)
				}
			}
			if strings.Contains(prompt, raddleModeRules(!tt.interactive, RaddleProposal(layout))) {
				t.Errorf("prompt carries the %v-mode text; want only the %v-mode text", !tt.interactive, tt.interactive)
			}
			if strings.Contains(prompt, "{{") || strings.Contains(prompt, "scribe:") || unboundedAskOperator(prompt) {
				t.Errorf("prompt carries an unrendered marker, a skill name or an ask-the-operator sentence:\n%s", prompt)
			}
		})
	}

	t.Run("AnEmptySlugIsRefused", func(t *testing.T) {
		t.Parallel()
		layout := discussionTableLayout(t)
		if _, err := RaddleTable(layout, newTestStencilsDir(t), discussionTableSpecsDir, Config{Raddle: "opus[high]"}, reg, "", false, ""); err == nil {
			t.Error("RaddleTable() with an empty slug = nil error; want one")
		}
	})
}
