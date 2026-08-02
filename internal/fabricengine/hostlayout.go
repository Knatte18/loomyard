// hostlayout.go provides the guarded per-host-worktree Layout deriver shared by
// Status and Reconcile: it avoids re-spawning git for the common case where the
// enumerated worktree is a hub sibling of the caller's already-resolved Layout.
// Its non-sibling fallback resolves via the gate-free hubgeometry.ResolveWorktree,
// not the gated Resolve, since it derives another worktree's geometry from its
// root, above any subpath anchor.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// hostLayoutFor returns the per-host-worktree Layout using SiblingLayout for
// hub siblings (spawn-free optimization) and falling back to ResolveWorktree for
// worktrees outside the hub. Both paths are equivalent; the guard is purely a
// spawn-count optimization.
func hostLayoutFor(l *hubgeometry.Layout, worktreeRoot string) (*hubgeometry.Layout, error) {
	if filepath.Dir(worktreeRoot) != l.Hub {
		// worktreeRoot is not a direct child of l.Hub, so SiblingLayout's hardcoded
		// reuse of l.Hub would be wrong; fall back to the spawning, gate-free resolver.
		return hubgeometry.ResolveWorktree(worktreeRoot)
	}
	return l.SiblingLayout(worktreeRoot), nil
}
