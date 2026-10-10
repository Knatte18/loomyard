// rubric_test.go pins the marker allowlist the two Bouncer stencils' {{.rubric}} interpolation depends on for all three rubrics:
// a rubric may carry the specs_dir, stencils_dir, publish_failure and webster_record markers and nothing else, and only the Webster-Review rubric renders the Publish failure note and only the Plan-Review rubric renders the webster run record -- the old no-marker-at-all rule relaxed to the same shape rather than deleted, because a third marker is still invisible to the fill at the value-interpolation site and must still fail loudly.
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

// rubricMarkerAllowlist is the complete set of top-level stencil markers a rubric may carry: only specs_dir, stencils_dir, publish_failure and webster_record, which internal/shedadapters.ReadRubric fills as the rubric's own template at read time.
// A marker outside this set would sit inside the marker VALUE the Bouncer and Burler prompts interpolate the rubric as, invisible to the fill's required-marker check at the template actually being executed, so it must fail here exactly as loudly as the old no-marker-at-all rule made it fail.
var rubricMarkerAllowlist = map[string]bool{"specs_dir": true, "stencils_dir": true, "publish_failure": true, "webster_record": true}

// TestLoomRubrics asserts, for each of the three stencil-sourced rubrics:
//   - its top-level marker set is a subset of rubricMarkerAllowlist, the same allowed set for all three -- an asymmetric rule across the three is how the next author loses the invariant;
//   - it renders successfully through internal/shedadapters.ReadRubric given a non-empty specs directory, the production render path every Bouncer and Burler rubric read travels.
//     A marker-set check alone cannot see this failure mode: a bare "{{" an author writes in prose has no marker name to inspect and becomes a runtime parse-template error only once the rubric is actually filled;
//   - a review rubric's rendered text names its writer stencil's deployed path as the registry lays it out, so the named path is pinned to stencilstore.Path rather than to a hand-typed string.
//     The comparison is slash-normalised because the rubric text joins with "/" while the rendered prefix uses the OS separator.
func TestLoomRubrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		def  []byte
		// writer is the stencil the rendered rubric must name; empty for a rubric that names none.
		writer string
	}{
		{"loom-rubric-discussion-review", LoomRubricDiscussionReview, "loom-template-discussion"},
		{"loom-rubric-plan-review", LoomRubricPlanReview, "loom-template-plan"},
		{"loom-rubric-webster-review", LoomRubricWebsterReview, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			markers, err := stencil.TopLevelMarkers(tt.def)
			if err != nil {
				t.Fatalf("stencil.TopLevelMarkers(%s) = _, %v; want nil error", tt.name, err)
			}
			for _, marker := range markers {
				if !rubricMarkerAllowlist[marker] {
					t.Errorf("%s carries the top-level marker %q, which is outside the marker allowlist %v", tt.name, marker, rubricMarkerAllowlist)
				}
			}

			dir := t.TempDir()
			path := stencilstore.Path(dir, tt.name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("os.MkdirAll(%q) = %v; want nil error", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, tt.def, 0o644); err != nil {
				t.Fatalf("os.WriteFile(%q) = %v; want nil error", path, err)
			}

			const note = "Publish failed on landing config's `publish_verify` command."
			const record = "Webster has begun these batches: batch 1 (done)."
			got, err := shedadapters.ReadRubric(dir, tt.name, "/abs/specs/dir", note, record)
			if err != nil {
				t.Fatalf("shedadapters.ReadRubric(%q, %q) = _, %v; want nil error", dir, tt.name, err)
			}
			if carriesNote := strings.Contains(got, note); carriesNote != (tt.name == "loom-rubric-webster-review") {
				t.Errorf("rendered %s carries the Publish failure note = %v; only the Webster-Review rubric carries the marker", tt.name, carriesNote)
			}
			if carriesRecord := strings.Contains(got, record); carriesRecord != (tt.name == "loom-rubric-plan-review") {
				t.Errorf("rendered %s carries the webster run record = %v; only the Plan-Review rubric carries the marker", tt.name, carriesRecord)
			}
			if tt.writer == "" {
				return
			}
			want := filepath.ToSlash(stencilstore.Path(dir, tt.writer))
			if !strings.Contains(filepath.ToSlash(got), want) {
				t.Errorf("rendered %s does not contain the writer stencil path %q", tt.name, want)
			}
		})
	}
}
