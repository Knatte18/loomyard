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
		{"twill-directive", TwillDirective, []string{"disposition_place", "format_step", "lens_list", "ready_marker", "rejection_rule", "reviewed_files", "rubric_path", "self_check_command", "self_check_step", "severity_rule", "spawn_point"}},
		{"twill-write-scope", TwillWriteScope, []string{"ready_marker"}},
		{"webster-fragment-locate-quarry", WebsterFragmentLocateQuarry, nil},
		{"webster-fragment-locate-pack", WebsterFragmentLocatePack, []string{"specs_dir"}},
		{"webster-fragment-pack-section", WebsterFragmentPackSection, []string{"pack_cards"}},
		{"webster-fragment-pack-card", WebsterFragmentPackCard, []string{"card_path", "pack_entries", "pack_unresolved"}},
		{"webster-fragment-pack-unresolved", WebsterFragmentPackUnresolved, []string{"pack_unresolved"}},
		{"webster-fragment-pack-empty", WebsterFragmentPackEmpty, nil},
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
