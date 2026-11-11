// paths.go declares the shed run-directory path constructors.
// They cover the durable paths under _lym and the ephemeral paths under .lym.
// They also cover the anchor-relative paths for commit pathspecs and the tree-relative seed and status paths.
// The hub-scoped locks and notice log sit above any single run-id.
// Every constructor that returns an absolute path joins onto the given *lymcwd.Location's AnchorPath().
// The *Rel constructors and TreeRunFileRels return relative paths, and none of them calls os.Getwd or any git command.

package shedrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// shedDirName is the relative-path segment that scopes every shed-run-owned path under its own subdirectory.
// It is joined onto lymdirs.LymDirName or lymdirs.DotLymDirName.
// internal/shedrun is this segment's sole declarer.
const shedDirName = "shed"

// driveReportsDirName is the segment under a run's directory that holds its drive reports.
const driveReportsDirName = "drive-reports"

// arbiterDirName is the segment under a run's directory that holds its arbiter attempts.
const arbiterDirName = "arbiter"

// tacklerDirName is the segment under a run's directory that holds its tackler attempts.
const tacklerDirName = "tackler"

// tacklerMarkerFileName is the filename of the tackler in-flight marker, whose path TacklerMarker returns.
const tacklerMarkerFileName = "tackler-inflight.json"

// originOwedMarkerFileName is the filename of the origin-owed marker, whose path OriginOwedMarker returns.
const originOwedMarkerFileName = "origin-owed.json"

// driverLockFileName is the filename of the Go-driver lock, whose path DriverLock returns.
const driverLockFileName = "driver.lock"

// settleRecordsFileName is the filename of the Go driver's settle records, whose path SettleRecords returns.
const settleRecordsFileName = "go-settle.jsonl"

// alignmentDirName is the segment under a run's directory that holds its alignment attempts, one directory per point.
const alignmentDirName = "alignment"

// gateFindingsDirName is the segment under a run's scratch directory that holds one gate findings file per row.
const gateFindingsDirName = "gate-findings"

// gateFindingsFileSuffix is the suffix of a row's gate findings file, whose path GateFindingsFile returns.
const gateFindingsFileSuffix = ".md"

// arbiterMarkerFileName is the filename of the arbiter in-flight marker, whose path ArbiterMarker returns.
const arbiterMarkerFileName = "arbiter-inflight.json"

// ParkMarkerFileName is the filename of the driver park marker, whose path ParkMarker returns.
// It is exported because the driver stencil names the same filename, and a loomcli test pins the two together.
// The file's content is the path of the stop report the driver parked on.
// A loom-launched driver writes it at a hand-back and removes it before a self-initiated re-step;
// `lym loom start` removes it when it resumes the driver, or before it spawns a fresh one.
const ParkMarkerFileName = "driver-parked"

// BattenWatchedFileName is the filename of the batten-watched marker, whose path BattenWatchedMarker returns.
// The file holds, on its first line, the pid in decimal of the live batten process watching the run.
// When known, the next two lines hold the prime's batten scratch directory and the Run-Shed row's name.
// A batten writes it while it watches and removes it when it stops;
// a driver launched fresh renders its parent-notification rule from whether the pid in it is live.
const BattenWatchedFileName = "batten-watched"

// BattenWatchedContent renders the batten-watched marker's content.
// The pid goes in decimal on the first line;
// when either scratchDir or producer is non-empty, both go on the next two lines.
func BattenWatchedContent(pid int, scratchDir, producer string) []byte {
	content := strconv.Itoa(pid) + "\n"
	if scratchDir != "" || producer != "" {
		content += scratchDir + "\n" + producer + "\n"
	}
	return []byte(content)
}

// ParseBattenWatched reads a batten-watched marker's content: the first line is the pid.
// The next two lines, when present, are the scratch directory and the row's name.
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

