// mergepaths.go implements the conflicts-are-reported-as-unified-worktree-relative-paths decision:
// resolving the geometry a merge call needs to map weft-side conflicted paths onto the single
// visible worktree, and the pure mapping function itself.

package fabricengine

import (
	"os"
	"path"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// resolveMergeGeometry resolves the geometry a merge call needs to map conflicted paths onto the
// single visible worktree: l's AnchorRel, and the repo-wide wired junction-name set.
// l is the caller's own already-resolved lyxcwd.ResolveWorktree result, so a merge call resolves the
// worktree once, not twice.
// It is called once per merge call, before any mutation, and never cached on the Fabric handle —
// the same re-read-per-call precedent Fabric.Commit's config load follows, since a merge is rare
// enough that one extra read is irrelevant beside answering against a config a reconcile just
// changed.
// l's WorktreePath() is NOT the config base; RepoWiredNames derives the repo-wide `weft:main` base
// itself.
func resolveMergeGeometry(l *lyxcwd.Location) (anchorRel string, wiredNames []string, err error) {
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
//
// The two arguments do not arrive in the same separator convention, and reconciling them is what the
// separator conversion below is doing here rather than decoration. weftPath is git's own output,
// which is always forward-slash. anchorRel comes from lyxcwd.ValidateAnchorRel, which returns
// filepath.Clean(filepath.FromSlash(raw)) — an OS-separator path. On Windows a multi-segment anchor
// recorded in .lyx-anchor as "apps/backend" therefore arrives as `apps\backend`, and joining it with
// the slash-only path package yields the prefix `apps\backend/_lyx/` while git reports
// `apps/backend/_lyx/…`. The prefix test then fails for a perfectly mappable conflict,
// unifyConflictPaths marks it unmappable, and MergeIn self-aborts the entire merge on both sides
// with *ErrUnmergeableState. Single-segment anchors and "." carry no separator and were never
// affected, which is why every existing fixture passes either way.
// Replacing only the OS separator — not every backslash — is the correct conversion, because on a
// platform whose separator already is '/' a directory name legitimately containing a backslash must
// never be rewritten into a path component boundary that does not exist.
//
// The conversion is written against an explicit separator (weftPathVisibleWithSeparator) rather than
// calling filepath.ToSlash directly, and that is a testability decision with a concrete reason.
// filepath.ToSlash IS the identity function whenever os.PathSeparator == '/', so on every host this
// project builds and tests on, no test can distinguish the call from its absence — deleting it left
// the entire hermetic and integration suite green, which is exactly how a Windows-only fix rots. The
// separator-parameterised form lets a Linux test drive the Windows separator directly, so both wrong
// implementations (dropping the conversion, and blanket-replacing every backslash) fail here.
func weftPathVisible(weftPath, anchorRel string, wiredNames []string) bool {
	return weftPathVisibleWithSeparator(weftPath, anchorRel, wiredNames, os.PathSeparator)
}

// weftPathVisibleWithSeparator is weftPathVisible's separator-explicit form: separator is the OS path
// separator anchorRel is spelled with, and is what weftPathVisible supplies as os.PathSeparator.
// A separator of '/' makes the conversion a no-op, matching filepath.ToSlash's own contract.
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

// unifyConflictPaths maps warpConflicts and weftConflicts onto one flat, lexically sorted,
// worktree-relative path list, per the conflicts-are-reported-as-unified-worktree-relative-paths
// decision.
// A warp path (already repo-root-relative) passes through unchanged.
// A weft path maps by identity iff weftPathVisible reports true for it; a weft path outside the
// wired set sets unmappable true, as does a unified path produced by both sides (the theoretical
// collision) — the caller aborts the merge on both sides and returns *ErrUnmergeableState in either
// case.
// Path comparison and the returned list both use forward-slash git-style paths throughout, since
// that is git's own output form. anchorRel is the one argument that does NOT arrive that way — it is
// an OS-separator path — so weftPathVisible converts its separator to '/' before joining;
// see that function's own comment for what breaks on Windows without the conversion.
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
