// focusdirective_test.go pins burler-focus-directive.md's marker set and the four rules of its
// rubric-over-directive precedence statement, as substring pins in rubric_test.go's shape.

package stencils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

func TestBurlerFocusDirective_MarkersAndPrecedenceRules(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(BurlerFocusDirective)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers() = %v; want nil", err)
	}
	if want := []string{"focus_path"}; !reflect.DeepEqual(markers, want) {
		t.Errorf("TopLevelMarkers = %v; want %v", markers, want)
	}

	body := string(BurlerFocusDirective)
	for _, phrase := range []string{
		"The rubric binds over the focus directive",
		"steers attention and order, not verdicts",
		"a valid verdict",
		"Severity comes from the rubric's mapping",
		"advisory and yields to evidence",
		"`exclude_lenses` keeps its mechanical meaning",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("burler-focus-directive.md does not contain %q", phrase)
		}
	}
}