// StartNotParkedKind is the envelope "kind" of the `lym loom start` refusal.
// It fires for a live driver whose run has halted at a hand-back but whose park marker is not written yet.
// That refusal is retryable.
// The driver is still writing its stop report and committing its records, and parks within seconds.
// It is exported so battencli recognises the refusal by this one declared value rather than by its message text.
const StartNotParkedKind = "driver_not_parked"

// StartDriverBusyKind is the envelope "kind" of the `lym loom start` and `lym loom resume` refusal.
// It is for a parked driver whose pane would not take the resume line:
// shuttle held the send on a draft, an active operator or another busy reading.
// That refusal is retryable once the draft is cleared or sent.
// It is exported so battencli recognises the refusal by this one declared value rather than by its message text.
const StartDriverBusyKind = "driver_busy"

// StartOwnedElsewhereKind is the envelope "kind" of the `lym loom start` and `lym loom resume` refusal.
// It is for a run another machine owns.
// That refusal is NOT retryable: the run is taken over with `lym adopt` or left alone.
// It is exported so batten's InnerRun row recognises the refusal by this one declared value, not by its message text.
// Batten's InnerRun row never counts it as a failed spawn.
const StartOwnedElsewhereKind = "owned_elsewhere"

// ResumeReportFileName is the filename of the resume-report file, whose path ResumeReport returns.
// The file holds the park marker's content on its first line.
// On its second line it holds the stop-report path the resume branch picked for that park.
const ResumeReportFileName = "driver-resume-report"

// StartMergeInProgressKind is the envelope "kind" of the `lym loom start` refusal.
// It is for a start that would spawn or resume a driver over a pair carrying an unfinished merge.
// That refusal is NOT retryable: the operator must resolve, conclude or abort the merge first.
// It is exported so an unattended caller recognises it by this one declared value rather than by message text.
const StartMergeInProgressKind = "merge_in_progress"

// RunDir returns the path to the durable, fabric-synced directory holding a single run's seed.json and status.json.
// It joins the given *lymcwd.Location's AnchorPath() with lymdirs.LymDirName, shedDirName and the run's segment.
// runSegment picks the segment: the worktree slug for SelfRunID.
// The legacy "self" directory serves when only that one exists.
func RunDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lymdirs.LymDirName, shedDirName, runSegment(l, runID))
}

// StopMarkerFileName is the filename of a run's writer stop marker, whose path StopMarker returns.
const StopMarkerFileName = "stop-marker"

// StopMarker returns the path to the marker a run's writer creates to say it stopped instead of making the change.
// The file is StopMarkerFileName under RunDir(l, runID).
// The run's producer removes it before each spawn and reads its presence after the session as a halt.
func StopMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), StopMarkerFileName)
}

// ConditionStopMarkerFileName is the filename of a run's writer condition-stop marker, whose path ConditionStopMarker returns.
const ConditionStopMarkerFileName = "condition-stop-marker"

// ConditionStopMarker returns the path to the marker a run's writer creates beside its stop marker.
// It says the writer stopped on a condition the task's body names.
// The file is ConditionStopMarkerFileName under RunDir(l, runID).
// The run's producer removes it before each spawn.
// It reads its presence, together with the stop marker, after the session.
func ConditionStopMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), ConditionStopMarkerFileName)
}

// SeedFile returns the path to a run's durable seed.json, under RunDir(l, runID).
func SeedFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), "seed.json")
}

// StatusFile returns the path to a run's durable status.json, under RunDir(l, runID).
func StatusFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), "status.json")
}

// ScratchDir returns the path to the ephemeral, never-tracked scratch directory of a run.
// It joins AnchorPath() with lymdirs.DotLymDirName, shedDirName and the same run segment RunDir uses.
// Per the Durable-vs-Ephemeral State Invariant, it sits at the mirrored subpath of RunDir.
func ScratchDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, shedDirName, runSegment(l, runID))
}

// StepsDir returns the path to the ephemeral directory of a run's `lym shed step` records.
// It sits under ScratchDir(l, runID).
func StepsDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "steps")
}

