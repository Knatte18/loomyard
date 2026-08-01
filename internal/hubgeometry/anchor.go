// anchor.go implements the recorded lyx-anchor subpath marker: the plain
// single-line ".fabric-anchor" file at the weft:main root that records the
// repo-wide subpath a fabric clone anchors lyx at (e.g. "backend" or ".").
// It is read here, never written — the write side lives in fabricengine's
// weft:main commit choke point (see the plan's "record wins" shared
// decision). This file stays stdlib-only so hubgeometry never gains a YAML
// dependency.

package hubgeometry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// FabricAnchorName is the filename of the recorded lyx-anchor subpath marker
// at the weft:main root (<BoardDir(hub)>/.fabric-anchor). It holds only the
// subpath string (e.g. "backend" or "."). This is a structural geometry
// artifact — a fixed per-repo anchor recorded once at clone/create — never a
// config/env override; per the Hub Geometry Invariant, only hubgeometry
// constructs and reads this path.
const FabricAnchorName = ".fabric-anchor"

// ErrCwdOutsideAnchor is the hard-error sentinel Resolve returns when cwd is
// not at or below the worktree subtree recorded by the .fabric-anchor
// marker. It exists so a lyx invocation from outside the anchored subpath
// (e.g. from a sibling directory of a subpath-anchored repo) fails loudly
// instead of silently resolving the wrong RelPath.
var ErrCwdOutsideAnchor = errors.New("cwd is outside the recorded fabric anchor subtree")

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
