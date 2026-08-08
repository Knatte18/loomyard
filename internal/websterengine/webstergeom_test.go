// webstergeom_test.go — pure path-math unit tests for the webster geometry accessors
// (Dir/ReportsDir/PromptsDir).
// These tests do not require a git repository and run under standard unit test verification.

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestWebsterGeometryHelpers pins the three _lyx/webster path joins.
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

	t.Run("PromptsDir", func(t *testing.T) {
		t.Parallel()

		got := PromptsDir(l)
		want := filepath.Join(baseDir, lyxdirs.LyxDirName, "webster", "prompts")

		if got != want {
			t.Errorf("PromptsDir(l) = %q; want %q", got, want)
		}
	})
}
