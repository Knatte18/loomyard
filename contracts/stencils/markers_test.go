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
		{"seat-directive-chair", SeatDirectiveChair, []string{"advisor_names", "failed_advisors", "inputs", "output_files"}},
		{"seat-directive-advisor", SeatDirectiveAdvisor, []string{"chair_name", "output_files", "seat_name"}},
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
