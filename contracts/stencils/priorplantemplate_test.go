// priorplantemplate_test.go pins loom-template-prior-plan.md's marker set and the instructions it gives a respawned Plan-Write session.

package stencils

import (
	"reflect"
	"strings"
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

// TestLoomTemplatePriorPlan_StatesInstructions asserts the stencil carries each instruction as a short, distinctive substring, following plantemplate_test.go's precedent.
func TestLoomTemplatePriorPlan_StatesInstructions(t *testing.T) {
	text := string(LoomTemplatePriorPlan)

	tests := []struct {
		name   string
		phrase string
	}{
		{"names the section", "## Prior plan"},
		{"read before writing", "Read the archived plan before writing anything"},
		{"carry forward by copying", "by copying it into the plan directory rather than re-deriving it"},
		{"re-verify with quarry", "Re-verify every glyph you carry forward with `lyx quarry`"},
		{"never modify the archive", "Never modify the archive directory"},
		{"never copy overview", "Never copy `00-overview.md` from the archive"},
		{"overview unapproved", "`approved: false`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomTemplatePriorPlan does not contain %q", tt.phrase)
			}
		})
	}
}
