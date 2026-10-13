// paths.go declares the shed run-directory path constructors: the durable paths under _lyx (the run directory, its seed and status files, and the drive-reports directory), the ephemeral paths under .lyx (including the driver park marker), the anchor-relative paths for fabric commit pathspecs (including the run-records root), the tree-relative seed and status paths a git tree read takes, and the hub-scoped ephemeral files that sit one level above any single run-id: its locks and the notice log.
// Every constructor that returns an absolute path joins onto the given *lyxcwd.Location's AnchorPath(), per the Cwd Resolution Invariant; the *Rel constructors and TreeRunFileRels return relative paths; none of them calls os.Getwd or any git command.

package shedrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// shedDirName is the relative-path segment shedrun joins onto lyxdirs.LyxDirName or
// lyxdirs.DotLyxDirName to scope every shed-run-owned path under its own subdirectory.
// internal/shedrun is this segment's sole declarer, per the Shed Run-Directory Invariant.
const shedDirName = "shed"

// driveReportsDirName is the segment under a run's directory that holds its drive reports.
const driveReportsDirName = "drive-reports"

// arbiterDirName is the segment under a run's directory that holds its arbiter attempts.
const arbiterDirName = "arbiter"

// tacklerDirName is the segment under a run's directory that holds its tackler attempts.
const tacklerDirName = "tackler"

// tacklerMarkerFileName is the filename of the tackler in-flight marker, whose path TacklerMarker returns.
const tacklerMarkerFileName = "tackler-inflight.json"

// driverLockFileName is the filename of the Go-driver lock, whose path DriverLock returns.
const driverLockFileName = "driver.lock"

// settleRecordsFileName is the filename of the Go driver's settle records, whose path SettleRecordsFile returns.
const settleRecordsFileName = "go-settle.jsonl"

// alignmentDirName is the segment under a run's directory that holds its alignment attempts, one directory per point.
const alignmentDirName = "alignment"

// arbiterMarkerFileName is the filename of the arbiter in-flight marker, whose path ArbiterMarker returns.
const arbiterMarkerFileName = "arbiter-inflight.json"

// ParkMarkerFileName is the filename of the driver park marker, whose path ParkMarker returns.
// It is exported because the driver stencil names the same filename, and a loomcli test pins the two together.
// The file's content is the path of the stop report the driver parked on.
// A loom-launched driver writes it at a hand-back and removes it before a self-initiated re-step;
// `lyx loom start` removes it when it resumes the driver, or before it spawns a fresh one.
const ParkMarkerFileName = "driver-parked"

// BattenWatchedFileName is the filename of the batten-watched marker, whose path BattenWatchedMarker returns.
// The file holds the pid, in decimal, of the live batten process watching the run on its first line, then, when known, the prime's batten scratch directory and the Run-Shed row's name on the next two lines.
// A batten writes it while it watches and removes it when it stops; a driver launched fresh renders its parent-notification rule from whether the pid in it is live.
const BattenWatchedFileName = "batten-watched"

// BattenWatchedContent renders the batten-watched marker's content: pid in decimal on the first line, then, when either is non-empty, scratchDir and producer on the next two lines.
func BattenWatchedContent(pid int, scratchDir, producer string) []byte {
	content := strconv.Itoa(pid) + "\n"
	if scratchDir != "" || producer != "" {
		content += scratchDir + "\n" + producer + "\n"
	}
	return []byte(content)
}

// ParseBattenWatched reads a batten-watched marker's content: the first line is the pid, and the next two lines, when present, are the scratch directory and the row's name.
// A one-line marker parses with an empty scratch directory and producer.
// A marker whose first line is not a positive pid is an error.
func ParseBattenWatched(data []byte) (pid int, scratchDir, producer string, err error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	pid, err = strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return 0, "", "", fmt.Errorf("shedrun: batten-watched marker: the first line %q is not a positive pid", lines[0])
	}
	if len(lines) > 1 {
		scratchDir = lines[1]
	}
	if len(lines) > 2 {
		producer = lines[2]
	}
	return pid, scratchDir, producer, nil
}

