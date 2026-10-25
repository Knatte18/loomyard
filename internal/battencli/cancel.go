// cancel.go holds the decisions and order of "lyx batten cancel": cancelRun judges a run against origin and the local records, ends it, and removes what the verb's flags name, over injected seams.
//
// The verb and its production seams live beside the CLI; this file reaches no substrate itself, so one untagged table test drives every branch with fakes.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// windowsGOOS is the GOOS value whose process table cannot tell a recycled pid from the registered process.
const windowsGOOS = "windows"

// cancelDeps carries every substrate cancelRun touches, as function values; none is nil.
// The production builder fills each one over the hub, and the test fills each one with a fake.
type cancelDeps struct {
	// HubPaused reports whether a live hub-scope pause entry covers the prime.
	HubPaused func() (bool, error)
	// ExemptGate exempts the process from every pause gate and returns the restore.
	ExemptGate func() (restore func(), err error)
	// TaskWorktreePresent reports whether the slug's task worktree is on disk.
	TaskWorktreePresent func() (bool, error)
	// Reconcile runs the run-scope origin reconcile for the slug, judging only: it moves no ref.
	Reconcile func() (fabricengine.OriginReport, error)
	// PullBolt brings the hub's Bolt level with origin; an unreachable origin wraps boardengine.ErrOwnerOriginUnreachable.
	PullBolt func() error
	// Owner returns this machine's name and the slug's effective owner on the local Bolt, empty when none is recorded.
	Owner func() (machine, owner string, err error)
	// ReadStatus reads batten's status file for the slug.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// BoardStatus reads the status of the slug's entry on the local board.
	BoardStatus func() (status *string, found bool, err error)
	// RecordsPresent reports whether the slug's records run directory exists, or is deleted on disk but still tracked at the records HEAD.
	RecordsPresent func() (bool, error)
	// ChildReachedParent reports whether the child's work reached its parent, given batten's status.
	ChildReachedParent func(batten shedengine.Status, battenFound bool) (bool, error)
	// ReadMarker reads the slug's cancel marker and the liveness of its writer.
	ReadMarker func() (cancelMarkerReading, error)
	// TakeMarker takes the cancel marker's lock and writes the marker.
	// release drops the lock and removes the marker when asked; fresh is false for a marker a dead writer left.
	TakeMarker func() (release func(remove bool) error, fresh bool, err error)
	// GOOS is the operating system, which decides whether a registered pid is judged against its creation time.
	GOOS string
	// LiveRecords lists the slug's registered processes.
	LiveRecords func() ([]runproc.Record, error)
	// CreatedAt reads when the process of a pid was created; ok is false when it cannot be read.
	CreatedAt func(pid int) (created time.Time, ok bool)
	// PairParts reads which parts of the slug's pair are present locally.
	PairParts func() (fabricengine.PairParts, error)
	// SessionLive reports whether the pair's reed session is live.
	SessionLive func() (bool, error)
	// EndForCancel runs the teardown's cancel phase.
	EndForCancel func(ctx context.Context, req pairteardown.CancelRequest) (pairteardown.CancelResult, error)
	// TakeBattenLocks takes batten's prime-side driver lock and run lock, each waited for at most pairteardown.CancelLockWait.
	// A non-empty busy names the lock still held, and then nothing is held.
	TakeBattenLocks func() (release func(), busy string, err error)
	// StopTackler stops the strand the slug's tackler marker names and returns its guid, empty when none was named.
	StopTackler func() (guid string, err error)
	// RemovePair removes the pair through the teardown's removal.
	RemovePair func(req pairteardown.Request) (fabricengine.RemoveResult, error)
	// TaskBranchOnOrigin reports whether the slug's task branch is on origin.
	TaskBranchOnOrigin func() (bool, error)
	// DiscardTaskBranch archives and deletes the task branch, locally and on origin.
	DiscardTaskBranch func() (fabricengine.DiscardResult, error)
	// RemoveRecords removes the slug's records and scratch under batten's status lock, held only for the call.
	// committed reports whether a records commit was made.
	RemoveRecords func() (committed bool, err error)
	// ClearClaim clears a run-form status of the slug's board entry and reports whether it cleared one and whether an entry was found.
	ClearClaim func() (cleared, found bool, err error)
	// LiftPauses lifts the slug's own live slug-scope pause entries and returns their scopes.
	LiftPauses func() ([]string, error)
	// SpawnPush starts the detached push of the records side.
	SpawnPush func() error
	// Notify posts line to the orch.
	Notify func(line string) error
}

