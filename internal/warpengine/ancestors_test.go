// ancestors_test.go tests the pruneEmptyAncestors helper.

package warpengine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPruneEmptyAncestors tests the pruneEmptyAncestors helper.
//
// Test cases:
//   - Empty mirrored ancestors are removed up to but not including the stop dir
//   - A non-empty intermediate dir halts the walk (dirs above it survive)
//   - Calling with start == stop is a no-op
//   - The helper is idempotent on an already-pruned tree
func TestPruneEmptyAncestors(t *testing.T) {
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
				// Check that start and its ancestors are gone up to stop
				if _, err := os.Stat(start); err == nil {
					t.Error("start dir still exists")
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat start: %v", err)
				}

				// Check intermediate dirs are gone
				for _, subpath := range []string{"a", "a/b", "a/b/c"} {
					dir := filepath.Join(stop, subpath)
					if _, err := os.Stat(dir); err == nil {
						t.Errorf("intermediate dir %s still exists", subpath)
					} else if !os.IsNotExist(err) {
						t.Fatalf("stat %s: %v", subpath, err)
					}
				}

				// Check that stop itself remains
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
				// Check that start/c/slug are gone
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
				// Check that stop still exists (was not removed)
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

				// First prune
				pruneEmptyAncestors(start, stop)

				return
			},
			verify: func(t *testing.T, tempDir string, start, stop string) {
				// Run prune again on the already-pruned tree
				pruneEmptyAncestors(start, stop)

				// Verify stop still exists
				if _, err := os.Stat(stop); err != nil {
					t.Errorf("stop dir was removed on second prune: %v", err)
				}

				// Verify start is still gone
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
