// portals.go declares the name of the retired `_portals` hub container, and sweeps the portal links and workspace file a hub still holds from it.
// No verb creates a portal link any more; the name stays reserved and `lyx fabric reconcile` runs the sweep.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// portalsDirName is the directory name of the retired hub-level portals container (<hub>/_portals).
// It stays declared here, and reserved by HubReservedNames, so no pair can take the name of a link a hub may still hold.
// Declared here rather than in internal/lyxcwd: it is not a resolution primitive lyxcwd needs to expose.
const portalsDirName = "_portals"

// retiredPortalTarget returns <hub>/<name>/<AnchorRel>/_lyx, the target a fabric portal link named name pointed at.
func retiredPortalTarget(l *lyxcwd.Location, name string) string {
	return filepath.Join(l.HubPath, name, l.AnchorRel, lyxdirs.LyxDirName)
}

// retiredHubWorkspacePath returns <hub>/_launchers/<AnchorRel>/<primeName>.code-workspace, where the retired hub workspace file lived.
func retiredHubWorkspacePath(l *lyxcwd.Location, primeName string) string {
	return filepath.Join(launchersDir(l), l.AnchorRel, primeName+hubWorkspaceExt)
}

// retiredPortalChain returns the hub-relative directories from the portals container down to its anchor directory, in that order.
func retiredPortalChain(l *lyxcwd.Location) []string {
	chain := []string{portalsDirName}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(l.AnchorRel)), "/") {
		if part == "." || part == "" {
			continue
		}
		chain = append(chain, filepath.Join(chain[len(chain)-1], part))
	}
	return chain
}

// readRetiredPortalDir lists the entry names of dirRel, a hub-relative directory, in byte order, through root.
// A component that is a symlink escaping the hub fails the read.
func readRetiredPortalDir(root *os.Root, dirRel string) ([]string, error) {
	dir, err := root.Open(dirRel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// isRetiredPortalLink reports whether the entry name of the hub-relative directory dirRel is a link whose raw target is retiredPortalTarget(l, name), whether or not that target still exists.
func isRetiredPortalLink(l *lyxcwd.Location, dirRel, name string) bool {
	path := filepath.Join(l.HubPath, dirRel, name)
	isLink, err := fslink.IsLink(path)
	if err != nil || !isLink {
		return false
	}
	raw, err := fslink.RawTarget(path)
	return err == nil && filepath.Clean(raw) == filepath.Clean(retiredPortalTarget(l, name))
}

// retiredHubArtifactsPresent reports whether the hub still holds a fabric-shaped portal link, or a file or link at the retired workspace path.
// It reads through an os.Root at the hub, so an escaping component reads as no artifact; the sweep reports it instead.
func retiredHubArtifactsPresent(l *lyxcwd.Location) bool {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return false
	}
	defer func() { _ = root.Close() }()

	chain := retiredPortalChain(l)
	anchorRel := chain[len(chain)-1]
	if names, err := readRetiredPortalDir(root, anchorRel); err == nil {
		for _, name := range names {
			if isRetiredPortalLink(l, anchorRel, name) {
				return true
			}
		}
	}

	prime, err := PrimeName(l)
	if err != nil {
		return false
	}
	rel, err := hubRel(l.HubPath, retiredHubWorkspacePath(l, prime))
	if err != nil {
		return false
	}
	info, err := root.Lstat(rel)
	return err == nil && !info.IsDir()
}

// sweepRetiredHubArtifacts removes the retired portal links, their emptied directory chain and the retired hub workspace file, and returns one detail per entry it kept or refused.
// Anything that is not a fabric-shaped portal link, or a file or link at the retired workspace path, is kept.
// A refusal never stops the sweep; an absent artifact is a no-op that records nothing.
// The workspace file is left alone when the prime cannot be resolved.
// Both are keyed on the prime's own anchor, whichever worktree runs the sweep.
func sweepRetiredHubArtifacts(rec *Mutations, l *lyxcwd.Location) []string {
	prime, err := PrimeName(l)
	if err != nil {
		return sweepRetiredPortals(rec, l)
	}
	primeLoc, err := lyxcwd.NamedWorktree(l.HubPath, prime)
	if err != nil {
		return []string{fmt.Sprintf("left %s untouched: read the prime's anchor: %v", portalsDirName, err)}
	}
	details := sweepRetiredPortals(rec, primeLoc)
	return append(details, sweepRetiredHubWorkspace(rec, primeLoc)...)
}

// keptDetail words a sweep detail naming path, shown hub-relative, and the reason it was kept.
func keptDetail(l *lyxcwd.Location, path, reason string) string {
	return fmt.Sprintf("kept %s: %s", hubRelativeTarget(l.HubPath, path), reason)
}

// sweepRetiredPortals removes the fabric-shaped links under <hub>/_portals/<AnchorRel>/ and then every directory of that chain left empty, deepest first.
// An escaping component fails the listing, which removes nothing and yields one detail.
func sweepRetiredPortals(rec *Mutations, l *lyxcwd.Location) []string {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return []string{fmt.Sprintf("left %s untouched: open hub root: %v", portalsDirName, err)}
	}
	defer func() { _ = root.Close() }()

	chain := retiredPortalChain(l)
	var details []string
	var links []string
	listed := 0
	lastKept := -1
	for level, dirRel := range chain {
		names, err := readRetiredPortalDir(root, dirRel)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return []string{fmt.Sprintf("left %s untouched: %v", filepath.ToSlash(dirRel), err)}
		}
		listed = level + 1
		isAnchor := level == len(chain)-1
		next := ""
		if !isAnchor {
			next = filepath.Base(chain[level+1])
		}
		descend := false
		for _, name := range names {
			switch {
			case isAnchor && isRetiredPortalLink(l, dirRel, name):
				links = append(links, name)
			case name == next && isDirectoryAt(root, filepath.Join(dirRel, name)):
				descend = true
			default:
				details = append(details, keptDetail(l, filepath.Join(l.HubPath, dirRel, name), "not a retired fabric portal link"))
				lastKept = level
			}
		}
		if !descend {
			break
		}
	}

	portalsContainer := filepath.Join(l.HubPath, portalsDirName)
	anchorRel := chain[len(chain)-1]
	for _, name := range links {
		link := filepath.Join(l.HubPath, anchorRel, name)
		err := removeLink(rec, pathRequest{
			what:      "remove retired portal link",
			container: portalsContainer,
			target:    link,
			slug:      nil,
			ownership: ownedWiredJunction([]string{link}, retiredPortalTarget(l, name)),
			dirtiness: dirtinessNA("a retired portal link holds no content"),
			force:     false,
		})
		if err != nil {
			details = append(details, keptDetail(l, link, err.Error()))
			lastKept = len(chain) - 1
		}
	}

	for level := listed - 1; level > lastKept; level-- {
		dir := filepath.Join(l.HubPath, chain[level])
		container := portalsContainer
		if level == 0 {
			container = l.HubPath
		}
		err := checkPathRequest(pathRequest{
			what:      "remove emptied retired portal directory",
			container: container,
			target:    dir,
			slug:      nil,
			ownership: ownedUnderGeometryRoot(portalsContainer),
			dirtiness: dirtinessNA("an emptied directory holds no content"),
			force:     false,
		})
		if err == nil {
			var removed bool
			removed, _, err = removeContainedPath(container, dir, false)
			if err == nil && removed {
				rec.Append(KindPathRemoved, dir, "single")
			}
		}
		if err != nil {
			// A directory that gained an entry after the listing is not a failure: the OS refuses it and it stays.
			if !directoryHoldsEntries(root, chain[level]) {
				details = append(details, keptDetail(l, dir, err.Error()))
			}
			break
		}
	}
	return details
}

