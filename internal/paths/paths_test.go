// paths_test.go covers Layout resolution, the geometry accessors, and the
// ErrNotAGitRepo path for directories outside a git repo.

package paths_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestResolve_FromWorktreeRoot verifies that Resolve from the worktree root
// yields empty RelPath (or ".") and correct other fields.
func TestResolve_FromWorktreeRoot(t *testing.T) {
	hub := newTestRepo(t)
	defer func() {
		// Clean up the test repo
		_ = os.RemoveAll(filepath.Dir(hub))
	}()

	layout, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	if layout == nil {
		t.Fatal("Resolve() returned nil layout")
	}

	// RelPath should be "." when cwd == worktree root
	if layout.RelPath != "." {
		t.Errorf("layout.RelPath = %q; want %q", layout.RelPath, ".")
	}

	// Cwd should be the hub (worktree root)
	if layout.Cwd != layout.WorktreeRoot {
		t.Errorf("layout.Cwd = %q; layout.WorktreeRoot = %q; want equal", layout.Cwd, layout.WorktreeRoot)
	}

	// Container should be the parent of WorktreeRoot
	expectedContainer := filepath.Dir(hub)
	if layout.Container != expectedContainer {
		t.Errorf("layout.Container = %q; want %q", layout.Container, expectedContainer)
	}

	// MainWorktree should be set to the hub path
	if layout.MainWorktree != hub {
		t.Errorf("layout.MainWorktree = %q; want %q", layout.MainWorktree, hub)
	}
}

// TestResolve_FromSubdirectory verifies that Resolve from a subdirectory
// yields the correct relative RelPath.
func TestResolve_FromSubdirectory(t *testing.T) {
	hub := newTestRepo(t)
	defer func() {
		_ = os.RemoveAll(filepath.Dir(hub))
	}()

	// Create a subdirectory structure
	subDir := filepath.Join(hub, "subdir", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	layout, err := paths.Resolve(subDir)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	if layout == nil {
		t.Fatal("Resolve() returned nil layout")
	}

	// RelPath should reflect the subdirectory
	expectedRelPath := filepath.Join("subdir", "nested")
	if layout.RelPath != expectedRelPath {
		t.Errorf("layout.RelPath = %q; want %q", layout.RelPath, expectedRelPath)
	}

	// Cwd should be the subdir
	if layout.Cwd != subDir {
		t.Errorf("layout.Cwd = %q; want %q", layout.Cwd, subDir)
	}
}

// TestResolve_GeometryMethods verifies that geometry methods produce expected paths.
func TestResolve_GeometryMethods(t *testing.T) {
	hub := newTestRepo(t)
	defer func() {
		_ = os.RemoveAll(filepath.Dir(hub))
	}()

	layout, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	// Test LyxDir
	expectedLyxDir := filepath.Join(hub, "_lyx")
	if got := layout.LyxDir(); got != expectedLyxDir {
		t.Errorf("LyxDir() = %q; want %q", got, expectedLyxDir)
	}

	// Test WorktreePath
	slug := "test-wt"
	expectedWtPath := filepath.Join(layout.Container, slug)
	if got := layout.WorktreePath(slug); got != expectedWtPath {
		t.Errorf("WorktreePath(%q) = %q; want %q", slug, got, expectedWtPath)
	}

	// Test PortalsDir
	expectedPortalsDir := filepath.Join(layout.Container, "_portals")
	if got := layout.PortalsDir(); got != expectedPortalsDir {
		t.Errorf("PortalsDir() = %q; want %q", got, expectedPortalsDir)
	}

	// Test PortalTarget
	expectedPortalTarget := filepath.Join(layout.Container, slug, ".", "_lyx")
	if got := layout.PortalTarget(slug); got != expectedPortalTarget {
		t.Errorf("PortalTarget(%q) = %q; want %q", slug, got, expectedPortalTarget)
	}

	// Test LaunchersDir
	expectedLaunchersDir := filepath.Join(layout.Container, "_launchers")
	if got := layout.LaunchersDir(); got != expectedLaunchersDir {
		t.Errorf("LaunchersDir() = %q; want %q", got, expectedLaunchersDir)
	}

	// Test LauncherDir
	expectedLauncherDir := filepath.Join(expectedLaunchersDir, slug)
	if got := layout.LauncherDir(slug); got != expectedLauncherDir {
		t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, expectedLauncherDir)
	}

	// Test HubName
	expectedHubName := filepath.Base(hub)
	if got := layout.HubName(); got != expectedHubName {
		t.Errorf("HubName() = %q; want %q", got, expectedHubName)
	}
}

// TestResolve_ForwardSlashNormalization verifies that forward-slash output
// from --show-toplevel is reconciled with backslash cwd on Windows.
func TestResolve_ForwardSlashNormalization(t *testing.T) {
	hub := newTestRepo(t)
	defer func() {
		_ = os.RemoveAll(filepath.Dir(hub))
	}()

	// Call Resolve normally; both cwd and --show-toplevel output get normalized
	layout, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	// Verify paths are clean and use the platform's separator
	if layout.Cwd != filepath.Clean(hub) {
		t.Errorf("layout.Cwd = %q; want %q", layout.Cwd, filepath.Clean(hub))
	}

	if layout.WorktreeRoot != filepath.Clean(hub) {
		t.Errorf("layout.WorktreeRoot = %q; want %q", layout.WorktreeRoot, filepath.Clean(hub))
	}
}

