// warplayout.go derives the Location of each warp worktree from the caller's already-resolved Location.
// It avoids spawning git for a hub sibling of that Location.
// Any other worktree goes through the gate-free lymcwd.ResolveWorktree, not the gated Resolve.
// That resolver derives another worktree's geometry from its root, above any subpath anchor.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// warpLayoutFor returns the per-warp-worktree Location at that worktree's own anchor.
// A hub sibling is named through lymcwd.NamedWorktree without spawning git;
// a worktree outside the hub falls back to the spawning, gate-free lymcwd.ResolveWorktree.
// Both paths answer the same Location; the guard is purely a spawn-count optimization.
func warpLayoutFor(l *lymcwd.Location, worktreeRoot string) (*lymcwd.Location, error) {
	if filepath.Dir(worktreeRoot) != l.HubPath {
		// worktreeRoot is not a direct child of l.HubPath, so reusing l.HubPath below
		// would be wrong; fall back to the spawning, gate-free resolver.
		return lymcwd.ResolveWorktree(worktreeRoot)
	}

	// A hub sibling gets its own anchor from its own marker, so the Location is named rather than copied from l.
	// No git is spawned.
	return lymcwd.NamedWorktree(l.HubPath, filepath.Base(filepath.Clean(worktreeRoot)))
}
