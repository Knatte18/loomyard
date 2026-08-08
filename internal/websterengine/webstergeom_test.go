// webstergeom_test.go — pure path-math unit tests for the webster geometry accessors
// (Dir/ReportsDir/PromptsDir/ScratchDir).
// These tests do not require a git repository and run under standard unit test verification.
// They pin the scratch-dir split: Dir and ReportsDir (durable, fabric-synced) stay under
// lyxdirs.LyxDirName, while ScratchDir and PromptsDir (never-tracked) resolve under
// lyxdirs.DotLyxDirName at the same mirrored subpath, for both an unanchored and a
// subpath-anchored *lyxcwd.Location.

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestWebsterGeometryHelpers pins every webster path constructor for an unanchored Location
// (AnchorRel == ".").
func TestWebsterGeometryHelpers(t *testing.T) {
	t.Parallel()

	baseDir := "/home/user/project"
	l := &lyxcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "."}

	t.Run("Dir", func(t *testing.T) {
		t.Parallel()

		got := Dir(l)
		want := filepath.Join(baseDir, lyxdirs.LyxDirName, "webster")

		if got != want {
			t.Errorf("Dir(l) = %q; want %q", got, want)
		}
	})

	t.Run("ReportsDir", func(t *testing.T) {
		t.Parallel()

		got := ReportsDir(l)
		want := filepath.Join(baseDir, lyxdirs.LyxDirName, "webster", "reports")

		if got != want {
			t.Errorf("ReportsDir(l) = %q; want %q", got, want)
		}
	})

	t.Run("ScratchDir", func(t *testing.T) {
		t.Parallel()

		got := ScratchDir(l)
		want := filepath.Join(baseDir, lyxdirs.DotLyxDirName, "webster")

		if got != want {
			t.Errorf("ScratchDir(l) = %q; want %q", got, want)
		}
	})

	t.Run("PromptsDir", func(t *testing.T) {
		t.Parallel()

		got := PromptsDir(l)
		want := filepath.Join(baseDir, lyxdirs.DotLyxDirName, "webster", "prompts")

		if got != want {
			t.Errorf("PromptsDir(l) = %q; want %q", got, want)
		}
	})
}

// TestWebsterGeometryHelpers_SubpathAnchored proves every accessor resolves consistently at a
// nested AnchorRel too: Dir/ReportsDir (AnchorPath-anchored, durable) and ScratchDir/PromptsDir
// (AnchorPath-anchored, never-tracked) all move down by AnchorRel together, since both trees share
// the Cwd Resolution Invariant's per-module anchoring.
func TestWebsterGeometryHelpers_SubpathAnchored(t *testing.T) {
	t.Parallel()

	worktree := "/home/user/project"
	l := &lyxcwd.Location{HubPath: filepath.Dir(worktree), WorktreeName: filepath.Base(worktree), AnchorRel: "backend"}
	anchor := l.AnchorPath()

	t.Run("Dir", func(t *testing.T) {
		t.Parallel()

		got := Dir(l)
		want := filepath.Join(anchor, lyxdirs.LyxDirName, "webster")

		if got != want {
			t.Errorf("Dir(l) = %q; want %q", got, want)
		}
	})

	t.Run("ReportsDir", func(t *testing.T) {
		t.Parallel()

		got := ReportsDir(l)
		want := filepath.Join(anchor, lyxdirs.LyxDirName, "webster", "reports")

		if got != want {
			t.Errorf("ReportsDir(l) = %q; want %q", got, want)
		}
	})

	t.Run("ScratchDir", func(t *testing.T) {
		t.Parallel()

		got := ScratchDir(l)
		want := filepath.Join(anchor, lyxdirs.DotLyxDirName, "webster")

		if got != want {
			t.Errorf("ScratchDir(l) = %q; want %q", got, want)
		}
	})

	t.Run("PromptsDir", func(t *testing.T) {
		t.Parallel()

		got := PromptsDir(l)
		want := filepath.Join(anchor, lyxdirs.DotLyxDirName, "webster", "prompts")

		if got != want {
			t.Errorf("PromptsDir(l) = %q; want %q", got, want)
		}
	})
}
