package worktree

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/mhgo/internal/paths"
)

// CreatePortal creates a portal junction from <container>/_portals/<slug> to <container>/<slug>/<relpath>/_mhgo.
//
// Delegates to createJunction with the computed link and target paths.
func CreatePortal(l *paths.Layout, slug string) error {
	link := filepath.Join(l.PortalsDir(), slug)
	target := l.PortalTarget(slug)
	return createJunction(link, target)
}

// RemovePortal removes the portal junction at <container>/_portals/<slug>.
//
// Uses os.Remove to delete only the link itself, never recursing into the target.
// Returns nil if the link does not exist (idempotent). Returns an error if removal fails.
func RemovePortal(l *paths.Layout, slug string) error {
	link := filepath.Join(l.PortalsDir(), slug)
	if err := os.Remove(link); err != nil {
		if os.IsNotExist(err) {
			return nil // Idempotent: already absent
		}
		return fmt.Errorf("remove portal %s: %w", link, err)
	}
	return nil
}
