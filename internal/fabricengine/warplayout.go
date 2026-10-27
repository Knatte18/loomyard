// warplayout.go provides the guarded per-warp-worktree Location deriver shared by Status and
// Reconcile: it avoids re-spawning git for the common case where the enumerated worktree is a hub
// sibling of the caller's already-resolved Location.
// Its non-sibling fallback resolves via the gate-free lymcwd.ResolveWorktree, not the gated
// Resolve, since it derives another worktree's geometry from its root, above any subpath anchor.

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
