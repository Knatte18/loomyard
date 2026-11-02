// markers_test.go pins the exact top-level marker set of each stencil whose markers no render test reads in its real file,
// and the position of each Go-filled marker a row anchors on a heading or the whole text.
// The rubrics' marker allowlist is pinned in rubric_test.go.

package stencils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/orchrole"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStencils_TopLevelMarkers asserts each stencil declares exactly its required top-level markers.
//
//testtiming:keep pins the exact marker set of each stencil, which the rubric test's subset check and the render tests do not
func TestStencils_TopLevelMarkers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		def  []byte
		want []string
	}{
		{"burler-focus-directive", BurlerFocusDirective, []string{"focus_path"}},
		{"burler-fork-reading", BurlerForkReading, nil},
		{"burler-cluster-solo", BurlerClusterSolo, nil},
		{"burler-cluster-fan", BurlerClusterFan, []string{"lens_list", "lens_report_path", "lens_report_spec_path"}},
		{"burler-fix-self-check", BurlerFixSelfCheck, []string{"lens_list", "lens_report_path", "lens_report_spec_path", "rubric_path"}},
		{"burler-fix-check-rubric", BurlerFixCheckRubric, nil},
		{"loom-template-prior-plan", LoomTemplatePriorPlan, []string{"archive_dir", "moved_files"}},
		{"orch-role", orchrole.Role, []string{"command_index", "decision_note_title"}},
		{"loom-block-plan-glyphs", LoomBlockPlanGlyphs, nil},
		{"loom-block-plan-format", LoomBlockPlanFormat, []string{"specs_dir"}},
		{"loom-block-plan-overview", LoomBlockPlanOverview, []string{"specs_dir"}},
		{"loom-block-plan-cards", LoomBlockPlanCards, []string{"gate_tiers", "specs_dir"}},
		{"loom-template-parts", LoomTemplateParts, []string{"decision_record_path", "depth_block", "edit_directive", "friction_directive", "parent_directive", "parts_dir", "parts_max", "parts_path", "pattern_directive", "relevant_path", "specs_dir"}},
		{"loom-fragment-parts-light", LoomFragmentPartsLight, nil},
		{"loom-fragment-parts-medium", LoomFragmentPartsMedium, nil},
		{"loom-template-part-writer", LoomTemplatePartWriter, []string{"decision_record_path", "edit_directive", "friction_directive", "overview_path", "parent_directive", "part_name", "parts_path", "pattern_directive", "plan_dir", "relevant_path", "specs_dir"}},
		{"loom-template-parts-chair", LoomTemplatePartsChair, []string{"decision_record_path", "edit_directive", "friction_directive", "join_path", "overview_path", "parent_directive", "parts_dir", "parts_path", "pattern_directive", "plan_dir", "specs_dir"}},
		{"seat-directive-chair", SeatDirectiveChair, []string{"advisor_names", "inputs", "output_files"}},
		{"seat-directive-advisor", SeatDirectiveAdvisor, []string{"chair_name", "output_files", "seat_name"}},
		{"seat-directive-writer", SeatDirectiveWriter, []string{"chair_name", "output_files", "seat_name"}},
		{"seat-directive-writer-chair", SeatDirectiveWriterChair, []string{"advisor_names", "inputs", "output_files"}},
		{"twill-directive", TwillDirective, []string{"checked_clause", "closing_step", "judge_steps", "lens_list", "lens_report_path", "lens_report_spec_path", "relaunch_rules", "reviewed_files", "rubric_path", "severity_rule", "spawn_point", "writer_fork"}},
		{"twill-writer-fork-gated", TwillWriterForkGated, []string{"disposition_place", "explore_step", "files_dir", "format_step", "input_record", "last_file", "reviewed_files"}},
		{"loom-fragment-self-check-plan", LoomFragmentSelfCheckPlan, nil},
		{"loom-fragment-self-check-plan-gated", LoomFragmentSelfCheckPlanGated, nil},
		{"loom-fragment-self-check-discussion", LoomFragmentSelfCheckDiscussion, nil},
		{"loom-fragment-self-check-discussion-gated", LoomFragmentSelfCheckDiscussionGated, nil},
		{"twill-judge-critical", TwillJudgeCritical, []string{"disposition_place", "dispute_evidence", "governing_input"}},
		{"twill-judge-plain", TwillJudgePlain, []string{"disposition_place", "rejection_rule"}},
		{"twill-writer-fork", TwillWriterFork, []string{"disposition_place", "explore_step", "files_dir", "format_step", "input_record", "last_file", "reviewed_files", "self_check_command", "self_check_step"}},
		{"twill-relaunch", TwillRelaunch, []string{"disposition_place", "format_step", "relaunch_files", "self_check_command", "self_check_step"}},
		{"drift-template-baseline", DriftTemplateBaseline, []string{"baseline_path", "edit_directive", "parent_directive", "scope_note", "subject_reading"}},
		{"drift-template-verify", DriftTemplateVerify, []string{"baseline_head", "baseline_path", "compare_log", "edit_directive", "judging_rules", "max_runs", "mode_rules", "parent_directive", "pattern_directive", "report_path", "scope_path", "subject_reading"}},
		{"loom-template-discussion-chair", LoomTemplateDiscussionChair, nil},
		{"loom-template-discussion-advisor", LoomTemplateDiscussionAdvisor, []string{"decision_record_path", "edit_directive", "output_files", "parent_directive", "pattern_directive", "slug", "support_log_path"}},
		{"webster-body-verify-fix", WebsterBodyVerifyFix, []string{"fix_gate", "outcome_path", "plan_dir", "report_path", "summary_path", "worktree_root"}},
		{"webster-prefix-verify-fix-fork", WebsterPrefixVerifyFixFork, []string{"friction_directive"}},
		{"webster-fragment-card-fixer", WebsterFragmentCardFixer, nil},
		{"gatefix-frame-cold", GatefixFrameCold, []string{"edit_directive", "friction_directive", "parent_directive", "pattern_directive", "reply_path", "report_path"}},
		{"gatefix-body-darn", GatefixBodyDarn, []string{"decision_record_path", "fix_gate", "worktree_root"}},
		{"gatefix-body-plan", GatefixBodyPlan, []string{"lens_review_path", "plan_dir", "recheck_command"}},
		{"gatefix-body-discussion", GatefixBodyDiscussion, []string{"decision_record_path", "recheck_command", "relevant_path", "support_log_path"}},
		{"gatefix-body-card", GatefixBodyCard, []string{"card_ids", "gate_call", "worktree_root"}},
		{"webster-template-plan-fix", WebsterTemplatePlanFix, []string{"edit_directive", "friction_directive", "parent_directive", "plan_dir", "record_path", "reply_path", "specs_dir", "worktree_root"}},
		{"bouncer-template-escalation", BouncerTemplateEscalation, []string{"cause", "cause_detail", "decide_options", "decision_record_path", "judge_stencil_path", "ledger_path", "review_path", "round", "segment", "slug", "verdict_path", "worktree"}},
		{"bouncer-fragment-decide-capped", BouncerFragmentDecideCapped, []string{"slug"}},
		{"bouncer-fragment-decision", BouncerFragmentDecision, []string{"blocking_findings", "decide_verbs", "decision_findings", "refusal", "slug"}},
		{"pattern-fragment-changes", PatternFragmentChanges, []string{"pattern_change_list"}},
		{"loom-directive-project-record", LoomDirectiveProjectRecord, []string{"project_records"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := stencil.TopLevelMarkers(tt.def)
			if err != nil {
				t.Fatalf("stencil.TopLevelMarkers(%s) = _, %v; want nil error", tt.name, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TopLevelMarkers(%s) = %v; want %v", tt.name, got, tt.want)
			}
		})
	}
}