// cancelReport is what one cancel did, or with EndOnly what it ended before stopping.
type cancelReport struct {
	// Slug names the run.
	Slug string
	// State and Row are batten's state and current row when the cancel read its status; both are empty when none was readable.
	State string
	Row   string
	// Ended lists the registered processes the cancel ended, and StrandsStopped the strands it stopped through shuttle.
	Ended          []runproc.Ended
	StrandsStopped []string
	// TacklerStopped is the guid of the tackler strand stopped from the marker, empty when none was.
	TacklerStopped string
	// PairRemoved is true when the cancel removed the pair, or found nothing of it left.
	PairRemoved bool
	// SnapshotTag names the snapshot tag that holds the pair's uncommitted paths, empty when none was taken.
	SnapshotTag string
	// ArchiveTags lists the archive tags that hold the deleted task branch tips.
	ArchiveTags []string
	// RecordsRemoved is true when the run's records and scratch were removed, and RecordsCommitted when that made a records commit.
	RecordsRemoved   bool
	RecordsCommitted bool
	// ClaimCleared is true when a run-form board status was cleared.
	ClaimCleared bool
	// Lifted lists the scopes of the slug's own pause entries that were lifted.
	Lifted []string
	// PushSpawned is true when the records push was started, and NoticePosted when the orch notice went out.
	PushSpawned  bool
	NoticePosted bool
	// EndOnly is true when origin was unreachable, so the cancel ended the run's processes and stopped.
	EndOnly bool
	// WayForward names the step that follows the cancel, empty when none does.
	WayForward string

	// hold is what endRunPhase leaves held for the rest of the cancel; nil once the phase refused.
	hold *cancelHold
	// pairKept is true when the pair was present and the cancel keeps it.
	pairKept bool
}

// cancelHold is what endRunPhase leaves held on success: the pause-gate exemption, batten's locks and the cancel marker.
// The marker lock is dropped, and the marker file removed or kept, by finishMarker.
type cancelHold struct {
	restoreGate   func()
	releaseLocks  func()
	releaseMarker func(remove bool) error
	markerDone    bool
}

// releaseBattenLocks drops batten's locks once; it does nothing when none is held.
func (h *cancelHold) releaseBattenLocks() {
	if h.releaseLocks != nil {
		h.releaseLocks()
		h.releaseLocks = nil
	}
}

// finishMarker drops the marker lock once, removing the marker file when remove is set; it does nothing when no marker was taken or it is already dropped.
func (h *cancelHold) finishMarker(remove bool) error {
	if h.releaseMarker == nil || h.markerDone {
		return nil
	}
	h.markerDone = true
	return h.releaseMarker(remove)
}

// end drops whatever is still held, keeping the marker file when finishMarker did not remove it, and restores the pause gate last.
func (h *cancelHold) end(slug string) {
	h.releaseBattenLocks()
	if err := h.finishMarker(false); err != nil {
		logger.Warn("battencli: could not release the cancel marker", "slug", slug, "cause", err)
	}
	if h.restoreGate != nil {
		h.restoreGate()
	}
}

// cancelCommand words the cancel's own command for slug, with its --remove flag when remove is set.
func cancelCommand(slug string, remove bool) string {
	if remove {
		return fmt.Sprintf("lyx batten cancel %s --remove", slug)
	}
	return fmt.Sprintf("lyx batten cancel %s", slug)
}

// cancelRun ends the batten run of slug for good, in the order every refusal and acceptance of the verb follows.
// It refuses before it ends or removes anything unless a step's text says otherwise, and every step is a no-op once done, so a cancel run again finishes an interrupted one.
// An unreachable origin makes the cancel end-only: the run's processes and strands are ended, and the rest waits for a cancel run once origin answers.
func cancelRun(ctx context.Context, slug string, remove bool, deps cancelDeps) (cancelReport, error) {
	rerun := fmt.Sprintf("%q", cancelCommand(slug, remove))

	report, err := endRunPhase(ctx, slug, remove, rerun, deps)
	if err != nil {
		return report, err
	}
	defer report.hold.end(slug)
	if report.EndOnly {
		postNotice(&report, deps)
		return report, nil
	}

	if err := clearRunPhase(slug, remove, rerun, &report, deps); err != nil {
		return report, err
	}
	postNotice(&report, deps)
	if err := report.hold.finishMarker(true); err != nil {
		return report, fmt.Errorf("battencli: could not remove the cancel marker of %s: %w; way forward: run %s again", slug, err, rerun)
	}
	return report, nil
}

