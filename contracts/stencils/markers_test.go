//go:build tier0

// markers_test.go pins the exact top-level marker set of each stencil whose markers no render test reads in its real file,
// and the position of each Go-filled marker a row anchors on a heading or the whole text.
// The rubrics' marker allowlist is pinned in rubric_test.go.

package stencils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestStencils_TopLevelMarkers asserts each stencil declares exactly its required top-level markers.
//
//testtiming:keep pins the exact marker set of each stencil, which the rubric test's subset check and the render tests do not
func TestStencils_TopLevelMarkers(t *testing.T) {
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
		{"orch-template-role", OrchTemplateRole, []string{"command_index", "decision_note_title", "hub_rules", "orch_rules"}},
		{"seat-directive-chair", SeatDirectiveChair, []string{"advisor_names", "inputs", "output_files"}},
		{"seat-directive-advisor", SeatDirectiveAdvisor, []string{"chair_name", "output_files", "seat_name"}},
		{"twill-directive", TwillDirective, []string{"final_self_check_command", "judge_steps", "lens_list", "lens_report_path", "lens_report_spec_path", "relaunch_rules", "reviewed_files", "rubric_path", "severity_rule", "spawn_point", "writer_fork"}},
		{"twill-judge-critical", TwillJudgeCritical, []string{"disposition_place", "dispute_evidence", "governing_input"}},
		{"twill-judge-plain", TwillJudgePlain, []string{"disposition_place", "rejection_rule"}},
		{"twill-writer-fork", TwillWriterFork, []string{"disposition_place", "explore_step", "files_dir", "format_step", "input_record", "last_file", "reviewed_files", "self_check_command", "self_check_step"}},
		{"twill-relaunch", TwillRelaunch, []string{"disposition_place", "format_step", "relaunch_files", "self_check_command", "self_check_step"}},
		{"loom-template-parent-review-keepalive", LoomTemplateParentReviewKeepalive, []string{"ping"}},
		{"loom-template-discussion-chair", LoomTemplateDiscussionChair, nil},
		{"loom-template-discussion-advisor", LoomTemplateDiscussionAdvisor, []string{"decision_record_path", "edit_directive", "output_files", "parent_directive", "pattern_directive", "slug", "support_log_path"}},
		{"landing-template-describe-darn", LandingTemplateDescribeDarn, []string{"decision_record_path", "description_path", "edit_directive", "merge_base", "parent_branch", "parent_directive", "review_dir", "slug", "task_branch"}},
		{"webster-body-verify-fix", WebsterBodyVerifyFix, []string{"fix_gate", "outcome_path", "plan_dir", "report_path", "summary_path", "worktree_root"}},
		{"webster-prefix-verify-fix-fork", WebsterPrefixVerifyFixFork, []string{"friction_directive"}},
		{"webster-prefix-verify-fix-cold", WebsterPrefixVerifyFixCold, []string{"edit_directive", "friction_directive", "parent_directive", "pattern_directive", "reply_path"}},
		{"bouncer-template-escalation", BouncerTemplateEscalation, []string{"cause", "decide_options", "decision_record_path", "judge_stencil_path", "ledger_path", "review_path", "round", "segment", "slug", "verdict_path", "worktree"}},
		{"bouncer-fragment-decide-capped", BouncerFragmentDecideCapped, []string{"slug"}},
		{"pattern-fragment-changes", PatternFragmentChanges, []string{"pattern_change_list"}},
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
