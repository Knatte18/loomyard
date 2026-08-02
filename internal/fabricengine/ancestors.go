// ancestors.go implements the empty-directory sweeper that prunes empty ancestors
// of removed worktrees and portals.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
)

// pruneEmptyAncestors walks upward from start, removing empty directories up to
// (but not including) stop. All errors are silently swallowed. The helper is
// idempotent: calling it on an already-pruned tree is safe.
func pruneEmptyAncestors(start, stop string) {
	cur := start

	for {
		// Boundary guard: check if cur is still strictly under stop
		rel, err := filepath.Rel(stop, cur)
		if err != nil {
			// Filesystem error on Rel (rare)
			return
		}

		// Check if cur is the stop dir or above it (outside the target subtree)
		if rel == "." || strings.HasPrefix(rel, "..") {
			// We've reached or passed the boundary; halt
			return
		}

		// Attempt to remove the empty directory
		if err := os.Remove(cur); err != nil {
			// Directory is not empty, already gone, or other error; halt
			return
		}

		// Successfully removed; move to parent
		cur = filepath.Dir(cur)
	}
}