// endRunPhase is the first half of a cancel: it judges the run, takes the cancel marker and ends the run's processes and strands, stopping before the pair, the records and the claim.
// rerun is the quoted command every refusal names as the step to run again.
// On success it leaves the pause-gate exemption, batten's locks and the marker held in the report, for the caller to finish or drop; a refusal drops them itself.
// An unreachable origin makes the phase end-only: the report says so, holds no batten lock and names a re-run once origin answers.
func endRunPhase(ctx context.Context, slug string, remove bool, rerun string, deps cancelDeps) (cancelReport, error) {
	report := cancelReport{Slug: slug}
	hold := &cancelHold{}
	handedOver := false
	defer func() {
		if !handedOver {
			hold.end(slug)
		}
	}()

	paused, err := deps.HubPaused()
	if err != nil {
		return report, probeRefusal("the hub pause record", slug, rerun, err)
	}
	if paused {
		return report, fmt.Errorf("a hub pause covers the prime; way forward: run \"lyx tender resume\", then %s again", rerun)
	}
	hold.restoreGate, err = deps.ExemptGate()
	if err != nil {
		return report, probeRefusal("the pause gate", slug, rerun, err)
	}

	endOnly, err := judgeOrigin(slug, rerun, deps)
	if err != nil {
		return report, err
	}

	status, statusFound, err := deps.ReadStatus()
	if err != nil {
		return report, probeRefusal("batten's status", slug, rerun, err)
	}
	if statusFound {
		report.State, report.Row = string(status.State), status.CurrentProducer
	}
	reached, err := deps.ChildReachedParent(status, statusFound)
	if err != nil {
		return report, probeRefusal("whether the work reached its parent", slug, rerun, err)
	}
	if reached {
		return report, fmt.Errorf("the work of %s reached its parent; way forward: run \"lyx batten run %s\" to finish the landing, then \"lyx batten cleanup --apply\"", slug, slug)
	}

	reading, err := deps.ReadMarker()
	if err != nil {
		return report, probeRefusal("the cancel marker", slug, rerun, err)
	}
	parts, err := deps.PairParts()
	if err != nil {
		return report, probeRefusal("the pair's parts", slug, rerun, err)
	}
	if err := refuseNothingLeft(slug, remove, endOnly, rerun, reading, parts, deps); err != nil {
		return report, err
	}
	if reading.Present && reading.WriterAlive {
		return report, cancelMarkerRefusal(slug, reading)
	}

	var fresh bool
	hold.releaseMarker, fresh, err = deps.TakeMarker()
	if err != nil {
		hold.releaseMarker = nil
		return report, err
	}

	if live, err := windowsLive(deps); err != nil {
		return report, probeRefusal("the registered processes", slug, rerun, err)
	} else if len(live) > 0 {
		return report, errors.Join(windowsLiveRefusal(slug, rerun, live), hold.finishMarker(fresh))
	}

	ended, err := deps.EndForCancel(ctx, pairteardown.CancelRequest{
		Slug: slug,
		Keep: !remove,
		AfterEnd: func() (string, error) {
			release, busy, err := deps.TakeBattenLocks()
			if err != nil || busy != "" {
				return busy, err
			}
			live, err := windowsLive(deps)
			if err == nil && len(live) > 0 {
				err = windowsLiveRefusal(slug, rerun, live)
			}
			if err != nil {
				release()
				return "", err
			}
			hold.releaseLocks = release
			return "", nil
		},
	})
	report.Ended, report.StrandsStopped = ended.Ended, ended.StrandsStopped
	if err != nil {
		return report, err
	}
	report.TacklerStopped, err = deps.StopTackler()
	if err != nil {
		return report, fmt.Errorf("battencli: could not stop the tackler strand of %s: %w; way forward: fix the cause, then run %s again", slug, err, rerun)
	}

	handedOver = true
	report.hold = hold
	if endOnly {
		hold.releaseBattenLocks()
		report.EndOnly = true
		report.WayForward = fmt.Sprintf("run %s once origin answers", rerun)
		logger.Warn("battencli: origin was unreachable, so the cancel only ended the run's processes", "slug", slug)
		return report, nil
	}
	report.pairKept = parts.Any() && !remove
	return report, nil
}

