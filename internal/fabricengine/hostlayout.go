// hostlayout.go provides the guarded per-host-worktree Location deriver shared by Status and Reconcile: it avoids re-spawning git for the common case where the enumerated worktree is a hub sibling of the caller's already-resolved Location.
// Its non-sibling fallback resolves via the gate-free lyxcwd.ResolveWorktree, not the gated Resolve, since it derives another worktree's geometry from its root, above any subpath anchor.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// hostLayoutFor returns the per-host-worktree Location for a hub sibling
// (spawn-free optimization, constructed inline below now that
// lyxcwd.Location.SiblingLayout no longer exists) or, for a worktree
// outside the hub, by falling back to the spawning, gate-free
// lyxcwd.ResolveWorktree. Both paths are equivalent; the guard is purely
// a spawn-count optimization.
func hostLayoutFor(l *lyxcwd.Location, worktreeRoot string) (*lyxcwd.Location, error) {
	if filepath.Dir(worktreeRoot) != l.HubPath {
		// worktreeRoot is not a direct child of l.HubPath, so reusing l.HubPath below
		// would be wrong; fall back to the spawning, gate-free resolver.
		return lyxcwd.ResolveWorktree(worktreeRoot)
	}

	// A hub sibling shares the hub-wide recorded anchor with l, so AnchorRel is
	// reused directly rather than re-read from the marker.
	return &lyxcwd.Location{
		HubPath:      l.HubPath,
		WorktreeName: filepath.Base(filepath.Clean(worktreeRoot)),
		AnchorRel:    l.AnchorRel,
	}, nil
}
