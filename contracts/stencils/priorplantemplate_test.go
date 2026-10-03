// priorplantemplate_test.go pins loom-template-prior-plan.md's marker set; its instructions are pinned in claims_test.go.

package stencils

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestLoomTemplatePriorPlan_Markers asserts the stencil declares exactly the two required top-level markers.
func TestLoomTemplatePriorPlan_Markers(t *testing.T) {
	got, err := stencil.TopLevelMarkers(LoomTemplatePriorPlan)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers() = %v; want nil", err)
	}
	want := []string{"archive_dir", "moved_files"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TopLevelMarkers = %v; want %v", got, want)
	}
}