// isDirectoryAt reports whether the hub-relative rel is a real directory, never a link to one.
func isDirectoryAt(root *os.Root, rel string) bool {
	info, err := root.Lstat(rel)
	return err == nil && info.IsDir()
}

// directoryHoldsEntries reports whether the hub-relative directory rel lists at least one entry.
func directoryHoldsEntries(root *os.Root, rel string) bool {
	names, err := readRetiredPortalDir(root, rel)
	return err == nil && len(names) > 0
}

// sweepRetiredHubWorkspace removes the retired workspace file when the prime resolves, and returns one detail for a directory kept there or a refusal.
// A link at the path is removed as a link, so its target is left untouched.
func sweepRetiredHubWorkspace(rec *Mutations, l *lyxcwd.Location) []string {
	prime, err := PrimeName(l)
	if err != nil {
		return nil
	}
	path := retiredHubWorkspacePath(l, prime)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []string{keptDetail(l, path, err.Error())}
	}
	if info.IsDir() {
		return []string{keptDetail(l, path, "a directory, not the retired workspace file")}
	}
	err = checkPathRequest(pathRequest{
		what:      "remove retired hub workspace file",
		container: launchersDir(l),
		target:    path,
		slug:      nil,
		ownership: ownedUnderGeometryRoot(launchersDir(l)),
		dirtiness: dirtinessNA("the retired workspace file is a generated artifact, never edited content"),
		force:     false,
	})
	if err != nil {
		return []string{keptDetail(l, path, err.Error())}
	}
	removed, _, err := removeContainedPath(launchersDir(l), path, false)
	if err != nil {
		return []string{keptDetail(l, path, err.Error())}
	}
	if removed {
		rec.Append(KindPathRemoved, path, "single")
	}
	return nil
}
