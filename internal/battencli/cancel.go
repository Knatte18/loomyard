// cancel.go holds the decisions and order of "lym batten cancel".
// cancelRun judges a run against origin and the local records, ends it, and removes what the verb's flags name.
//
// The verb and its production seams live beside the CLI.
// This file reaches no substrate itself, so one untagged table test drives every branch with fakes.

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

// cancelDeps carries every substrate call a cancel makes, as function values.
// Production fills each one over the hub, and a test replaces each one with a fake.
// None is nil except BeamProject and LandingHeld.
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
	// It also returns the subject that `lym adopt` takes to move the owner.
	Owner func() (machine, owner, subject string, err error)
	// ReadStatus reads batten's status file for the slug.
	ReadStatus func() (status shedengine.Status, found bool, err error)
	// OwnerHeld reports whether an owner pause covers the slug.
	// A refusal that sends the operator to remove the pair reads it to word the step that settles the run's record.
	OwnerHeld func() (bool, error)
	// BoardStatus reads the status of the slug's entry on the local board.
	BoardStatus func() (status *string, found bool, err error)
	// RecordsPresent reports whether the slug's records run directory exists.
	// A directory deleted on disk still counts as present while the records HEAD tracks it.
	RecordsPresent func() (bool, error)
	// ChildReachedParent reports whether the child's work reached its parent, given batten's status.
	ChildReachedParent func(batten shedengine.Status, battenFound bool) (bool, error)
	// ReadMarker reads the slug's cancel marker and the liveness of its writer.
	ReadMarker func() (cancelMarkerReading, error)
	// TakeMarker takes the cancel marker's lock and writes the marker.
	// release drops the lock and removes the marker when asked.
	// fresh is false for a marker a dead writer left.
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
	// TakeBattenLocks takes batten's prime-side driver lock and run lock.
	// Each wait lasts at most pairteardown.CancelLockWait.
	// A non-empty busy names the lock still held; nothing is held in that case.
	TakeBattenLocks func() (release func(), busy string, err error)
	// StopTackler stops the strand the slug's tackler marker names and returns its guid, empty when none was named.
	StopTackler func() (guid string, err error)
	// RemovePair removes the pair through a teardown request and returns the removal's result.
	RemovePair func(req pairteardown.Request) (fabricengine.RemoveResult, error)
	// TaskBranchOnOrigin reports whether the slug's task branch is on origin.
	TaskBranchOnOrigin func() (bool, error)
	// DiscardTaskBranch archives and deletes the task branch, locally and on origin.
	DiscardTaskBranch func() (fabricengine.DiscardResult, error)
	// RemoveRecords removes the slug's records and scratch under batten's status lock, held only for the call.
	// committed reports whether a records commit was made.
	RemoveRecords func() (committed bool, err error)
	// ClearClaim clears a run-form status of the slug's board entry.
	// It reports whether it cleared one, and whether an entry was found.
	ClearClaim func() (cleared, found bool, err error)
	// LiftPauses lifts the slug's own live slug-scope pause entries and returns their scopes.
	LiftPauses func() ([]string, error)
	// SpawnPush starts the detached push of the records side.
	SpawnPush func() error
	// Notify posts line to the orch.
	Notify func(line string) error
	// BeamProject reports whether the slug's board entry is a beam project.
	// A nil BeamProject reads as no beam; on a beam, the cancel judges the beam seams that follow.
	BeamProject func() (bool, error)
	// ReconcileHub runs the hub-scope origin reconcile from the prime, which brings the Bolt level with origin.
	ReconcileHub func() (fabricengine.OriginReport, error)
	// LandRefusal refuses while work remains under the beam, read over the Bolt the reconcile left.
	LandRefusal func() error
	// LandingHeld reports whether the beam's pawl run holds its entry, read over the Bolt the reconcile left.
	// A nil LandingHeld reads as no hold.
	LandingHeld func() (bool, error)
	// LandBlockers reads what still works under the beam, over the Bolt the reconcile left.
	LandBlockers func() (BeamLandBlockers, error)
	// StopBeam stops the beam's orch and watcher when its pair is present locally.
	StopBeam func() error
}

