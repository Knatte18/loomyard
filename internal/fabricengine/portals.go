// portals.go creates and removes the per-worktree portal junction (<container>/_portals/<slug> ->
// the worktree's _lyx/), with idempotent removal.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// portalsDirName is the directory name for the hub-level portals container
// (i.e. <hub>/_portals). Declared here rather than in internal/lyxcwd: the
// portal surface is fabric's own illusion-maintenance plumbing, not a
// resolution primitive lyxcwd needs to expose.
const portalsDirName = "_portals"

// PortalsDir returns the path to the _portals directory in the hub.
// Exported because its live test caller (add_rollback_adopt_test.go) sits in the external package
// fabricengine_test, where an unexported identifier does not compile.
func PortalsDir(l *lyxcwd.Location) string {
	return filepath.Join(l.HubPath, portalsDirName)
}

// PortalLink returns the path to the mirrored portal junction link for the given slug.
// It is mirrored into the repo subpath structure, including AnchorRel segments.
// Exported for the same external-test-package reason as PortalsDir.
func PortalLink(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, portalsDirName, l.AnchorRel, slug)
}

// portalTarget returns the path to the _lyx directory within a portal for the given slug.
func portalTarget(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, slug, l.AnchorRel, lyxdirs.LyxDirName)
}

// createPortal creates a portal junction from <container>/_portals/<RelPath>/<slug> to <container>/<slug>/<relpath>/_lyx.
//
// The portal's parent chain is materialised through an os.Root rooted at the hub BEFORE the leaf link
// is handed to fslink, and that is a containment property, not a redundant mkdir: _portals is a
// structural directory directly under the hub, and fslink.CreateDirLink's own os.MkdirAll(filepath.Dir(link))
// followed a symlink planted at the _portals container (or an intermediate AnchorRel segment) straight
// out of the hub, planting the junction outside it while reporting success — the create-side twin of the
// delete-side M3 the removePortal gate already closes (the leaf vector is separately refused by fslink's
// own refuse-to-clobber guard). Rooting the parent creation at the hub refuses any escaping component at
// mkdir time; once the parent is a real contained directory, fslink's own MkdirAll is a no-op and the
// leaf symlink lands inside the hub.
// rec is the calling verb's own recorder; on success it records KindLinkCreated with the link path
// (never the link's own target) as the Target.
func createPortal(rec *Mutations, l *lyxcwd.Location, slug string) error {
	link := PortalLink(l, slug)
	target := portalTarget(l, slug)
	if err := ensureContainedLinkParent(l.HubPath, link); err != nil {
		return err
	}
	if err := fslink.CreateDirLink(link, target); err != nil {
		return err
	}
	rec.Append(KindLinkCreated, link, "")
	return nil
}

// ensureContainedLinkParent creates the parent directory chain of link through an os.Root rooted at
// hubPath, so a symlink planted at any component of that chain that escapes the hub is refused at mkdir
// time rather than followed. It is the create-side containment createPortal needs before handing a leaf
// to fslink, standing in for the os.Root the delete-side removeLink removes through: fslink.CreateDirLink
// would otherwise os.MkdirAll straight through an escaping container symlink.
// A parent that already exists as a real contained directory is left untouched — os.Root.MkdirAll is a
// no-op there — so only an escaping component fails, and it fails closed.
func ensureContainedLinkParent(hubPath, link string) error {
	root, err := os.OpenRoot(hubPath)
	if err != nil {
		return fmt.Errorf("open hub root %s: %w", hubPath, err)
	}
	defer func() { _ = root.Close() }()

	parentRel, err := hubRel(hubPath, filepath.Dir(link))
	if err != nil {
		return err
	}
	if err := root.MkdirAll(parentRel, 0o755); err != nil {
		return fmt.Errorf("create contained parent for %s: %w", link, err)
	}
	return nil
}

// removePortal removes the portal junction, deletes only the link (not the
// target), and prunes empty ancestors. Returns nil if the link does not exist.
// rec is the calling verb's own recorder, passed straight through to removeLink.
func removePortal(rec *Mutations, l *lyxcwd.Location, slug string) error {
	link := PortalLink(l, slug)
	if err := refuseUncontainedPath(PortalsDir(l), link, "portal"); err != nil {
		return err
	}
	req := pathRequest{
		what:      "remove portal",
		container: PortalsDir(l),
		target:    link,
		slug:      nil,
		ownership: ownedWiredJunction([]string{PortalLink(l, slug)}, portalTarget(l, slug)),
		dirtiness: dirtinessNA("a junction holds no content; the weft target it points at is untouched"),
		force:     false,
	}
	if err := removeLink(rec, req); err != nil {
		return fmt.Errorf("remove portal %s: %w", link, err)
	}
	// Successful/idempotent removal; prune empty ancestors
	pruneEmptyAncestors(filepath.Dir(link), PortalsDir(l))
	return nil
}
