// annotations_test.go asserts the exact literal values of this package's cobra-annotation constants,
// so a rename cannot silently decouple a producer command (e.g. reed statusline) from the consumer gate
// (cmd/lyx's skipStencilSeed).

package clihelp

import "testing"

// TestAnnotationLiterals pins the exact string value of each annotation constant.
//
//testtiming:keep pins the literal annotation strings that a cross-package consumer matches on, which no covering test asserts
func TestAnnotationLiterals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"SkipStencilSeedAnnotation", SkipStencilSeedAnnotation, "lyx.skip-stencil-seed"},
		{"AnnotationEnabled", AnnotationEnabled, "true"},
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
