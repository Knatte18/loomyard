// rubric_test.go pins loom-rubric-discussion-review.md's required content: the six items
// manifest/designs/loom.md's two "Discussion-Review rubric" subsections require, and the
// marker-value-not-template constraint the two Bouncer stencils' {{.rubric}} interpolation depends
// on.

package stencils

import (
	"strings"
	"testing"
)

// TestLoomRubricDiscussionReview_NamesEveryRequiredItem asserts LoomRubricDiscussionReview's bytes
// contain a distinctive phrase for each of the six items manifest/designs/loom.md's two
// "Discussion-Review rubric" subsections require: three do-not-flag items and three also-flag items.
// Following internal/burlerengine/template_test.go's TestTemplate_StatesRoundDiscipline as precedent,
// each assertion is a short, distinctive substring rather than a whole paragraph, so ordinary prose
// edits do not break this test.
func TestLoomRubricDiscussionReview_NamesEveryRequiredItem(t *testing.T) {
	text := string(LoomRubricDiscussionReview)

	tests := []struct {
		name   string
		phrase string
	}{
		{"missing Notes for the plan writer is not a deficiency", "Notes for the plan writer"},
		{"missing rejected alternatives is by design", "Rejected alternatives"},
		{"incomplete cross-reference enumeration belongs to the compiler and Plan-Sweep", "Plan-Sweep"},
		{"relocation and exclusion findings are legitimate", "Relocation and exclusion findings"},
		{"completeness-before-leanness test", "completeness-before-leanness test"},
		{"writer/reviewer symmetry note", "writer/reviewer symmetry note"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomRubricDiscussionReview does not contain %q", tt.phrase)
			}
		})
	}
}

// TestLoomRubricDiscussionReview_CarriesNoStencilMarkers asserts LoomRubricDiscussionReview's bytes
// contain no "{{." substring: the rubric is interpolated as a marker value into the Bouncer and
// Burler prompts, and a marker inside it would either render literally into the judge prompt or,
// worse, be silently swallowed.
func TestLoomRubricDiscussionReview_CarriesNoStencilMarkers(t *testing.T) {
	text := string(LoomRubricDiscussionReview)

	if strings.Contains(text, "{{.") {
		t.Errorf("LoomRubricDiscussionReview contains a stencil marker (\"{{.\"); want none")
	}
}