// RunLock returns the path to a run's ephemeral advisory lock guarding the whole duration of the run.
// It sits under ScratchDir(l, runID).
// It must never equal StatusLock(l, runID):
// shedengine.Shed's own validation rejects LockPath == StatusLockPath outright,
// and a shared file would hang on the first persist rather than fail.
func RunLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "run.lock")
}

// StatusLock returns the path to the advisory lock guarding concurrent access to StatusFile(l, runID).
// It sits under ScratchDir(l, runID).
func StatusLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "status.json.lock")
}

// PullRequestRecord returns the path to the ephemeral, never-tracked record of the run's pull request.
// It is pull-request.json under ScratchDir(l, runID).
func PullRequestRecord(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "pull-request.json")
}

// PullRequestRecordLock returns the path to the advisory lock guarding PullRequestRecord(l, runID).
// It sits beside that record under ScratchDir(l, runID).
func PullRequestRecordLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "pull-request.json.lock")
}

// ParentNoticeFile returns the path to the ephemeral file, pre-push-parent-notice under ScratchDir(l, runID).
// It holds the key of the parent-branch notice a run's pre-push check last posted.
func ParentNoticeFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "pre-push-parent-notice")
}

// SamplerRecord returns the path to the ephemeral, never-tracked record of the run's Sampler-Session row.
// It is sampler.json under ScratchDir(l, runID).
func SamplerRecord(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "sampler.json")
}

// SamplerRecordLock returns the path to the advisory lock guarding SamplerRecord(l, runID).
// It sits beside that record under ScratchDir(l, runID).
func SamplerRecordLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "sampler.json.lock")
}

// LastCommitMarker returns the path to the ephemeral marker recording the last commit this run's fabric sync observed.
// It sits under ScratchDir(l, runID).
func LastCommitMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "last-commit")
}

// ParkMarker returns the path to the ephemeral driver park marker, ParkMarkerFileName under ScratchDir(l, runID).
// It sits in the directory that the step record in the full step envelope reports as scratch_dir.
func ParkMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), ParkMarkerFileName)
}

// ResumeReport returns the path to the ephemeral resume-report file, ResumeReportFileName under ScratchDir(l, runID).
// It keeps the stop-report path the resume branch picked for the current park,
// so every rerun against that park types the same resume line.
func ResumeReport(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), ResumeReportFileName)
}

// BattenWatchedMarker returns the path to the ephemeral batten-watched marker.
// The file is BattenWatchedFileName under ScratchDir(l, runID).
func BattenWatchedMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), BattenWatchedFileName)
}

// GateFindingsFile returns the path to the ephemeral gate findings file of the row named row.
// It sits under ScratchDir(l, runID).
// There is one file per row: the row's producer writes it when its gate is exhausted,
// and the row's next call whose gates pass removes it.
// A goto, a resume and a pause leave the scratch directory alone, so the file outlives them.
func GateFindingsFile(l *lymcwd.Location, runID, row string) string {
	return filepath.Join(ScratchDir(l, runID), gateFindingsDirName, row+gateFindingsFileSuffix)
}

// SeedRel returns the worktree-anchor-relative form of SeedFile's path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and "seed.json".
// It exists so a caller building a fabric commit pathspec never has to name a directory segment shedrun owns.
// Such a pathspec is the relPaths argument of fabricengine.CommitAnchoredPaths.
// It takes l for the same alias resolution RunDir applies, so it names the file WriteSeed writes.
func SeedRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), "seed.json")
}

// StatusRel returns the worktree-anchor-relative form of StatusFile's path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and "status.json".
// It exists so a caller building a fabric commit pathspec never has to name a directory segment shedrun owns.
// Such a pathspec is the relPaths argument of fabricengine.CommitAnchoredPaths.
// It takes l for the same alias resolution RunDir applies, so it names the file StatusFile addresses.
func StatusRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), "status.json")
}

