// mergepaths.go implements the conflicts-are-reported-as-unified-worktree-relative-paths decision:
// resolving the geometry a merge call needs to map weft-side conflicted paths onto the single
// visible worktree, and the pure mapping function itself.

package fabricengine

import (
	"path"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// resolveMergeGeometry resolves the geometry a merge call needs to map conflicted paths onto the
// single visible worktree: warpPath's AnchorRel, and the repo-wide wired junction-name set.
// It is called once per merge call, before any mutation, and never cached on the Fabric handle —
// the same re-read-per-call precedent Fabric.Commit's config load follows, since a merge is rare
// enough that one extra read is irrelevant beside answering against a config a reconcile just
// changed.
// filepath.Dir(warpPath) is NOT the config base; RepoWiredNames derives the repo-wide `weft:main`
// base itself.
func resolveMergeGeometry(warpPath string) (anchorRel string, wiredNames []string, err error) {
	l, err := lyxcwd.ResolveWorktree(warpPath)
	if err != nil {
		return "", nil, err
	}
	wiredNames, err = RepoWiredNames(l)
	if err != nil {
		return "", nil, err
	}
	return l.AnchorRel, wiredNames, nil
}

// weftPathVisible reports whether weftPath — a weft-relative, forward-slash conflicted path — lies
// under <anchorRel>/<name>/ for some name in wiredNames, with anchorRel == "." meaning <name>/
// directly.
// The junction geometry guarantees the weft checkout mirrors the anchor subpath, so a path that
// passes this check *is* the visible worktree-relative path, unchanged — no transformation, only a
// membership test.
func weftPathVisible(weftPath, anchorRel string, wiredNames []string) bool {
	for _, name := range wiredNames {
		prefix := name + "/"
		if anchorRel != "." {
			prefix = path.Join(anchorRel, name) + "/"
		}
		if strings.HasPrefix(weftPath, prefix) {
			return true
		}
	}
	return false
}

// unifyConflictPaths maps warpConflicts and weftConflicts onto one flat, lexically sorted,
// worktree-relative path list, per the conflicts-are-reported-as-unified-worktree-relative-paths
// decision.
// A warp path (already repo-root-relative) passes through unchanged.
// A weft path maps by identity iff weftPathVisible reports true for it; a weft path outside the
// wired set sets unmappable true, as does a unified path produced by both sides (the theoretical
// collision) — the caller aborts the merge on both sides and returns *ErrUnmergeableState in either
// case.
// Path comparison and the returned list both use forward-slash git-style paths throughout, since
// that is git's own output form; anchorRel is normalized with path.Join semantics, never
// filepath's OS-dependent one.
// The result is empty-never-nil, and carries no duplicate: warp/weft entries never repeat within
// their own side (git's own conflicted-file listing does not), and a same-path entry from both
// sides is exactly the collision case that sets unmappable instead of appearing twice.
func unifyConflictPaths(warpConflicts, weftConflicts []string, anchorRel string, wiredNames []string) (unified []string, unmappable bool) {
	seen := make(map[string]bool, len(warpConflicts)+len(weftConflicts))
	result := make([]string, 0, len(warpConflicts)+len(weftConflicts))

	for _, p := range warpConflicts {
		if seen[p] {
			continue
		}
		seen[p] = true
		result = append(result, p)
	}

	for _, p := range weftConflicts {
		if !weftPathVisible(p, anchorRel, wiredNames) {
			unmappable = true
			continue
		}
		if seen[p] {
			// The theoretical collision: this unified path was already produced by the warp side.
			unmappable = true
			continue
		}
		seen[p] = true
		result = append(result, p)
	}

	sort.Strings(result)
	return result, unmappable
}
