// ancestors_test.go tests the pruneEmptyAncestors helper.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPruneEmptyAncestors tests the pruneEmptyAncestors helper with various scenarios:
// empty ancestors removed to stop; non-empty dirs halt the walk; start == stop is a no-op;
// and idempotency on already-pruned trees.
func TestPruneEmptyAncestors(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		setup  func(t *testing.T, tempDir string) (start, stop string)
		verify func(t *testing.T, tempDir string, start, stop string)
	}{
		{
			name: "PruneEmptyAncestorsUpToStop",
			setup: func(t *testing.T, tempDir string) (start, stop string) {
				// Create a tree: tempDir/_portals/a/b/c/slug
				// start = tempDir/_portals/a/b/c/slug
				// stop = tempDir/_portals
				// Expected: a/b/c/slug removed; _portals remains
				stop = filepath.Join(tempDir, "_portals")
				start = filepath.Join(stop, "a", "b", "c", "slug")
				if err := os.MkdirAll(start, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return
			},
			verify: func(t *testing.T, tempDir string, start, stop string) {
				if _, err := os.Stat(start); err == nil {
					t.Error("start dir still exists")
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat start: %v", err)
				}

				for _, subpath := range []string{"a", "a/b", "a/b/c"} {
					dir := filepath.Join(stop, subpath)
					if _, err := os.Stat(dir); err == nil {
						t.Errorf("intermediate dir %s still exists", subpath)
					} else if !os.IsNotExist(err) {
						t.Fatalf("stat %s: %v", subpath, err)
					}
				}

				if _, err := os.Stat(stop); err != nil {
					t.Errorf("stop dir was removed: %v", err)
				}
			},
		},
		{
			name: "NonEmptyIntermediateHaltsWalk",
			setup: func(t *testing.T, tempDir string) (start, stop string) {
				// Create a tree: tempDir/_portals/a/b/c/slug with a file in a/b
				// start = tempDir/_portals/a/b/c/slug
				// stop = tempDir/_portals
				// Expected: c/slug removed, but a/b survives because it has a file
				stop = filepath.Join(tempDir, "_portals")
				start = filepath.Join(stop, "a", "b", "c", "slug")
				if err := os.MkdirAll(start, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				// Add a file to a/b to make it non-empty
				filePath := filepath.Join(stop, "a", "b", "keepme.txt")
				if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}

				return
			},
			verify: func(t *testing.T, tempDir string, start, stop string) {
				if _, err := os.Stat(start); err == nil {
					t.Error("start dir still exists")
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat start: %v", err)
				}

				// Check that a/b/c is gone (c is empty after slug is removed)
				cDir := filepath.Join(stop, "a", "b", "c")
				if _, err := os.Stat(cDir); err == nil {
					t.Error("a/b/c still exists")
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat c: %v", err)
				}

				// Check that a/b survives (it has keepme.txt)
				bDir := filepath.Join(stop, "a", "b")
				if _, err := os.Stat(bDir); err != nil {
					t.Errorf("a/b was removed: %v", err)
				}

				// Check that a remains (it's not empty because it contains b)
				aDir := filepath.Join(stop, "a")
				if _, err := os.Stat(aDir); err != nil {
					t.Errorf("a was removed: %v", err)
				}
			},
		},
		{
			name: "StartEqualStopIsNoop",
			setup: func(t *testing.T, tempDir string) (start, stop string) {
				// Create a tree and then try to prune from the root itself
				stop = filepath.Join(tempDir, "_portals")
				if err := os.MkdirAll(stop, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				start = stop
				return
			},
			verify: func(t *testing.T, tempDir string, start, stop string) {
				if _, err := os.Stat(stop); err != nil {
					t.Errorf("stop dir was removed: %v", err)
				}
			},
		},
		{
			name: "IdempotentOnAlreadyPruned",
			setup: func(t *testing.T, tempDir string) (start, stop string) {
				// Create a tree, prune it, then prune again
				stop = filepath.Join(tempDir, "_portals")
				start = filepath.Join(stop, "a", "b", "c")
				if err := os.MkdirAll(start, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				pruneEmptyAncestors(start, stop)

				return
			},
			verify: func(t *testing.T, tempDir string, start, stop string) {
				pruneEmptyAncestors(start, stop)

				if _, err := os.Stat(stop); err != nil {
					t.Errorf("stop dir was removed on second prune: %v", err)
				}

				if _, err := os.Stat(start); err == nil {
					t.Error("start dir still exists after second prune")
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat start on second prune: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			start, stop := tt.setup(t, tempDir)
			pruneEmptyAncestors(start, stop)
			tt.verify(t, tempDir, start, stop)
		})
	}
}

// TestRefuseUncontainedPath asserts that a path escaping its container is refused, including paths derived from "..".
//
//testtiming:keep the containment assertion shared by the launcher teardown helpers refusing ".."-derived paths that once let `lym fabric remove ..` delete a hub; coverage of its blocks by other tests does not show an assertion of this
func TestRefuseUncontainedPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const container = "/hub/_launchers"

	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"direct child", "/hub/_launchers/my-task", false},
		{"nested child", "/hub/_launchers/backend/my-task", false},
		{"the container itself", "/hub/_launchers", true},
		{"the container's parent", "/hub", true},
		{"escaping sibling", "/hub/_portals/my-task", true},
		{"unrelated absolute path", "/etc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := refuseUncontainedPath(container, filepath.Clean(tt.target), "launcher dir")
			if tt.wantErr && err == nil {
				t.Errorf("refuseUncontainedPath(%q, %q) = nil; want an error", container, tt.target)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("refuseUncontainedPath(%q, %q) = %v; want nil", container, tt.target, err)
			}
		})
	}
}

// TestPruneEmptyAncestors_RefusesEscapingIntermediate proves the sweep's removal is rooted at stop
// rather than acting on the nominal path.
//
// A symlink planted at an intermediate component must not carry the removal out of the hub.
// The attacker-plantable containers are the hub-level <hub>/_launchers and <hub>/_portals.
// The OS refuses to remove a non-empty directory, so only an empty out-of-container directory can be destroyed;
// the victim below is empty, so a nominal removal would succeed on it.
// This assertion is therefore real rather than a vacuous OS refusal.
//
//testtiming:keep the empty-ancestor sweep refusing a symlinked intermediate that escapes the hub, with an empty out-of-container victim that survives; coverage of its blocks by other tests does not show an assertion of this
func TestPruneEmptyAncestors_RefusesEscapingIntermediate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stop := t.TempDir()
	outside := t.TempDir()

	victim := filepath.Join(outside, "api")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(stop, "services")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	// start is <stop>/services/api, whose `services` component escapes the container.
	pruneEmptyAncestors(filepath.Join(stop, "services", "api"), stop)

	if _, err := os.Stat(victim); err != nil {
		t.Errorf("pruneEmptyAncestors() destroyed the out-of-container directory at %s: %v; want it untouched", victim, err)
	}
	if _, err := os.Lstat(filepath.Join(stop, "services")); err != nil {
		t.Errorf("pruneEmptyAncestors() removed the planted link at %s: %v; want the walk to halt on the refusal instead", filepath.Join(stop, "services"), err)
	}
}
