// rubric_test.go pins which top-level markers a Bouncer or Burler rubric may carry.
// It also pins that every rubric renders through the production render helper.
// It also pins that both review rubrics name their writer's deployed stencil path.

package stencils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rubricMarkerAllowlist is the complete set of top-level stencil markers a rubric may carry:
// only specs_dir, stencils_dir, webster_record, gate_tiers, amendment and review_base.
// internal/shedadapters.ReadRubric fills these markers as the rubric's own template at read time.
// A marker outside this set would sit inside the value the Bouncer and Burler prompts interpolate the rubric as.
// The fill's required-marker check cannot see such a marker, so this test must fail on it.
var rubricMarkerAllowlist = map[string]bool{"specs_dir": true, "stencils_dir": true, "webster_record": true, "gate_tiers": true, "amendment": true, "review_base": true}

// TestLoomRubrics asserts, for each stencil-sourced rubric:
//   - its top-level marker set is a subset of rubricMarkerAllowlist, the same allowed set for every rubric:
//     an asymmetric rule across them is how the next author loses the invariant;
//   - it renders successfully through internal/shedadapters.ReadRubric given a non-empty specs directory:
//     the production render path every Bouncer and Burler rubric read travels;
//     A marker-set check alone cannot see this failure mode:
//     a bare "{{" an author writes in prose has no marker name to inspect,
//     so it becomes a runtime parse-template error only once the rubric is filled;
//   - the amendment marker is carried by every rubric, and the rendered text holds the note;
//   - a review rubric's rendered text names its writer stencil's deployed path as the registry lays it out,
//     so the named path is pinned to stencilstore.Path rather than to a hand-typed string.
//     The comparison is slash-normalised: the rubric text joins with "/", but the rendered prefix uses the OS separator.
func TestLoomRubrics(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		def  []byte
		// writer is the stencil the rendered rubric must name; empty for a rubric that names none.
		writer string
		// namesSpecs is whether the rubric carries the specs_dir marker, so its rendered text holds the specs directory.
		namesSpecs bool
	}{
		{"loom-rubric-discussion-review", LoomRubricDiscussionReview, "loom-template-discussion", true},
		{"loom-rubric-plan-review", LoomRubricPlanReview, "loom-template-plan", true},
		{"loom-rubric-code-review", LoomRubricCodeReview, "", true},
		{"loom-rubric-raddle-review", LoomRubricRaddleReview, "raddle-template-agent", true},
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

			const record = "Webster has begun these batches: batch 1 (done)."
			const tier = "- tier1: the hub's first tier"
			const amendment = "Review only the change since commit abc123."
			const reviewBase = "0123abcd4567ef"
			got, err := shedadapters.ReadRubric(dir, tt.name, "/abs/specs/dir", shedadapters.RubricNotes{WebsterRecord: record, GateTiers: tier, Amendment: amendment, ReviewBase: reviewBase})
			if err != nil {
				t.Fatalf("shedadapters.ReadRubric(%q, %q) = _, %v; want nil error", dir, tt.name, err)
			}
			if carriesRecord := strings.Contains(got, record); carriesRecord != (tt.name == "loom-rubric-plan-review") {
				t.Errorf("rendered %s carries the webster run record = %v; only the Plan-Review rubric carries the marker", tt.name, carriesRecord)
			}
			if carriesTier := strings.Contains(got, tier); carriesTier != (tt.name == "loom-rubric-plan-review") {
				t.Errorf("rendered %s carries the gate tier line = %v; only the Plan-Review rubric carries the marker", tt.name, carriesTier)
			}
			if !strings.Contains(got, amendment) {
				t.Errorf("rendered %s does not carry the amendment note; every rubric carries the marker", tt.name)
			}
			if carriesBase := strings.Contains(got, "git diff "+reviewBase+"..HEAD"); carriesBase != (tt.name == "loom-rubric-code-review") {
				t.Errorf("rendered %s carries the review range over the review base = %v; only the Code-Review rubric carries the marker", tt.name, carriesBase)
			}
			if carriesSpecs := strings.Contains(got, "/abs/specs/dir"); carriesSpecs != tt.namesSpecs {
				t.Errorf("rendered %s carries the specs directory = %v; want %v", tt.name, carriesSpecs, tt.namesSpecs)
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
