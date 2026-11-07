// portallauncher_test.go covers the launcher path accessors and the launcher teardown.
// It pins the `_launchers` layout and the launcher relative-path math across root and subpath AnchorRel cases.
// A wrong HubPath base or a broken filepath.Rel fails here rather than in a live hub.

package fabricengine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantMenuLauncherName returns the menu launcher filename for runtime.GOOS.
// The selection mirrors the GOOS check in launchers.go, so the tests hold on every OS.
func wantMenuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// TestLaunchersDir verifies that launchersDir and LauncherDir join the hub, the launchers directory, and the slug.
//
//testtiming:keep launchersDir and LauncherDir joining the hub, the _launchers name and the slug; coverage of its blocks by other tests does not show an assertion of this
func TestLaunchersDir(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := filepath.Join("repos", "loomyard-LYMHUB")
	l := &lymcwd.Location{HubPath: hub, WorktreeName: "loomyard", AnchorRel: "."}
	slug := "test-wt"

	if got, want := launchersDir(l), filepath.Join(hub, "_launchers"); got != want {
		t.Errorf("launchersDir() = %q; want %q", got, want)
	}

	if got, want := LauncherDir(l, slug), filepath.Join(hub, "_launchers", slug); got != want {
		t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
	}
}

// TestMirroredLauncherMethods tests the launcher path accessors at the anchor root and at a nested subpath.
// It also checks that two distinct subpaths never collide.
//
//testtiming:keep the AnchorRel-mirrored launcher accessors at the root and a nested subpath, with distinct subpaths never colliding; coverage of its blocks by other tests does not show an assertion of this
func TestMirroredLauncherMethods(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := filepath.Join("repos", "loomyard-LYMHUB")
	const worktreeName = "loomyard"

	t.Run("LauncherDir", func(t *testing.T) {
		t.Parallel()

		t.Run("at root (backward compat)", func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(hub, worktreeName, ".")
			slug := "test-slug"
			got := LauncherDir(l, slug)
			// At the root, the expected path is the launchers directory joined with the slug.
			want := filepath.Join(launchersDir(l), slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(hub, worktreeName, filepath.Join("services", "api"))
			slug := "test-slug"
			got := LauncherDir(l, slug)
			want := filepath.Join(l.HubPath, "_launchers", "services", "api", slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			l1 := locationkit.Location(hub, worktreeName, filepath.Join("services", "api"))
			l2 := locationkit.Location(hub, worktreeName, filepath.Join("services", "web"))
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

			l := locationkit.Location(hub, worktreeName, ".")
			got := menuLauncherPath(l)
			want := filepath.Join(l.HubPath, "_launchers", wantMenuLauncherName())
			if got != want {
				t.Errorf("menuLauncherPath() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			l := locationkit.Location(hub, worktreeName, filepath.Join("services", "api"))
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

			l := locationkit.Location(hub, worktreeName, ".")
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

			l := locationkit.Location(hub, worktreeName, filepath.Join("services", "api"))
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

			l := locationkit.Location(hub, worktreeName, ".")
			primeName := worktreeName
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

			l := locationkit.Location(hub, worktreeName, filepath.Join("services", "api"))
			primeName := worktreeName
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

// TestRemoveLaunchers_PreservesForeignContent proves the launcher teardown deletes only the scripts fabric wrote.
// A file the operator placed beside the launchers survives the teardown.
func TestRemoveLaunchers_PreservesForeignContent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	l := locationkit.Location(hub, "prime", ".")
	const slug = "my-task"

	launcherDir := LauncherDir(l, slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatalf("mkdir launcher dir: %v", err)
	}
	ext := launcherExt(runtime.GOOS)
	for _, name := range []string{"ide" + ext, "fabric-checkout" + ext, "run" + ext} {
		if err := os.WriteFile(filepath.Join(launcherDir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	operatorFile := filepath.Join(launcherDir, "operator-notes.txt")
	if err := os.WriteFile(operatorFile, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("seed operator file: %v", err)
	}

	// The directory cannot be removed while the operator's file is in it.
	// That refusal is the point: teardown reports it rather than deleting what it does not own.
	if err := removeLaunchers(NewMutations(""), l, slug); err == nil {
		t.Error("removeLaunchers() = nil; want the non-empty-directory refusal to be reported")
	}

	if _, err := os.Stat(operatorFile); err != nil {
		t.Errorf("removeLaunchers() destroyed the operator's own file at %s: %v", operatorFile, err)
	}
	if _, err := os.Stat(filepath.Join(launcherDir, "ide"+ext)); !os.IsNotExist(err) {
		t.Errorf("removeLaunchers() left ide%s in place; want fabric's own scripts removed", ext)
	}
	if _, err := os.Stat(filepath.Join(launcherDir, "run"+ext)); !os.IsNotExist(err) {
		t.Errorf("removeLaunchers() left run%s in place; want fabric's own scripts removed", ext)
	}
}

// TestRemoveLaunchers_DirRemovalIsContained proves the launcher-directory removal stays inside the launchers container.
//
// The escape is deterministic here rather than raced.
// The test calls removeContainedPath, which removeLaunchers uses, on an escaping intermediate component.
// The out-of-container victim must survive.
//
// The routing itself is guarded by `TestNoDestructiveBypass_FabricengineProductionSource`.
// It lives in cmd/lym/destructiveguard_test.go and fails if a raw os.Remove appears in launchers.go.
//
//testtiming:keep launcher-directory removal acting through the containment root so an escaping intermediate cannot delete an outside directory; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveLaunchers_DirRemovalIsContained(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	outside := t.TempDir()
	l := locationkit.Location(hub, "prime", "escapes")
	const slug = "my-task"

	// The victim the escaping link points at is an empty directory: a plain os.Remove would succeed on it,
	// so the assertion below fails without the containment guard.
	victim := filepath.Join(outside, slug)
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	if err := os.MkdirAll(launchersDir(l), 0o755); err != nil {
		t.Fatalf("mkdir launchers container: %v", err)
	}
	// AnchorRel component "escapes" is the intermediate segment, planted as a link out of the hub.
	if err := os.Symlink(outside, filepath.Join(launchersDir(l), "escapes")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	removed, _, err := removeContainedPath(launchersDir(l), LauncherDir(l, slug), false)
	if err == nil {
		t.Errorf("removeContainedPath(...) error = nil; want a refusal of the escaping intermediate component")
	}
	if removed {
		t.Error("removeContainedPath(...) removed = true; want false — nothing inside the container was removed")
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Errorf("the out-of-container victim at %s was destroyed: %v; want it untouched", victim, statErr)
	}
}

// TestRemoveLaunchers_EmptyDirRemovedAndRecorded pins the success path of removeLaunchers:
// an empty launcher directory is removed and recorded exactly once, and a second call is a silent, unrecorded no-op.
// removeContainedPath reports removed=false for an absent target,
// and that keeps the idempotence every teardown caller relies on.
//
//testtiming:keep an empty launcher directory being removed and recorded exactly once with a silent second call; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveLaunchers_EmptyDirRemovedAndRecorded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	l := locationkit.Location(hub, "prime", ".")
	const slug = "my-task"

	launcherDir := LauncherDir(l, slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		t.Fatalf("mkdir launcher dir: %v", err)
	}

	rec := NewMutations(hub)
	if err := removeLaunchers(rec, l, slug); err != nil {
		t.Fatalf("removeLaunchers() error = %v; want nil", err)
	}
	if _, err := os.Stat(launcherDir); !os.IsNotExist(err) {
		t.Errorf("removeLaunchers() left %s in place; want it removed", launcherDir)
	}
	if got := rec.Len(); got != 1 {
		t.Errorf("removeLaunchers() recorded %d mutations; want 1 (the directory removal)", got)
	}

	// Second call: idempotent, and it records nothing, since an absent target is a successful no-op and not a removal.
	rec2 := NewMutations(hub)
	if err := removeLaunchers(rec2, l, slug); err != nil {
		t.Fatalf("removeLaunchers() second call error = %v; want nil", err)
	}
	if got := rec2.Len(); got != 0 {
		t.Errorf("removeLaunchers() second call recorded %d mutations; want 0", got)
	}
}

// TestRemoveLaunchers_ContainmentRefusalSurvivesSurfaceRefusal proves that the containment refusal is not discarded:
// removeLaunchers opens with refuseUncontainedPath.
// Every best-effort call site in Remove and Prune wraps its error in surfaceRefusal.
// surfaceRefusal returns nil for anything that is not a *destructiveRefusal,
// so a bare containment error would be dropped unnoticed.
//
// The assertion checks surfaceRefusal's result, not only a non-nil error:
// a bare error from refuseUncontainedPath would still be non-nil here, and only the surfaceRefusal round-trip fails.
func TestRemoveLaunchers_ContainmentRefusalSurvivesSurfaceRefusal(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	outside := t.TempDir()
	l := locationkit.Location(hub, "prime", "escapes")
	const slug = "my-task"

	if err := os.MkdirAll(launchersDir(l), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", launchersDir(l), err)
	}
	if err := os.Symlink(outside, filepath.Join(launchersDir(l), "escapes")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	// The teardown targets must exist out there, so the guard refuses on containment rather than
	// short-circuiting on an absent target.
	if err := os.MkdirAll(filepath.Join(outside, slug), 0o755); err != nil {
		t.Fatalf("mkdir escaped target: %v", err)
	}

	err := removeLaunchers(NewMutations(hub), l, slug)
	if err == nil {
		t.Fatal("launchers teardown error = nil; want a containment refusal")
	}
	if surfaceRefusal(err) == nil {
		t.Error("surfaceRefusal(launchers teardown error) = nil; want the refusal propagated so " +
			"Remove/Prune report it instead of exiting ok:true with artifacts left on disk")
	}
	refusal, ok := RefusalOf(err)
	if !ok {
		t.Fatal("RefusalOf(launchers teardown error) ok = false; want a typed gate refusal")
	}
	if refusal.Check != CheckContainment {
		t.Errorf("RefusalOf(...).Check = %q; want %q", refusal.Check, CheckContainment)
	}
}