// StartNotParkedKind is the envelope "kind" of the `lyx loom start` refusal for a live driver whose run has halted at a hand-back but whose park marker is not written yet.
// That refusal is retryable: the driver is still writing its stop report and committing its records, and parks within seconds.
// It is exported so battencli recognises the refusal by this one declared value rather than by its message text.
const StartNotParkedKind = "driver_not_parked"

// StartDriverBusyKind is the envelope "kind" of the `lyx loom start` and `lyx loom resume` refusal for a parked driver whose pane would not take the resume line:
// shuttle held the send on a draft, an active operator or another busy reading.
// That refusal is retryable once the draft is cleared or sent.
// It is exported so battencli recognises the refusal by this one declared value rather than by its message text.
const StartDriverBusyKind = "driver_busy"

// ResumeReportFileName is the filename of the resume-report file, whose path ResumeReport returns.
// The file holds the park marker's content on its first line and, on its second, the stop-report path the resume branch picked for that park.
const ResumeReportFileName = "driver-resume-report"

// StartMergeInProgressKind is the envelope "kind" of the `lyx loom start` refusal for a start that would spawn or resume a driver over a pair carrying an unfinished merge.
// That refusal is NOT retryable: the operator must resolve, conclude or abort the merge first.
// It is exported so an unattended caller tells it apart from the retryable StartNotParkedKind by this one declared value rather than by message text.
const StartMergeInProgressKind = "merge_in_progress"

// noticeLogFileName is the filename of the hub notice log, whose path NoticeLog returns.
const noticeLogFileName = "notices.log"

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

// ParkMarker returns the path to the ephemeral driver park marker, ParkMarkerFileName under ScratchDir(l, runID), so it sits in the directory the full step envelope, held by the step record, reports as scratch_dir.
func ParkMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), ParkMarkerFileName)
}

// ResumeReport returns the path to the ephemeral resume-report file, ResumeReportFileName under ScratchDir(l, runID), beside the park marker.
// It keeps the stop-report path the resume branch picked for the current park, so every rerun against that park types the same resume line.
func ResumeReport(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), ResumeReportFileName)
}

// BattenWatchedMarker returns the path to the ephemeral batten-watched marker, BattenWatchedFileName under ScratchDir(l, runID).
func BattenWatchedMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), BattenWatchedFileName)
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

// ArbiterDir returns the path to the durable directory holding every arbiter attempt of a run:
// RunDir(l, runID) joined with arbiterDirName.
func ArbiterDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), arbiterDirName)
}

// ArbiterRel returns the worktree-anchor-relative form of ArbiterDir's path: the join of lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and arbiterDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func ArbiterRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), arbiterDirName)
}

// ArbiterAttemptDir returns the path to one arbiter attempt's durable directory, attempt-<N> under ArbiterDir(l, runID), where attempt is the attempt's 1-based number.
func ArbiterAttemptDir(l *lyxcwd.Location, runID string, attempt int) string {
	return filepath.Join(ArbiterDir(l, runID), "attempt-"+strconv.Itoa(attempt))
}

// AlignmentDir returns the path to the durable directory holding the alignment agent's attempts at one point of a run:
// RunDir(l, runID) joined with alignmentDirName and point, which is a single path segment.
func AlignmentDir(l *lyxcwd.Location, runID, point string) string {
	return filepath.Join(RunDir(l, runID), alignmentDirName, point)
}

// AlignmentRel returns the worktree-anchor-relative form of the run's alignment directory, which holds one directory per point: the join of lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and alignmentDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func AlignmentRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), alignmentDirName)
}

// ArbiterMarker returns the path to the ephemeral arbiter in-flight marker, arbiterMarkerFileName under ScratchDir(l, runID).
func ArbiterMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), arbiterMarkerFileName)
}

// DriverLock returns the path to the ephemeral Go-driver lock, driverLockFileName under ScratchDir(l, runID).
// It is distinct from the run lock and the status lock, so a live Go driver is observable without holding either.
func DriverLock(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), driverLockFileName)
}

