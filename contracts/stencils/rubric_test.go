// rubric_test.go pins the two-marker allowlist the two Bouncer stencils' {{.rubric}} interpolation depends on for all three rubrics:
// a rubric may carry the specs_dir and stencils_dir markers and nothing else -- the old no-marker-at-all rule relaxed to the same shape rather than deleted, because a third marker is still invisible to the fill at the value-interpolation site and must still fail loudly.
// It additionally pins that the markers render successfully through the production render helper and that both review rubrics name their writer's deployed stencil path.
// The rubrics' wording is pinned in claims_test.go.

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

// rubricMarkerAllowlist is the complete set of top-level stencil markers a rubric may carry: only specs_dir and stencils_dir, which internal/shedadapters.ReadRubric fills as the rubric's own template at read time.
// A marker outside this set would sit inside the marker VALUE the Bouncer and Burler prompts interpolate the rubric as, invisible to the fill's required-marker check at the template actually being executed, so it must fail here exactly as loudly as the old no-marker-at-all rule made it fail.
var rubricMarkerAllowlist = map[string]bool{"specs_dir": true, "stencils_dir": true}

// assertRubricMarkersWithinAllowlist fails when markers contains any name outside
// rubricMarkerAllowlist.
func assertRubricMarkersWithinAllowlist(t *testing.T, rubricName string, markers []string) {
	t.Helper()
	for _, marker := range markers {
		if !rubricMarkerAllowlist[marker] {
			t.Errorf("%s carries the top-level marker %q, which is outside the marker allowlist %v", rubricName, marker, rubricMarkerAllowlist)
		}
	}
}

// TestLoomRubricDiscussionReview_MarkersWithinAllowlist asserts LoomRubricDiscussionReview's top-level marker set is a subset of rubricMarkerAllowlist, the same allowed set as the other two rubrics' -- an asymmetric rule across the three is how the next author loses the invariant.
func TestLoomRubricDiscussionReview_MarkersWithinAllowlist(t *testing.T) {
	markers, err := stencil.TopLevelMarkers(LoomRubricDiscussionReview)
	if err != nil {
		t.Fatalf("stencil.TopLevelMarkers(LoomRubricDiscussionReview) = _, %v; want nil error", err)
	}
	assertRubricMarkersWithinAllowlist(t, "LoomRubricDiscussionReview", markers)
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

// TestLoomRubrics_NameTheWriterStencil asserts each review rubric, rendered through internal/shedadapters.ReadRubric, names its writer stencil's deployed path as the registry lays it out, so the named path is pinned to stencilstore.Path rather than to a hand-typed string.
// The comparison is slash-normalised because the rubric text joins with "/" while the rendered prefix uses the OS separator.
func TestLoomRubrics_NameTheWriterStencil(t *testing.T) {
	tests := []struct {
		name   string
		def    []byte
		writer string
	}{
		{"loom-rubric-discussion-review", LoomRubricDiscussionReview, "loom-template-discussion"},
		{"loom-rubric-plan-review", LoomRubricPlanReview, "loom-template-plan"},
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

			got, err := shedadapters.ReadRubric(dir, tt.name, "/abs/specs/dir")
			if err != nil {
				t.Fatalf("shedadapters.ReadRubric(%q, %q) = _, %v; want nil error", dir, tt.name, err)
			}
			want := filepath.ToSlash(stencilstore.Path(dir, tt.writer))
			if !strings.Contains(filepath.ToSlash(got), want) {
				t.Errorf("rendered %s does not contain the writer stencil path %q", tt.name, want)
			}
		})
	}
}
