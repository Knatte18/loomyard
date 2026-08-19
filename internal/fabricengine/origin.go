// origin.go declares fabric's provenance record for one worktree pair: the first *tracked*
// fabric-owned record under the durable lyx directory (_lyx/fabric/origin.json), committed by
// Topology.Add on the weft branch.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// Origin is fabric's provenance record for one worktree pair, written once at pair-creation time
// and read thereafter — never inferred.
type Origin struct {
	// ParentBranch is the warp branch the pair was forked from, recorded at creation time and
	// never inferred.
	ParentBranch string `json:"parent_branch"`
}

// originRecordDirName, originRecordFileName, and originRecordLockFileName are the segments of the
// origin record's path and lock path under the durable lyx directory.
// internal/fabricengine is their sole declarer, per the Cwd Resolution Invariant's "a module's own
// durable-storage subdirectory is that module's own private relative-path constant" rule.
const (
	originRecordDirName      = "fabric"
	originRecordFileName     = "origin.json"
	originRecordLockFileName = "origin.json.lock"
)

// OriginRecordRel returns the origin record's path relative to a worktree's anchor: the form both
// path accessors and every commit pathspec are built from, so the segments are joined in exactly
// one place.
func OriginRecordRel() string {
	return filepath.Join(lyxdirs.LyxDirName, originRecordDirName, originRecordFileName)
}

// OriginRecordPath returns the origin record's path for reading, through l's own anchored worktree.
// l.AnchorPath() already carries AnchorRel, so a caller reading through the warp junction in a
// subpath-anchored hub needs no extra join.
func OriginRecordPath(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), OriginRecordRel())
}

// OriginRecordPathFor returns the origin record's path for writing, in the weft worktree for slug.
// It mirrors the existing WeftWorktree -> AnchorRel -> durable-dir shape, and exists because during
// Add the new pair is not the acting worktree — a bare WeftWorktreePath(l, slug) root would be
// wrong in any subpath-anchored hub.
func OriginRecordPathFor(l *lyxcwd.Location, slug string) string {
	return filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, OriginRecordRel())
}
