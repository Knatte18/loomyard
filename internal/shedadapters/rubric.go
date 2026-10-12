// rubric.go implements ReadRubric, the shared read-strip-fill helper every stencil-sourced rubric
// site routes through.

package shedadapters

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// noNote is what the optional publish_failure, webster_record and merge_review markers render as when their note is blank.
const noNote = "none"

// RubricNotes are the optional notes a rubric's markers are filled from.
// A blank note fills its marker as `none`, and a rubric with no such marker ignores it.
type RubricNotes struct {
	// PublishFailure fills the publish_failure marker, the rendered Publish failure note only the Webster-Review rubric carries.
	PublishFailure string
	// WebsterRecord fills the webster_record marker, the rendered webster run record only the Plan-Review rubric carries.
	WebsterRecord string
	// MergeReview fills the merge_review marker, the rendered merge record only the Merge-Review rubric carries.
	MergeReview string
}

// ReadRubric reads the rubric stencil name from stencilsDir, strips its stamp banner, fills it as its own template with specsDir, stencilsDir and notes, and returns the filled result.
//
// The strip is about value semantics, not about protecting Fill. stencil.Fill already strips a
// leading banner from the template it parses, but never from a marker value -- and a rubric's
// destination is a marker value, interpolated into the Bouncer templates' {{.rubric}}. Unstripped
// bytes would inject the "<!-- lyx-stencil: sha256=... -->" line into the middle of the judge
// prompt.
//
// The strip happens before the fill, not after: Fill strips its own template internally, so
// stripping afterwards would be too late to keep the banner out of the returned value.
//
// specs_dir and stencils_dir are required markers here -- plain Fill, never FillOptional.
// A rubric that carries {{.specs_dir}} or {{.stencils_dir}} and is handed an empty value must error rather than render a blank path, because a blank path is precisely the dead reference this task removes.
// A rubric that carries no marker at all renders unchanged, so this helper is safe to route every stencil-sourced rubric through from the moment it exists.
//
// This helper is for stencil-sourced rubrics only. A literal rubric string -- author-written prose
// reaching a producer through a config key rather than through the stencil store -- never goes
// through here, because running author prose through Fill turns any bare {{ in it into a
// parse-template error and imposes specs-dir semantics on text that never had them.
func ReadRubric(stencilsDir, name, specsDir string, notes RubricNotes) (string, error) {
	raw, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("shedadapters: read rubric %q: %w", name, err)
	}
	stripped := stencil.StripLeadingComment(string(raw))
	filled, err := stencil.Fill([]byte(stripped), map[string]string{
		"specs_dir":       specsDir,
		"stencils_dir":    stencilsDir,
		"publish_failure": noteOrNone(notes.PublishFailure),
		"webster_record":  noteOrNone(notes.WebsterRecord),
		"merge_review":    noteOrNone(notes.MergeReview),
	})
	if err != nil {
		return "", fmt.Errorf("shedadapters: read rubric %q: %w", name, err)
	}
	return string(filled), nil
}

// noteOrNone returns note, or `none` when it is blank.
func noteOrNone(note string) string {
	if strings.TrimSpace(note) == "" {
		return noNote
	}
	return note
}