// TestResolve_NotAGitRepo verifies that Resolve in a non-git temp directory
// returns ErrNotAGitRepo.
func TestResolve_NotAGitRepo(t *testing.T) {
	nonGitDir := t.TempDir()

	layout, err := paths.Resolve(nonGitDir)

	if layout != nil {
		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
	}

	if !errors.Is(err, paths.ErrNotAGitRepo) {
		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
	}
}

// TestMirroredMethods tests the subpath-mirrored geometry methods.
func TestMirroredMethods(t *testing.T) {
	hub := newTestRepo(t)
	defer func() {
		_ = os.RemoveAll(filepath.Dir(hub))
	}()

	t.Run("PortalLink", func(t *testing.T) {
		t.Run("at root", func(t *testing.T) {
			layout, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.Container, "_portals", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := paths.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.Container, "_portals", "services", "api", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			subDir1 := filepath.Join(hub, "services", "api")
			subDir2 := filepath.Join(hub, "services", "web")
			if err := os.MkdirAll(subDir1, 0755); err != nil {
				t.Fatalf("failed to create subdir1: %v", err)
			}
			if err := os.MkdirAll(subDir2, 0755); err != nil {
				t.Fatalf("failed to create subdir2: %v", err)
			}

			layout1, err := paths.Resolve(subDir1)
			if err != nil {
				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
			}

			layout2, err := paths.Resolve(subDir2)
			if err != nil {
				t.Fatalf("Resolve(subDir2) error = %v; want nil", err)
			}

			slug := "test-slug"
			link1 := layout1.PortalLink(slug)
			link2 := layout2.PortalLink(slug)

			if link1 == link2 {
				t.Errorf("PortalLink produced collision: %q == %q", link1, link2)
			}
		})
	})

	t.Run("LauncherDir", func(t *testing.T) {
		t.Run("at root (backward compat)", func(t *testing.T) {
			layout, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherDir(slug)
			// At root, should still equal Join(LaunchersDir(), slug)
			want := filepath.Join(layout.LaunchersDir(), slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := paths.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherDir(slug)
			want := filepath.Join(layout.Container, "_launchers", "services", "api", slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			subDir1 := filepath.Join(hub, "services", "api")
			subDir2 := filepath.Join(hub, "services", "web")
			if err := os.MkdirAll(subDir1, 0755); err != nil {
				t.Fatalf("failed to create subdir1: %v", err)
			}
			if err := os.MkdirAll(subDir2, 0755); err != nil {
				t.Fatalf("failed to create subdir2: %v", err)
			}

			layout1, err := paths.Resolve(subDir1)
			if err != nil {
				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
			}

			layout2, err := paths.Resolve(subDir2)
			if err != nil {
				t.Fatalf("Resolve(subDir2) error = %v; want nil", err)
			}

			slug := "test-slug"
			dir1 := layout1.LauncherDir(slug)
			dir2 := layout2.LauncherDir(slug)

			if dir1 == dir2 {
				t.Errorf("LauncherDir produced collision: %q == %q", dir1, dir2)
			}
		})
	})

	t.Run("MenuLauncherPath", func(t *testing.T) {
		t.Run("at root", func(t *testing.T) {
			layout, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.Container, "_launchers", "ide-menu.cmd")
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := paths.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.Container, "_launchers", "services", "api", "ide-menu.cmd")
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})
	})

	t.Run("LauncherSpawnRel", func(t *testing.T) {
		t.Run("at root", func(t *testing.T) {
			layout, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherSpawnRel(slug)

			// Recompute expected via filepath.Rel
			launcherDir := layout.LauncherDir(slug)
			targetPath := filepath.Join(layout.WorktreePath(slug), layout.RelPath)
			want, _ := filepath.Rel(launcherDir, targetPath)

			if got != want {
				t.Errorf("LauncherSpawnRel(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := paths.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherSpawnRel(slug)

			// Recompute expected via filepath.Rel
			launcherDir := layout.LauncherDir(slug)
			targetPath := filepath.Join(layout.WorktreePath(slug), layout.RelPath)
			want, _ := filepath.Rel(launcherDir, targetPath)

			if got != want {
				t.Errorf("LauncherSpawnRel(%q) = %q; want %q", slug, got, want)
			}
		})
	})

	t.Run("MenuLauncherRel", func(t *testing.T) {
		t.Run("at root", func(t *testing.T) {
			layout, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherRel()

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.MainWorktree, layout.RelPath)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := paths.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherRel()

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.MainWorktree, layout.RelPath)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel() = %q; want %q", got, want)
			}
		})
	})
}
