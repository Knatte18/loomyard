// pattern_test.go covers the _pattern geometry surface: the PatternDirName constant,
// the free PatternDir/PatternFile helpers, and the six Location accessors that mirror
// their existing _lyx counterparts. Every case here is pure filepath.Join arithmetic —
// no subprocess is spawned and no fixture tree is copied — so this file stays untagged.

package lyxcwd_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

// TestPatternDir_Free asserts PatternDir(baseDir)'s join.
func TestPatternDir_Free(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
	}{
		{"root base", filepath.Join("C:", "hub", "wt")},
		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lyxcwd.PatternDir(tt.baseDir)
			want := filepath.Join(tt.baseDir, "_pattern")
			if got != want {
				t.Errorf("PatternDir(%q) = %q; want %q", tt.baseDir, got, want)
			}
		})
	}
}

// TestPatternFile_Free asserts PatternFile(baseDir)'s join.
func TestPatternFile_Free(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
	}{
		{"root base", filepath.Join("C:", "hub", "wt")},
		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lyxcwd.PatternFile(tt.baseDir)
			want := filepath.Join(tt.baseDir, "_pattern", "PATTERN.md")
			if got != want {
				t.Errorf("PatternFile(%q) = %q; want %q", tt.baseDir, got, want)
			}
		})
	}
}

// TestLocation_PatternAccessors asserts each _pattern Location accessor's join for both
// AnchorRel == "." and a nested AnchorRel of at least two segments.
func TestLocation_PatternAccessors(t *testing.T) {
	hub := filepath.Join("C:", "hub")
	worktreeRoot := filepath.Join(hub, "wt")
	slug := "test-slug"

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

			t.Run("WeftPatternDir", func(t *testing.T) {
				got := l.WeftPatternDir()
				want := filepath.Join(l.WeftWorktree(), l.AnchorRel, "_pattern")
				if got != want {
					t.Errorf("WeftPatternDir() = %q; want %q", got, want)
				}
			})

			t.Run("WeftPatternDirFor", func(t *testing.T) {
				got := l.WeftPatternDirFor(slug)
				want := filepath.Join(l.WeftWorktreePath(slug), l.AnchorRel, "_pattern")
				if got != want {
					t.Errorf("WeftPatternDirFor(%q) = %q; want %q", slug, got, want)
				}
			})

			t.Run("HostPatternLink", func(t *testing.T) {
				got := l.HostPatternLink(slug)
				want := filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel, "_pattern")
				if got != want {
					t.Errorf("HostPatternLink(%q) = %q; want %q", slug, got, want)
				}
			})

			t.Run("HostPatternLinkHere", func(t *testing.T) {
				got := l.HostPatternLinkHere()
				want := filepath.Join(l.WorktreePath(), l.AnchorRel, "_pattern")
				if got != want {
					t.Errorf("HostPatternLinkHere() = %q; want %q", got, want)
				}
			})

			t.Run("PatternFileHere", func(t *testing.T) {
				got := l.PatternFileHere()
				want := lyxcwd.PatternFile(filepath.Join(l.WorktreePath(), l.AnchorRel))
				if got != want {
					t.Errorf("PatternFileHere() = %q; want %q", got, want)
				}
			})
		})
	}
}

// TestPatternFileHere_EqualsPatternFileOfAnchorPath pins the equality PatternFileHere()
// relies on: for any Location, PatternFileHere() equals PatternFile(l.AnchorPath())
// exactly, since filepath.Join(WorktreePath(), AnchorRel) collapses to AnchorPath() itself.
func TestPatternFileHere_EqualsPatternFileOfAnchorPath(t *testing.T) {
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

			got := l.PatternFileHere()
			want := lyxcwd.PatternFile(l.AnchorPath())
			if got != want {
				t.Errorf("PatternFileHere() = %q; want PatternFile(l.AnchorPath()) = %q", got, want)
			}
		})
	}
}
