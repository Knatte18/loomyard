// markers_test.go pins the exact top-level marker set of the stencils whose marker set is a contract on its own: burler-focus-directive.md and loom-template-prior-plan.md.
// The wording of those stencils is pinned in claims_test.go, and the rubrics' marker allowlist in rubric_test.go.

package stencils

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestStencils_TopLevelMarkers asserts each stencil declares exactly its required top-level markers.
//
//testtiming:keep pins the exact marker set of each stencil, which the rubric test's subset check and the wording claims do not
func TestStencils_TopLevelMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		def  []byte
		want []string
	}{
		{"burler-focus-directive", BurlerFocusDirective, []string{"focus_path"}},
		{"loom-template-prior-plan", LoomTemplatePriorPlan, []string{"archive_dir", "moved_files"}},
		{"orch-template-role", OrchTemplateRole, []string{"command_index"}},
		{"seat-directive-chair", SeatDirectiveChair, []string{"advisor_names", "inputs", "output_files"}},
		{"seat-directive-advisor", SeatDirectiveAdvisor, []string{"chair_name", "output_files", "seat_name"}},
		{"twill-directive", TwillDirective, []string{"final_self_check_command", "judge_steps", "lens_list", "relaunch_rules", "reviewed_files", "rubric_path", "severity_rule", "spawn_point", "writer_fork"}},
		{"twill-judge-critical", TwillJudgeCritical, []string{"disposition_place", "dispute_evidence", "governing_input"}},
		{"twill-judge-plain", TwillJudgePlain, []string{"disposition_place", "rejection_rule"}},
		{"twill-writer-fork", TwillWriterFork, []string{"disposition_place", "explore_step", "files_dir", "format_step", "input_record", "last_file", "reviewed_files", "self_check_command", "self_check_step"}},
		{"twill-relaunch", TwillRelaunch, []string{"disposition_place", "format_step", "self_check_command", "self_check_step"}},
		{"webster-fragment-locate-quarry", WebsterFragmentLocateQuarry, nil},
		{"webster-fragment-locate-pack", WebsterFragmentLocatePack, []string{"specs_dir"}},
		{"webster-fragment-card-read-plain", WebsterFragmentCardReadPlain, nil},
		{"webster-fragment-card-read-pack", WebsterFragmentCardReadPack, nil},
		{"webster-fragment-fresh-read-plain", WebsterFragmentFreshReadPlain, nil},
		{"webster-fragment-fresh-read-pack", WebsterFragmentFreshReadPack, nil},
		{"webster-fragment-pack-section", WebsterFragmentPackSection, []string{"pack_cards"}},
		{"webster-fragment-pack-read", WebsterFragmentPackRead, []string{"pack_path"}},
		{"webster-fragment-pack-card", WebsterFragmentPackCard, []string{"card_path", "pack_entries", "pack_unresolved"}},
		{"webster-fragment-pack-unresolved", WebsterFragmentPackUnresolved, []string{"pack_unresolved"}},
		{"webster-fragment-pack-empty", WebsterFragmentPackEmpty, nil},
		{"webster-fragment-guard-scan-error", WebsterFragmentGuardScanError, []string{"guard_error"}},
		{"webster-fragment-guard-too-long", WebsterFragmentGuardTooLong, []string{"guard_length", "guard_limit"}},
		{"webster-fragment-guard-fix", WebsterFragmentGuardFix, []string{"card_path", "guard_file"}},
		{"loom-template-parent-review-keepalive", LoomTemplateParentReviewKeepalive, []string{"ping"}},
		{"loom-template-discussion-chair", LoomTemplateDiscussionChair, nil},
		{"loom-template-discussion-advisor", LoomTemplateDiscussionAdvisor, []string{"decision_record_path", "edit_directive", "output_files", "parent_directive", "pattern_directive", "slug", "support_log_path"}},
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
