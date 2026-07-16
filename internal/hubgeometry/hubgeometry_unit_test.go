// hubgeometry_unit_test.go — pure path-math unit tests for config helpers, constants,
// and the unexported deriveRepo helper. These tests do not require a git repository and
// run under standard unit test verification. This file uses the internal `hubgeometry`
// package (rather than `hubgeometry_test`) specifically so TestDeriveRepo can call the
// unexported deriveRepo directly without going through Resolve (Test Tier Purity:
// untagged tests spawn nothing).

package hubgeometry

import (
	"path/filepath"
	"testing"
)

// TestConfigHelpers tests the free-function config path helpers.
func TestConfigHelpers(t *testing.T) {
	t.Parallel()

	t.Run("ConfigDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := ConfigDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "config")

		if got != want {
			t.Errorf("ConfigDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("ConfigFile", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		module := "myapp"
		got := ConfigFile(baseDir, module)
		want := filepath.Join(baseDir, LyxDirName, "config", "myapp.yaml")

		if got != want {
			t.Errorf("ConfigFile(%q, %q) = %q; want %q", baseDir, module, got, want)
		}
	})

	t.Run("PerchRunsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := PerchRunsDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "perch")

		if got != want {
			t.Errorf("PerchRunsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("PlanDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := PlanDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "plan")

		if got != want {
			t.Errorf("PlanDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := BuilderDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "builder")

		if got != want {
			t.Errorf("BuilderDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("BuilderReportsDir", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := BuilderReportsDir(baseDir)
		want := filepath.Join(baseDir, LyxDirName, "builder", "reports")

		if got != want {
			t.Errorf("BuilderReportsDir(%q) = %q; want %q", baseDir, got, want)
		}
	})

	t.Run("DotEnv", func(t *testing.T) {
		t.Parallel()

		baseDir := "/home/user/project"
		got := DotEnv(baseDir)
		want := filepath.Join(baseDir, ".env")

		if got != want {
			t.Errorf("DotEnv(%q) = %q; want %q", baseDir, got, want)
		}
	})
}

// TestLyxDirNameConstant verifies that LyxDirName is exported and has the expected value.
func TestLyxDirNameConstant(t *testing.T) {
	t.Parallel()

	if LyxDirName != "_lyx" {
		t.Errorf("LyxDirName = %q; want %q", LyxDirName, "_lyx")
	}
}

// TestDotLyxDir verifies that DotLyxDir resolves to "<Cwd>/.lyx" and is distinct from
// LyxDir ("<Cwd>/_lyx"), since the two directories serve different durability
// contracts (ephemeral/machine-bound vs. durable/weft-synced).
func TestDotLyxDir(t *testing.T) {
	t.Parallel()

	cwd := filepath.Join("home", "user", "project")
	layout := &Layout{Cwd: cwd}

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
	layout := &Layout{Hub: hub}

	got := layout.HubLogsDir()
	want := filepath.Join(hub, ".lyx", "logs")

	if got != want {
		t.Errorf("HubLogsDir() = %q; want %q", got, want)
	}
}

// TestDeriveRepo covers deriveRepo's two branches (non-empty Prime, empty-Prime
// fallback to worktreeRoot) plus a trailing-slash input, using plain string inputs
// rather than a resolved Layout — deriveRepo is pure and deliberately spawn-free.
func TestDeriveRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		prime        string
		worktreeRoot string
		want         string
	}{
		{
			name:         "non-empty prime yields base of prime",
			prime:        filepath.Join("home", "user", "loomyard-HUB", "loomyard"),
			worktreeRoot: filepath.Join("home", "user", "loomyard-HUB", "feature-branch"),
			want:         "loomyard",
		},
		{
			name:         "empty prime falls back to worktreeRoot",
			prime:        "",
			worktreeRoot: filepath.Join("home", "user", "loomyard-HUB", "loomyard"),
			want:         "loomyard",
		},
		{
			name:         "trailing slash on prime is handled by filepath.Base",
			prime:        filepath.Join("home", "user", "loomyard-HUB", "loomyard") + string(filepath.Separator),
			worktreeRoot: filepath.Join("home", "user", "loomyard-HUB", "feature-branch"),
			want:         "loomyard",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := deriveRepo(tt.prime, tt.worktreeRoot)
			if got != tt.want {
				t.Errorf("deriveRepo(%q, %q) = %q; want %q", tt.prime, tt.worktreeRoot, got, tt.want)
			}
		})
	}
}
