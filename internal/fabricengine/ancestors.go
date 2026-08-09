// ancestors.go implements the empty-directory sweeper that prunes empty ancestors of removed
// worktrees and portals, and the containment assertion its destructive callers share.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refuseUncontainedPath returns an error unless target lies strictly below container.
//
// It guards the teardown helpers that delete a path derived by joining a caller-supplied slug onto
// hub geometry.
// Slug validation is the primary defence and catches every input this repo can produce today, but a
// derived path that has escaped its container is being handed to os.RemoveAll, so the cost of
// asserting it is nothing against the cost of being wrong once.
func refuseUncontainedPath(container, target, what string) error {
	rel, err := filepath.Rel(container, target)
	if err != nil {
		return fmt.Errorf("refusing to remove %s: cannot relate %s to %s: %w", what, target, container, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove %s: %s is not inside %s", what, target, container)
	}
	return nil
}

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