// SettleRecordsRel returns the worktree-anchor-relative form of SettleRecords' path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and settleRecordsFileName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func SettleRecordsRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), settleRecordsFileName)
}

// noticeEpisodeFileName is the file in a run's durable directory holding the keys of the notices dobby sent for it.
// It holds one key per line.
const noticeEpisodeFileName = "notice-episode"

// NoticeEpisodeFile returns the path of the run notices' marker: RunDir(l, runID) joined with noticeEpisodeFileName.
// It is durable so the keys of the notices sent reach the next machine with the run's records.
func NoticeEpisodeFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), noticeEpisodeFileName)
}

// NoticeEpisodeRel returns the worktree-anchor-relative form of NoticeEpisodeFile's path:
// the pathspec a records commit takes for it.
func NoticeEpisodeRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), noticeEpisodeFileName)
}

// DriveReportsDir returns the path to the durable directory holding a run's drive reports:
// RunDir(l, runID) joined with driveReportsDirName.
func DriveReportsDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), driveReportsDirName)
}

// DriveReportsRel returns the worktree-anchor-relative form of DriveReportsDir's path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and driveReportsDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func DriveReportsRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), driveReportsDirName)
}

// ArbiterDir returns the path to the durable directory holding every arbiter attempt of a run:
// RunDir(l, runID) joined with arbiterDirName.
func ArbiterDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), arbiterDirName)
}

// ArbiterRel returns the worktree-anchor-relative form of ArbiterDir's path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and arbiterDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func ArbiterRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), arbiterDirName)
}

// ArbiterAttemptDir returns the path to one arbiter attempt's durable directory, attempt-<N> under ArbiterDir(l, runID).
// The attempt argument is the attempt's 1-based number.
func ArbiterAttemptDir(l *lymcwd.Location, runID string, attempt int) string {
	return filepath.Join(ArbiterDir(l, runID), "attempt-"+strconv.Itoa(attempt))
}

// ArbiterMarker returns the path to the ephemeral arbiter in-flight marker.
// The file is arbiterMarkerFileName under ScratchDir(l, runID).
func ArbiterMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), arbiterMarkerFileName)
}

// DriverLock returns the path to the ephemeral Go-driver lock, driverLockFileName under ScratchDir(l, runID).
// It is distinct from the run lock and the status lock, so a live Go driver is observable without holding either.
func DriverLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), driverLockFileName)
}

// TacklerDir returns the path to the durable directory holding every tackler attempt of a run:
// RunDir(l, runID) joined with tacklerDirName.
func TacklerDir(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), tacklerDirName)
}

// TacklerRel returns the worktree-anchor-relative form of TacklerDir's path:
// the join of lymdirs.LymDirName, shedDirName, the run's directory segment under l, and tacklerDirName.
// It exists so a caller building a fabric commit pathspec never has to name a segment shedrun owns.
func TacklerRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), tacklerDirName)
}

// DriverRecordsRel returns the anchor-relative paths of a run's driver records for a commit pathspec.
// They are DriveReportsRel, ArbiterRel and TacklerRel, in that order.
// Each is included only when its directory holds a file anywhere beneath it.
// A pathspec that matches nothing fails the commit, so an empty directory is left out.
func DriverRecordsRel(l *lymcwd.Location, runID string) []string {
	dirs := []struct{ abs, rel string }{
		{DriveReportsDir(l, runID), DriveReportsRel(l, runID)},
		{ArbiterDir(l, runID), ArbiterRel(l, runID)},
		{TacklerDir(l, runID), TacklerRel(l, runID)},
	}
	var rels []string
	for _, dir := range dirs {
		if HoldsFile(dir.abs) {
			rels = append(rels, dir.rel)
		}
	}
	return rels
}

