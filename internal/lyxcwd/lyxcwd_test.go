//go:build integration

// lyxcwd_test.go covers Location resolution, the geometry accessors, and the
// ErrNotAGitRepo path for directories outside a git repo.

package lyxcwd_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// newTestLocation builds a Location by hand for join-arithmetic assertions,
// mirroring the field derivation Resolve performs, without spawning git. It
// used to live in this package's pattern_test.go, which card 25 relocated to
// internal/pattern along with the _pattern accessors it covered; this file's
// own TestJunctionPaths still needs it, so it is restored here.
func newTestLocation(hub, worktreeRoot, relPath string) *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      hub,
		WorktreeName: filepath.Base(worktreeRoot),
		AnchorRel:    relPath,
	}
}

// wantMenuLauncherName returns the expected menu launcher filename for the
// current runtime.GOOS, mirroring the GOOS-aware selection in lyxcwd.go
// so these tests are green on the Windows host now and on Linux later.
func wantMenuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// TestResolve_FromWorktreeRoot verifies that Resolve from the worktree root
// yields AnchorRel "." and correct other fields.
func TestResolve_FromWorktreeRoot(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	if layout == nil {
		t.Fatal("Resolve() returned nil layout")
	}

	// AnchorRel should be "." when no anchor is recorded, regardless of cwd.
	if layout.AnchorRel != "." {
		t.Errorf("layout.AnchorRel = %q; want %q", layout.AnchorRel, ".")
	}

	// WorktreePath() should be the hub (worktree root)
	if layout.WorktreePath() != filepath.Clean(hub) {
		t.Errorf("layout.WorktreePath() = %q; want %q", layout.WorktreePath(), filepath.Clean(hub))
	}

	// HubPath should be the parent of WorktreePath()
	expectedContainer := filepath.Dir(hub)
	if layout.HubPath != expectedContainer {
		t.Errorf("layout.HubPath = %q; want %q", layout.HubPath, expectedContainer)
	}

	// RepoName is derived by trimming HubSuffix off the container directory's base
	// name — this fixture's container has no "-HUB" suffix, so RepoName is simply
	// its base name unchanged.
	wantRepoName := strings.TrimSuffix(filepath.Base(layout.HubPath), lyxcwd.HubSuffix)
	if layout.RepoName != wantRepoName {
		t.Errorf("layout.RepoName = %q; want %q", layout.RepoName, wantRepoName)
	}
}

// TestResolve_FromSubdirectory verifies that, for an unanchored repo, Resolve
// from a subdirectory errors under the strict cwd gate: with no anchor
// recorded, AnchorRel is only ever ".", so cwd is only ever accepted at the
// worktree root itself, never in a subdirectory.
func TestResolve_FromSubdirectory(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Create a subdirectory structure
	subDir := filepath.Join(hub, "subdir", "nested")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	layout, err := lyxcwd.Resolve(subDir)
	if layout != nil {
		t.Errorf("Resolve(%q) returned non-nil layout; want nil", subDir)
	}
	if !errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
		t.Errorf("Resolve(%q) error = %v; want wrapped ErrCwdOutsideAnchor", subDir, err)
	}
}

// TestResolve_GeometryMethods verifies that geometry methods produce expected paths.
func TestResolve_GeometryMethods(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	// WorktreePath(slug) itself moved to fabricengine.WorktreePath(l, slug) in
	// this card; its own coverage now lives with fabricengine's retargeted
	// callers. The slug below is still needed by PortalTarget/LauncherDir.
	slug := "test-wt"

	// Test PortalsDir
	expectedPortalsDir := filepath.Join(layout.HubPath, "_portals")
	if got := layout.PortalsDir(); got != expectedPortalsDir {
		t.Errorf("PortalsDir() = %q; want %q", got, expectedPortalsDir)
	}

	// Test PortalTarget
	expectedPortalTarget := filepath.Join(layout.HubPath, slug, ".", "_lyx")
	if got := layout.PortalTarget(slug); got != expectedPortalTarget {
		t.Errorf("PortalTarget(%q) = %q; want %q", slug, got, expectedPortalTarget)
	}

	// Test LaunchersDir
	expectedLaunchersDir := filepath.Join(layout.HubPath, "_launchers")
	if got := layout.LaunchersDir(); got != expectedLaunchersDir {
		t.Errorf("LaunchersDir() = %q; want %q", got, expectedLaunchersDir)
	}

	// Test LauncherDir
	expectedLauncherDir := filepath.Join(expectedLaunchersDir, slug)
	if got := layout.LauncherDir(slug); got != expectedLauncherDir {
		t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, expectedLauncherDir)
	}

}

