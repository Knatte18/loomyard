// origin.go declares fabric's provenance record for one worktree pair: the first *tracked*
// fabric-owned record under the durable lyx directory (_lyx/fabric/origin.json), committed by
// Topology.Add on the weft branch.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// Beam marks the pair as a beam.
	// A record written before the field existed decodes with it false.
	Beam bool `json:"beam,omitempty"`
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

// originRecordSubjectPrefix is the commit subject prefix of the commit that records a new pair's origin.
const originRecordSubjectPrefix = "fabric: record parent branch for "

// OriginRecordSubject returns the commit subject of the origin-record commit that opens the pair for slug.
// That commit is the oldest commit of the pair's own records history: everything below it is the parent's.
func OriginRecordSubject(slug string) string {
	return originRecordSubjectPrefix + slug
}

// OriginRecordSlug returns the slug an origin-record commit subject names.
// It is false for any other subject and for an empty slug.
func OriginRecordSlug(subject string) (string, bool) {
	slug, ok := strings.CutPrefix(subject, originRecordSubjectPrefix)
	if !ok || slug == "" {
		return "", false
	}
	return slug, true
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
			return Origin{}, false, fmt.Errorf("weft worktree %s has an origin record but its %s lock directory is missing; run `lyx fabric reconcile` to restore it; a session lyx refuses the verb from reports status: FAILED and the orch runs it", weftPath, weftLockDirName)
		}
		return Origin{}, false, err
	}
	return state.ReadJSON[Origin](path, filepath.Join(lockDir, originRecordLockFileName))
}

// ParentSlug returns the slug of the pair slug was forked from: the hub's pair worktree whose branch slug's origin record names as its parent branch.
// It returns "" when the parent branch is the prime's, when slug has no local pair or no origin record, and when no live pair holds the parent branch.
// A read error is returned as ReadOriginFor and the worktree listing report it.
func ParentSlug(l *lyxcwd.Location, slug string) (string, error) {
	origin, found, err := ReadOriginFor(l, slug)
	if err != nil || !found {
		return "", err
	}
	entries, err := List(l.AnchorPath())
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.Prunable || entry.Branch != origin.ParentBranch {
			continue
		}
		if entry.Main {
			return "", nil
		}
		return filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path))), nil
	}
	return "", nil
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

// beamMarkSubjectPrefix is the commit subject prefix of the commit that marks a pair as a beam.
const beamMarkSubjectPrefix = "fabric: mark beam "

// MarkBeam writes the beam mark into the origin record of slug's pair, keeping the record's parent fields, and commits that one path on the pair's records branch.
// It reports false and writes nothing when the record already carries the mark.
// A pair with no origin record is refused: a mark on a missing record would invent its parents.
// The caller pushes the commit through the pair's detached push; MarkBeam pushes nothing.
func MarkBeam(rec *Mutations, l *lyxcwd.Location, slug string) (bool, error) {
	weftPath := RecordsWorktreePath(l, slug)
	origin, found, err := ReadOriginFor(l, slug)
	if err != nil {
		return false, err
	}
	if !found {
		return false, missingOriginRecordError(l, slug, weftPath)
	}
	if origin.Beam {
		return false, nil
	}
	origin.Beam = true
	if err := WriteOrigin(rec, l, slug, origin); err != nil {
		return false, err
	}
	if _, _, err := CommitRecordsPaths(rec, weftPath, l.AnchorRel, []string{OriginRecordRel()}, beamMarkSubjectPrefix+slug, SyncOptions{}); err != nil {
		return false, fmt.Errorf("commit the beam mark of %s: %w", slug, err)
	}
	return true, nil
}

// missingOriginRecordError refuses a beam mark on slug's pair that has no origin record, naming the way forward for each of the two causes.
func missingOriginRecordError(l *lyxcwd.Location, slug, weftPath string) error {
	if _, err := os.Stat(weftPath); err != nil {
		return fmt.Errorf("pair %q has no records worktree at %s, so its origin record cannot be marked; way forward: run `lyx fabric reconcile`, which recreates the records worktree, then re-run the verb", slug, weftPath)
	}
	recordPath := filepath.ToSlash(filepath.Join(l.AnchorRel, OriginRecordRel()))
	return fmt.Errorf("pair %q has no origin record at %s in its records worktree %s, and a mark on a missing record would invent its parents; way forward: restore the record from the records history in that worktree, with `git checkout HEAD -- %s` while the deletion is uncommitted, else from the commit before the one that deleted it (`git log -1 --format=%%H -- %s` names that commit, and `git checkout <commit>^ -- %s` restores it), then re-run the verb; `lyx fabric reconcile` never rewrites a deleted record", slug, recordPath, weftPath, recordPath, recordPath, recordPath)
}