// HoldsFile reports whether dir is a directory holding at least one non-directory entry anywhere beneath it.
// It stops the walk at the first such entry.
// An absent, unreadable or non-directory dir holds none.
// A commit pathspec names a directory only when this holds, since a pathspec that matches nothing fails the commit.
func HoldsFile(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	found := false
	// A walk error can only end the walk before a file is found, so found alone is the answer.
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// TacklerAttemptDir returns the path to one tackler attempt's durable directory, attempt-<N> under TacklerDir(l, runID).
// The attempt argument is the attempt's 1-based number.
func TacklerAttemptDir(l *lymcwd.Location, runID string, attempt int) string {
	return filepath.Join(TacklerDir(l, runID), "attempt-"+strconv.Itoa(attempt))
}

// TacklerMarker returns the path to the ephemeral tackler in-flight marker.
// The file is tacklerMarkerFileName under ScratchDir(l, runID).
func TacklerMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), tacklerMarkerFileName)
}

// OriginOwedMarker returns the path to the ephemeral marker of a resume that judged its owner without origin.
// The file is originOwedMarkerFileName under ScratchDir(l, runID).
// While it stands, the run's passes stay full until one reaches origin.
func OriginOwedMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), originOwedMarkerFileName)
}

// CancelMarker returns the path to the ephemeral batten cancel marker.
// The file is <run segment>.cancel.json directly under ScratchRoot(l).
// It sits beside the per-slug scratch directories, never inside RunDir or ScratchDir.
// Removing a slug's records and scratch leaves it in place.
func CancelMarker(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchRoot(l), runSegment(l, runID)+".cancel.json")
}

// CancelMarkerLock returns the path to the lock whose holder is the cancel marker's live writer.
// The file is <run segment>.cancel.lock directly under ScratchRoot(l).
func CancelMarkerLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchRoot(l), runSegment(l, runID)+".cancel.lock")
}

// SettleRecords returns the path to the Go driver's settle records, settleRecordsFileName under RunDir(l, runID).
// It sits beside the drive reports, and a status commit stages it through SettleRecordsRel.
func SettleRecords(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), settleRecordsFileName)
}

// RunsRootRel returns the worktree-anchor-relative path of the run-records root:
// the join of lymdirs.LymDirName and shedDirName, the directory every run's RunDir sits under.
// It takes no *lymcwd.Location: the root carries no run-id, so it needs none of runSegment's alias resolution.
// A caller that needs the whole run-records tree uses it, so none of them names the shed segment itself.
// The path is anchor-relative, so it is suitable as a fabricengine commit's relPaths.
func RunsRootRel() string {
	return filepath.Join(lymdirs.LymDirName, shedDirName)
}

// RunsRoot returns the absolute durable directory every run directory sits under:
// the given *lymcwd.Location's AnchorPath() joined with RunsRootRel().
func RunsRoot(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), RunsRootRel())
}

// ScratchRoot returns the absolute ephemeral directory every run's scratch directory sits under:
// the given *lymcwd.Location's AnchorPath() joined with lymdirs.DotLymDirName and shedDirName.
// The hub-scoped prime run lock and landing lock live in it too.
func ScratchRoot(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, shedDirName)
}

// RunDirRel returns the anchor-relative form of the run's durable directory.
// It resolves the directory segment the same way the run's status path does.
// It is the pathspec a records commit takes to delete a whole run directory.
func RunDirRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID))
}

// RunTreeRels holds the slash-separated, tree-relative paths of one run's records, as a git tree read takes them.
type RunTreeRels struct {
	Dir          string
	Seed         string
	Status       string
	DriveReports string
	Arbiter      string
	Alignment    string
	// SettleRecords is the Go driver's settle-records file.
	SettleRecords string
}

// TreeRunRels returns the tree-relative record paths of runID.
// It validates runID and applies no "self" alias and no legacy directory fallback:
// a tree read from git has no disk to resolve against.
// A runID that fails validation returns that error and a zero value.
func TreeRunRels(runID string) (RunTreeRels, error) {
	if err := ValidateRunID(runID); err != nil {
		return RunTreeRels{}, err
	}
	dir := path.Join(lymdirs.LymDirName, shedDirName, runID)
	return RunTreeRels{
		Dir:           dir,
		Seed:          path.Join(dir, "seed.json"),
		Status:        path.Join(dir, "status.json"),
		DriveReports:  path.Join(dir, driveReportsDirName),
		Arbiter:       path.Join(dir, arbiterDirName),
		Alignment:     path.Join(dir, alignmentDirName),
		SettleRecords: path.Join(dir, settleRecordsFileName),
	}, nil
}