// TestResolve_ForwardSlashNormalization verifies that forward-slash output
// from --show-toplevel is reconciled with backslash cwd on Windows.
func TestResolve_ForwardSlashNormalization(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	// Call Resolve normally; both cwd and --show-toplevel output get normalized
	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	// Verify paths are clean and use the platform's separator
	if layout.WorktreePath() != filepath.Clean(hub) {
		t.Errorf("layout.WorktreePath() = %q; want %q", layout.WorktreePath(), filepath.Clean(hub))
	}
}

// TestResolve_NotAGitRepo verifies that Resolve in a non-git temp directory
// returns ErrNotAGitRepo.
func TestResolve_NotAGitRepo(t *testing.T) {
	t.Parallel()

	nonGitDir := t.TempDir()

	layout, err := lyxcwd.Resolve(nonGitDir)

	if layout != nil {
		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
	}

	if !errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
	}

	// Pin the bare-sentinel behavior: git's raw stderr must never leak into the
	// error text, and no other content may be appended to the sentinel message.
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Resolve() error = %q; must not contain raw git stderr (\"fatal:\")", err.Error())
	}
	if err.Error() != lyxcwd.ErrNotAGitRepo.Error() {
		t.Errorf("Resolve() error = %q; want exactly %q", err.Error(), lyxcwd.ErrNotAGitRepo.Error())
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

			layout, err := lyxcwd.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.HubPath, "_portals", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			// A hand-built Location at a nested AnchorRel, rather than a real
			// Resolve(subDir): under the strict cwd gate, Resolve only ever
			// succeeds at the recorded anchor itself, so exercising
			// PortalLink's subpath-mirroring here is pure path arithmetic on
			// a synthetic Location, matching pattern_test.go/weft_test.go.
			layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))

			slug := "test-slug"
			got := layout.PortalLink(slug)
			want := filepath.Join(layout.HubPath, "_portals", "services", "api", slug)
			if got != want {
				t.Errorf("PortalLink(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			layout1 := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))
			layout2 := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "web"))

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

			layout, err := lyxcwd.Resolve(hub)
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

			// A hand-built Location at a nested AnchorRel: see PortalLink's
			// "at subpath" sub-test above for why this replaces Resolve(subDir).
			layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))

			slug := "test-slug"
			got := layout.LauncherDir(slug)
			want := filepath.Join(layout.HubPath, "_launchers", "services", "api", slug)
			if got != want {
				t.Errorf("LauncherDir(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("no collision between different subpaths", func(t *testing.T) {
			t.Parallel()

			layout1 := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))
			layout2 := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "web"))

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

			layout, err := lyxcwd.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.HubPath, "_launchers", wantMenuLauncherName())
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))

			got := layout.MenuLauncherPath()
			want := filepath.Join(layout.HubPath, "_launchers", "services", "api", wantMenuLauncherName())
			if got != want {
				t.Errorf("MenuLauncherPath() = %q; want %q", got, want)
			}
		})
	})

	t.Run("LauncherSpawnRel", func(t *testing.T) {
		t.Parallel()

		t.Run("at root", func(t *testing.T) {
			t.Parallel()

			layout, err := lyxcwd.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			slug := "test-slug"
			got := layout.LauncherSpawnRel(slug)

			// Recompute expected via filepath.Rel
			launcherDir := layout.LauncherDir(slug)
			targetPath := filepath.Join(filepath.Join(layout.HubPath, slug), layout.AnchorRel)
			want, _ := filepath.Rel(launcherDir, targetPath)

			if got != want {
				t.Errorf("LauncherSpawnRel(%q) = %q; want %q", slug, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))

			slug := "test-slug"
			got := layout.LauncherSpawnRel(slug)

			// Recompute expected via filepath.Rel
			launcherDir := layout.LauncherDir(slug)
			targetPath := filepath.Join(filepath.Join(layout.HubPath, slug), layout.AnchorRel)
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

			layout, err := lyxcwd.Resolve(hub)
			if err != nil {
				t.Fatalf("Resolve() error = %v; want nil", err)
			}

			// primeName is the fixture's own main-worktree basename — CopyHostHub
			// creates a single-worktree fixture, so hub is its own prime — the
			// value MenuLauncherRel formerly read off layout.Prime, now sourced
			// explicitly since fabricengine.PrimeName owns that resolution.
			primeName := filepath.Base(hub)
			got := layout.MenuLauncherRel(primeName)

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.HubPath, primeName, layout.AnchorRel)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel(%q) = %q; want %q", primeName, got, want)
			}
		})

		t.Run("at subpath", func(t *testing.T) {
			t.Parallel()

			// primeName is the fixture's own main-worktree basename, same
			// reasoning as the "at root" sub-test above: hub's git worktree list
			// still resolves hub itself as the sole (Main) entry, regardless of
			// which subpath this synthetic Location is anchored at.
			layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))
			primeName := filepath.Base(hub)
			got := layout.MenuLauncherRel(primeName)

			// Recompute expected via filepath.Rel
			menuDir := filepath.Dir(layout.MenuLauncherPath())
			targetPath := filepath.Join(layout.HubPath, primeName, layout.AnchorRel)
			want, _ := filepath.Rel(menuDir, targetPath)

			if got != want {
				t.Errorf("MenuLauncherRel(%q) = %q; want %q", primeName, got, want)
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

	layout, err := lyxcwd.Resolve(hub)
	if err != nil {
		t.Fatalf("Resolve() error = %v; want nil", err)
	}

	t.Run("PortalTarget", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		got := layout.PortalTarget(slug)
		want := filepath.Join(layout.HubPath, slug, ".", "_lyx")

		if got != want {
			t.Errorf("PortalTarget(%q) = %q; want %q", slug, got, want)
		}
	})

	// WeftLyxDir and WeftLyxDirFor relocated to internal/fabricengine in this
	// batch; their coverage lives in fabricengine/weftpaths_test.go now — this
	// package cannot import fabricengine (fabricengine imports lyxcwd).

	t.Run("HostLyxLink", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		got := layout.HostLyxLink(slug)
		want := filepath.Join(filepath.Join(layout.HubPath, slug), ".", "_lyx")

		if got != want {
			t.Errorf("HostLyxLink(%q) = %q; want %q", slug, got, want)
		}
	})

	t.Run("HostLyxLinkHere", func(t *testing.T) {
		t.Parallel()

		got := layout.HostLyxLinkHere()
		want := filepath.Join(layout.WorktreePath(), ".", "_lyx")

		if got != want {
			t.Errorf("HostLyxLinkHere() = %q; want %q", got, want)
		}
	})

	t.Run("HostJunctions", func(t *testing.T) {
		t.Parallel()

		slug := "test-slug"
		junctions := layout.HostJunctions(slug, []string{"_lyx", "_pattern"})

		if len(junctions) != 2 {
			t.Fatalf("HostJunctions() returned %d junctions; want 2", len(junctions))
		}

		lyxJunction := junctions[0]
		if lyxJunction.Name != "_lyx" {
			t.Errorf("HostJunctions()[0].Name = %q; want %q", lyxJunction.Name, "_lyx")
		}
		if lyxJunction.Link != layout.HostLyxLink(slug) {
			t.Errorf("HostJunctions()[0].Link = %q; want %q", lyxJunction.Link, layout.HostLyxLink(slug))
		}
		wantLyxWeftTarget := filepath.Join(weftname.SiblingPath(layout.HubPath, slug), ".", configengine.LyxDirName)
		if lyxJunction.Target != wantLyxWeftTarget {
			t.Errorf("HostJunctions()[0].Target = %q; want %q", lyxJunction.Target, wantLyxWeftTarget)
		}

		patternJunction := junctions[1]
		if patternJunction.Name != "_pattern" {
			t.Errorf("HostJunctions()[1].Name = %q; want %q", patternJunction.Name, "_pattern")
		}
		if patternJunction.Link != layout.HostPatternLink(slug) {
			t.Errorf("HostJunctions()[1].Link = %q; want %q", patternJunction.Link, layout.HostPatternLink(slug))
		}
		if patternJunction.Target != layout.WeftPatternDirFor(slug) {
			t.Errorf("HostJunctions()[1].Target = %q; want %q", patternJunction.Target, layout.WeftPatternDirFor(slug))
		}
	})
}