// TacklerDir returns the path to the durable directory holding every tackler attempt of a run:
// RunDir(l, runID) joined with tacklerDirName.
func TacklerDir(l *lyxcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), tacklerDirName)
}

// TacklerRel returns the worktree-anchor-relative form of TacklerDir's path: the join of lyxdirs.LyxDirName, shedDirName, the run's directory segment under l, and tacklerDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func TacklerRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID), tacklerDirName)
}

// TacklerAttemptDir returns the path to one tackler attempt's durable directory, attempt-<N> under TacklerDir(l, runID), where attempt is the attempt's 1-based number.
func TacklerAttemptDir(l *lyxcwd.Location, runID string, attempt int) string {
	return filepath.Join(TacklerDir(l, runID), "attempt-"+strconv.Itoa(attempt))
}

// TacklerMarker returns the path to the ephemeral tackler in-flight marker, tacklerMarkerFileName under ScratchDir(l, runID).
func TacklerMarker(l *lyxcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), tacklerMarkerFileName)
}

// SettleRecordsFile returns the path to the Go driver's settle records, settleRecordsFileName in the told friction directory frictionDir.
// The name is not markdown, so the friction note scan never archives it and the caps counted from it survive a reflection.
func SettleRecordsFile(frictionDir string) string {
	return filepath.Join(frictionDir, settleRecordsFileName)
}

// RunsRootRel returns the worktree-anchor-relative path of the run-records root: the join of lyxdirs.LyxDirName and shedDirName, the directory every run's RunDir sits under.
// It takes no *lyxcwd.Location, because the root carries no run-id and so needs none of runSegment's alias resolution.
// It exists so fabricengine's Add can drop the whole run-records tree from a freshly forked pair without naming the shed segment itself.
// The path is anchor-relative, the same shape SeedRel and StatusRel return, suitable as a fabricengine commit's relPaths.
func RunsRootRel() string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName)
}

// RunsRoot returns the absolute durable directory every run directory sits under: the given *lyxcwd.Location's AnchorPath() joined with RunsRootRel().
func RunsRoot(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), RunsRootRel())
}

// ScratchRoot returns the absolute ephemeral directory every run's scratch directory sits under, the mirror of RunsRoot at the .lyx subpath.
// The hub-scoped prime run lock and landing lock live in it too.
func ScratchRoot(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName)
}

// RunDirRel returns the anchor-relative form of the run's durable directory, with the same directory-segment alias resolution the run's status path applies.
// It is the pathspec a records commit takes to delete a whole run directory.
func RunDirRel(l *lyxcwd.Location, runID string) string {
	return filepath.Join(lyxdirs.LyxDirName, shedDirName, runSegment(l, runID))
}

// RunTreeRels holds the slash-separated, tree-relative paths of one run's records, as a git tree read takes them.
type RunTreeRels struct {
	Dir          string
	Seed         string
	Status       string
	DriveReports string
	Arbiter      string
	Alignment    string
}

// TreeRunRels returns the tree-relative record paths of runID.
// It validates runID and applies no "self" alias and no legacy directory fallback, since a tree read from git has no disk to resolve against.
// A runID that fails validation returns that error and a zero value.
func TreeRunRels(runID string) (RunTreeRels, error) {
	if err := ValidateRunID(runID); err != nil {
		return RunTreeRels{}, err
	}
	dir := path.Join(lyxdirs.LyxDirName, shedDirName, runID)
	return RunTreeRels{
		Dir:          dir,
		Seed:         path.Join(dir, "seed.json"),
		Status:       path.Join(dir, "status.json"),
		DriveReports: path.Join(dir, driveReportsDirName),
		Arbiter:      path.Join(dir, arbiterDirName),
		Alignment:    path.Join(dir, alignmentDirName),
	}, nil
}

