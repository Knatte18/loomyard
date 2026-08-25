// rubric_test.go pins loom-rubric-discussion-review.md's and loom-rubric-plan-review.md's required
// content: the six items manifest/designs/loom.md's two "Discussion-Review rubric" subsections
// require, the eight items its "Plan-Review rubric" subsections require, and the
// marker-value-not-template constraint the two Bouncer stencils' {{.rubric}} interpolation depends
// on for both rubrics.

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

// TestLoomRubricPlanReview_NamesEveryRequiredItem asserts LoomRubricPlanReview's bytes contain a
// distinctive phrase for each of the eight items required: the four "Also flag" items, the three
// "Do not flag" items, and the named support-log exclusion.
// Following TestLoomRubricDiscussionReview_NamesEveryRequiredItem as precedent, each assertion is a
// short, distinctive substring rather than a whole paragraph, so ordinary prose edits do not break
// this test.
func TestLoomRubricPlanReview_NamesEveryRequiredItem(t *testing.T) {
	text := string(LoomRubricPlanReview)

	tests := []struct {
		name   string
		phrase string
	}{
		{"granularity is one card per independently reviewable/testable unit", "independently reviewable/testable unit"},
		{"ImpactSummary carries a real blast-radius conclusion", "blast-radius conclusion"},
		{"Custom is a last resort", "is a last resort"},
		{"fidelity to the decision record at its anchor-relative path", "_lyx/discussion/decision-record.md"},
		{"anything Plan-Validate already checks, through commit-subject-mismatch", "commit-subject-mismatch"},
		{"dependency edges are derived, never authored", "Dependency edges are derived, never authored"},
		{"Rename carries no ImpactSummary because there is no graded blast radius", "no graded blast radius to summarise"},
		{"support-log.md is outside this review entirely", "support-log.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomRubricPlanReview does not contain %q", tt.phrase)
			}
		})
	}
}

// TestLoomRubricPlanReview_CarriesNoStencilMarkers asserts LoomRubricPlanReview's bytes contain no
// "{{." substring: the rubric is interpolated as a marker value into the Bouncer and Burler prompts,
// and a marker inside it would either render literally into the judge prompt or, worse, be silently
// swallowed.
func TestLoomRubricPlanReview_CarriesNoStencilMarkers(t *testing.T) {
	text := string(LoomRubricPlanReview)

	if strings.Contains(text, "{{.") {
		t.Errorf("LoomRubricPlanReview contains a stencil marker (\"{{.\"); want none")
	}
}
