// anchor.go implements the recorded lyx-anchor subpath marker: the plain
// single-line ".fabric-anchor" file at the weft:main root that records the
// repo-wide subpath a fabric clone anchors lyx at (e.g. "backend" or ".").
// It is read here, never written — the write side lives in fabricengine's
// weft:main commit choke point (see the plan's "record wins" shared
// decision). This file stays stdlib-only so hubgeometry never gains a YAML
// dependency.

package lyxcwd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FabricAnchorName is the filename of the recorded lyx-anchor subpath marker
// at the weft:main root (<BoardDir(hub)>/.fabric-anchor). It holds only the
// subpath string (e.g. "backend" or "."). This is a structural geometry
// artifact — a fixed per-repo anchor recorded once at clone/create — never a
// config/env override; per the Hub Geometry Invariant, only hubgeometry
// constructs and reads this path.
const FabricAnchorName = ".fabric-anchor"

// ErrCwdOutsideAnchor is the hard-error sentinel Resolve returns when cwd does
// not equal the anchored directory exactly. It exists so a lyx invocation from
// anywhere else in the worktree — a subdirectory, a parent, or a sibling of a
// subpath-anchored repo — fails loudly and immediately instead of resolving
// the wrong AnchorRel and dying later downstream with a confusing error.
var ErrCwdOutsideAnchor = errors.New("cwd is outside the recorded fabric anchor subtree")

// samePath reports whether a and b name the same filesystem location. Each
// side is normalized through filepath.EvalSymlinks then filepath.Clean,
// falling back to Clean-only for whichever side EvalSymlinks fails on (the
// path may not exist yet, e.g. during clone). Normalization is not optional:
// the worktree side comes from git rev-parse --show-toplevel while cwd comes
// from os.Getwd, and the two disagree routinely — macOS's /tmp is a symlink
// to /private/tmp, lyxtest fixtures live under symlinked temp dirs, and
// Windows/macOS filesystems are case-insensitive while Go string comparison
// is not. Comparison is byte-exact on Linux/macOS and case-insensitive
// (strings.EqualFold) on Windows.
func samePath(a, b string) bool {
	na := normalizePath(a)
	nb := normalizePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(na, nb)
	}
	return na == nb
}

// normalizePath resolves symlinks in p, falling back to filepath.Clean alone
// when EvalSymlinks fails (p may not exist on disk yet).
func normalizePath(p string) string {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(resolved)
}

// checkCwdAnchorGate returns nil when cwd is exactly the anchored directory
// (filepath.Join(worktreePath, anchorRel)), and otherwise wraps
// ErrCwdOutsideAnchor in a message naming both sides and the marker file.
func checkCwdAnchorGate(cwd, anchorRel, worktreePath string) error {
	anchorAbs := filepath.Join(worktreePath, anchorRel)
	if samePath(cwd, anchorAbs) {
		return nil
	}
	return fmt.Errorf("%w: cwd %s does not equal the anchored directory %s (recorded by %s)",
		ErrCwdOutsideAnchor, cwd, anchorAbs, FabricAnchorName)
}

// readRecordedAnchor reads the recorded lyx-anchor subpath marker from
// <BoardDir(hub)>/.fabric-anchor and reports whether a usable anchor was
// found. It returns ("", false) on any error — an absent board directory, an
// absent marker file, or an unreadable file — because every one of those
// cases means the caller must fall back to today's cwd-derived RelPath
// (mid-clone, a lyxtest synthetic hub, or a non-fabric git repo). An
// empty or whitespace-only marker after trimming is also treated as absent:
// an anchor must never resolve to an empty subpath. This helper spawns no
// git and stays stdlib-only.
func readRecordedAnchor(hub string) (anchor string, found bool) {
	data, err := os.ReadFile(filepath.Join(BoardDir(hub), FabricAnchorName))
	if err != nil {
		return "", false
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}