// TreeRunFileRels returns the slash-separated, tree-relative paths of a run's seed.json and status.json, as a git tree read takes them.
// It validates runID and applies no "self" alias and no legacy directory fallback, since a tree read from git has no disk to resolve against.
// A runID that fails validation returns that error and empty paths.
func TreeRunFileRels(runID string) (seedRel, statusRel string, err error) {
	if err := ValidateRunID(runID); err != nil {
		return "", "", err
	}
	return path.Join(lyxdirs.LyxDirName, shedDirName, runID, "seed.json"),
		path.Join(lyxdirs.LyxDirName, shedDirName, runID, "status.json"),
		nil
}

// PrimeRunLock returns the path to the hub-scoped ephemeral advisory lock that sits one level above
// any single run-id's own RunLock: the given *lyxcwd.Location's AnchorPath() joined with
// lyxdirs.DotLyxDirName, shedDirName, and "run.lock".
// Unlike RunLock, PrimeRunLock takes no runID: the lock it names is shared across every run this
// hub addresses, not scoped to one.
func PrimeRunLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, "run.lock")
}

// LandLock returns the path to the hub-scoped ephemeral advisory lock one landing holds for its whole body, beside PrimeRunLock and built the same way:
// the given *lyxcwd.Location's AnchorPath() joined with lyxdirs.DotLyxDirName, shedDirName, and "land.lock".
func LandLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, shedDirName, "land.lock")
}

// NoticeLog returns the path to the hub's ephemeral notice log, notices.log directly under the given *lyxcwd.Location's AnchorPath() joined with lyxdirs.DotLyxDirName.
// It sits outside RunsRoot and ScratchRoot, so no run-directory sweep reads it as a run or removes it.
func NoticeLog(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), NoticeLogRel())
}

// NoticeLogRel returns the anchor-relative form of NoticeLog's path.
func NoticeLogRel() string {
	return filepath.Join(lyxdirs.DotLyxDirName, noticeLogFileName)
}

// LoopLock returns the path to the loop's liveness lock, loop.lock under StepsDir(l, runID), which the detached loop holds for its whole life.
func LoopLock(l *lyxcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.lock")
}

// LoopPIDFile returns the path to loop.pid under StepsDir(l, runID), the record of the loop's process.
func LoopPIDFile(l *lyxcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.pid")
}

// LoopLog returns the path to loop.log under StepsDir(l, runID), the detached loop's stdout and stderr.
func LoopLog(l *lyxcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.log")
}

// LoopEnvelope returns the path to loop-envelope.json under StepsDir(l, runID), the file the waiter prints and retires.
func LoopEnvelope(l *lyxcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop-envelope.json")
}

// LoopDelivered returns the path a retired loop envelope is renamed into, one slot per loop id under StepsDir(l, runID).
// Every waiter on that loop can still print it, and a later loop's delivery never replaces it.
func LoopDelivered(l *lyxcwd.Location, runID, loopID string) string {
	return filepath.Join(StepsDir(l, runID), "loop-envelope."+loopID+".delivered.json")
}

// LoopJobName returns the Windows job-object name of a run's loop: "lyx-loop-" followed by a hex hash of the steps directory's absolute path.
// It is unique across hubs and free of path separators.
func LoopJobName(l *lyxcwd.Location, runID string) string {
	sum := sha256.Sum256([]byte(StepsDir(l, runID)))
	return "lyx-loop-" + hex.EncodeToString(sum[:8])
}

// StepTraceCopy returns the path to the copy of a step's trace the loop writes beside the step record, <traceID>.trace.log under StepsDir(l, runID).
// Retention sweeps the trace directory alone, so it never reaches the copy.
func StepTraceCopy(l *lyxcwd.Location, runID, traceID string) string {
	return filepath.Join(StepsDir(l, runID), traceID+".trace.log")
}

// StepStderr returns the path to a step child's captured stderr, <traceID>.stderr.log under StepsDir(l, runID).
func StepStderr(l *lyxcwd.Location, runID, traceID string) string {
	return filepath.Join(StepsDir(l, runID), traceID+".stderr.log")
}
