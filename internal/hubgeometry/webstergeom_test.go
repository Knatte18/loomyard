// webstergeom_test.go — pure path-math unit tests for the webster geometry
// accessors (WebsterDir/WebsterReportsDir/WebsterPromptsDir). These tests do
// not require a git repository and run under standard unit test
// verification.

package hubgeometry

import (
	"path/filepath"
	"testing"
)

// TestWebsterGeometryHelpers pins the three _lyx/webster path joins.
func TestWebsterGeometryHelpers(t *testing.T) {
	t.Parallel()

	t.Run("WebsterDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := WebsterDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "webster")

		if got != want {
			t.Errorf("WebsterDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("WebsterReportsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := WebsterReportsDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "webster", "reports")

		if got != want {
			t.Errorf("WebsterReportsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("WebsterPromptsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := WebsterPromptsDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "webster", "prompts")

		if got != want {
			t.Errorf("WebsterPromptsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})
}
