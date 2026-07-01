//go:build integration

// hubgeometry_test.go covers Layout resolution, the geometry accessors, and the
// ErrNotAGitRepo path for directories outside a git repo.

package hubgeometry_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestResolve_FromWorktreeRoot verifies that Resolve from the worktree root
// yields empty RelPath (or ".") and correct other fields.
func TestResolve_FromWorktreeRoot(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := hubgeometry.Resolve(hub)
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

	// Hub should be the parent of WorktreeRoot
	expectedContainer := filepath.Dir(hub)
	if layout.Hub != expectedContainer {
		t.Errorf("layout.Hub = %q; want %q", layout.Hub, expectedContainer)
	}

	// Prime should be set to the hub path
	if layout.Prime != hub {
		t.Errorf("layout.Prime = %q; want %q", layout.Prime, hub)
	}
}

// TestResolve_FromSubdirectory verifies that Resolve from a subdirectory
// yields the correct relative RelPath.
func TestResolve_FromSubdirectory(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Create a subdirectory structure
	subDir := filepath.Join(hub, "subdir", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	layout, err := hubgeometry.Resolve(subDir)
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
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := hubgeometry.Resolve(hub)
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
	expectedWtPath := filepath.Join(layout.Hub, slug)
	if got := layout.WorktreePath(slug); got != expectedWtPath {
		t.Errorf("WorktreePath(%q) = %q; want %q", slug, got, expectedWtPath)
	}

	// Test PortalsDir
	expectedPortalsDir := filepath.Join(layout.Hub, "_portals")
	if got := layout.PortalsDir(); got != expectedPortalsDir {
		t.Errorf("PortalsDir() = %q; want %q", got, expectedPortalsDir)
	}

	// Test PortalTarget
	expectedPortalTarget := filepath.Join(layout.Hub, slug, ".", "_lyx")
	if got := layout.PortalTarget(slug); got != expectedPortalTarget {
		t.Errorf("PortalTarget(%q) = %q; want %q", slug, got, expectedPortalTarget)
	}

	// Test LaunchersDir
	expectedLaunchersDir := filepath.Join(layout.Hub, "_launchers")
	if got := layout.LaunchersDir(); got != expectedLaunchersDir {
		t.Errorf("LaunchersDir() = %q; want %q", got, expectedLaunchersDir)
	}

	// Test LauncherDir
	expectedLauncherDir := filepath.Join(expectedLaunchersDir, slug)
	if got := layout.LauncherDir(slug); got != expectedLauncherDir {
		t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, expectedLauncherDir)
	}

	// Test PrimeName
	expectedHubName := filepath.Base(hub)
	if got := layout.PrimeName(); got != expectedHubName {
		t.Errorf("PrimeName() = %q; want %q", got, expectedHubName)
	}
}

// TestResolve_ForwardSlashNormalization verifies that forward-slash output
// from --show-toplevel is reconciled with backslash cwd on Windows.
func TestResolve_ForwardSlashNormalization(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Call Resolve normally; both cwd and --show-toplevel output get normalized
	layout, err := hubgeometry.Resolve(hub)
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
	t.Parallel()

	nonGitDir := t.TempDir()

	layout, err := hubgeometry.Resolve(nonGitDir)

	if layout != nil {
		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
	}

	if !errors.Is(err, hubgeometry.ErrNotAGitRepo) {
		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
	}

	// Pin the bare-sentinel behavior: git's raw stderr must never leak into the
	// error text, and no other content may be appended to the sentinel message.
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Resolve() error = %q; must not contain raw git stderr (\"fatal:\")", err.Error())
	}
	if err.Error() != hubgeometry.ErrNotAGitRepo.Error() {
		t.Errorf("Resolve() error = %q; want exactly %q", err.Error(), hubgeometry.ErrNotAGitRepo.Error())
	}
}

