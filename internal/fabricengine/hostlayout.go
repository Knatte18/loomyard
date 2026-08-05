// hostlayout.go provides the guarded per-host-worktree Layout deriver shared by
// Status and Reconcile: it avoids re-spawning git for the common case where the
// enumerated worktree is a hub sibling of the caller's already-resolved Layout.
// Its non-sibling fallback resolves via the gate-free hubgeometry.ResolveWorktree,
// not the gated Resolve, since it derives another worktree's geometry from its
// root, above any subpath anchor.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// hostLayoutFor returns the per-host-worktree Layout for a hub sibling
// (spawn-free optimization, constructed inline below now that
// hubgeometry.Layout.SiblingLayout no longer exists) or, for a worktree
// outside the hub, by falling back to the spawning, gate-free
// hubgeometry.ResolveWorktree. Both paths are equivalent; the guard is purely
// a spawn-count optimization.
func hostLayoutFor(l *hubgeometry.Layout, worktreeRoot string) (*hubgeometry.Layout, error) {
	if filepath.Dir(worktreeRoot) != l.Hub {
		// worktreeRoot is not a direct child of l.Hub, so reusing l.Hub below
		// would be wrong; fall back to the spawning, gate-free resolver.
		return hubgeometry.ResolveWorktree(worktreeRoot)
	}

	c := filepath.Clean(worktreeRoot)

	// Read the recorded lyx-anchor marker the same way hubgeometry's own
	// resolvers do (see clone.go's markerPath read for the established
	// in-package precedent): an absent board dir, absent marker, or
	// whitespace-only content all fall back to RelPath ".".
	relPath := "."
	markerPath := filepath.Join(hubgeometry.BoardDir(l.Hub), hubgeometry.FabricAnchorName)
	if data, err := os.ReadFile(markerPath); err == nil {
		if anchor := strings.TrimSpace(string(data)); anchor != "" {
			relPath = anchor
		}
	}

	return &hubgeometry.Layout{
		Cwd:          c,
		WorktreeRoot: c,
		Hub:          l.Hub,
		RelPath:      relPath,
		Repo:         l.Repo,
	}, nil
}
