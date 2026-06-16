// portals.go creates and removes the per-worktree portal junction
// (<container>/_portals/<slug> -> the worktree's _lyx/), with idempotent removal.

package worktree

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/mhgo/internal/paths"
)

// createPortal creates a portal junction from <container>/_portals/<RelPath>/<slug> to <container>/<slug>/<relpath>/_lyx.
//
// Delegates to createJunction with the computed link and target paths.
// createJunction already MkdirAll's filepath.Dir(link), creating the mirrored _portals/<RelPath>/ chain.
func createPortal(l *paths.Layout, slug string) error {
	link := l.PortalLink(slug)
	target := l.PortalTarget(slug)
	return createJunction(link, target)
}

// removePortal removes the portal junction at <container>/_portals/<RelPath>/<slug>.
//
// Uses os.Remove to delete only the link itself, never recursing into the target.
// After successful/idempotent removal, prunes empty mirrored ancestors up to but not
// including <container>/_portals/. Returns nil if the link does not exist (idempotent).
// Returns an error if removal fails.
func removePortal(l *paths.Layout, slug string) error {
	link := l.PortalLink(slug)
	if err := os.Remove(link); err != nil {
		if os.IsNotExist(err) {
			// Idempotent: already absent, but still prune ancestors
			pruneEmptyAncestors(filepath.Dir(link), l.PortalsDir())
			return nil
		}
		return fmt.Errorf("remove portal %s: %w", link, err)
	}
	// Successful removal; prune empty ancestors
	pruneEmptyAncestors(filepath.Dir(link), l.PortalsDir())
	return nil
}
