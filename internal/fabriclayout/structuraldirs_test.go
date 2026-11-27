// structuraldirs_test.go covers that no caller can change a structural directory set.

package fabriclayout

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The overwrite is what fails a shared package slice:
// an append to a slice with no spare capacity always allocates.
func TestStructuralDirs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		dirs func() []string
	}{
		{"StructuralCommittedDirs", StructuralCommittedDirs},
		{"StructuralNeverCommittedDirs", StructuralNeverCommittedDirs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.dirs()
			want := slices.Clone(got)
			got[0] = "overwritten"
			_ = append(got, "appended")
			if again := tt.dirs(); !slices.Equal(again, want) {
				t.Errorf("%s() after changing a returned slice = %v; want %v", tt.name, again, want)
			}
		})
	}
}