// clearRunPhase is the second half of a cancel: it removes the pair and its task branch when remove is set, removes the run's records, clears the board claim, lifts the slug's own pauses and spawns the records push.
// It releases the batten locks report holds once the claim is cleared, and leaves the cancel marker and the pause gate to the caller.
// Its errors name rerun as the step to run again.
func clearRunPhase(slug string, remove bool, rerun string, report *cancelReport, deps cancelDeps) error {
	if remove {
		if err := removePairAndBranch(slug, rerun, report, deps); err != nil {
			return err
		}
	}

	committed, err := deps.RemoveRecords()
	if err != nil {
		return fmt.Errorf("battencli: could not remove the records of %s: %w; way forward: fix the cause, then run %s again", slug, err, rerun)
	}
	report.RecordsRemoved, report.RecordsCommitted = true, committed

	report.ClaimCleared, _, err = deps.ClearClaim()
	if err != nil {
		return fmt.Errorf("battencli: could not clear the board claim of %s: %w; way forward: fix the cause, then run %s again", slug, err, rerun)
	}

	if report.hold != nil {
		report.hold.releaseBattenLocks()
	}
	lifted, err := deps.LiftPauses()
	if err != nil {
		logger.Warn("battencli: could not lift the slug's pause entries", "slug", slug, "cause", err)
	}
	report.Lifted = lifted
	if committed {
		if err := deps.SpawnPush(); err != nil {
			logger.Warn("battencli: could not spawn the records push", "slug", slug, "cause", err)
		} else {
			report.PushSpawned = true
		}
	}
	if report.pairKept {
		report.WayForward = fmt.Sprintf("run %q before rerunning it", cancelCommand(slug, true))
	}
	return nil
}

// judgeOrigin judges the run against origin and refuses what origin or the local Bolt rules out.
// rerun is the quoted command a refusal names as the step to run again.
// endOnly is true when origin was unreachable and the local Bolt does not own the slug elsewhere.
func judgeOrigin(slug, rerun string, deps cancelDeps) (endOnly bool, err error) {
	present, err := deps.TaskWorktreePresent()
	if err != nil {
		return false, probeRefusal("the task worktree", slug, rerun, err)
	}

	var report fabricengine.OriginReport
	if present {
		report, err = deps.Reconcile()
		if err != nil {
			return false, fmt.Errorf("battencli: origin reconcile of %q failed: %w; way forward: fix the cause it names, then run %s again", slug, err, rerun)
		}
		endOnly = report.Unreachable
	} else {
		switch err := deps.PullBolt(); {
		case errors.Is(err, boardengine.ErrOwnerOriginUnreachable):
			endOnly = true
		case err != nil:
			return false, fmt.Errorf("battencli: pulling the board of %q failed: %w; way forward: fix the cause it names, then run %s again", slug, err, rerun)
		}
	}

	if endOnly || !present {
		machine, owner, err := deps.Owner()
		if err != nil {
			return false, probeRefusal("the slug's owner", slug, rerun, err)
		}
		if owner != "" && owner != machine {
			return false, cancelOwnedElsewhere(slug, owner)
		}
	}
	if endOnly {
		return true, nil
	}

	for _, entry := range report.Entries {
		if entry.Slug != slug {
			continue
		}
		switch entry.State {
		case fabricengine.EntryOwnedElsewhere:
			return false, cancelOwnedElsewhere(slug, entry.Owner)
		case fabricengine.EntryStopped:
			if stoppedOnlyByAnchor(slug, report.Branches) {
				break
			}
			return false, stoppedRefusal(slug, "cancel", fmt.Sprintf("then run %s again", rerun), entry, report.Branches)
		case fabricengine.EntryEnded:
			if entry.How != "done" {
				break
			}
			recordsHere, err := deps.RecordsPresent()
			if err != nil {
				return false, probeRefusal("the run's records", slug, rerun, err)
			}
			if recordsHere {
				return false, fmt.Errorf("%s was landed by another machine; way forward: run \"lyx cleanup --apply\" from the prime, whose reconcile tears it down", slug)
			}
		}
	}
	return false, nil
}

