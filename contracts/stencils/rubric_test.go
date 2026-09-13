// rubric_test.go pins loom-rubric-discussion-review.md's, loom-rubric-plan-review.md's, and
// loom-rubric-webster-review.md's required content: the six items manifest/designs/loom.md's two
// "Discussion-Review rubric" subsections require, the eight items its "Plan-Review rubric"
// subsections require, the nine items loom-rubric-webster-review.md's own sections require, and the
// one-marker allowlist the two Bouncer stencils' {{.rubric}} interpolation depends on for all three
// rubrics: a rubric may carry the specs_dir marker and nothing else -- the old no-marker-at-all rule
// relaxed to the same shape rather than deleted, because a second marker is still invisible to the
// fill at the value-interpolation site and must still fail loudly. It additionally pins that
// specs_dir renders successfully through the production render helper, and that every stencil
// carrying a normative citation actually declares the literal {{.specs_dir}} marker.
// It also pins the one property that matters across all four friction directive stencils: each
// states that writing the note is optional and that an absent note is normal.

package stencils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// rubricMarkerAllowlist is the complete set of top-level stencil markers a rubric may carry: only
// specs_dir, which internal/shedadapters.ReadRubric fills as the rubric's own single-marker template
// at read time. A marker outside this set would sit inside the marker VALUE the Bouncer and Burler
// prompts interpolate the rubric as, invisible to the fill's required-marker check at the template
// actually being executed, so it must fail here exactly as loudly as the old no-marker-at-all rule
// made it fail.
var rubricMarkerAllowlist = map[string]bool{"specs_dir": true}

// assertRubricMarkersWithinAllowlist fails when markers contains any name outside
// rubricMarkerAllowlist.
func assertRubricMarkersWithinAllowlist(t *testing.T, rubricName string, markers []string) {
	t.Helper()
	for _, marker := range markers {
		if !rubricMarkerAllowlist[marker] {
			t.Errorf("%s carries the top-level marker %q, which is outside the one-marker allowlist %v", rubricName, marker, rubricMarkerAllowlist)
		}
	}
}

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

// TestLoomRubricDiscussionReview_MarkersWithinAllowlist asserts LoomRubricDiscussionReview's
// top-level marker set is a subset of rubricMarkerAllowlist. This rubric gains no marker in this
// task, but its allowed set is the same as the other two rubrics' -- an asymmetric rule across the
// three is how the next author loses the invariant.
func TestLoomRubricDiscussionReview_MarkersWithinAllowlist(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(LoomRubricDiscussionReview)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers(LoomRubricDiscussionReview) = _, %v; want nil error", err)
	}
	assertRubricMarkersWithinAllowlist(t, "LoomRubricDiscussionReview", markers)
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

// TestLoomRubricPlanReview_MarkersWithinAllowlist asserts LoomRubricPlanReview's top-level marker
// set is a subset of rubricMarkerAllowlist: this rubric now carries specs_dir, and a second marker
// would still be invisible at the value-interpolation site, so it must still fail loudly.
func TestLoomRubricPlanReview_MarkersWithinAllowlist(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(LoomRubricPlanReview)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers(LoomRubricPlanReview) = _, %v; want nil error", err)
	}
	assertRubricMarkersWithinAllowlist(t, "LoomRubricPlanReview", markers)
}

// TestLoomRubricWebsterReview_NamesEveryRequiredItem asserts LoomRubricWebsterReview's bytes contain
// a distinctive phrase for each of the nine items required: the diff-review base statement, the two
// review-range derivation steps, the four "Do not flag" items, and the two "Also flag" items.
// Following TestLoomRubricDiscussionReview_NamesEveryRequiredItem as precedent, each assertion is a
// short, distinctive substring rather than a whole paragraph, so ordinary prose edits do not break
// this test.
func TestLoomRubricWebsterReview_NamesEveryRequiredItem(t *testing.T) {
	text := string(LoomRubricWebsterReview)

	tests := []struct {
		name   string
		phrase string
	}{
		{"ordinary diff review is the base", "Ordinary diff review is the base"},
		{"the review range is derived via git merge-base", "git merge-base"},
		{"an undeterminable review range raises a BLOCKING finding", "could not be determined"},
		{"anything Plan-Validate or Plan-Revalidate already checks", "Plan-Revalidate"},
		{"the plan is the measuring stick and never the subject", "measuring stick and never the subject"},
		{"a missing ImpactSummary belongs to Plan-Review", "Both belong to "},
		{"this segment's own round artifacts are never the subject", ".lyx/loom/reviews/webster/"},
		{"comment-convention compliance checks the target repository's own conventions", "target repository's own conventions"},
		{"per-card mechanical check names assert-no-callers for a Delete card", "assert-no-callers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(text, tt.phrase) {
				t.Errorf("LoomRubricWebsterReview does not contain %q", tt.phrase)
			}
		})
	}
}