// TestMirroredMethods tests the subpath-mirrored geometry methods.
func TestMirroredMethods(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	t.Run("PortalLink", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			layout, err := hubgeometry.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.Hub, "_portals", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := hubgeometry.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.Hub, "_portals", "services", "api", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			subDir1 := filepath.Join(hub, "services", "api")
			subDir2 := filepath.Join(hub, "services", "web")
			if err := os.MkdirAll(subDir1, 0755); err != nil {
				t.Fatalf("failed to create subdir1: %v", err)
			}
			if err := os.MkdirAll(subDir2, 0755); err != nil {
				t.Fatalf("failed to create subdir2: %v", err)
			}

			layout1, err := hubgeometry.Resolve(subDir1)
			if err != nil {
				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
			}

			layout2, err := hubgeometry.Resolve(subDir2)
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
		t.Parallel()

		t.Run("at root (backward compat)", func(t *testing.T) {
			t.Parallel()

			layout, err := hubgeometry.Resolve(hub)
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
			t.Parallel()

			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := hubgeometry.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherDir(slug)
			want := filepath.Join(layout.Hub, "_launchers", "services", "api", slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			subDir1 := filepath.Join(hub, "services", "api")
			subDir2 := filepath.Join(hub, "services", "web")
			if err := os.MkdirAll(subDir1, 0755); err != nil {
				t.Fatalf("failed to create subdir1: %v", err)
			}
			if err := os.MkdirAll(subDir2, 0755); err != nil {
				t.Fatalf("failed to create subdir2: %v", err)
			}

			layout1, err := hubgeometry.Resolve(subDir1)
			if err != nil {
				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
			}

			layout2, err := hubgeometry.Resolve(subDir2)
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
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			layout, err := hubgeometry.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.Hub, "_launchers", "ide-menu.cmd")
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := hubgeometry.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.Hub, "_launchers", "services", "api", "ide-menu.cmd")
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})
	})

	t.Run("LauncherSpawnRel", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			layout, err := hubgeometry.Resolve(hub)
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
			t.Parallel()

			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := hubgeometry.Resolve(subDir)
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
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			layout, err := hubgeometry.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherRel()

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.Prime, layout.RelPath)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			subDir := filepath.Join(hub, "services", "api")
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("failed to create subdir: %v", err)
			}

			layout, err := hubgeometry.Resolve(subDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherRel()

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.Prime, layout.RelPath)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel() = %q; want %q", got, want)
			}
		})
	})
}

// TestRefactoredMethods verifies that refactored methods using LyxDirName
// still produce the same paths as before the refactor (backward compatibility guard).
func TestRefactoredMethods(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := hubgeometry.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	t.Run("LyxDir", func(t *testing.T) {
		t.Parallel()

		got := layout.LyxDir()
		want := filepath.Join(hub, "_lyx")

		if got != want {
			t.Errorf("LyxDir() = %q; want %q", got, want)
		}
	})

	t.Run("PortalTarget", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		got := layout.PortalTarget(slug)
		want := filepath.Join(layout.Hub, slug, ".", "_lyx")

		if got != want {
			t.Errorf("PortalTarget(%q) = %q; want %q", slug, got, want)
		}
	})

	t.Run("WeftLyxDir", func(t *testing.T) {
		t.Parallel()

		got := layout.WeftLyxDir()
		want := filepath.Join(layout.WeftWorktree(), ".", "_lyx")

		if got != want {
			t.Errorf("WeftLyxDir() = %q; want %q", got, want)
		}
	})

	t.Run("WeftLyxDirFor", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		got := layout.WeftLyxDirFor(slug)
		want := filepath.Join(layout.WeftWorktreePath(slug), ".", "_lyx")

		if got != want {
			t.Errorf("WeftLyxDirFor(%q) = %q; want %q", slug, got, want)
		}
	})

	t.Run("HostLyxLink", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		got := layout.HostLyxLink(slug)
		want := filepath.Join(layout.WorktreePath(slug), ".", "_lyx")

		if got != want {
			t.Errorf("HostLyxLink(%q) = %q; want %q", slug, got, want)
		}
	})

	t.Run("HostLyxLinkHere", func(t *testing.T) {
		t.Parallel()

		got := layout.HostLyxLinkHere()
		want := filepath.Join(layout.WorktreeRoot, ".", "_lyx")

		if got != want {
			t.Errorf("HostLyxLinkHere() = %q; want %q", got, want)
		}
	})

	t.Run("HostJunctions", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		junctions := layout.HostJunctions(slug)

		if len(junctions) != 1 {
			t.Fatalf("HostJunctions() returned %d junctions; want 1", len(junctions))
		}

		junction := junctions[0]
		if junction.Name != "_lyx" {
			t.Errorf("HostJunctions()[0].Name = %q; want %q", junction.Name, "_lyx")
		}
	})
}
