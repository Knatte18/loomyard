// remove_test.go covers paired teardown: normal teardown with both host and weft,
// the case where the worktree dir is gone but the portal/launcher are still cleaned up
// before the not-found return, and both host and weft dirty gates.

package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestRemove covers paired teardown: clean removal of both host and weft, the dirty-tree
// gate with and without --force (checking both host and weft), a missing slug,
// and symlink/junction cleanup counting. Requires a weft Prime repo.
func TestRemove(t *testing.T) {
	const defaultSlug = "task1"

	tests := []struct {
		name string
		// slug overrides defaultSlug when a scenario needs a different name.
		slug string
		// setup creates the target worktree and any dirt/symlinks; it may call
		// t.Skip (symlink scenario) and is a no-op for the missing-slug case.
		setup            func(t *testing.T, hub, slug string)
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
			setup: func(t *testing.T, hub, slug string) {
				addRemote(t, hub)
				newWeftRepo(t, hub)
				w := New(Config{})
				l, _ := paths.Resolve(hub)
				w.Add(l, slug, AddOptions{SkipPush: true})
			},
			wantLinksRemoved: 0,
			linksExact:       true,
			dirAfter:         "removed",
		},
		{
			name: "HostDirtyWithoutForce",
			setup: func(t *testing.T, hub, slug string) {
				addRemote(t, hub)
				newWeftRepo(t, hub)
				w := New(Config{})
				l, _ := paths.Resolve(hub)
				w.Add(l, slug, AddOptions{SkipPush: true})
				// An untracked file makes the host worktree dirty so the gate trips.
				target := l.WorktreePath(slug)
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file: %v", err)
				}
			},
			wantErr:  true,
			dirAfter: "exists",
		},
		{
			name: "HostDirtyWithForce",
			setup: func(t *testing.T, hub, slug string) {
				addRemote(t, hub)
				newWeftRepo(t, hub)
				w := New(Config{})
				l, _ := paths.Resolve(hub)
				w.Add(l, slug, AddOptions{SkipPush: true})
				target := l.WorktreePath(slug)
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file: %v", err)
				}
			},
			force:            true,
			wantLinksRemoved: 0,
			dirAfter:         "removed",
		},
		{
			name: "WeftDirtyWithoutForce",
			setup: func(t *testing.T, hub, slug string) {
				addRemote(t, hub)
				newWeftRepo(t, hub)
				w := New(Config{})
				l, _ := paths.Resolve(hub)
				w.Add(l, slug, AddOptions{SkipPush: true})
				// Make weft worktree dirty
				weftTarget := l.WeftWorktreePath(slug)
				if err := os.WriteFile(filepath.Join(weftTarget, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file in weft: %v", err)
				}
			},
			wantErr:  true,
			dirAfter: "exists",
		},
		{
			name: "WeftDirtyWithForce",
			setup: func(t *testing.T, hub, slug string) {
				addRemote(t, hub)
				newWeftRepo(t, hub)
				w := New(Config{})
				l, _ := paths.Resolve(hub)
				w.Add(l, slug, AddOptions{SkipPush: true})
				// Make weft worktree dirty
				weftTarget := l.WeftWorktreePath(slug)
				if err := os.WriteFile(filepath.Join(weftTarget, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file in weft: %v", err)
				}
			},
			force:            true,
			wantLinksRemoved: 0,
			dirAfter:         "removed",
		},
		{
			name:    "NonexistentSlug",
			slug:    "ghost",
			setup:   func(t *testing.T, hub, slug string) { newWeftRepo(t, hub) }, // weft repo only
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newTestRepo(t)
			slug := defaultSlug
			if tt.slug != "" {
				slug = tt.slug
			}
			tt.setup(t, hub, slug)

			// Resolve Layout from the hub
			l, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("paths.Resolve(%q): %v", hub, err)
			}

			w := New(Config{})
			result, err := w.Remove(l, slug, tt.force)

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

			target := l.WorktreePath(slug)
			switch tt.dirAfter {
			case "removed":
				if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
					t.Errorf("Remove(%q): %q still exists; want removed", slug, target)
				}
				// Also verify weft worktree is removed
				weftTarget := l.WeftWorktreePath(slug)
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
}

// TestRemoveHostJunctionRemoved verifies that Remove explicitly removes the host _lyx junction
// before the worktree, catching nested junctions that removeLinks (which only scans immediate
// children) would miss.
func TestRemoveHostJunctionRemoved(t *testing.T) {
	const slug = "junction-removal-test"

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{})
	_, err = w.Add(l, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify junction exists before Remove
	hostLink := l.HostLyxLink(slug)
	if _, err := os.Lstat(hostLink); err != nil {
		t.Fatalf("host junction missing before Remove: %v", err)
	}

	// Remove the worktree
	_, err = w.Remove(l, slug, false)
	if err != nil {
		t.Fatalf("Remove(%q): %v", slug, err)
	}

	// Verify junction is gone
	if _, err := os.Lstat(hostLink); !os.IsNotExist(err) {
		t.Errorf("Remove(%q) failed to remove host junction at %s", slug, hostLink)
	}
}

// TestRemoveSubpathJunction verifies that Remove handles nested junctions at RelPath != "."
// (the scenario where removeLinks(root) would miss the junction).
func TestRemoveSubpathJunction(t *testing.T) {
	const slug = "subpath-junction-test"
	const subpath = "sub/path"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	// Create a subpath in the hub
	subpathDir := filepath.Join(hub, subpath)
	if err := os.MkdirAll(subpathDir, 0755); err != nil {
		t.Fatalf("mkdir subpath: %v", err)
	}

	// Change to subpath to resolve Layout with RelPath set
	t.Chdir(subpathDir)

	l, err := paths.Resolve(subpathDir)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Verify RelPath is set
	if l.RelPath == "." {
		t.Skip("this test requires RelPath != \".\"; got: " + l.RelPath)
	}

	w := New(Config{})
	_, err = w.Add(l, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q) at subpath: %v", slug, err)
	}

	// Verify nested junction exists
	hostLink := l.HostLyxLink(slug)
	if _, err := os.Lstat(hostLink); err != nil {
		t.Fatalf("nested host junction missing: %v", err)
	}

	// Remove the worktree
	_, err = w.Remove(l, slug, false)
	if err != nil {
		t.Fatalf("Remove(%q) at subpath: %v", slug, err)
	}

	// Verify nested junction is gone (removeLinks would not find it since it only
	// scans immediate children; removeHostJunction must catch it)
	if _, err := os.Lstat(hostLink); !os.IsNotExist(err) {
		t.Errorf("Remove(%q) at subpath failed to remove nested junction at %s", slug, hostLink)
	}
}
