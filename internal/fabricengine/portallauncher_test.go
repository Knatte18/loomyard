// portallauncher_test.go covers the portal and launcher path accessors relocated from
// internal/lyxcwd in this batch — PortalsDir, PortalLink, portalTarget, launchersDir, LauncherDir,
// menuLauncherPath, launcherSpawnRel and menuLauncherRel — pinning the _portals/_launchers layout
// and the launcher relative-path math across root and subpath AnchorRel cases, the reason a wrong
// HubPath base or a broken filepath.Rel would be caught here rather than in a live hub.

package fabricengine

import (
	"os"
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

// TestRemoveLaunchers_PreservesForeignContent proves the launcher teardown deletes only the scripts
// fabric wrote.
// It used to os.RemoveAll the whole directory, destroying anything the operator had placed beside
// the launchers, while the portal half of the same teardown (fslink.Remove) correctly declines to
// delete a non-empty real directory.
func TestRemoveLaunchers_PreservesForeignContent(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	l := newPortalLauncherTestLocation(hub, filepath.Join(hub, "prime"), ".")
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

	// The directory cannot be removed while the operator's file is in it, and that refusal is the
	// point: teardown reports it rather than deleting what it does not own.
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

// TestRemoveLaunchers_DirRemovalIsContained proves the launcher-DIRECTORY removal acts through an
// os.Root rooted at the launchers container rather than on the nominal path.
//
// This is fabric's R8 finding M1. The directory removal used to be a raw os.Remove(launcherDir)
// performed after checkPathRequest had already returned, which made it the one arbitrary-path removal
// left in the package still resolving containment at one instant and unlinking at a later one — the
// window R3 closed for removePath/removeLink by routing them through removeContainedPath. A symlink
// planted at an intermediate segment and flipped live-and-escaping inside that window carried the
// unlink outside the hub while the mutation record named the hub-relative path.
//
// The escape is deterministic here rather than raced: the removal is driven through the exact helper
// removeLaunchers now calls, with an intermediate component that escapes the container, and the
// out-of-container victim must survive. The routing itself is separately guarded mechanically —
// launchers.go is no longer on cmd/lyx/destructiveguard_test.go's allowlist, so reintroducing a raw
// os.Remove( there fails TestNoDestructiveBypass_FabricengineProductionSource.
func TestRemoveLaunchers_DirRemovalIsContained(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	outside := t.TempDir()
	l := newPortalLauncherTestLocation(hub, filepath.Join(hub, "prime"), "escapes")
	const slug = "my-task"

	// The victim the escaping link points at: an EMPTY directory, so a nominal os.Remove would
	// succeed on it and the assertion below is a real one rather than a vacuous OS refusal.
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

// TestRemoveLaunchers_EmptyDirRemovedAndRecorded pins the ordinary success path across the R8 M1
// routing change: an empty launcher directory is removed and recorded exactly once, and a second
// call is a silent, unrecorded no-op. removeContainedPath reports removed=false for an absent target,
// which is what keeps the idempotence every teardown caller relies on — the raw os.Remove this
// replaced relied on an os.IsNotExist check to get the same answer.
func TestRemoveLaunchers_EmptyDirRemovedAndRecorded(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	l := newPortalLauncherTestLocation(hub, filepath.Join(hub, "prime"), ".")
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

	// Second call: idempotent, and records nothing — an absent target is a successful no-op, never a
	// claimed removal.
	rec2 := NewMutations(hub)
	if err := removeLaunchers(rec2, l, slug); err != nil {
		t.Fatalf("removeLaunchers() second call error = %v; want nil", err)
	}
	if got := rec2.Len(); got != 0 {
		t.Errorf("removeLaunchers() second call recorded %d mutations; want 0", got)
	}
}

// TestRemoveLaunchersAndPortal_ContainmentRefusalSurvivesSurfaceRefusal proves the pre-gate
// containment guard's refusal reaches a best-effort call site rather than being silently discarded.
//
// This is fabric's R8 finding L2. removeLaunchers and removePortal both open with
// refuseUncontainedPath, which returned a bare fmt.Errorf. Every best-effort call site in Remove and
// Prune wraps them in surfaceRefusal, which by design returns nil for anything that is not a
// *destructiveRefusal — so a containment refusal evaporated and `lyx fabric remove` reported
// "ok":true with an exit 0 while the launcher scripts were still on disk. Reproduced live against
// the deployed binary with a static symlink at the _launchers container, no race required.
//
// The assertion is on surfaceRefusal specifically, not merely on a non-nil error, because a non-nil
// error was never the missing half: reverting refuseUncontainedPath to a bare fmt.Errorf still
// returns an error here, and only the surfaceRefusal round-trip below fails.
func TestRemoveLaunchersAndPortal_ContainmentRefusalSurvivesSurfaceRefusal(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	outside := t.TempDir()
	l := newPortalLauncherTestLocation(hub, filepath.Join(hub, "prime"), "escapes")
	const slug = "my-task"

	for _, dir := range []string{launchersDir(l), PortalsDir(l)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "escapes")); err != nil {
			t.Skipf("symlink unsupported on this platform: %v", err)
		}
	}
	// The teardown targets must exist out there, so the guard refuses on containment rather than
	// short-circuiting on an absent target.
	if err := os.MkdirAll(filepath.Join(outside, slug), 0o755); err != nil {
		t.Fatalf("mkdir escaped target: %v", err)
	}

	tests := []struct {
		name string
		run  func(rec *Mutations) error
	}{
		{"launchers", func(rec *Mutations) error { return removeLaunchers(rec, l, slug) }},
		{"portal", func(rec *Mutations) error { return removePortal(rec, l, slug) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewMutations(hub))
			if err == nil {
				t.Fatalf("%s teardown error = nil; want a containment refusal", tt.name)
			}
			if surfaceRefusal(err) == nil {
				t.Errorf("surfaceRefusal(%s teardown error) = nil; want the refusal propagated so "+
					"Remove/Prune report it instead of exiting ok:true with artifacts left on disk", tt.name)
			}
			refusal, ok := RefusalOf(err)
			if !ok {
				t.Fatalf("RefusalOf(%s teardown error) ok = false; want a typed gate refusal", tt.name)
			}
			if refusal.Check != CheckContainment {
				t.Errorf("RefusalOf(...).Check = %q; want %q", refusal.Check, CheckContainment)
			}
		})
	}
}
