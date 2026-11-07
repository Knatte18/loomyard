// mergepaths.go maps conflicted paths of a merge onto the single visible worktree.
// It resolves the geometry a merge call needs and holds the pure function that does the mapping.

package fabricengine

import (
	"os"
	"path"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// resolveMergeGeometry resolves the anchor-relative path and the repo-wide wired junction names.
// A merge call needs both to map conflicted paths onto the visible worktree.
// It takes the caller's already-resolved location, so a merge resolves the worktree once.
// It runs once per merge call, before any mutation, and is not cached on the Fabric handle:
// a merge is rare, so one extra config read is cheap, and a reconcile may have changed the config since.
// The config base is not l's WorktreePath(); RepoWiredNames derives the repo-wide `weft:main` base itself.
func resolveMergeGeometry(l *lymcwd.Location) (anchorRel string, wiredNames []string, err error) {
	wiredNames, err = RepoWiredNames(l)
	if err != nil {
		return "", nil, err
	}
	return l.AnchorRel, wiredNames, nil
}

// weftPathVisible reports whether weftPath lies under <anchorRel>/<name>/ for some name in wiredNames.
// weftPath is weft-relative and forward-slash; with anchorRel == ".", the prefix is <name>/ directly.
// The junction geometry makes the weft checkout mirror the anchor subpath,
// so a path that passes this check is already the visible worktree-relative path.
// The check is a membership test, with no transformation.
//
// weftPath is git's own output, always forward-slash.
// anchorRel comes from lymcwd.ValidateAnchorRel as an OS-separator path;
// so on Windows a multi-segment anchor arrives as `apps\backend` while git reports `apps/backend/`.
// The separator is converted before the prefix test, or a mappable conflict would be reported unmappable.
// Only the OS separator is replaced, not every backslash;
// on a '/'-separator platform a directory name may legitimately contain a backslash.
//
// The conversion takes an explicit separator so tests on any host can drive the Windows separator.
// filepath.ToSlash is the identity when os.PathSeparator is '/';
// so on such a host the call cannot be told from its absence.
// The tests reject both dropping the conversion and replacing every backslash.
func weftPathVisible(weftPath, anchorRel string, wiredNames []string) bool {
	return weftPathVisibleWithSeparator(weftPath, anchorRel, wiredNames, os.PathSeparator)
}

// weftPathVisibleWithSeparator reports whether weftPath lies under a wired name below anchorRel.
// separator is the OS path separator that anchorRel is spelled with.
// A separator of '/' makes the conversion a no-op, as with filepath.ToSlash.
func weftPathVisibleWithSeparator(weftPath, anchorRel string, wiredNames []string, separator rune) bool {
	slashAnchorRel := anchorRel
	if separator != '/' {
		slashAnchorRel = strings.ReplaceAll(anchorRel, string(separator), "/")
	}
	for _, name := range wiredNames {
		prefix := name + "/"
		if slashAnchorRel != "." {
			prefix = path.Join(slashAnchorRel, name) + "/"
		}
		if strings.HasPrefix(weftPath, prefix) {
			return true
		}
	}
	return false
}

// unifyConflictPaths maps warpConflicts and weftConflicts onto one lexically sorted, worktree-relative path list.
// A warp path is already repo-root-relative and passes through unchanged.
// A weft path maps by identity when weftPathVisible reports it visible.
// A weft path outside the wired set, or a path produced by both sides, sets unmappable,
// and the caller then aborts the merge on both sides with *ErrUnmergeableState.
// Paths use forward-slash git-style form, the form git itself reports.
// anchorRel is the one OS-separator argument, which weftPathVisible converts before joining.
// The result is never nil and holds no duplicates: each side lists a path at most once,
// and a path listed by both sides is the collision that sets unmappable.
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
			// A weft path already seen from the warp side is the theoretical collision.
			unmappable = true
			continue
		}
		seen[p] = true
		result = append(result, p)
	}

	sort.Strings(result)
	return result, unmappable
}
