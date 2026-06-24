// paths_unit_test.go — pure path-math unit tests for config helpers and constants.
// These tests do not require a git repository and run under standard unit test verification.

package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestConfigHelpers tests the free-function config path helpers.
func TestConfigHelpers(t *testing.T) {
	t.Parallel()

	t.Run("ConfigDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := paths.ConfigDir(baseDir)
		want := filepath.Join(baseDir, paths.LyxDirName, "config")

		if got != want {
			t.Errorf("ConfigDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("ConfigFile", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		module := "myapp"
		got := paths.ConfigFile(baseDir, module)
		want := filepath.Join(baseDir, paths.LyxDirName, "config", "myapp.yaml")

		if got != want {
			t.Errorf("ConfigFile(%q, %q) = %q; want %q", baseDir, module, got, want)
		}
	})

	t.Run("DotEnv", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := paths.DotEnv(baseDir)
		want := filepath.Join(baseDir, ".env")

		if got != want {
			t.Errorf("DotEnv(%q) = %q; want %q", baseDir, got, want)
		}
	})
}

// TestLyxDirNameConstant verifies that LyxDirName is exported and has the expected value.
func TestLyxDirNameConstant(t *testing.T) {
	t.Parallel()

	if paths.LyxDirName != "_lyx" {
		t.Errorf("LyxDirName = %q; want %q", paths.LyxDirName, "_lyx")
	}
}
