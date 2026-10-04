// patternpath_test.go covers the path-construction surface this package owns: the File constructor.
// Every case here is pure filepath.Join arithmetic — no subprocess is spawned and no fixture tree
// is copied — so this file stays untagged.

package pattern_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/pattern"
)

// TestFile_Free asserts File(worktreeRoot)'s join: the overview sits directly under the root.
func TestFile_Free(t *testing.T) {
	tests := []struct {
		name         string
		worktreeRoot string
	}{
		{"root base", filepath.Join("C:", "hub", "wt")},
		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pattern.File(tt.worktreeRoot)
			want := filepath.Join(tt.worktreeRoot, "PATTERN.md")
			if got != want {
				t.Errorf("File(%q) = %q; want %q", tt.worktreeRoot, got, want)
			}
		})
	}
}