// TestLoomRubricWebsterReview_MarkersWithinAllowlist asserts LoomRubricWebsterReview's top-level
// marker set is a subset of rubricMarkerAllowlist: this rubric now carries specs_dir, and a second
// marker would still be invisible at the value-interpolation site, so it must still fail loudly.
func TestLoomRubricWebsterReview_MarkersWithinAllowlist(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(LoomRubricWebsterReview)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers(LoomRubricWebsterReview) = _, %v; want nil error", err)
	}
	assertRubricMarkersWithinAllowlist(t, "LoomRubricWebsterReview", markers)
}

// TestLoomRubrics_ParseUnderTheRenderHelper asserts each of the three stencil-sourced rubrics
// renders successfully through internal/shedadapters.ReadRubric given a non-empty specs directory --
// the production render path every Bouncer and Burler rubric read actually travels. A marker-set
// check alone cannot see this failure mode: a bare "{{" an author writes in prose has no marker name
// to inspect and becomes a runtime parse-template error only once the rubric is actually filled.
func TestLoomRubrics_ParseUnderTheRenderHelper(t *testing.T) {
	tests := []struct {
		name string
		def  []byte
	}{
		{"loom-rubric-discussion-review", LoomRubricDiscussionReview},
		{"loom-rubric-plan-review", LoomRubricPlanReview},
		{"loom-rubric-webster-review", LoomRubricWebsterReview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := stencilstore.Path(dir, tt.name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("os.MkdirAll(%q) = %v; want nil error", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, tt.def, 0o644); err != nil {
				t.Fatalf("os.WriteFile(%q) = %v; want nil error", path, err)
			}

			if _, err := shedadapters.ReadRubric(dir, tt.name, "/abs/specs/dir"); err != nil {
				t.Errorf("shedadapters.ReadRubric(%q, %q) = _, %v; want nil error", dir, tt.name, err)
			}
		})
	}
}

// TestStencils_SpecsDirMarkerIsPresent asserts each of the four stencils carrying a normative
// citation this task rewrote -- the plan template, the two rubrics, and the implementer body --
// contains the literal {{.specs_dir}} marker. Without this a future edit could quietly revert a
// citation to a bare path, and only the bare-citation enforcement scan would notice, and only if the
// reverted path happened to match that scan's prefix rule.
func TestStencils_SpecsDirMarkerIsPresent(t *testing.T) {
	tests := []struct {
		name string
		def  []byte
	}{
		{"loom-template-plan", LoomTemplatePlan},
		{"loom-rubric-plan-review", LoomRubricPlanReview},
		{"loom-rubric-webster-review", LoomRubricWebsterReview},
		{"webster-body-implementer", WebsterBodyImplementer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(string(tt.def), "{{.specs_dir}}") {
				t.Errorf("%s does not contain the literal {{.specs_dir}} marker", tt.name)
			}
		})
	}
}

// TestFrictionDirectives_StateOptionalAndAbsenceIsNormal asserts each of the four friction directive
// stencils contains a short, distinctive substring for the one property that matters across all of
// them: that writing the note is optional, and that an absent note is normal.
// Following TestLoomRubricDiscussionReview_NamesEveryRequiredItem as precedent, each assertion is a
// short, distinctive substring rather than a whole paragraph, so ordinary prose edits do not break
// this test.
func TestFrictionDirectives_StateOptionalAndAbsenceIsNormal(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"FrictionDirectiveImplementer", string(FrictionDirectiveImplementer)},
		{"FrictionDirectiveReviewFix", string(FrictionDirectiveReviewFix)},
		{"FrictionDirectiveOrchestrator", string(FrictionDirectiveOrchestrator)},
		{"FrictionDirectiveInterview", string(FrictionDirectiveInterview)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.text, "**optional**") {
				t.Errorf("%s does not contain %q", tt.name, "**optional**")
			}
			if !strings.Contains(tt.text, "normal outcome and never an error") {
				t.Errorf("%s does not contain %q", tt.name, "normal outcome and never an error")
			}
		})
	}
}
