// focusdirective_test.go pins burler-focus-directive.md's marker set; the wording of its rubric-over-directive precedence statement is pinned in claims_test.go.

package stencils

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

func TestBurlerFocusDirective_Markers(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(BurlerFocusDirective)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers() = %v; want nil", err)
	}
	if want := []string{"focus_path"}; !reflect.DeepEqual(markers, want) {
		t.Errorf("TopLevelMarkers = %v; want %v", markers, want)
	}
}
