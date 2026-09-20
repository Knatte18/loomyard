// paths.go declares the ten shed run-directory path constructors: three durable paths under _lyx,
// four ephemeral paths under .lyx, two anchor-relative paths for fabric commit pathspecs, and one
// hub-scoped ephemeral lock that sits one level above any single run-id. Every constructor is a
// plain filepath.Join onto the given *lyxcwd.Location's AnchorPath(), per the Cwd Resolution
// Invariant -- none of them calls os.Getwd or any git command.

package shedrun

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// shedDirName is the relative-path segment shedrun joins onto lyxdirs.LyxDirName or
// lyxdirs.DotLyxDirName to scope every shed-run-owned path under its own subdirectory.
// internal/shedrun is this segment's sole declarer, per the Shed Run-Directory Invariant.
const shedDirName = "shed"

// RunDir returns the path to the durable, fabric-synced directory holding a single run's
// seed.json and status.json: the given *lyxcwd.Location's AnchorPath() joined with
// lyxdirs.LyxDirName, shedDirName, and runID.
func RunDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName, runID)
}

// SeedFile returns the path to a run's durable seed.json, under RunDir(l, runID).
func SeedFile(l *lyxcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), "seed.json")
}

// StatusFile returns the path to a run's durable status.json, under RunDir(l, runID).
func StatusFile(l *lyxcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), "status.json")
}

// ScratchDir returns the path to the ephemeral, never-tracked scratch directory mirroring RunDir at
// the .lyx subpath: the given *lyxcwd.Location's AnchorPath() joined with lyxdirs.DotLyxDirName,
// shedDirName, and runID.
// Per the Durable-vs-Ephemeral State Invariant, it sits at the mirrored subpath of RunDir.
func ScratchDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, runID)
}

// RunLock returns the path to a run's ephemeral advisory lock guarding the whole duration of the
// run, under ScratchDir(l, runID).
// It must never equal StatusLock(l, runID): shedengine.Shed's own validation rejects
// LockPath == StatusLockPath outright, and a shared file would hang on the first persist rather than
// fail.
func RunLock(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "run.lock")
}

// StatusLock returns the path to the advisory lock guarding concurrent access to
// StatusFile(l, runID), under ScratchDir(l, runID).
func StatusLock(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "status.json.lock")
}

// LastCommitMarker returns the path to the ephemeral marker recording the last commit this run's
// fabric sync observed, under ScratchDir(l, runID).
func LastCommitMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "last-commit")
}

// SeedRel returns the worktree-anchor-relative form of SeedFile's path: the join of
// lyxdirs.LyxDirName, shedDirName, runID, and "seed.json".
// It exists so a caller building a fabric commit pathspec for fabricengine.CommitAnchoredPaths'
// relPaths argument never has to name a directory segment shedrun owns.
func SeedRel(runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runID, "seed.json")
}

// StatusRel returns the worktree-anchor-relative form of StatusFile's path: the join of
// lyxdirs.LyxDirName, shedDirName, runID, and "status.json".
// It exists so a caller building a fabric commit pathspec for fabricengine.CommitAnchoredPaths'
// relPaths argument never has to name a directory segment shedrun owns.
func StatusRel(runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runID, "status.json")
}

// PrimeRunLock returns the path to the hub-scoped ephemeral advisory lock that sits one level above
// any single run-id's own RunLock: the given *lyxcwd.Location's AnchorPath() joined with
// lyxdirs.DotLyxDirName, shedDirName, and "run.lock".
// Unlike RunLock, PrimeRunLock takes no runID: the lock it names is shared across every run this
// hub addresses, not scoped to one.
func PrimeRunLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, "run.lock")
}