// headingSection returns the text from the line equal to heading, or starting with heading and a space, up to the next "## " heading, and whether the heading exists.
// A heading may carry a trailing clause after the part a caller names.
func headingSection(text, heading string) (string, bool) {
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		if line == heading || strings.HasPrefix(line, heading+" ") {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], ""), true
}

// TestStencils_MarkerPositions asserts each Go-filled marker sits where the prompt needs it, anchored on headings only.
// A row with a heading holds its marker inside that heading's section; a row without one holds it anywhere in the whole text.
func TestStencils_MarkerPositions(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		stencil string
		// heading limits the check to that heading's section; empty checks the whole text.
		heading string
		want    string
	}{
		{"plan twill directive", "loom-template-plan", "## Step 2 — Explore the codebase", "{{.twill_directive}}"},
		{"discussion twill directive", "loom-template-discussion", "## Step 4 — How to get answers", "{{.twill_directive}}"},
		{"plan self check", "loom-template-plan", "## Step 5 — Self-check before ending your turn", "{{.self_check}}"},
		{"discussion self check", "loom-template-discussion", "## Step 6 — Self-check before ending your turn", "{{.self_check}}"},
		{"discussion decision record path", "loom-template-discussion", "## What you may write", "{{.decision_record_path}}"},
		{"discussion support log path", "loom-template-discussion", "## What you may write", "{{.support_log_path}}"},
		{"driver parent notify", "shed-template-driver", "## Notifying the parent", "{{.parent_notify}}"},
		{"driver arbiter command", "shed-template-driver", "## Arbiter", "{{.arbiter_command}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def, ok := Registry().Default(tt.stencil)
			if !ok {
				t.Fatalf("stencil %q is not registered", tt.stencil)
			}
			text := string(def)
			if tt.heading != "" {
				section, ok := headingSection(text, tt.heading)
				if !ok {
					t.Fatalf("stencil %q has no %q heading", tt.stencil, tt.heading)
				}
				text = section
			}
			if !strings.Contains(text, tt.want) {
				t.Errorf("stencil %q (section %q) does not contain %q", tt.stencil, tt.heading, tt.want)
			}
		})
	}
}
