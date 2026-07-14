// hostlayout.go provides the guarded per-host-worktree Layout deriver shared by
// Status and Reconcile: it avoids re-spawning git for the common case where the
// enumerated worktree is a hub sibling of the caller's already-resolved Layout.

package warpengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// hostLayoutFor returns the per-host-worktree Layout for a worktree enumerated by
// hubgeometry.List, using the spawn-free Layout.SiblingLayout fast path for the
// normal case (worktreeRoot is a hub sibling of l) and falling back to the
// spawning hubgeometry.Resolve for any worktree that lives outside l's hub. Both
// paths are byte-for-byte equivalent to calling hubgeometry.Resolve(worktreeRoot)
// directly, so the guard is purely a spawn-count optimization with no behavior
// change: see hubgeometry's SiblingLayout godoc and the hubgeometry_test.go
// equivalence test for the proof.
func hostLayoutFor(l *hubgeometry.Layout, worktreeRoot string) (*hubgeometry.Layout, error) {
	if filepath.Dir(worktreeRoot) != l.Hub {
		// worktreeRoot is not a direct child of l.Hub, so SiblingLayout's hardcoded
		// reuse of l.Hub would be wrong; fall back to the spawning resolver.
		return hubgeometry.Resolve(worktreeRoot)
	}
	return l.SiblingLayout(worktreeRoot), nil
}
