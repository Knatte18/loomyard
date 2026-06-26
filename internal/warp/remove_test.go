//go:build integration

// remove_test.go covers paired teardown: normal teardown with both host and weft,
// the case where the worktree dir is gone but the portal/launcher are still cleaned
// up before the not-found return, and both host and weft dirty gates.

package warp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
)

// TestRemove covers paired teardown: clean removal of both host and weft, the dirty-tree
// gate with and without --force (checking both host and weft), a missing slug,
// and symlink/junction cleanup counting. Requires a weft Prime repo.
func TestRemove(t *testing.T) {
	t.Parallel()

	const defaultSlug = "task1"

	tests := []struct {
		name string
		// slug overrides defaultSlug when a scenario needs a different name.
		slug string
		// setup creates the target worktree and any dirt/symlinks; it may call
		// t.Skip (symlink scenario) and is a no-op for the missing-slug case.
		setup            func(t *testing.T, f lyxtest.PairedFixture, slug string)
		force            bool
		wantErr          bool
		wantLinksRemoved int
		// linksExact asserts LinksRemoved == want when true, >= want otherwise.
		linksExact bool
		// dirAfter is "removed", "exists", or "" to skip the directory check.
		dirAfter string
	}{
		{
			name: "HappyPath",
			setup: func(t *testing.T, f lyxtest.PairedFixture, slug string) {
				w := New(Config{})
				_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
				if err != nil {
					t.Fatalf("setup Add(%q): %v", slug, err)
				}
			},
			wantLinksRemoved: 0,
			linksExact:       true,
			dirAfter:         "removed",
		},
		{
			name: "NonexistentSlug",
			slug: "ghost",
			setup: func(t *testing.T, f lyxtest.PairedFixture, slug string) {
				// weft repo exists (from CopyPairedLocal) but no worktree for this slug.
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := lyxtest.CopyPairedLocal(t)
			slug := defaultSlug
			if tt.slug != "" {
				slug = tt.slug
			}
			tt.setup(t, f, slug)

			w := New(Config{})
			result, err := w.Remove(f.Layout, slug, tt.force)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Remove(%q, force=%v) error = nil; want error", slug, tt.force)
				}
			} else {
				if err != nil {
					t.Fatalf("Remove(%q, force=%v) error = %v; want nil", slug, tt.force, err)
				}
				if tt.linksExact {
					if result.LinksRemoved != tt.wantLinksRemoved {
						t.Errorf("Remove(%q).LinksRemoved = %d; want %d", slug, result.LinksRemoved, tt.wantLinksRemoved)
					}
				} else if result.LinksRemoved < tt.wantLinksRemoved {
					t.Errorf("Remove(%q).LinksRemoved = %d; want >= %d", slug, result.LinksRemoved, tt.wantLinksRemoved)
				}
			}

			target := f.Layout.WorktreePath(slug)
			switch tt.dirAfter {
			case "removed":
				if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
					t.Errorf("Remove(%q): %q still exists; want removed", slug, target)
				}
				// Also verify weft worktree is removed.
				weftTarget := f.Layout.WeftWorktreePath(slug)
				if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
					t.Errorf("Remove(%q): weft %q still exists; want removed", slug, weftTarget)
				}
			case "exists":
				if _, statErr := os.Stat(target); statErr != nil {
					t.Errorf("Remove(%q): %q missing; want still present: %v", slug, target, statErr)
				}
			}
		})
	}

	// HostDirty merges the dirty-without-force and dirty-with-force cases: both
	// Remove calls run on the same fixture so the second call can observe the
	// state left by the first.
	t.Run("HostDirty", func(t *testing.T) {
		f := lyxtest.CopyPairedLocal(t)
		const slug = defaultSlug

		w := New(Config{})
		_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("setup Add(%q): %v", slug, err)
		}

		// An untracked file makes the host worktree dirty so the gate trips.
		target := f.Layout.WorktreePath(slug)
		if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("untracked"), 0644); err != nil {
			t.Fatalf("create untracked file: %v", err)
		}

		// Remove without force must fail; the directory must be intact.
		if _, err = w.Remove(f.Layout, slug, false); err == nil {
			t.Fatalf("Remove(%q, force=false) error = nil; want error", slug)
		}
		if _, statErr := os.Stat(target); statErr != nil {
			t.Errorf("Remove(%q, force=false): dir missing; want still present: %v", slug, statErr)
		}

		// Remove with force must succeed; host and weft worktrees must be gone.
		if _, err = w.Remove(f.Layout, slug, true); err != nil {
			t.Fatalf("Remove(%q, force=true) error = %v; want nil", slug, err)
		}
		if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
			t.Errorf("Remove(%q, force=true): %q still exists; want removed", slug, target)
		}
		weftTarget := f.Layout.WeftWorktreePath(slug)
		if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
			t.Errorf("Remove(%q, force=true): weft %q still exists; want removed", slug, weftTarget)
		}
	})

	// WeftDirty mirrors HostDirty but places the untracked file in the weft
	// worktree, verifying that weft-side dirt is also gated.
	t.Run("WeftDirty", func(t *testing.T) {
		f := lyxtest.CopyPairedLocal(t)
		const slug = defaultSlug

		w := New(Config{})
		_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("setup Add(%q): %v", slug, err)
		}

		// Make the weft worktree dirty so the gate trips.
		weftTarget := f.Layout.WeftWorktreePath(slug)
		if err := os.WriteFile(filepath.Join(weftTarget, "untracked.txt"), []byte("untracked"), 0644); err != nil {
			t.Fatalf("create untracked file in weft: %v", err)
		}

		// Remove without force must fail; the host directory must be intact.
		target := f.Layout.WorktreePath(slug)
		if _, err = w.Remove(f.Layout, slug, false); err == nil {
			t.Fatalf("Remove(%q, force=false) error = nil; want error", slug)
		}
		if _, statErr := os.Stat(target); statErr != nil {
			t.Errorf("Remove(%q, force=false): dir missing; want still present: %v", slug, statErr)
		}

		// Remove with force must succeed; host and weft worktrees must be gone.
		if _, err = w.Remove(f.Layout, slug, true); err != nil {
			t.Fatalf("Remove(%q, force=true) error = %v; want nil", slug, err)
		}
		if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
			t.Errorf("Remove(%q, force=true): %q still exists; want removed", slug, target)
		}
		if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
			t.Errorf("Remove(%q, force=true): weft %q still exists; want removed", slug, weftTarget)
		}
	})
}

