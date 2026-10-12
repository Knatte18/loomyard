// Every test here is untagged Tier 1: it reads a seeded temporary stencils directory and spawns nothing.

package twilldirective

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// filledValues returns a Values with every field set.
func filledValues() Values {
	return Values{
		ReviewedFiles:    "`/abs/one.md` and `/abs/two.md`",
		SpawnPoint:       "SPAWN-POINT-SENTENCE",
		RejectionRule:    "REJECTION-RULE-SENTENCE",
		SeverityRule:     "SEVERITY-RULE-PHRASE",
		ReadyMarker:      "/abs/.ready/marker",
		RubricPath:       "/abs/rubric.md",
		Lenses:           []Lens{{Name: "alpha", Text: "Look at A.\nThen at B.\n"}, {Name: "beta", Text: "Look at C."}},
		FormatStep:       "STEP-FORMAT",
		SelfCheckStep:    "STEP-CHECK",
		SelfCheckCommand: "lyx check --now",
		DispositionPlace: "the PLACE section",
	}
}

// TestDirective_RendersEveryValueVerbatim covers the seeded stencil: each value lands, a lens's inner newlines collapse to spaces, and the text has no marker, leading banner or trailing newline.
func TestDirective_RendersEveryValueVerbatim(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := Directive(dir, filledValues())
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	for _, want := range []string{
		"read `/abs/one.md` and `/abs/two.md` and review them",
		"SPAWN-POINT-SENTENCE\n",
		"REJECTION-RULE-SENTENCE",
		"each with SEVERITY-RULE-PHRASE, the section or decision it concerns",
		"/abs/.ready/marker",
		"/abs/rubric.md",
		"- alpha: Look at A. Then at B.\n- beta: Look at C.",
		"STEP-FORMAT",
		"STEP-CHECK",
		"lyx check --now",
		"the PLACE section",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Directive = %q; want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("Directive = %q; want no marker", got)
	}
	if strings.HasPrefix(got, "<!--") {
		t.Errorf("Directive = %q; want no leading comment", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("Directive = %q; want no trailing newline", got)
	}
}

// TestDirective_MissingValueNamesTheMarker covers a host value or the lens list left empty: Directive errors and the error names that value's marker.
func TestDirective_MissingValueNamesTheMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		empty  func(v *Values)
		marker string
	}{
		{"ReviewedFiles", func(v *Values) { v.ReviewedFiles = "" }, "reviewed_files"},
		{"SpawnPoint", func(v *Values) { v.SpawnPoint = "" }, "spawn_point"},
		{"RejectionRule", func(v *Values) { v.RejectionRule = "" }, "rejection_rule"},
		{"SeverityRule", func(v *Values) { v.SeverityRule = "" }, "severity_rule"},
		{"Lenses", func(v *Values) { v.Lenses = nil }, "lens_list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			values := filledValues()
			tt.empty(&values)

			got, err := Directive(dir, values)
			if err == nil {
				t.Fatalf("Directive = %q, nil; want an error", got)
			}
			if !strings.Contains(err.Error(), tt.marker) {
				t.Errorf("error = %q; want it to name %q", err, tt.marker)
			}
		})
	}
}

// TestWriteScope_RendersTheMarker covers the seeded stencil: the marker lands, with no marker left over, banner or trailing newline.
func TestWriteScope_RendersTheMarker(t *testing.T) {
	t.Parallel()
	dir := stencilkit.Seed(t)

	got, err := WriteScope(dir, "/abs/.ready/marker")
	if err != nil {
		t.Fatalf("WriteScope = _, %v; want nil error", err)
	}
	if !strings.Contains(got, "`/abs/.ready/marker`") {
		t.Errorf("WriteScope = %q; want it to contain the marker path", got)
	}
	if strings.Contains(got, "{{") || strings.HasPrefix(got, "<!--") || strings.HasSuffix(got, "\n") {
		t.Errorf("WriteScope = %q; want no marker, leading comment or trailing newline", got)
	}
}

// TestRenderers_MissingStencilNamesIt covers a stencil removed from the directory: each renderer's error names it.
func TestRenderers_MissingStencilNamesIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stencil string
		render  func(dir string) (string, error)
	}{
		{"Directive", directiveStencil, func(dir string) (string, error) { return Directive(dir, Values{}) }},
		{"WriteScope", writeScopeStencil, func(dir string) (string, error) { return WriteScope(dir, "/abs/marker") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := stencilkit.Seed(t)
			stencilkit.Remove(t, dir, tt.stencil)

			got, err := tt.render(dir)
			if err == nil {
				t.Fatalf("%s = %q, nil; want an error", tt.name, got)
			}
			if !strings.Contains(err.Error(), tt.stencil) {
				t.Errorf("error = %q; want it to name %q", err, tt.stencil)
			}
		})
	}
}
