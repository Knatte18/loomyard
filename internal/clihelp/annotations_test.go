// annotations_test.go asserts the exact literal values of this package's cobra-annotation constants,
// so a rename cannot silently decouple a producer command from its consumer, the skipStencilSeed gate in cmd/lym.

package clihelp

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestAnnotationLiterals pins the exact string value of each annotation constant.
//
//testtiming:keep pins the literal annotation strings that a cross-package consumer matches on, which no covering test asserts
func TestAnnotationLiterals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"SkipStencilSeedAnnotation", SkipStencilSeedAnnotation, "lym.skip-stencil-seed"},
		{"AnnotationEnabled", AnnotationEnabled, "true"},
		{"AudienceAnnotation", AudienceAnnotation, "lym.audience"},
		{"AudienceOperator", AudienceOperator, "operator"},
		{"AudienceRole", AudienceRole, "role"},
		{"AudienceInternal", AudienceInternal, "internal"},
		{"IndexNoteAnnotation", IndexNoteAnnotation, "lym.index-note"},
		{"IndexNoteAudienceAnnotation", IndexNoteAudienceAnnotation, "lym.index-note-audience"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