// TreeRunFileRels returns the tree-relative paths of a run's seed.json and status.json.
// They are slash-separated, as a git tree read takes them.
// It validates runID and applies no "self" alias and no legacy directory fallback:
// a tree read from git has no disk to resolve against.
// A runID that fails validation returns that error and empty paths.
func TreeRunFileRels(runID string) (seedRel, statusRel string, err error) {
	if err := ValidateRunID(runID); err != nil {
		return "", "", err
	}
	return path.Join(lymdirs.LymDirName, shedDirName, runID, "seed.json"),
		path.Join(lymdirs.LymDirName, shedDirName, runID, "status.json"),
		nil
}

// PrimeRunLock returns the path to the hub-scoped ephemeral advisory lock that every run this hub addresses shares:
// the given *lymcwd.Location's AnchorPath() joined with lymdirs.DotLymDirName, shedDirName, and "run.lock".
// It takes no runID, since the lock is not scoped to any one run.
func PrimeRunLock(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, shedDirName, "run.lock")
}

// LandLock returns the path to the hub-scoped ephemeral advisory lock one landing holds for its whole body:
// the given *lymcwd.Location's AnchorPath() joined with lymdirs.DotLymDirName, shedDirName, and "land.lock".
func LandLock(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, shedDirName, "land.lock")
}

// LoopLock returns the path to the loop's liveness lock, loop.lock under StepsDir(l, runID).
// The detached loop holds it for its whole life.
func LoopLock(l *lymcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.lock")
}

// LoopPIDFile returns the path to loop.pid under StepsDir(l, runID), the record of the loop's process.
func LoopPIDFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.pid")
}

// LoopLog returns the path to loop.log under StepsDir(l, runID), the stdout and stderr of the detached loop and its keeper.
func LoopLog(l *lymcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop.log")
}

// LoopEnvelope returns the path to loop-envelope.json under StepsDir(l, runID), the file the waiter prints and retires.
func LoopEnvelope(l *lymcwd.Location, runID string) string {
	return filepath.Join(StepsDir(l, runID), "loop-envelope.json")
}

// LoopDelivered returns the path a retired loop envelope is renamed into, one slot per loop id under StepsDir(l, runID).
// Every waiter on that loop can still print it, and a later loop's delivery never replaces it.
func LoopDelivered(l *lymcwd.Location, runID, loopID string) string {
	return filepath.Join(StepsDir(l, runID), "loop-envelope."+loopID+".delivered.json")
}

// LoopJobName returns the Windows job-object name of a run's loop:
// "lym-loop-" followed by a hex hash of the steps directory's absolute path.
// It is unique across hubs and free of path separators.
func LoopJobName(l *lymcwd.Location, runID string) string {
	sum := sha256.Sum256([]byte(StepsDir(l, runID)))
	return "lym-loop-" + hex.EncodeToString(sum[:8])
}

// StepTraceCopy returns the path to the copy of a step's trace the loop writes:
// <traceID>.trace.log under StepsDir(l, runID), beside the step record.
// Retention sweeps the trace directory alone, so it never reaches the copy.
func StepTraceCopy(l *lymcwd.Location, runID, traceID string) string {
	return filepath.Join(StepsDir(l, runID), traceID+".trace.log")
}

// StepStderr returns the path to a step child's captured stderr, <traceID>.stderr.log under StepsDir(l, runID).
func StepStderr(l *lymcwd.Location, runID, traceID string) string {
	return filepath.Join(StepsDir(l, runID), traceID+".stderr.log")
}
