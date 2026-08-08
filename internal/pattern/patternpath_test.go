// patternpath_test.go covers the path-construction surface this package owns: the File/FileHere
// constructors and the PathspecFile/PathspecDir pathspec constants.
// Every case here is pure filepath.Join arithmetic — no subprocess is spawned and no fixture tree
// is copied — so this file stays untagged.

package pattern_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
)

// newTestLocation builds a Location by hand for join-arithmetic assertions, mirroring the
// field derivation Resolve performs, without spawning git, since this file is
// deliberately untagged.
func newTestLocation(hub, worktreeRoot, relPath string) *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      hub,
		WorktreeName: filepath.Base(worktreeRoot),
		AnchorRel:    relPath,
	}
}

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

// TestLocation_PatternAccessors asserts FileHere(l)'s join for both AnchorRel == "."
// and a nested AnchorRel of at least two segments.
// The four fabric-specific pattern/junction-link sub-tests that used to
// live here were dropped along with those four lyxcwd.Location accessors, which were deleted in
// an earlier batch;
// their coverage is re-asserted there against the generic junction path.
func TestLocation_PatternAccessors(t *testing.T) {
	hub := filepath.Join("C:", "hub")
	worktreeRoot := filepath.Join(hub, "wt")

	relPaths := []struct {
		name    string
		relPath string
	}{
		{"at root", "."},
		{"nested two segments", filepath.Join("services", "api")},
	}

	for _, rp := range relPaths {
		t.Run(rp.name, func(t *testing.T) {
			l := newTestLocation(hub, worktreeRoot, rp.relPath)

			t.Run("FileHere", func(t *testing.T) {
				got := pattern.FileHere(l)
				want := pattern.File(filepath.Join(l.WorktreePath(), l.AnchorRel))
				if got != want {
					t.Errorf("FileHere(l) = %q; want %q", got, want)
				}
			})
		})
	}
}

// TestFileHere_EqualsFileOfAnchorPath pins the equality FileHere() relies on: for any Location,
// FileHere(l) equals File(l.AnchorPath()) exactly, since filepath.Join(WorktreePath(), AnchorRel)
// collapses to AnchorPath() itself.
func TestFileHere_EqualsFileOfAnchorPath(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
	}{
		{"at root", "."},
		{"nested two segments", filepath.Join("services", "api")},
	}

	worktreeRoot := filepath.Join("C:", "hub", "wt")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestLocation(filepath.Join("C:", "hub"), worktreeRoot, tt.relPath)

			got := pattern.FileHere(l)
			want := pattern.File(l.AnchorPath())
			if got != want {
				t.Errorf("FileHere(l) = %q; want File(l.AnchorPath()) = %q", got, want)
			}
		})
	}
}
