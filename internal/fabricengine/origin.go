// origin.go declares fabric's provenance record for one worktree pair: the first *tracked*
// fabric-owned record under the durable lyx directory (_lyx/fabric/origin.json), committed by
// Topology.Add on the weft branch.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// Origin is fabric's provenance record for one worktree pair, written once at pair-creation time
// and read thereafter — never inferred.
// It names both the branch and the worktree the pair was created from.
type Origin struct {
	// ParentBranch is the warp branch the pair was forked from, recorded at creation time and
	// never inferred.
	ParentBranch string `json:"parent_branch"`
	// ParentWorktree is the WorktreeName of the worktree Topology.Add ran from, recorded at creation time and never inferred.
	// It is what a run's parent agent name is resolved from.
	// A record written before the field existed, and a legacy-worktree repair whose creator is unknown, leave it empty.
	ParentWorktree string `json:"parent_worktree,omitempty"`
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
// It mirrors the existing RecordsWorktree -> AnchorRel -> durable-dir shape, and exists because during
// Add the new pair is not the acting worktree — a bare RecordsWorktreePath(l, slug) root would be
// wrong in any subpath-anchored hub.
func OriginRecordPathFor(l *lyxcwd.Location, slug string) string {
	return filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, OriginRecordRel())
}

// originLockPath returns the origin record's lock file path within weftPath's .weft lock
// directory.
//
// This is NOT the package's usual path+".lock" idiom: that idiom is safe only for the two records
// that live in the weft gitdir, and a never-tracked ".lock" file sitting beside a tracked record
// under the durable lyx directory would violate the Durable-vs-Ephemeral State Invariant.
// ensureWeftLockDirAt already creates its own directory, which is what supplies the lock's parent
// without introducing a new raw write that TestNoUncontainedWrite_FabricengineProductionSource
// would flag.
func originLockPath(weftPath string) (string, error) {
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(lockDir, originRecordLockFileName), nil
}

// ReadOrigin reads the origin record for l's worktree pair.
// A false second return means no record exists for this worktree — the legacy-worktree case — and
// is not an error.
func ReadOrigin(l *lyxcwd.Location) (Origin, bool, error) {
	lockPath, err := originLockPath(RecordsWorktree(l))
	if err != nil {
		return Origin{}, false, err
	}
	return state.ReadJSON[Origin](OriginRecordPath(l), lockPath)
}

// ReadOriginFor reads the origin record of slug's worktree pair, for a caller that is not running
// from that pair — Remove runs from another worktree, where ReadOrigin would read the caller's own.
// A false second return means there is no record to read and is not an error:
// an absent weft worktree or record answers that way.
// A record whose weft worktree lacks its .weft lock directory is an error naming the worktree and `lyx fabric reconcile`, which restores the directory.
// It creates no directory and seeds no exclude in the other pair's weft worktree,
// so reading a half-present pair never recreates a weft path it has already lost.
func ReadOriginFor(l *lyxcwd.Location, slug string) (Origin, bool, error) {
	weftPath := RecordsWorktreePath(l, slug)
	path := OriginRecordPathFor(l, slug)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Origin{}, false, nil
		}
		return Origin{}, false, err
	}
	lockDir := filepath.Join(weftPath, weftLockDirName)
	if _, err := os.Stat(lockDir); err != nil {
		if os.IsNotExist(err) {
			return Origin{}, false, fmt.Errorf("weft worktree %s has an origin record but its %s lock directory is missing; run `lyx fabric reconcile` to restore it", weftPath, weftLockDirName)
		}
		return Origin{}, false, err
	}
	return state.ReadJSON[Origin](path, filepath.Join(lockDir, originRecordLockFileName))
}

// WriteOrigin writes the origin record o for slug's worktree pair, then records KindFileWritten in
// rec.
//
// slug is what lets one function serve both callers: Topology.Add passes the new pair's slug, and
// a caller repairing its own worktree passes that worktree's own name, which resolves to the same
// file the read side reaches through the junction.
func WriteOrigin(rec *Mutations, l *lyxcwd.Location, slug string, o Origin) error {
	weftPath := RecordsWorktreePath(l, slug)
	lockPath, err := originLockPath(weftPath)
	if err != nil {
		return err
	}
	path := OriginRecordPathFor(l, slug)
	if err := state.WriteJSON(path, lockPath, o); err != nil {
		return err
	}
	// Record only after WriteJSON observably succeeded, per the Mutation Record Invariant's
	// "after the primitive observably changed state" rule.
	rec.Append(KindFileWritten, path, "")
	return nil
}
