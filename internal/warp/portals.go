// portals.go creates and removes the per-worktree portal junction
// (<container>/_portals/<slug> -> the worktree's _lyx/), with idempotent removal.

package warp

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/paths"
)

// createPortal creates a portal junction from <container>/_portals/<RelPath>/<slug> to <container>/<slug>/<relpath>/_lyx.
//
// Delegates to fslink.CreateDirLink with the computed link and target paths.
// fslink.CreateDirLink already MkdirAll's filepath.Dir(link), creating the mirrored _portals/<RelPath>/ chain.
func createPortal(l *paths.Layout, slug string) error {
	link := l.PortalLink(slug)
	target := l.PortalTarget(slug)
	return fslink.CreateDirLink(link, target)
}

// removePortal removes the portal junction at <container>/_portals/<RelPath>/<slug>.
//
// Uses fslink.Remove to delete only the link itself, never recursing into the target.
// After successful/idempotent removal, prunes empty mirrored ancestors up to but not
// including <container>/_portals/. Returns nil if the link does not exist (idempotent).
// Returns an error if removal fails.
func removePortal(l *paths.Layout, slug string) error {
	link := l.PortalLink(slug)
	if err := fslink.Remove(link); err != nil {
		return fmt.Errorf("remove portal %s: %w", link, err)
	}
	// Successful/idempotent removal; prune empty ancestors
	pruneEmptyAncestors(filepath.Dir(link), l.PortalsDir())
	return nil
}