// cancelReport records what one cancel did; with EndOnly set, it records what the cancel ended before stopping.
type cancelReport struct {
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
	// StrayDirectoryDeleted is true when the cancel deleted a directory that is no worktree at the pair's task worktree path.
	StrayDirectoryDeleted bool
	// SnapshotTag names the snapshot tag that holds the pair's uncommitted paths, empty when none was taken.
	SnapshotTag string
	// ArchiveTags lists the archive tags that hold the deleted task branch tips.
	ArchiveTags []string
	// RecordsRemoved is true when the run's records and scratch were removed.
	// RecordsCommitted is true when the removal made a records commit.
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
	// Warnings lists what failed after the run was ended and so did not stop the cancel.
	Warnings []string

	// hold is what the cancel keeps held after its run phase succeeds; nil when that phase refused.
	hold *cancelHold
	// pairKept is true when the pair was present and the cancel keeps it.
	pairKept bool
}

// cancelHold keeps the pause-gate exemption, batten's locks and the cancel marker held.
// They stay held for the rest of the cancel once the run phase succeeds.
// The marker lock is dropped at most once, and the marker file is removed or kept along with it.
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

// finishMarker drops the marker lock once, removing the marker file when remove is set.
// It does nothing when no marker was taken or the lock is already dropped.
func (h *cancelHold) finishMarker(remove bool) error {
	if h.releaseMarker == nil || h.markerDone {
		return nil
	}
	h.markerDone = true
	return h.releaseMarker(remove)
}

// end drops whatever is still held and restores the pause gate last.
// The marker file is kept unless an earlier release already removed it.
func (h *cancelHold) end(slug string) {
	h.releaseBattenLocks()
	if err := h.finishMarker(false); err != nil {
		logger.Warn("battencli: could not release the cancel marker", "slug", slug, "cause", err)
	}
	if h.restoreGate != nil {
		h.restoreGate()
	}
}

// cancelCommand returns the `lym batten cancel` command for slug, with --remove when remove is set.
func cancelCommand(slug string, remove bool) string {
	if remove {
		return fmt.Sprintf("lym batten cancel %s --remove", slug)
	}
	return fmt.Sprintf("lym batten cancel %s", slug)
}

// ErrNothingLeft is the sentinel that a cancel refusal matches through errors.Is.
// The refusal applies when no record, claim, pair part, session, process or marker of the slug is left.
var ErrNothingLeft = errors.New("nothing of the slug is left to cancel")

// nothingLeftError is the refusal that nothing of a slug is left to cancel, carrying the message to show.
type nothingLeftError struct {
	text string
}

func (e *nothingLeftError) Error() string { return e.text }

// Is reports whether target is the nothing-left sentinel, which errors.Is uses to match this refusal.
func (e *nothingLeftError) Is(target error) bool { return target == ErrNothingLeft }

// cancelJudgingEndOnly runs the cancel and reads an end-only cancel as an error.
// An end-only cancel leaves the entry's run status and the run record in place.
// A caller that counted it done would go on to a removal the run lock refuses.
// The error names the cancel to run again once origin answers.
// It is a plain failure, not a nothing-left refusal.
func cancelJudgingEndOnly(ctx context.Context, slug string, remove bool, deps cancelDeps) error {
	report, err := cancelRun(ctx, slug, remove, deps)
	if err != nil {
		return err
	}
	if report.EndOnly {
		return fmt.Errorf("batten cancel of %s: origin was unreachable, so the cancel ended the run's processes and stopped; way forward: run %s again once origin answers", slug, cancelCommand(slug, remove))
	}
	return nil
}

// cancelRun ends the batten run of slug for good.
// It refuses before it ends or removes anything, unless a step's error says otherwise.
// Every step is a no-op once done, so running the cancel again finishes an interrupted one.
// An unreachable origin makes the cancel end-only: the run's processes and strands are ended.
// The rest of the cancel waits for it to run again once origin answers.
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

