// anchor.go implements the recorded lyx-anchor subpath marker: the plain single-line ".lyx-anchor"
// file at the board root that records the repo-wide subpath a fabric clone anchors lyx at (e.g.
// "backend" or ".").
// It is read here, never written — the write side lives in fabricengine's board-root commit choke
// point (see the plan's "record wins" shared decision).
// This file stays stdlib-only so lyxcwd never gains a YAML dependency.

package lyxcwd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// boardDirName is the name of the board data directory inside the hub (i.e.
// <hub>/_board). It stays private to lyxcwd: the exported BoardDir(hub)
// constructor moved to internal/fabricengine, which declares its own copy
// of this literal; readRecordedAnchor below is the sole reason lyxcwd still
// needs the name itself, to find the recorded-anchor marker.
const boardDirName = "_board"

// boardDir returns the absolute path to the board data directory inside hub.
// Private: see boardDirName's comment for why lyxcwd retains this one reader.
func boardDir(hub string) string {
	return filepath.Join(hub, boardDirName)
}

// AnchorFileName is the filename of the recorded lyx-anchor subpath marker at the board root
// (<boardDir(hub)>/.lyx-anchor).
// It holds only the subpath string (e.g. "backend" or ".").
// This is a structural geometry artifact — a fixed per-repo anchor recorded once at clone/create —
// never a config/env override;
// per the Cwd Resolution Invariant, only lyxcwd constructs and reads this path.
// There is no compatibility fallback read for the pre-rename ".fabric-anchor" name: the marker
// anchors the whole repo, not the fabric module, so the old name is simply wrong now, not
// merely renamed — see clone.go's stale-marker guard.
const AnchorFileName = ".lyx-anchor"

// StaleAnchorFileName is the pre-rename spelling of the recorded lyx-anchor marker.
// It is never read as an anchor value — the marker anchors the whole repo, not the fabric module,
// so the old name is simply wrong now rather than merely renamed — but its presence must be
// DETECTED, because falling back to "." for a hub that recorded a real subpath under the old name
// silently re-anchors the whole repo at its root.
// It is exported so fabric's clone-time guard names the same literal this package's read-time guard
// does, rather than each declaring its own copy.
const StaleAnchorFileName = ".fabric-anchor"

// ErrStaleAnchorMarker is the hard-error sentinel returned when a hub's board directory carries the
// pre-rename StaleAnchorFileName marker with no AnchorFileName beside it.
// Resolving such a hub at all would mean re-anchoring it at ".", so every resolver refuses instead
// of guessing.
var ErrStaleAnchorMarker = errors.New("stale pre-rename fabric anchor marker with no renamed marker beside it")

// ErrCwdOutsideAnchor is the hard-error sentinel Resolve returns when cwd does not equal the
// anchored directory exactly.
// It exists so a lyx invocation from anywhere else in the worktree — a subdirectory, a parent, or a
// sibling of a subpath-anchored repo — fails loudly and immediately instead of resolving the wrong
// AnchorRel and dying later downstream with a confusing error.
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
		ErrCwdOutsideAnchor, cwd, anchorAbs, AnchorFileName)
}

// readRecordedAnchor reads the recorded lyx-anchor subpath marker from
// <BoardDir(hub)>/.lyx-anchor and reports whether a usable anchor was
// found. It returns ("", false, nil) on any error — an absent board directory, an
// absent marker file, or an unreadable file — because every one of those
// cases means the caller must fall back to today's cwd-derived RelPath
// (mid-clone, a lyxtest synthetic hub, or a non-fabric git repo). An
// empty or whitespace-only marker after trimming is also treated as absent:
// an anchor must never resolve to an empty subpath. This helper spawns no
// git and stays stdlib-only.
//
// The one absence that is NOT a fallback is a hub whose board still carries the pre-rename
// StaleAnchorFileName marker and no renamed one: that hub did record a subpath, under the old name,
// and answering "." for it re-anchors the whole repo at its root — after which `lyx fabric
// reconcile` wires a second junction set there. That case returns ErrStaleAnchorMarker so every
// resolver refuses rather than guessing.
func readRecordedAnchor(hub string) (anchor string, found bool, err error) {
	board := boardDir(hub)

	data, readErr := os.ReadFile(filepath.Join(board, AnchorFileName))
	if readErr != nil {
		if _, staleErr := os.Stat(filepath.Join(board, StaleAnchorFileName)); staleErr == nil {
			return "", false, fmt.Errorf("%w: rename %s to %s in the hub's %s worktree and commit it (at %s)",
				ErrStaleAnchorMarker, StaleAnchorFileName, AnchorFileName, boardDirName, board)
		}
		return "", false, nil
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "", false, nil
	}
	return trimmed, true, nil
}
