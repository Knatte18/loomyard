// ancestors.go implements the empty-directory sweeper that prunes empty ancestors
// of removed worktrees and portals. This is a rename from the worktree package's prune.go
// to avoid collision with the future prune verb in batch 7.

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
)

// pruneEmptyAncestors walks upward from start, removing empty directories up to
// (but not including) stop. It is a void, best-effort helper: all errors
// (filesystem failures, boundary violations) are silently swallowed.
//
// The boundary guard is checked at the top of each iteration: if cur is not
// strictly under stop (checked by filepath.Rel), the walk halts without
// attempting removal. This ensures stop is never a removal candidate.
//
// On each iteration:
//  1. Compute rel, err := filepath.Rel(stop, cur)
//  2. If err != nil, rel == ".", or rel starts with "..", halt the walk
//  3. Otherwise, attempt os.Remove(cur) (succeeds only if empty)
//  4. On success, set cur = filepath.Dir(cur) and continue
//  5. On any error, halt and return
//
// The helper is idempotent: calling it on an already-pruned tree is safe.
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