// endRunPhase judges the batten run of slug, takes the cancel marker, and ends the run's processes and strands.
// It touches neither the pair, the records nor the claim.
// rerun is the quoted command every refusal names as the step to run again.
// On success it leaves the pause-gate exemption, batten's locks and the cancel marker held in the report.
// The caller finishes or drops them.
// A refusal drops them itself.
// An unreachable origin makes the phase end-only.
// The report says so, holds no batten lock, and names a re-run once origin answers.
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
		return report, fmt.Errorf("a hub pause covers the prime; way forward: run \"lym tender resume\", then %s again", rerun)
	}
	hold.restoreGate, err = deps.ExemptGate()
	if err != nil {
		return report, probeRefusal("the pause gate", slug, rerun, err)
	}
	if err := refuseBeamCancel(slug, rerun, deps); err != nil {
		return report, err
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
		return report, fmt.Errorf("the work of %s reached its parent; way forward: run \"lym batten run %s\" to finish the landing, then \"lym batten cleanup --apply\"", slug, slug)
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
	stopBeamOrch(&report, deps)
	report.pairKept = parts.Any() && !remove
	return report, nil
}

// stopBeamOrch stops the orch and watcher of a beam project once its run is ended.
// A task has neither.
// The run is already ended, so a failure is only a warning: it is logged and added to the report's warnings.
func stopBeamOrch(report *cancelReport, deps cancelDeps) {
	beam, err := isBeamProject(deps)
	if err == nil && beam {
		err = deps.StopBeam()
	}
	if err != nil {
		logger.Warn("battencli: could not stop the beam's orch after the cancel ended its run", "slug", report.Slug, "cause", err)
		report.Warnings = append(report.Warnings, fmt.Sprintf("the beam's orch was not stopped: %v", err))
	}
}

// isBeamProject reports whether the slug's board entry is a beam project.
// Without a BeamProject seam in deps, the entry reads as not a beam.
func isBeamProject(deps cancelDeps) (bool, error) {
	if deps.BeamProject == nil {
		return false, nil
	}
	return deps.BeamProject()
}

// refuseBeamCancel refuses the cancel of a beam project while work remains under it.
// It refuses before the cancel marker is taken or anything is ended.
// It first reconciles the hub against origin, so the members are read over the Bolt origin holds.
// An error or an unreachable origin refuses with a way forward.
// While the beam's pawl run does not hold its entry, the refusal is the beam's landing refusal.
// It names each member to land or cancel.
// Once the run holds its entry, a member's landing is refused,
// so each blocker names the cancel in the beam's parent worktree and the move off the beam.
// The blockers never name `lym batten run`.
// A task returns before any of this.
func refuseBeamCancel(slug, rerun string, deps cancelDeps) error {
	beam, err := isBeamProject(deps)
	if err != nil {
		return probeRefusal("the board entry", slug, rerun, err)
	}
	if !beam {
		return nil
	}
	report, err := deps.ReconcileHub()
	if err != nil {
		return fmt.Errorf("battencli: the origin reconcile of the hub failed: %w; way forward: fix the cause it names, then run %s again", err, rerun)
	}
	if report.Unreachable {
		return fmt.Errorf("origin was unreachable, so the members of the beam %s could not be read against it; way forward: run %s once origin answers", slug, rerun)
	}

	refusal := deps.LandRefusal()
	if refusal == nil {
		return nil
	}
	held := false
	if deps.LandingHeld != nil {
		if held, err = deps.LandingHeld(); err != nil {
			return probeRefusal("the beam's landing hold", slug, rerun, err)
		}
	}
	if !held {
		return fmt.Errorf("%w; then run %s again", refusal, rerun)
	}
	blockers, err := deps.LandBlockers()
	if err != nil {
		return probeRefusal("the members of the beam", slug, rerun, err)
	}
	if blockers.Empty() {
		return nil
	}
	return heldBeamCancelRefusal(slug, rerun, blockers)
}

