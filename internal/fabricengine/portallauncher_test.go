// portallauncher_test.go covers the portal and launcher path accessors relocated from
// internal/lyxcwd in this batch — PortalsDir, PortalLink, portalTarget, launchersDir, LauncherDir,
// menuLauncherPath, launcherSpawnRel and menuLauncherRel — pinning the _portals/_launchers layout
// and the launcher relative-path math across root and subpath AnchorRel cases, the reason a wrong
// HubPath base or a broken filepath.Rel would be caught here rather than in a live hub.

package fabricengine

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newPortalLauncherTestLocation builds a Location by hand for join-arithmetic
// assertions, mirroring the field derivation lyxcwd.Resolve performs,
// without spawning git.
func newPortalLauncherTestLocation(hub, worktreeRoot, relPath string) *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      hub,
		WorktreeName: filepath.Base(worktreeRoot),
		AnchorRel:    relPath,
	}
}

// wantMenuLauncherName returns the expected menu launcher filename for the
// current runtime.GOOS, mirroring the GOOS-aware selection in launchers.go
// so these tests are green regardless of warp OS.
func wantMenuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// TestPortalsDirAndLaunchersDir verifies that PortalsDir and launchersDir join the hub with their
// respective directory names,
// and that portalTarget joins the hub, slug, AnchorRel and _lyx.
func TestPortalsDirAndLaunchersDir(t *testing.T) {
	t.Parallel()

	hub := filepath.Join("repos", "loomyard-HUB")
	l := &lyxcwd.Location{HubPath: hub, WorktreeName: "loomyard", AnchorRel: "."}
	slug := "test-wt"

	if got, want := PortalsDir(l), filepath.Join(hub, "_portals"); got != want {
		t.Errorf("PortalsDir() = %q; want %q", got, want)
	}

	if got, want := portalTarget(l, slug), filepath.Join(hub, slug, ".", "_lyx"); got != want {
		t.Errorf("portalTarget(%q) = %q; want %q", slug, got, want)
	}

	if got, want := launchersDir(l), filepath.Join(hub, "_launchers"); got != want {
		t.Errorf("launchersDir() = %q; want %q", got, want)
	}

	if got, want := LauncherDir(l, slug), filepath.Join(hub, "_launchers", slug); got != want {
		t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
	}
}

// TestMirroredPortalLauncherMethods tests the AnchorRel-mirrored geometry accessors: PortalLink,
// LauncherDir, menuLauncherPath, launcherSpawnRel and menuLauncherRel, both at the anchor root and
// at a nested subpath, plus a no-collision check between two distinct subpaths.
func TestMirroredPortalLauncherMethods(t *testing.T) {
	t.Parallel()

	hub := filepath.Join("repos", "loomyard-HUB")
	worktreeRoot := filepath.Join(hub, "loomyard")

	t.Run("PortalLink", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, ".")
			slug := "test-slug"
			got := PortalLink(l, slug)
			want := filepath.Join(l.HubPath, "_portals", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			slug := "test-slug"
			got := PortalLink(l, slug)
			want := filepath.Join(l.HubPath, "_portals", "services", "api", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			l1 := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			l2 := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "web"))
			slug := "test-slug"
			link1 := PortalLink(l1, slug)
			link2 := PortalLink(l2, slug)
			if link1 == link2 {
				t.Errorf("PortalLink produced collision: %q == %q", link1, link2)
			}
		})
	})

	t.Run("LauncherDir", func(t *testing.T) {
		t.Parallel()

		t.Run("at root (backward compat)", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, ".")
			slug := "test-slug"
			got := LauncherDir(l, slug)
			// At root, should still equal Join(launchersDir(), slug)
			want := filepath.Join(launchersDir(l), slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			slug := "test-slug"
			got := LauncherDir(l, slug)
			want := filepath.Join(l.HubPath, "_launchers", "services", "api", slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			l1 := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			l2 := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "web"))
			slug := "test-slug"
			dir1 := LauncherDir(l1, slug)
			dir2 := LauncherDir(l2, slug)
			if dir1 == dir2 {
				t.Errorf("LauncherDir produced collision: %q == %q", dir1, dir2)
			}
		})
	})

	t.Run("menuLauncherPath", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, ".")
			got := menuLauncherPath(l)
			want := filepath.Join(l.HubPath, "_launchers", wantMenuLauncherName())
			if got != want {
				t.Errorf("menuLauncherPath() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			got := menuLauncherPath(l)
			want := filepath.Join(l.HubPath, "_launchers", "services", "api", wantMenuLauncherName())
			if got != want {
				t.Errorf("menuLauncherPath() = %q; want %q", got, want)
			}
		})
	})

	t.Run("launcherSpawnRel", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, ".")
			slug := "test-slug"
			got, err := launcherSpawnRel(l, slug)
			if err != nil {
				t.Fatalf("launcherSpawnRel returned an unexpected error: %v", err)
			}

			launcherDir := LauncherDir(l, slug)
			targetPath := filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel)
			want, _ := filepath.Rel(launcherDir, targetPath)

			if got != want {
				t.Errorf("launcherSpawnRel(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			slug := "test-slug"
			got, err := launcherSpawnRel(l, slug)
			if err != nil {
				t.Fatalf("launcherSpawnRel returned an unexpected error: %v", err)
			}

			launcherDir := LauncherDir(l, slug)
			targetPath := filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel)
			want, _ := filepath.Rel(launcherDir, targetPath)

			if got != want {
				t.Errorf("launcherSpawnRel(%q) = %q; want %q", slug, got, want)
			}
		})
	})

	t.Run("menuLauncherRel", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, ".")
			primeName := filepath.Base(worktreeRoot)
			got, err := menuLauncherRel(l, primeName)
			if err != nil {
				t.Fatalf("menuLauncherRel returned an unexpected error: %v", err)
			}

			menuDir := filepath.Dir(menuLauncherPath(l))
			targetPath := filepath.Join(l.HubPath, primeName, l.AnchorRel)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("menuLauncherRel(%q) = %q; want %q", primeName, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := newPortalLauncherTestLocation(hub, worktreeRoot, filepath.Join("services", "api"))
			primeName := filepath.Base(worktreeRoot)
			got, err := menuLauncherRel(l, primeName)
			if err != nil {
				t.Fatalf("menuLauncherRel returned an unexpected error: %v", err)
			}

			menuDir := filepath.Dir(menuLauncherPath(l))
			targetPath := filepath.Join(l.HubPath, primeName, l.AnchorRel)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("menuLauncherRel(%q) = %q; want %q", primeName, got, want)
			}
		})
	})
}
