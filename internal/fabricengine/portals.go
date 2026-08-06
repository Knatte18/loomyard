// portals.go creates and removes the per-worktree portal junction
// (<container>/_portals/<slug> -> the worktree's _lyx/), with idempotent removal.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// portalsDirName is the directory name for the hub-level portals container
// (i.e. <hub>/_portals). Declared here rather than in internal/lyxcwd: the
// portal surface is fabric's own illusion-maintenance plumbing, not a
// resolution primitive lyxcwd needs to expose.
const portalsDirName = "_portals"

// PortalsDir returns the path to the _portals directory in the hub. Exported
// because its live test caller (add_rollback_adopt_test.go) sits in the
// external package fabricengine_test, where an unexported identifier does
// not compile.
func PortalsDir(l *lyxcwd.Location) string {
	return filepath.Join(l.HubPath, portalsDirName)
}

// PortalLink returns the path to the mirrored portal junction link for the
// given slug. It is mirrored into the repo subpath structure, including
// AnchorRel segments. Exported for the same external-test-package reason as
// PortalsDir.
func PortalLink(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, portalsDirName, l.AnchorRel, slug)
}

// portalTarget returns the path to the _lyx directory within a portal for the given slug.
func portalTarget(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, configengine.LyxDirName)
}

// createPortal creates a portal junction from <container>/_portals/<RelPath>/<slug> to <container>/<slug>/<relpath>/_lyx.
//
// Delegates to fslink.CreateDirLink with the computed link and target paths.
// fslink.CreateDirLink already MkdirAll's filepath.Dir(link), creating the mirrored _portals/<RelPath>/ chain.
func createPortal(l *lyxcwd.Location, slug string) error {
	link := PortalLink(l, slug)
	target := portalTarget(l, slug)
	return fslink.CreateDirLink(link, target)
}

// removePortal removes the portal junction, deletes only the link (not the
// target), and prunes empty ancestors. Returns nil if the link does not exist.
func removePortal(l *lyxcwd.Location, slug string) error {
	link := PortalLink(l, slug)
	if err := fslink.Remove(link); err != nil {
		return fmt.Errorf("remove portal %s: %w", link, err)
	}
	// Successful/idempotent removal; prune empty ancestors
	pruneEmptyAncestors(filepath.Dir(link), PortalsDir(l))
	return nil
}