// TestHostJunctionsHere verifies the Here-anchored, slug-free junction-detection
// accessor: it must return the expected Name/Link/Target for both RelPath == "." and a
// nested RelPath, and it must agree entry-for-entry with HostJunctions(slug) when the
// layout's slug and current worktree coincide — the precondition every one of
// fabricengine's health-check call sites relies on.
func TestHostJunctionsHere(t *testing.T) {
	t.Parallel()

	fix := lyxtest.CopyHostHub(t)
	hub := fix.Hub

	t.Run("at root", func(t *testing.T) {
		t.Parallel()

		layout, err := lyxcwd.Resolve(hub)
		if err != nil {
			t.Fatalf("Resolve() error = %v; want nil", err)
		}

		junctions := layout.HostJunctionsHere([]string{"_lyx", "_pattern"})
		if len(junctions) != 2 {
			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lyxJunction := junctions[0]
		wantLyxLink := layout.HostLyxLinkHere()
		wantLyxTarget := filepath.Join(weftname.SiblingPath(layout.HubPath, filepath.Base(layout.WorktreePath())), layout.AnchorRel, configengine.LyxDirName)
		if lyxJunction.Name != "_lyx" {
			t.Errorf("HostJunctionsHere()[0].Name = %q; want %q", lyxJunction.Name, "_lyx")
		}
		if lyxJunction.Link != wantLyxLink {
			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
		}
		if lyxJunction.Target != wantLyxTarget {
			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
		}

		patternJunction := junctions[1]
		wantPatternLink := layout.HostPatternLinkHere()
		wantPatternTarget := layout.WeftPatternDir()
		if patternJunction.Name != "_pattern" {
			t.Errorf("HostJunctionsHere()[1].Name = %q; want %q", patternJunction.Name, "_pattern")
		}
		if patternJunction.Link != wantPatternLink {
			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
		}
		if patternJunction.Target != wantPatternTarget {
			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
		}
	})

	t.Run("at nested subpath", func(t *testing.T) {
		t.Parallel()

		// A hand-built Location at a nested AnchorRel, rather than a real
		// Resolve(subDir): under the strict cwd gate, Resolve only ever
		// succeeds at the recorded anchor itself.
		layout := newTestLocation(filepath.Dir(hub), hub, filepath.Join("services", "api"))

		junctions := layout.HostJunctionsHere([]string{"_lyx", "_pattern"})
		if len(junctions) != 2 {
			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
		}

		lyxJunction := junctions[0]
		wantLyxLink := layout.HostLyxLinkHere()
		wantLyxTarget := filepath.Join(weftname.SiblingPath(layout.HubPath, filepath.Base(layout.WorktreePath())), layout.AnchorRel, configengine.LyxDirName)
		if lyxJunction.Link != wantLyxLink {
			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
		}
		if lyxJunction.Target != wantLyxTarget {
			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
		}

		patternJunction := junctions[1]
		wantPatternLink := layout.HostPatternLinkHere()
		wantPatternTarget := layout.WeftPatternDir()
		if patternJunction.Link != wantPatternLink {
			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
		}
		if patternJunction.Target != wantPatternTarget {
			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
		}
	})

	t.Run("agrees with HostJunctions when slug matches current worktree", func(t *testing.T) {
		t.Parallel()

		layout, err := lyxcwd.Resolve(hub)
		if err != nil {
			t.Fatalf("Resolve() error = %v; want nil", err)
		}

		// The current worktree's own base name is the slug that makes HostJunctions(slug)
		// resolve to the same host worktree HostJunctionsHere() is already anchored at.
		slug := filepath.Base(layout.WorktreePath())
		names := []string{"_lyx", "_pattern"}

		here := layout.HostJunctionsHere(names)
		bySlug := layout.HostJunctions(slug, names)

		if len(here) != len(bySlug) {
			t.Fatalf("HostJunctionsHere() returned %d junctions; HostJunctions(%q) returned %d", len(here), slug, len(bySlug))
		}
		for i := range here {
			if here[i] != bySlug[i] {
				t.Errorf("HostJunctionsHere()[%d] = %+v; HostJunctions(%q)[%d] = %+v", i, here[i], slug, i, bySlug[i])
			}
		}
	})

	t.Run("names ordering regressions", func(t *testing.T) {
		t.Parallel()

		layout, err := lyxcwd.Resolve(hub)
		if err != nil {
			t.Fatalf("Resolve() error = %v; want nil", err)
		}

		regressionTests := []struct {
			name  string
			names []string
		}{
			{name: "empty names yields zero records", names: []string{}},
			{name: "3-name slice yields three records in input order", names: []string{"_lyx", "_pattern", "_extra"}},
			{name: "reversed 2-name slice preserves given order, no forced sort", names: []string{"_pattern", "_lyx"}},
		}

		for _, rt := range regressionTests {
			t.Run(rt.name, func(t *testing.T) {
				junctions := layout.HostJunctionsHere(rt.names)
				if len(junctions) != len(rt.names) {
					t.Fatalf("HostJunctionsHere(%v) returned %d entries; want %d", rt.names, len(junctions), len(rt.names))
				}
				for i, wantName := range rt.names {
					got := junctions[i]
					if got.Name != wantName {
						t.Errorf("HostJunctionsHere(%v)[%d].Name = %q; want %q", rt.names, i, got.Name, wantName)
					}
					wantLink := filepath.Join(layout.WorktreePath(), layout.AnchorRel, wantName)
					if got.Link != wantLink {
						t.Errorf("HostJunctionsHere(%v)[%d].Link = %q; want %q", rt.names, i, got.Link, wantLink)
					}
					wantTarget := filepath.Join(weftname.SiblingPath(layout.HubPath, filepath.Base(layout.WorktreePath())), layout.AnchorRel, wantName)
					if got.Target != wantTarget {
						t.Errorf("HostJunctionsHere(%v)[%d].Target = %q; want %q", rt.names, i, got.Target, wantTarget)
					}
				}
			})
		}
	})
}

// TestIsReservedHubName_Pattern pins _pattern into the reserved-name set alongside
// _lyx, _raddle, _board, _portals, and _launchers (see geometry_test.go's
// TestIsReservedHubName for the full table): a worktree slug must never claim the
// PATTERN constraint-injection surface's directory name.
func TestIsReservedHubName_Pattern(t *testing.T) {
	t.Parallel()

	if got := lyxcwd.IsReservedHubName("_pattern", []string{"_lyx", "_pattern"}); !got {
		t.Errorf("IsReservedHubName(%q, %v) = %v; want true", "_pattern", []string{"_lyx", "_pattern"}, got)
	}
}
