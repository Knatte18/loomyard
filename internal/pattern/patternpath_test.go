// patternpath_test.go covers the path-construction surface this package owns: the File
// constructor and the PathspecFile/PathspecDir pathspec constants.
// Every case here is pure filepath.Join arithmetic — no subprocess is spawned and no fixture tree
// is copied — so this file stays untagged.

package pattern_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// TestFile_Free asserts File(baseDir)'s join.
func TestFile_Free(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
	}{
		{"root base", filepath.Join("C:", "hub", "wt")},
		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pattern.File(tt.baseDir)
			want := filepath.Join(tt.baseDir, lyxdirs.LyxDirName, "PATTERN.md")
			if got != want {
				t.Errorf("File(%q) = %q; want %q", tt.baseDir, got, want)
			}
		})
	}
}

// TestPathspec_ExactStrings pins PathspecFile and PathspecDir's exact
// values: these are git-pathspec spellings, compared by fixed-string
// equality by internal/fabricengine, so a silent drift here would break
// that consumer without a compile error.
func TestPathspec_ExactStrings(t *testing.T) {
	if pattern.PathspecFile != "_lyx/PATTERN.md" {
		t.Errorf("pattern.PathspecFile = %q; want %q", pattern.PathspecFile, "_lyx/PATTERN.md")
	}
	if pattern.PathspecDir != "_lyx/pattern" {
		t.Errorf("pattern.PathspecDir = %q; want %q", pattern.PathspecDir, "_lyx/pattern")
	}
}

// TestPathspec_ForwardSlashedOnEveryPlatform pins that PathspecFile and
// PathspecDir are built by string concatenation, never filepath.Join, so
// they stay forward-slashed on every platform — a git pathspec is never
// backslash-separated, even on Windows.
func TestPathspec_ForwardSlashedOnEveryPlatform(t *testing.T) {
	if strings.ContainsRune(pattern.PathspecFile, filepath.Separator) && filepath.Separator != '/' {
		t.Errorf("pattern.PathspecFile = %q contains filepath.Separator %q", pattern.PathspecFile, filepath.Separator)
	}
	if strings.ContainsRune(pattern.PathspecDir, filepath.Separator) && filepath.Separator != '/' {
		t.Errorf("pattern.PathspecDir = %q contains filepath.Separator %q", pattern.PathspecDir, filepath.Separator)
	}
}
