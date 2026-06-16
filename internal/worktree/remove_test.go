// remove_test.go covers normal teardown and the case where the worktree dir is
// gone but the portal/launcher are still cleaned up before the not-found return.

package worktree_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestRemove covers worktree teardown: clean removal, the dirty-tree gate with
// and without --force, a missing slug, and symlink/junction cleanup counting.
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
				mustRun(t, hub, "git", "worktree", "add", filepath.Join(filepath.Dir(hub), slug))
			},
			wantLinksRemoved: 0,
			linksExact:       true,
			dirAfter:         "removed",
		},
		{
			name: "DirtyWithoutForce",
			setup: func(t *testing.T, hub, slug string) {
				target := filepath.Join(filepath.Dir(hub), slug)
				mustRun(t, hub, "git", "worktree", "add", target)
				// An untracked file makes the worktree dirty so the gate trips.
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file: %v", err)
				}
			},
			wantErr:  true,
			dirAfter: "exists",
		},
		{
			name: "DirtyWithForce",
			setup: func(t *testing.T, hub, slug string) {
				target := filepath.Join(filepath.Dir(hub), slug)
				mustRun(t, hub, "git", "worktree", "add", target)
				if err := os.WriteFile(filepath.Join(target, "untracked.txt"), []byte("untracked"), 0644); err != nil {
					t.Fatalf("create untracked file: %v", err)
				}
			},
			force:            true,
			wantLinksRemoved: 0,
			dirAfter:         "removed",
		},
		{
			name:    "NonexistentSlug",
			slug:    "ghost",
			setup:   func(t *testing.T, hub, slug string) {}, // nothing created
			wantErr: true,
		},
		{
			name: "WithSymlink",
			setup: func(t *testing.T, hub, slug string) {
				target := filepath.Join(filepath.Dir(hub), slug)
				mustRun(t, hub, "git", "worktree", "add", target)
				targetFile := filepath.Join(target, "target.txt")
				if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
					t.Fatalf("create symlink target: %v", err)
				}
				if err := os.Symlink(targetFile, filepath.Join(target, "my-symlink")); err != nil {
					t.Skipf("symlinks not permitted on this platform: %v", err)
				}
			},
			wantLinksRemoved: 1,
			dirAfter:         "removed",
		},
		{
			name: "TeardownBeforeMissingDir",
			slug: "missing-dir",
			setup: func(t *testing.T, hub, slug string) {
				// Pre-create a portal by simulating what a created worktree would have.
				// Since we can't directly call createPortal (private), we create
				// the directory structure that Remove expects to clean up.
				l, err := paths.Resolve(hub)
				if err != nil {
					t.Fatalf("paths.Resolve(%q): %v", hub, err)
				}

				// Create the portal link target (_lyx directory)
				targetParent := filepath.Join(l.Container, slug, l.RelPath)
				if err := os.MkdirAll(targetParent, 0755); err != nil {
					t.Fatalf("mkdir target parent: %v", err)
				}
				targetDir := filepath.Join(targetParent, "_lyx")
				if err := os.Mkdir(targetDir, 0755); err != nil {
					t.Fatalf("mkdir target _lyx: %v", err)
				}

				// Create the portal junction itself.
				// We'll create a simple directory as a placeholder for the portal.
				portalsDir := l.PortalsDir()
				if err := os.MkdirAll(portalsDir, 0755); err != nil {
					t.Fatalf("mkdir portals dir: %v", err)
				}
				portalLink := filepath.Join(portalsDir, slug)
				// Note: we can't create a real junction without the private createJunction function,
				// so we create a marker file that Remove will attempt to clean.
				if err := os.WriteFile(portalLink, []byte("portal"), 0644); err != nil {
					t.Fatalf("create portal marker: %v", err)
				}

				// Now delete the worktree directory itself (not the _lyx target, just the worktree root)
				worktreeDir := filepath.Join(l.Container, slug)
				if err := os.RemoveAll(worktreeDir); err != nil {
					t.Fatalf("remove worktree dir: %v", err)
				}
			},
			wantErr: true, // will fail because dir doesn't exist, but portal cleanup should still run
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

			w := worktree.New(worktree.Config{})
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
			case "exists":
				if _, statErr := os.Stat(target); statErr != nil {
					t.Errorf("Remove(%q): %q missing; want still present: %v", slug, target, statErr)
				}
			}

			// For TeardownBeforeMissingDir test, verify the portal was cleaned up
			if tt.name == "TeardownBeforeMissingDir" {
				portalLink := filepath.Join(l.PortalsDir(), slug)
				if _, statErr := os.Stat(portalLink); !os.IsNotExist(statErr) {
					t.Errorf("Remove(%q): portal %q still exists; want removed", slug, portalLink)
				}
			}
		})
	}
}