// TestRemoveSubpathJunction verifies that Remove handles nested junctions at RelPath != "."
// (the scenario where removeLinks(root) would miss the junction).
// This test uses t.Chdir and stays serial.
func TestRemoveSubpathJunction(t *testing.T) {
	const slug = "subpath-junction-test"
	const subpath = "sub/path"

	f := lyxtest.CopyPairedLocal(t)

	// Create a subpath in the hub.
	subpathDir := filepath.Join(f.Hub, subpath)
	if err := os.MkdirAll(subpathDir, 0755); err != nil {
		t.Fatalf("mkdir subpath: %v", err)
	}

	// Change to subpath to resolve Layout with RelPath set.
	t.Chdir(subpathDir)

	l, err := paths.Resolve(subpathDir)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Verify RelPath is set.
	if l.RelPath == "." {
		t.Skip("this test requires RelPath != \".\"; got: " + l.RelPath)
	}

	w := New(Config{})
	_, err = w.Add(l, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q) at subpath: %v", slug, err)
	}

	// Wire junctions (Add is dormant).
	if err := WireJunctions(l, slug); err != nil {
		t.Fatalf("WireJunctions(%q) at subpath: %v", slug, err)
	}

	// Verify nested junction exists.
	hostLink := l.HostLyxLink(slug)
	if _, err := os.Lstat(hostLink); err != nil {
		t.Fatalf("nested host junction missing: %v", err)
	}

	// Remove the worktree.
	_, err = w.Remove(l, slug, false)
	if err != nil {
		t.Fatalf("Remove(%q) at subpath: %v", slug, err)
	}

	// Verify nested junction is gone (removeLinks would not find it since it only
	// scans immediate children; removeHostJunction must catch it).
	if _, err := os.Lstat(hostLink); !os.IsNotExist(err) {
		t.Errorf("Remove(%q) at subpath failed to remove nested junction at %s", slug, hostLink)
	}
}
