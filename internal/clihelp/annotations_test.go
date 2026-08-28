// annotations_test.go asserts the exact literal values of this package's cobra-annotation constants,
// so a rename cannot silently decouple a producer command (e.g. reed header) from the consumer gate
// (cmd/lyx's skipStencilSeed).

package clihelp

import "testing"

// TestSkipStencilSeedAnnotation_Literal pins SkipStencilSeedAnnotation's exact string value.
func TestSkipStencilSeedAnnotation_Literal(t *testing.T) {
	if got, want := SkipStencilSeedAnnotation, "lyx.skip-stencil-seed"; got != want {
		t.Errorf("SkipStencilSeedAnnotation = %q; want %q", got, want)
	}
}

// TestAnnotationEnabled_Literal pins AnnotationEnabled's exact string value.
func TestAnnotationEnabled_Literal(t *testing.T) {
	if got, want := AnnotationEnabled, "true"; got != want {
		t.Errorf("AnnotationEnabled = %q; want %q", got, want)
	}
}
