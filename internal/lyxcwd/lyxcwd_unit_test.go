// lyxcwd_unit_test.go — pure path-math unit tests for the _lyx-anchored
// path helpers that remain in this package and a couple of Location methods with
// distinct anchoring rules (DotLyxDir, HubLogsDir). These tests do not require
// a git repository and run under standard unit test verification.

package lyxcwd

import (
	"path/filepath"
	"testing"
)

// TestConfigHelpers tests the free-function _lyx-anchored path helpers that
// remain in hubgeometry (PerchRunsDir, PlanDir, BuilderDir, BuilderReportsDir).
// ConfigDir, ConfigFile, DotEnv and the exported LyxDirName constant moved to
// internal/configengine (and DotEnv to internal/envsource); their own
// coverage now lives in those packages' test files. The want-expressions here
// use the private, unexported lyxDirName transitional const, since this file
// is in-package and these methods still anchor on it until they relocate.
func TestConfigHelpers(t *testing.T) {
	t.Parallel()

	t.Run("PerchRunsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := PerchRunsDir(baseDir)
		want := filepath.Join(baseDir, lyxDirName, "perch")

		if got != want {
			t.Errorf("PerchRunsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("PlanDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := PlanDir(baseDir)
		want := filepath.Join(baseDir, lyxDirName, "plan")

		if got != want {
			t.Errorf("PlanDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := BuilderDir(baseDir)
		want := filepath.Join(baseDir, lyxDirName, "builder")

		if got != want {
			t.Errorf("BuilderDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderReportsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := BuilderReportsDir(baseDir)
		want := filepath.Join(baseDir, lyxDirName, "builder", "reports")

		if got != want {
			t.Errorf("BuilderReportsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})
}

// TestDotLyxDir verifies that DotLyxDir resolves to "<AnchorPath>/.lyx" and is distinct
// from LyxDir ("<AnchorPath>/_lyx"), since the two directories serve different durability
// contracts (ephemeral/machine-bound vs. durable/weft-synced).
func TestDotLyxDir(t *testing.T) {
	t.Parallel()

	hubPath := filepath.Join("home", "user")
	loc := &Location{HubPath: hubPath, WorktreeName: "project", AnchorRel: "."}
	want := filepath.Join(hubPath, "project", ".lyx")

	got := loc.DotLyxDir()

	if got != want {
		t.Errorf("DotLyxDir() = %q; want %q", got, want)
	}

	if got == loc.LyxDir() {
		t.Errorf("DotLyxDir() = %q; must be distinct from LyxDir() = %q", got, loc.LyxDir())
	}
}

// TestHubLogsDir verifies that HubLogsDir resolves to "<HubPath>/.lyx/logs" — pure
// path math, hub-anchored (not worktree-anchored), with no filesystem I/O.
func TestHubLogsDir(t *testing.T) {
	t.Parallel()

	hubPath := filepath.Join("home", "user", "project-HUB")
	loc := &Location{HubPath: hubPath}

	got := loc.HubLogsDir()
	want := filepath.Join(hubPath, ".lyx", "logs")

	if got != want {
		t.Errorf("HubLogsDir() = %q; want %q", got, want)
	}
}
