// rubric.go holds ReadRubric, which reads a stencil-sourced rubric, strips its banner and fills its markers.

package shedadapters

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// noNote is what each optional rubric marker renders when its note is blank.
const noNote = "none"

// redundancyReportMarker is the marker the Code-Review rubric carries for the gate's redundancy report.
const redundancyReportMarker = "redundancy_report"

// RubricNotes are the optional notes a rubric's markers are filled from.
// A blank note fills its marker as `none`, and a rubric with no such marker ignores it.
type RubricNotes struct {
	// WebsterRecord fills webster_record with the rendered webster run record; only the Plan-Review rubric carries it.
	WebsterRecord string
	// GateTiers fills gate_tiers with the rendered tier and gate-list lines; only the Plan-Review rubric carries it.
	GateTiers string
	// Amendment fills amendment with the note scoping an amendment generation's review to changes since its base.
	Amendment string
	// ReviewBase fills review_base with the commit the Code-Review rubric's review range starts from.
	ReviewBase string
	// RedundancyReport returns the note redundancy_report is filled from, the gate's redundancy report as review reads it.
	// It is called only for a rubric carrying the marker, since reading the report matches it against a commit's tree.
	// Nil and a blank answer render `none`.
	RedundancyReport func() string
}

// ReadRubric reads the rubric stencil name from stencilsDir and strips its stamp banner.
// It fills the markers with specsDir, stencilsDir and notes, and returns the filled text.
//
// The banner is stripped before the fill.
// The filled text is interpolated into the Bouncer templates as a marker value,
// and a banner inside a marker value is not stripped, so it would reach the judge prompt.
//
// specs_dir and stencils_dir are required, so a rubric carrying either marker errors on an empty value.
// A blank path would be a dead reference, so the fill errors instead.
// The optional markers always get a value, since a blank note renders as none.
// A rubric with no marker renders unchanged.
//
// Only stencil-sourced rubrics go through here.
// A literal rubric string, author prose set through a config key, must not:
// a bare {{ in it fails to parse, and its text would take on specs_dir semantics it never had.
func ReadRubric(stencilsDir, name, specsDir string, notes RubricNotes) (string, error) {
	raw, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", fmt.Errorf("shedadapters: read rubric %q: %w", name, err)
	}
	stripped := stencil.StripLeadingComment(string(raw))
	redundancyReport, err := redundancyReportNote([]byte(stripped), notes.RedundancyReport)
	if err != nil {
		return "", fmt.Errorf("shedadapters: read rubric %q: %w", name, err)
	}
	filled, err := stencil.Fill([]byte(stripped), map[string]string{
		"specs_dir":            specsDir,
		"stencils_dir":         stencilsDir,
		"webster_record":       noteOrNone(notes.WebsterRecord),
		"gate_tiers":           noteOrNone(notes.GateTiers),
		"amendment":            noteOrNone(notes.Amendment),
		"review_base":          noteOrNone(notes.ReviewBase),
		redundancyReportMarker: redundancyReport,
	})
	if err != nil {
		return "", fmt.Errorf("shedadapters: read rubric %q: %w", name, err)
	}
	return string(filled), nil
}

// redundancyReportNote returns the redundancy_report value for rubric.
// read is called only when rubric carries the marker.
func redundancyReportNote(rubric []byte, read func() string) (string, error) {
	if read == nil {
		return noNote, nil
	}
	markers, err := stencil.TopLevelMarkers(rubric)
	if err != nil {
		return "", err
	}
	if !slices.Contains(markers, redundancyReportMarker) {
		return noNote, nil
	}
	return noteOrNone(read()), nil
}

// noteOrNone returns note, or `none` when it is blank.
func noteOrNone(note string) string {
	if strings.TrimSpace(note) == "" {
		return noNote
	}
	return note
}
