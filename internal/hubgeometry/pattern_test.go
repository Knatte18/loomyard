// pattern_test.go covers the _pattern geometry surface: the PatternDirName constant,
// the free PatternDir/PatternFile helpers, and the six Layout accessors that mirror
// their existing _lyx counterparts. Every case here is pure filepath.Join arithmetic —
// no subprocess is spawned and no fixture tree is copied — so this file stays untagged.

package hubgeometry_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newTestLayout builds a Layout by hand for join-arithmetic assertions, mirroring the
// field derivation Resolve performs (RelPath = filepath.Rel(WorktreeRoot, Cwd)) without
// spawning git, since this file is deliberately untagged.
func newTestLayout(hub, worktreeRoot, relPath string) *hubgeometry.Layout {
	return &hubgeometry.Layout{
		Cwd:          filepath.Join(worktreeRoot, relPath),
		WorktreeRoot: worktreeRoot,
		Hub:          hub,
		RelPath:      relPath,
		Prime:        worktreeRoot,
		Repo:         filepath.Base(worktreeRoot),
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
			got := hubgeometry.PatternDir(tt.baseDir)
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
			got := hubgeometry.PatternFile(tt.baseDir)
			want := filepath.Join(tt.baseDir, "_pattern", "PATTERN.md")
			if got != want {
				t.Errorf("PatternFile(%q) = %q; want %q", tt.baseDir, got, want)
			}
		})
	}
}

// TestLayout_PatternAccessors asserts each _pattern Layout accessor's join for both
// RelPath == "." and a nested RelPath of at least two segments.
func TestLayout_PatternAccessors(t *testing.T) {
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
			l := newTestLayout(hub, worktreeRoot, rp.relPath)

			t.Run("WeftPatternDir", func(t *testing.T) {
				got := l.WeftPatternDir()
				want := filepath.Join(l.WeftWorktree(), l.RelPath, "_pattern")
				if got != want {
					t.Errorf("WeftPatternDir() = %q; want %q", got, want)
				}
			})

			t.Run("WeftPatternDirFor", func(t *testing.T) {
				got := l.WeftPatternDirFor(slug)
				want := filepath.Join(l.WeftWorktreePath(slug), l.RelPath, "_pattern")
				if got != want {
					t.Errorf("WeftPatternDirFor(%q) = %q; want %q", slug, got, want)
				}
			})

			t.Run("HostPatternLink", func(t *testing.T) {
				got := l.HostPatternLink(slug)
				want := filepath.Join(l.WorktreePath(slug), l.RelPath, "_pattern")
				if got != want {
					t.Errorf("HostPatternLink(%q) = %q; want %q", slug, got, want)
				}
			})

			t.Run("HostPatternLinkHere", func(t *testing.T) {
				got := l.HostPatternLinkHere()
				want := filepath.Join(l.WorktreeRoot, l.RelPath, "_pattern")
				if got != want {
					t.Errorf("HostPatternLinkHere() = %q; want %q", got, want)
				}
			})

			t.Run("PatternFileHere", func(t *testing.T) {
				got := l.PatternFileHere()
				want := hubgeometry.PatternFile(filepath.Join(l.WorktreeRoot, l.RelPath))
				if got != want {
					t.Errorf("PatternFileHere() = %q; want %q", got, want)
				}
			})
		})
	}
}

// TestPatternFileHere_EqualsPatternFileOfCwd pins the equality PatternFileHere() relies
// on: for any Layout whose RelPath was derived as filepath.Rel(WorktreeRoot, Cwd) — the
// way Resolve derives it — PatternFileHere() equals PatternFile(l.Cwd) exactly, since
// filepath.Join(WorktreeRoot, RelPath) collapses to Cwd itself.
func TestPatternFileHere_EqualsPatternFileOfCwd(t *testing.T) {
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
			l := newTestLayout(filepath.Join("C:", "hub"), worktreeRoot, tt.relPath)

			got := l.PatternFileHere()
			want := hubgeometry.PatternFile(l.Cwd)
			if got != want {
				t.Errorf("PatternFileHere() = %q; want PatternFile(l.Cwd) = %q", got, want)
			}
		})
	}
}