// heldBeamCancelRefusal words the refusal of a beam's cancel while its pawl run holds its entry.
// It names every blocker with the step that ends it, then the cancel's re-run.
// A member with a pair or a run in flight is cancelled in the beam's parent worktree, then moved off the beam.
// The cancel leaves that member a `draft`, which still blocks the landing, so the move is needed.
// A member with no run status is moved alone.
func heldBeamCancelRefusal(slug, rerun string, blockers BeamLandBlockers) error {
	moveOff := "move its `project` off the beam with `lym board upsert`"
	var ways []string
	for _, member := range blockers.Members {
		if member.HasPair || member.Running {
			ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam: end it with `lym batten cancel %s --remove`, run in the beam's parent worktree, then %s, since the cancel leaves a `draft` member, which still blocks the landing", member.Slug, member.Slug, moveOff))
			continue
		}
		ways = append(ways, fmt.Sprintf("the open entry %s is a member of the beam with no run status: %s", member.Slug, moveOff))
	}
	for _, pair := range blockers.ChildPairs {
		ways = append(ways, fmt.Sprintf("the pair %s was forked from the beam: end it with `lym batten cancel %s --remove`, run in the beam's parent worktree", pair, pair))
	}
	return fmt.Errorf("battencli: the beam %s cannot be cancelled while work remains under it; way forward: %s; then run %s again", slug, strings.Join(ways, "; "), rerun)
}

// clearRunPhase removes what is left of the run after it is ended.
// It removes the pair and its task branch when remove is set.
// It removes the run's records and clears the board claim.
// It lifts the slug's own pauses and, if the records removal committed, spawns the records push.
// It releases the batten locks the report holds once the claim is cleared.
// The cancel marker and the pause gate stay with the caller.
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
		machine, owner, subject, err := deps.Owner()
		if err != nil {
			return false, probeRefusal("the slug's owner", slug, rerun, err)
		}
		if owner != "" && owner != machine {
			return false, cancelOwnedElsewhere(slug, owner, subject)
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
			return false, cancelOwnedElsewhere(slug, entry.Owner, entry.AdoptSubject)
		case fabricengine.EntryStopped:
			if stoppedOnlyByAnchor(slug, report.Branches) {
				break
			}
			return false, cancelStoppedRefusal(slug, fmt.Sprintf("then run %s again", rerun), entry, report.Branches, deps)
		case fabricengine.EntryEnded:
			if entry.How != "done" {
				break
			}
			recordsHere, err := deps.RecordsPresent()
			if err != nil {
				return false, probeRefusal("the run's records", slug, rerun, err)
			}
			if recordsHere {
				return false, fmt.Errorf("%s was landed by another machine; way forward: run \"lym cleanup --apply\" from the prime, whose reconcile tears it down", slug)
			}
		}
	}
	return false, nil
}

// cancelStoppedRefusal words the refusal of a cancel whose run origin's judgment stopped, ending in reentry.
// A refusal that names the pair's removal ends instead with the step that settles the run's batten record.
// That step is read from batten's status and the owner pause.
// The removal leaves that record standing, and a cancel at Land would release a landed entry.
// A read that fails is logged and keeps reentry, and so does an empty follow-up.
func cancelStoppedRefusal(slug, reentry string, entry fabricengine.EntryOutcome, branches []fabricengine.BranchOutcome, deps cancelDeps) error {
	refusal := stoppedRefusal(slug, "cancel", reentry, entry, branches)
	if !strings.Contains(refusal.Error(), fabricRemoveCommand) {
		return refusal
	}
	status, found, err := deps.ReadStatus()
	if err != nil {
		logger.Warn("battencli: batten's status could not be read, so the refusal names no follow-up of the removal", "slug", slug, "cause", err)
		return refusal
	}
	held, err := deps.OwnerHeld()
	if err != nil {
		logger.Warn("battencli: the owner pause could not be read, so the refusal names no follow-up of the removal", "slug", slug, "cause", err)
		return refusal
	}
	followUp := pairteardown.BattenFollowUp(slug, status, found, held)
	if followUp == "" {
		return refusal
	}
	return withFollowUp(refusal, reentry, followUp)
}

