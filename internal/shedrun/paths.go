// paths.go declares the shed run-directory path constructors: the durable paths under _lyx (the run directory, its seed and status files, and the drive-reports directory), the ephemeral paths under .lyx (including the driver park marker), the anchor-relative paths for fabric commit pathspecs (including the run-records root), and one hub-scoped ephemeral lock that sits one level above any single run-id.
// Every constructor is a plain filepath.Join onto the given *lyxcwd.Location's AnchorPath(), per the Cwd Resolution Invariant -- none of them calls os.Getwd or any git command.

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

// driveReportsDirName is the segment under a run's directory that holds its drive reports.
const driveReportsDirName = "drive-reports"

// ParkMarkerFileName is the filename of the driver park marker, whose path ParkMarker returns.
// It is exported because the ly-drive skill names the same filename, and a loomcli test pins the two together.
// The file's content is the path of the stop report the driver parked on.
// A loom-launched ly-drive driver writes it at a hand-back and removes it before a self-initiated re-step;
// `lyx loom start` removes it when it resumes the driver, or before it spawns a fresh one.
const ParkMarkerFileName = "driver-parked"

// StartNotParkedKind is the envelope "kind" of the `lyx loom start` refusal for a live driver whose run has halted at a hand-back but whose park marker is not written yet.
// That refusal is retryable: the driver is still writing its stop report and committing its records, and parks within seconds.
// It is exported so battencli recognises the refusal by this one declared value rather than by its message text.
const StartNotParkedKind = "driver_not_parked"

// StartMergeInProgressKind is the envelope "kind" of the `lyx loom start` refusal for a start that would spawn or resume a driver over a pair carrying an unfinished merge.
// That refusal is NOT retryable: the operator must resolve, conclude or abort the merge first.
// It is exported so an unattended caller tells it apart from the retryable StartNotParkedKind by this one declared value rather than by message text.
const StartMergeInProgressKind = "merge_in_progress"

// RunDir returns the path to the durable, fabric-synced directory holding a single run's
// seed.json and status.json: the given *lyxcwd.Location's AnchorPath() joined with
// lyxdirs.LyxDirName, shedDirName, and the run's directory segment (runSegment: the worktree slug
// for SelfRunID, or the legacy "self" directory when only that one exists).
func RunDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, shedDirName, runSegment(l, runID))
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
// shedDirName, and the same directory segment RunDir joins.
// Per the Durable-vs-Ephemeral State Invariant, it sits at the mirrored subpath of RunDir.
func ScratchDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, runSegment(l, runID))
}

// StepsDir returns the path to the ephemeral directory holding a run's `lyx shed step` records, under
// ScratchDir(l, runID), so it sits under .lyx per the Durable-vs-Ephemeral State Invariant.
func StepsDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "steps")
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

// ParkMarker returns the path to the ephemeral driver park marker, ParkMarkerFileName under ScratchDir(l, runID), so it sits in the directory the step envelope reports as scratch_dir.
func ParkMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), ParkMarkerFileName)
}

// SeedRel returns the worktree-anchor-relative form of SeedFile's path: the join of
// lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and "seed.json".
// It exists so a caller building a fabric commit pathspec for fabricengine.CommitAnchoredPaths'
// relPaths argument never has to name a directory segment shedrun owns.
// It takes l for the same alias resolution RunDir applies, so it names the file WriteSeed writes.
func SeedRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), "seed.json")
}

// StatusRel returns the worktree-anchor-relative form of StatusFile's path: the join of
// lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and "status.json".
// It exists so a caller building a fabric commit pathspec for fabricengine.CommitAnchoredPaths'
// relPaths argument never has to name a directory segment shedrun owns.
// It takes l for the same alias resolution RunDir applies, so it names the file StatusFile addresses.
func StatusRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), "status.json")
}

// DriveReportsDir returns the path to the durable directory holding a run's drive reports:
// RunDir(l, runID) joined with driveReportsDirName.
func DriveReportsDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), driveReportsDirName)
}

// DriveReportsRel returns the worktree-anchor-relative form of DriveReportsDir's path: the join of lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and driveReportsDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func DriveReportsRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), driveReportsDirName)
}

// RunsRootRel returns the worktree-anchor-relative path of the run-records root: the join of
// lyxdirs.LyxDirName and shedDirName, the directory every run's RunDir sits under.
// It takes no *lyxcwd.Location, because the root carries no run-id and so needs none of runSegment's alias resolution.
// It exists so fabricengine's Add can drop the whole run-records tree from a freshly forked pair without naming the shed segment itself.
// The path is anchor-relative, the same shape SeedRel and StatusRel return, suitable as a fabricengine commit's relPaths.
func RunsRootRel() string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName)
}

// PrimeRunLock returns the path to the hub-scoped ephemeral advisory lock that sits one level above
// any single run-id's own RunLock: the given *lyxcwd.Location's AnchorPath() joined with
// lyxdirs.DotLyxDirName, shedDirName, and "run.lock".
// Unlike RunLock, PrimeRunLock takes no runID: the lock it names is shared across every run this
// hub addresses, not scoped to one.
func PrimeRunLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, "run.lock")
}
