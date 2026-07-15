// hubgeometry_unit_test.go — pure path-math unit tests for config helpers and constants.
// These tests do not require a git repository and run under standard unit test verification.

package hubgeometry_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestConfigHelpers tests the free-function config path helpers.
func TestConfigHelpers(t *testing.T) {
	t.Parallel()

	t.Run("ConfigDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.ConfigDir(baseDir)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "config")

		if got != want {
			t.Errorf("ConfigDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("ConfigFile", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		module := "myapp"
		got := hubgeometry.ConfigFile(baseDir, module)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "config", "myapp.yaml")

		if got != want {
			t.Errorf("ConfigFile(%q, %q) = %q; want %q", baseDir, module, got, want)
		}
	})

	t.Run("PerchRunsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.PerchRunsDir(baseDir)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "perch")

		if got != want {
			t.Errorf("PerchRunsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("PlanDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.PlanDir(baseDir)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "plan")

		if got != want {
			t.Errorf("PlanDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.BuilderDir(baseDir)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "builder")

		if got != want {
			t.Errorf("BuilderDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderReportsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.BuilderReportsDir(baseDir)
		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "builder", "reports")

		if got != want {
			t.Errorf("BuilderReportsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("DotEnv", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := hubgeometry.DotEnv(baseDir)
		want := filepath.Join(baseDir, ".env")

		if got != want {
			t.Errorf("DotEnv(%q) = %q; want %q", baseDir, got, want)
		}
	})
}

// TestLyxDirNameConstant verifies that LyxDirName is exported and has the expected value.
func TestLyxDirNameConstant(t *testing.T) {
	t.Parallel()

	if hubgeometry.LyxDirName != "_lyx" {
		t.Errorf("LyxDirName = %q; want %q", hubgeometry.LyxDirName, "_lyx")
	}
}

// TestDotLyxDir verifies that DotLyxDir resolves to "<Cwd>/.lyx" and is distinct from
// LyxDir ("<Cwd>/_lyx"), since the two directories serve different durability
// contracts (ephemeral/machine-bound vs. durable/weft-synced).
func TestDotLyxDir(t *testing.T) {
	t.Parallel()

	cwd := filepath.Join("home", "user", "project")
	layout := &hubgeometry.Layout{Cwd: cwd}

	got := layout.DotLyxDir()
	want := filepath.Join(cwd, ".lyx")

	if got != want {
		t.Errorf("DotLyxDir() = %q; want %q", got, want)
	}

	if got == layout.LyxDir() {
		t.Errorf("DotLyxDir() = %q; must be distinct from LyxDir() = %q", got, layout.LyxDir())
	}
}

// TestHubLogsDir verifies that HubLogsDir resolves to "<Hub>/.lyx/logs" — pure
// path math, hub-anchored (not worktree-anchored), with no filesystem I/O.
func TestHubLogsDir(t *testing.T) {
	t.Parallel()

	hub := filepath.Join("home", "user", "project-HUB")
	layout := &hubgeometry.Layout{Hub: hub}

	got := layout.HubLogsDir()
	want := filepath.Join(hub, ".lyx", "logs")

	if got != want {
		t.Errorf("HubLogsDir() = %q; want %q", got, want)
	}
}