// stoppedOnlyByAnchor reports whether every branch stopping the entry of slug is anchor_stopped, and at least one is.
// The `--set` an anchor stop names refuses a worktree with an unfinished run,
// so the cancel that clears the run is the exit to take first.
// The branches counted are the entry's own, bar its run's busy records branch.
// Branches every pair shares count too, bar a busy one.
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
// The way forward names the adopt of subject, the beam or `root` that takes the slug over.
// It names the slug instead when subject is empty.
func cancelOwnedElsewhere(slug, owner, subject string) error {
	return fmt.Errorf("%s is owned by machine %s; way forward: run \"lym adopt %s\" on this machine first, or \"lym batten cancel %s\" on %s", slug, owner, subjectOrSlug(subject, slug), slug, owner)
}

// refuseNothingLeft refuses a cancel when no record, claim, pair part, session, process or marker of the slug is left.
// Without remove, it also refuses when only a task branch or a stray directory is left.
// A stray directory is one that is no worktree at the task worktree's path.
// With remove, neither a local task branch nor a stray directory refuses.
// A task branch on origin counts as left, and it is read only when nothing else is left and origin was reached.
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
		if parts.StrayPath {
			return fmt.Errorf("a directory that is no worktree is all that is left of %s, at its task worktree path; way forward: run %q, which deletes it", slug, cancelCommand(slug, true))
		}
		return nothingLeft(slug)
	}
	if parts.TaskBranch || parts.StrayPath {
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
	return nothingLeft(slug)
}

// nothingLeft is the refusal that nothing of slug is left to cancel; errors.Is matches it as such.
func nothingLeft(slug string) error {
	return &nothingLeftError{text: fmt.Sprintf("nothing of %s is left to cancel; way forward: check it with \"lym board list\"", slug)}
}

// removePairAndBranch removes the pair, snapshotting its uncommitted paths first.
// It then archives and deletes the task branch while one is left.
// A pair with nothing left reads as removed.
// A directory that is no worktree at the pair's task worktree path is deleted with the pair.
// Left behind, it would block the slug's next run.
// A failed delete refuses and names the cancel to run again.
func removePairAndBranch(slug, rerun string, report *cancelReport, deps cancelDeps) error {
	result, err := deps.RemovePair(pairteardown.Request{Slug: slug, Force: true, Snapshot: true, Remote: true, LeaveClaim: true, ClearStray: true})
	report.SnapshotTag = result.SnapshotTag
	report.StrayDirectoryDeleted = result.StrayPathDeleted
	if err != nil && !errors.Is(err, fabricengine.ErrPairNotFound) {
		return fmt.Errorf("battencli: removing the pair of %s refused: %w; way forward: fix the cause it names, then run \"lym batten cancel %s --remove\" again", slug, err, slug)
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
		return fmt.Errorf("battencli: discarding the task branch of %s failed: %w; way forward: fix the cause it names, then run \"lym batten cancel %s --remove\" again", slug, err, slug)
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
// That holds when the child's Finalize is past its parent-side merge.
// It also holds when the child's history has a Finalize that is done.
// Batten past Run-Shed also counts when the child is done.
// Batten at Worktree-Teardown reports reached whatever the child's state, since only the teardown is left after Land.
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

// windowsLive lists the registered processes that are alive, on Windows only;
// every other platform lists none, since the owner-record end is trusted there.
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

// liveOnWindows drops every record whose pid was recycled: its process was created after the record was registered.
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

// windowsLiveRefusal words the refusal for registered processes that a Windows cancel cannot end.
// It names each process with the command that ends it, and the quoted rerun command to run after ending them.
func windowsLiveRefusal(slug, rerun string, records []runproc.Record) error {
	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("pid %d (%s): taskkill /PID %d /T /F", record.PID, record.Role, record.PID))
	}
	return fmt.Errorf("processes of %s are still alive: %s; way forward: end each with its taskkill line, then run %s again", slug, strings.Join(lines, "; "), rerun)
}