// stoppedOnlyByAnchor reports whether every branch stopping the entry of slug is anchor_stopped, and at least one is.
// The `--set` an anchor stop names refuses a worktree with an unfinished run, so the cancel that clears the run is the exit to take first.
// The branches counted are the ones stoppedRefusal names: the entry's own, bar its run's busy records branch, and those every pair shares, bar a busy one.
func stoppedOnlyByAnchor(slug string, branches []fabricengine.BranchOutcome) bool {
	anchorStopped := false
	for _, branch := range branches {
		own := branch.Slug == slug && !(branch.Repo == sideRecords && branch.State == fabricengine.BranchBusy)
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !own && !(shared && branch.State != fabricengine.BranchBusy) {
			continue
		}
		if branch.WayForward == "" {
			continue
		}
		if branch.State != fabricengine.BranchAnchorStopped {
			return false
		}
		anchorStopped = true
	}
	return anchorStopped
}

// cancelOwnedElsewhere words the refusal of a cancel on a machine that does not own the slug.
func cancelOwnedElsewhere(slug, owner string) error {
	return fmt.Errorf("%s is owned by machine %s; way forward: run \"lyx adopt %s\" on this machine first, or \"lyx batten cancel %s\" on %s", slug, owner, slug, slug, owner)
}

// refuseNothingLeft refuses a cancel when no record, claim, pair part, session, process or marker of the slug is left, and for a task branch left alone when remove is not set.
// With remove the task branch on origin counts as left, read only when nothing else is left and origin was reached.
func refuseNothingLeft(slug string, remove, endOnly bool, rerun string, reading cancelMarkerReading, parts fabricengine.PairParts, deps cancelDeps) error {
	recordsHere, err := deps.RecordsPresent()
	if err != nil {
		return probeRefusal("the run's records", slug, rerun, err)
	}
	boardStatus, _, err := deps.BoardStatus()
	if err != nil {
		return probeRefusal("the board entry", slug, rerun, err)
	}
	sessionLive, err := deps.SessionLive()
	if err != nil {
		return probeRefusal("the pair's session", slug, rerun, err)
	}
	records, err := deps.LiveRecords()
	if err != nil {
		return probeRefusal("the registered processes", slug, rerun, err)
	}

	otherLeft := recordsHere || boardengine.IsRunStatus(boardStatus) || sessionLive || len(records) > 0 || reading.Present ||
		parts.TaskWorktree || parts.RecordsWorktree || parts.RecordsBranch || parts.Launchers
	if otherLeft {
		return nil
	}
	if !remove {
		if parts.TaskBranch {
			return fmt.Errorf("the task branch of %s is all that is left; way forward: run %q, which archives and deletes its task branch", slug, cancelCommand(slug, true))
		}
		return fmt.Errorf("nothing of %s is left to cancel; way forward: check it with \"lyx board list\"", slug)
	}
	if parts.TaskBranch {
		return nil
	}
	if endOnly {
		return fmt.Errorf("origin was unreachable, so the task branch of %s on origin could not be read; way forward: run %s again once origin answers", slug, rerun)
	}
	onOrigin, err := deps.TaskBranchOnOrigin()
	if err != nil {
		return probeRefusal("the task branch on origin", slug, rerun, err)
	}
	if onOrigin {
		return nil
	}
	return fmt.Errorf("nothing of %s is left to cancel; way forward: check it with \"lyx board list\"", slug)
}

// removePairAndBranch removes the pair, snapshotting its uncommitted paths first, then archives and deletes the task branch while one is left.
// A pair with nothing left reads as removed.
func removePairAndBranch(slug, rerun string, report *cancelReport, deps cancelDeps) error {
	result, err := deps.RemovePair(pairteardown.Request{Slug: slug, Force: true, Snapshot: true, Remote: true, LeaveClaim: true})
	report.SnapshotTag = result.SnapshotTag
	if err != nil && !errors.Is(err, fabricengine.ErrPairNotFound) {
		return fmt.Errorf("battencli: removing the pair of %s refused: %w; way forward: fix the cause it names, then run \"lyx batten cancel %s --remove\" again", slug, err, slug)
	}
	report.PairRemoved = true

	parts, err := deps.PairParts()
	if err != nil {
		return probeRefusal("the pair's parts", slug, rerun, err)
	}
	onOrigin, err := deps.TaskBranchOnOrigin()
	if err != nil {
		return probeRefusal("the task branch on origin", slug, rerun, err)
	}
	if !parts.TaskBranch && !onOrigin {
		return nil
	}
	discard, err := deps.DiscardTaskBranch()
	report.ArchiveTags = discard.ArchiveTags
	if err != nil {
		return fmt.Errorf("battencli: discarding the task branch of %s failed: %w; way forward: fix the cause it names, then run \"lyx batten cancel %s --remove\" again", slug, err, slug)
	}
	return nil
}

// postNotice posts the cancel's notice and records whether it went out; a failure only warns.
func postNotice(report *cancelReport, deps cancelDeps) {
	if err := deps.Notify(cancelNotice(*report)); err != nil {
		logger.Warn("battencli: could not post the cancel notice", "slug", report.Slug, "cause", err)
		return
	}
	report.NoticePosted = true
}

// cancelNotice renders the one line the orch is told: the run, where it stood, and the pair's or the cancel's next step.
func cancelNotice(report cancelReport) string {
	stood := "state no longer readable"
	if report.State != "" {
		stood = fmt.Sprintf("state %s at %s", report.State, report.Row)
	}
	switch {
	case report.EndOnly:
		return fmt.Sprintf("batten cancel of %s ended its processes only (%s); origin was unreachable, so %s", report.Slug, stood, report.WayForward)
	case report.PairRemoved:
		return fmt.Sprintf("batten cancel of %s ended the run (%s); its pair was removed", report.Slug, stood)
	case report.WayForward != "":
		return fmt.Sprintf("batten cancel of %s ended the run (%s); its pair was kept, so %s", report.Slug, stood, report.WayForward)
	}
	return fmt.Sprintf("batten cancel of %s ended the run (%s); no pair was left", report.Slug, stood)
}

// childReachedParent reports whether the child's work reached its parent, which only a run to its landing finishes.
// That is a child Finalize past its parent-side merge, a Finalize done in the child's history, or batten past Run-Shed with the child done.
// Batten at Worktree-Teardown, which runs only after Land, reads reached whatever the child reads, since only the teardown is left.
func childReachedParent(batten shedengine.Status, battenFound bool, child shedengine.Status, childFound bool, pastMerge bool) bool {
	if pastMerge {
		return true
	}
	if battenFound && batten.CurrentProducer == battenrecipe.NameWorktreeTeardown {
		return true
	}
	if !childFound {
		return false
	}
	for _, entry := range child.History {
		if entry.Producer == loomshed.NameFinalize && entry.Outcome == shedengine.Done {
			return true
		}
	}
	pastRunShed := battenFound && (batten.CurrentProducer == battenrecipe.NameLand || batten.CurrentProducer == battenrecipe.NameWorktreeTeardown)
	return pastRunShed && child.State == shedengine.StateDone
}

// windowsLive lists the registered processes that are alive, on Windows only; every other platform lists none, since the owner-record end is trusted there.
func windowsLive(deps cancelDeps) ([]runproc.Record, error) {
	if deps.GOOS != windowsGOOS {
		return nil, nil
	}
	records, err := deps.LiveRecords()
	if err != nil {
		return nil, err
	}
	return liveOnWindows(records, deps.CreatedAt), nil
}

// liveOnWindows drops every record whose pid's process was created after the record was registered, which is a recycled pid.
// A record whose creation time cannot be read is kept.
func liveOnWindows(records []runproc.Record, createdAt func(pid int) (time.Time, bool)) []runproc.Record {
	var live []runproc.Record
	for _, record := range records {
		if created, ok := createdAt(record.PID); ok && created.After(record.RegisteredAt) {
			continue
		}
		live = append(live, record)
	}
	return live
}

// windowsLiveRefusal words the refusal for registered processes that a Windows cancel cannot end, naming each with the command that ends it and the quoted rerun command to run after.
func windowsLiveRefusal(slug, rerun string, records []runproc.Record) error {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("pid %d (%s): taskkill /PID %d /T /F", record.PID, record.Role, record.PID))
	}
	return fmt.Errorf("processes of %s are still alive: %s; way forward: end each with its taskkill line, then run %s again", slug, strings.Join(lines, "; "), rerun)
}
