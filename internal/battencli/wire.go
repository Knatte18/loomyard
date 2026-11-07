// wire.go builds the assembly seam the battencli run and status verbs need: the shedrecipe.Env and
// shedbuild.ShedPaths values, and the receiver field an abandoned session is recorded into.
//
// Every seam that touches the managed task worktree resolves inside its closure body on Call, never at wiring time.
// The task worktree does not exist until WorktreeCreate has run,
// so resolving its paths earlier would name a path that is not there yet.
// It holds for the status-path pair, the spawn directory and both teardown halves, not only for ResolveStatus.
// ResolveStatus's own signature makes that laziness visible.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// spawnPairPush launches the detached push of a task pair.
// Tests replace it, since a detached push forks the running binary.
var spawnPairPush = fabricengine.SpawnPairPush

// driverAliveFrom reports whether the child's driver strand is live.
// It takes the status through an injected reader, so its answers are testable without tmux.
// An absent task worktree reports false without reading status.
// An absent reed session reports false too, since no session means no driver.
// Every other status error is returned unchanged.
func driverAliveFrom(present bool, status func() (reedengine.StatusResult, error)) (bool, error) {
	if !present {
		return false, nil
	}
	res, err := status()
	if errors.Is(err, reedengine.ErrNoSession) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, s := range res.Strands {
		if loomengine.IsDriverStrand(s.Name) && s.Live {
			return true, nil
		}
	}
	return false, nil
}

// childDriverAlive reports whether the driver strand of slug's child is live.
// It reads the task worktree's reed status when the worktree is present.
func childDriverAlive(prime *lymcwd.Location, slug string, locateTask func() (*lymcwd.Location, error)) (bool, error) {
	present, err := taskWorktreePresent(prime, slug)
	if err != nil {
		return false, err
	}
	return driverAliveFrom(present, func() (reedengine.StatusResult, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return reedengine.StatusResult{}, err
		}
		reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "reed")
		if err != nil {
			return reedengine.StatusResult{}, err
		}
		reedGeom, err := hubgeom.ReedGeometry(taskLocation)
		if err != nil {
			return reedengine.StatusResult{}, err
		}
		return reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub()).Status()
	})
}

// driverStrandFrom reports the child's driver strand from reed's sessionless directory.
// It takes the directory through an injected reader, so its answers are testable without tmux.
// An absent task worktree and a directory with no driver row are none;
// a live row is retiring when its Retiring is set and live otherwise;
// a row that is not live is dead.
// A directory read error is returned unchanged.
func driverStrandFrom(present bool, directory func() ([]reedengine.DirectoryRow, error)) (battenshed.ChildDriverStrand, error) {
	if !present {
		return battenshed.ChildDriverNone, nil
	}
	rows, err := directory()
	if err != nil {
		return battenshed.ChildDriverNone, err
	}
	for _, row := range rows {
		if !loomengine.IsDriverStrand(row.Name) {
			continue
		}
		switch {
		case !row.Live:
			return battenshed.ChildDriverDead, nil
		case row.Retiring:
			return battenshed.ChildDriverRetiring, nil
		default:
			return battenshed.ChildDriverLive, nil
		}
	}
	return battenshed.ChildDriverNone, nil
}

// arbiterInFlightFrom reads the child's in-flight arbiter marker through readMarker and reports its deadline.
// Alive is true only when a live row of the pair's reed directory carries the marker's strand guid.
// An absent task worktree and an absent marker are not found;
// a marker read error and a directory read error are returned unchanged.
// A directory with no live row, as with no reed session, reads as not alive.
func arbiterInFlightFrom(present bool, readMarker func() (loomengine.ArbiterMarker, bool, error), directory func() ([]reedengine.DirectoryRow, error)) (battenshed.ChildArbiter, bool, error) {
	if !present {
		return battenshed.ChildArbiter{}, false, nil
	}
	marker, found, err := readMarker()
	if err != nil || !found {
		return battenshed.ChildArbiter{}, false, err
	}
	rows, err := directory()
	if err != nil {
		return battenshed.ChildArbiter{}, false, err
	}
	for _, row := range rows {
		if row.Live && row.GUID == marker.StrandGUID {
			return battenshed.ChildArbiter{Deadline: marker.Deadline, Alive: true}, true, nil
		}
	}
	return battenshed.ChildArbiter{Deadline: marker.Deadline}, true, nil
}

// stopDeadArbiterStrands calls stop with the guid of every arbiter row that is not live, and returns the first error.
// A live arbiter row is left alone: it keeps working, and its verdict waits for the revived driver.
func stopDeadArbiterStrands(rows []reedengine.DirectoryRow, stop func(guid string) error) error {
	for _, row := range rows {
		if !loomengine.IsArbiterStrand(row.Name) || row.Live {
			continue
		}
		if err := stop(row.GUID); err != nil {
			return err
		}
	}
	return nil
}

// runLockHeld reports whether the run lock at lockPath is held, probing it without keeping it.
// The lock's directory is created first, since a child that never started has none.
func runLockHeld(lockPath string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return false, err
	}
	probe, free, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, err
	}
	if free {
		_ = probe.Release()
	}
	return !free, nil
}

// taskWorktreeLocation returns the *lymcwd.Location of the managed task worktree for slug, resolved from prime.
// It joins the worktree root with fabricengine.WorktreePath.
// It resolves that root with lymcwd.ResolveWorktree, which applies no cwd gate:
// the caller holds a worktree root, not an acting cwd, so the gate would fire spuriously.
//
// An absent pair, or a directory that does not resolve as a worktree, is refused by name.
// Batten's status is durable,
// so a run resumed on another machine can reach every row past Worktree-Create without a local pair.
// The refusal names "lym batten run" as the resume.
// The resume's pre-run re-adopts a live pair and otherwise stops and names the restore of each branch.
// It names no fabric command as a substitute.
// "lym fabric checkout" switches the caller's own worktree.
// Run from prime, that switch would mutate prime itself rather than restore anything.
// It does not create the worktree.
func taskWorktreeLocation(prime *lymcwd.Location, slug string) (*lymcwd.Location, error) {
	worktreePath, err := presentTaskWorktreePath(prime, slug)
	if err != nil {
		return nil, err
	}
	return lymcwd.ResolveWorktree(worktreePath)
}

// presentTaskWorktreePath returns the root of the managed task worktree for slug and refuses an absent one by name.
// The refusal also applies to a worktree that disappears mid-run.
// It applies as well to the cancel and goto verbs, which resolve the task worktree without a resume.
func presentTaskWorktreePath(prime *lymcwd.Location, slug string) (string, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	present, err := taskWorktreePresent(prime, slug)
	if err != nil {
		return "", err
	}
	if !present {
		return "", absentTaskWorktreeRefusal(slug, worktreePath)
	}
	return worktreePath, nil
}

// absentTaskWorktreeRefusal is the by-name refusal of an absent task worktree for slug, whose root would be worktreePath.
func absentTaskWorktreeRefusal(slug, worktreePath string) error {
	return fmt.Errorf(
		"battencli: the task worktree for %q is not present at %s; this run's durable status says it was already created, so it is either on another machine or was removed by hand; way forward: run \"lym batten run %s\" from the prime, whose resume re-adopts the pair when its branches are still live and otherwise stops naming the restore of each branch, and then repeating the refused command",
		slug, worktreePath, slug,
	)
}

// taskWorktreeLocator returns a function that resolves the location of the task worktree for slug.
// The function runs on every poll of a wait, so it spawns git only until the first success.
// That success is kept, since a present pair's location does not change.
// Later calls stat the pair instead, with no git spawn.
// They return the refusal for an absent worktree when it is gone.
func taskWorktreeLocator(prime *lymcwd.Location, slug string) func() (*lymcwd.Location, error) {
	var resolved *lymcwd.Location
	return func() (*lymcwd.Location, error) {
		if resolved == nil {
			taskLocation, err := taskWorktreeLocation(prime, slug)
			if err != nil {
				return nil, err
			}
			resolved = taskLocation
			return resolved, nil
		}
		worktreePath := fabricengine.WorktreePath(prime, slug)
		if _, err := os.Stat(worktreePath); err != nil {
			if os.IsNotExist(err) {
				return nil, absentTaskWorktreeRefusal(slug, worktreePath)
			}
			return nil, err
		}
		return resolved, nil
	}
}

// taskWorktreePresent reports whether the task worktree for slug stands under prime's hub and resolves as a worktree.
// The verbs that gate on or resolve the task worktree read this answer, the re-adoption's gone check included.
// A path that stands but does not resolve as a worktree reads as absent, as a missing path does.
// That is the case for a directory a removal left holding ignored build output.
// Those verbs meet such a path as a gone pair instead of failing on it,
// and the add that creates the worktree refuses the stray path by name.
// A stat error other than not-exist is returned.
func taskWorktreePresent(prime *lymcwd.Location, slug string) (bool, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := lymcwd.ResolveWorktree(worktreePath); err != nil {
		return false, nil
	}
	return true, nil
}

// taskWorktreeComplete reports whether the task worktree for slug is fully created, not just present.
// A bare directory check cannot tell a complete pair from one an interrupted create left half-built.
// Creation is a multi-step transaction.
// It rolls back only when it observes an error, never when the process running it is killed,
// so a check that the worktree resolves is satisfied by the transaction's first step alone.
// The window also covers a create that returned before its transition was persisted.
// A row re-entered after its worktree was created must report success, not ask fabric to create the same worktree twice.
//
// present is false only when the worktree is absent or does not resolve as one, and the create still runs then.
// incompleteReason is set only when the worktree exists but is not a complete pair.
// Its text is worded for this package's refusal.
func taskWorktreeComplete(prime *lymcwd.Location, slug string) (present, complete bool, incompleteReason string, err error) {
	present, err = taskWorktreePresent(prime, slug)
	if err != nil || !present {
		return present, false, "", err
	}
	taskLocation, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return present, false, "", err
	}
	complete, _, err = fabricengine.PairComplete(taskLocation)
	if err != nil {
		return present, false, "", err
	}
	if !complete {
		return present, false, "its pair is not fully created -- most likely a create interrupted partway through, since the process that ran it was killed rather than erroring, so its own rollback never ran", nil
	}
	return present, true, "", nil
}

// incompletePairRemedy returns the manual cleanup for a task pair that is not fully created.
// Running "lym fabric remove --force" from prime removes whatever part of the pair was created, in one command.
// That covers the task worktree, its sibling, their junctions, the launcher entries, and the sibling's branch.
// No plain git commands can do that from here, since the sibling is a worktree of another repository.
// The remove deletes the task branch when its work is already on another ref.
// That is always so for a create interrupted before any work; otherwise the remove keeps the branch.
// So the branch deletion is conditional on the branch surviving the removal:
// an unconditional "git branch -D" fails on a branch the removal already deleted.
// The task branch is named with fabric's branch prefix, since the create refuses a leftover one.
func incompletePairRemedy(slug, branch string) string {
	return fmt.Sprintf("remove it by hand from here (\"lym fabric remove --force %s\"), then, only if its branch %s is still present afterwards, delete that branch too (\"git branch -D %s\")", slug, branch, branch)
}

// reconcileTaskPair reconciles and commits the config of the task pair for slug, whatever the hub's build stamp says.
// The pair forks the prime's records branch.
// An earlier walk may have skipped that branch as mid-merge,
// and another holder's walk may not have committed it yet.
// A repository outside a hub has nothing to reconcile.
func reconcileTaskPair(prime *lymcwd.Location, slug string) error {
	geometry, inHub := hubgeom.ReconcileGeometry(prime)
	if !inHub {
		return nil
	}
	return hubreconcile.Ensure(geometry, hubreconcile.Options{Pair: fabricengine.WorktreePath(prime, slug)})
}

// createRefusal maps the create refusals no tackler can settle to a *battenshed.PreconditionError.
// Every other create error passes through verbatim.
// A tracked-dirty source worktree is settled by committing or reverting its changes.
// A task branch left from an earlier pair of the slug, locally or on origin, is settled by the cancel verb.
// That verb archives the branch and deletes it.
// The refusal fabric words for a caller inside a pair names "lym fabric checkout".
// From the prime, that command would switch the prime's own pair onto the task's branches, so its text is replaced.
// A remote leftover on any other branch, the records side included, passes through.
// A directory at the task worktree's path is reached only when it does not resolve as a worktree.
// Such a directory, for example one a cancel's late writer refilled, is settled by deleting it.
// taskPathStands reports whether that path holds a registered worktree.
// It is asked only for that refusal.
// A registered worktree is settled by removing the pair, not by deleting a plain directory.
func createRefusal(err error, slug, taskBranch string, taskPathStands func() bool) error {
	leftoverWayForward := fmt.Sprintf(`run "lym batten cancel %s --remove" from the prime, which archives and deletes it, then "lym batten run %s"`, slug, slug)

	var sourceDirty *fabricengine.ErrSourceDirty
	if errors.As(err, &sourceDirty) {
		return &battenshed.PreconditionError{
			Unmet:      "the source worktree has uncommitted tracked changes: " + strings.Join(sourceDirty.Paths, ", "),
			WayForward: fmt.Sprintf(`commit or revert them in the source worktree, then run "lym batten run %s" again`, slug),
			Err:        err,
		}
	}
	var branchExists *fabricengine.ErrBranchExists
	if errors.As(err, &branchExists) {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("task branch %s is left from an earlier pair of this slug", branchExists.Branch),
			WayForward: leftoverWayForward,
			Err:        err,
		}
	}
	var remoteLeftover *fabricengine.ErrRemoteLeftover
	if errors.As(err, &remoteLeftover) && remoteLeftover.Branch == taskBranch {
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("task branch %s is left on origin from an earlier pair of this slug", remoteLeftover.Branch),
			WayForward: leftoverWayForward,
			Err:        err,
		}
	}
	var pathTaken *fabricengine.ErrTaskPathTaken
	if errors.As(err, &pathTaken) {
		if taskPathStands() {
			return &battenshed.PreconditionError{
				Unmet:      fmt.Sprintf("the task worktree %s is registered but does not resolve as a worktree", pathTaken.Path),
				WayForward: incompletePairRemedy(slug, taskBranch) + ", then resume this run to create it fresh",
				Err:        err,
			}
		}
		return &battenshed.PreconditionError{
			Unmet:      fmt.Sprintf("the directory %s stands where the task worktree goes and is not a worktree", pathTaken.Path),
			WayForward: fmt.Sprintf(`save anything worth keeping from it, then delete it, and run "lym batten run %s" again`, slug),
			Err:        err,
		}
	}
	return err
}

// finishRemoval turns the outcome of a pair removal into the verdict of the Worktree-Teardown row.
// A pair with nothing of it left is the row's done state.
// shedengine persists a transition only after its producer returns,
// so a process killed right after the removal re-enters the row.
// The removal hands its remote steps to a detached child and returns without their outcome;
// the child mails the orchestrator a failed step with its way forward.
// A failed remote deletion that the removal did report, when the steps ran in process, is returned here,
// so the row halts resumable and the resumed row retries the deletion.
func finishRemoval(res fabricengine.RemoveResult, err error, slug string) error {
	if errors.Is(err, fabricengine.ErrPairNotFound) {
		logger.Info("battencli: teardown found nothing of the pair left, done", "slug", slug)
		return nil
	}
	logger.Info("battencli: teardown worktree", "slug", slug, "mutations", res.Mutated(), "remote_skipped_reason", res.RemoteSkippedReason, "archive_tag", res.ArchiveTag, "archive_skipped_reason", res.ArchiveSkippedReason)
	if err != nil {
		return teardownRefusal(err, slug)
	}
	if res.RemoteBranchError != "" {
		return fmt.Errorf("the pair for %q was removed but its other-side branch's remote copy was not deleted: %s; re-run lym batten run %s to retry the deletion", slug, res.RemoteBranchError, slug)
	}
	return nil
}

// teardownRequest is the composite's request for a batten-driven teardown.
// Remote is true: a batten-driven teardown is the task's own final removal, never a step toward re-adopting the pair.
// Nothing will ever need the pair's other-side branch again,
// and a remote copy left behind makes a later create of this slug refuse its push.
// It never forces.
// It neither waits for a quiet driver nor refuses a busy one.
// The InnerRun done arm already waited its driver_exit_grace_s, which makes both unnecessary.
// Landed is true: the row runs only after a done child,
// so the task's board entry is marked done once the pair is removed.
// ClearStray is true: the run created the pair at its path.
// So a directory a late writer leaves there once the pair is removed is the run's to delete,
// and a resumed row deletes one an earlier attempt kept.
func teardownRequest(slug string) pairteardown.Request {
	return pairteardown.Request{Slug: slug, Force: false, Remote: true, QuietWait: 0, RefuseWhenBusy: false, Landed: true, ClearStray: true}
}

// teardownRefusal rewords the teardown refusal for content in the pair's sibling that the teardown could not commit.
// It names the resume for a failed archive of the run records,
// and it passes every other error through unchanged.
//
// fabric's own refusal for a dirty pair sibling leaves --force as the way out.
// --force would discard the records this task exists to keep.
// The teardown commits the run records itself,
// so this refusal means content outside the record paths in the pair's sibling worktree under the hub.
// The remedy names inspecting it, committing or removing it by hand, and resuming.
// A push of the pair still running past the teardown's wait is refused before any mutation too,
// and the remedy names the resume once the push finishes.
// A failed archive runs before any teardown mutation,
// so the pair is still whole and a plain resume retries it once the failure is fixed.
// That failure is most often an unreachable remote, but not always.
// So the remedy points at the wrapped cause rather than naming one.
// A directory the teardown could not delete at the task worktree's path names the resume once it is deleted.
func teardownRefusal(err error, slug string) error {
	if errors.Is(err, fabricengine.ErrStrayPathKept) {
		return fmt.Errorf("the pair for %q was removed, but a directory stays at its task worktree path: %w; once it is deleted, resume this run with \"lym batten run %s\"", slug, err, slug)
	}
	if errors.Is(err, fabricengine.ErrArchiveFailed) {
		return fmt.Errorf("the pair for %q was left in place because archiving its run records to the remote failed: %w; fix the failure named here (most often an unreachable remote), then resume this run with \"lym batten run %s\"", slug, err, slug)
	}
	if errors.Is(err, fabricengine.ErrPairPushRunning) {
		return fmt.Errorf("the pair for %q was left in place because a push of it is still running; once it finishes, resume this run with \"lym batten run %s\": %w", slug, slug, err)
	}
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		return err
	}
	return siblingDirtyRefusal{
		msg: fmt.Sprintf("the pair for %q was left in place because its sibling worktree under the hub holds content outside the run record paths, which the teardown cannot commit; inspect that sibling worktree, commit or remove that content by hand, then resume this run with \"lym batten run %s\"", slug, slug),
		err: err,
	}
}

// siblingDirtyRefusal carries batten's own remedy text for a dirty pair sibling.
// It unwraps to fabric's refusal without repeating that refusal's text, which names --force.
type siblingDirtyRefusal struct {
	msg string
	err error
}

func (r siblingDirtyRefusal) Error() string { return r.msg }

func (r siblingDirtyRefusal) Unwrap() error { return r.err }

// maxChildOutputInError caps the child output folded into a spawn error.
// It keeps a misbehaving child from pushing an unbounded string into a status file committed onto prime's own pair.
const maxChildOutputInError = 2000

// battenProgram is the program name batten signs its notices to the orch with.
const battenProgram = "BATTEN"

// childSpawnError folds a failed child bootstrap's own output into runErr,
// so the returned error carries the child's diagnosis rather than only its exit status.
// It returns nil for a nil runErr and runErr unchanged when the child said nothing.
// It truncates output past maxChildOutputInError with an explicit marker.
// An output carrying the not-parked refusal (kind shedrun.StartNotParkedKind) also wraps battenshed.ErrChildNotParked.
// An output carrying the busy-pane refusal (kind shedrun.StartDriverBusyKind) also wraps battenshed.ErrChildDriverBusy.
// The caller retries on those two rather than failing.
// An output carrying a held-run-lock refusal (kind shedengine.ShedBusyKind) also wraps battenshed.ErrChildRunBusy.
// The owned-elsewhere refusal (kind shedrun.StartOwnedElsewhereKind) also wraps a *battenshed.ChildOwnedElsewhereError.
// The error names the owner the refusal carries, so the caller pauses rather than failing the spawn.
//
// The cut at maxChildOutputInError lands on a rune boundary only by chance.
// strings.ToValidUTF8 drops the partial rune the cut leaves dangling, so the error text never holds invalid UTF-8.
func childSpawnError(runErr error, childOutput string) error {
	if runErr == nil {
		return nil
	}
	trimmed := strings.TrimSpace(childOutput)
	if trimmed == "" {
		return runErr
	}
	kind := childRefusalKind(trimmed)
	if len(trimmed) > maxChildOutputInError {
		trimmed = strings.ToValidUTF8(trimmed[:maxChildOutputInError], "") + " ... (truncated)"
	}
	switch kind {
	case shedrun.StartNotParkedKind:
		return fmt.Errorf("%w: %w: %s", battenshed.ErrChildNotParked, runErr, trimmed)
	case shedrun.StartDriverBusyKind:
		return fmt.Errorf("%w: %w: %s", battenshed.ErrChildDriverBusy, runErr, trimmed)
	case shedengine.ShedBusyKind:
		return fmt.Errorf("%w: %w: %s", battenshed.ErrChildRunBusy, runErr, trimmed)
	case shedrun.StartOwnedElsewhereKind:
		return fmt.Errorf("%w: %w: %s", &battenshed.ChildOwnedElsewhereError{Owner: childRefusalOwner(childOutput)}, runErr, trimmed)
	}
	return fmt.Errorf("%w: %s", runErr, trimmed)
}

// childRefusalOwner returns the "owner" of the first refusal envelope in a child bootstrap's output that carries one.
// It returns "" when no line does.
func childRefusalOwner(childOutput string) string {
	for line := range strings.Lines(childOutput) {
		var envelope struct {
			OK    bool   `json:"ok"`
			Owner string `json:"owner"`
		}
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if !envelope.OK && envelope.Owner != "" {
			return envelope.Owner
		}
	}
	return ""
}

// ownerVerdictOf maps a run-scope origin report onto the owner verdict for slug.
// An unreachable report is Unreachable,
// and the slug's owned-elsewhere entry is Elsewhere with its owner and adopt subject.
// Any other report, one without the slug included, is owned here.
func ownerVerdictOf(report fabricengine.OriginReport, slug string) battenshed.OwnerVerdict {
	if report.Unreachable {
		return battenshed.OwnerVerdict{Unreachable: true}
	}
	for _, entry := range report.Entries {
		if entry.Slug == slug && entry.State == fabricengine.EntryOwnedElsewhere {
			return battenshed.OwnerVerdict{Owner: entry.Owner, AdoptSubject: entry.AdoptSubject, Elsewhere: true}
		}
	}
	return battenshed.OwnerVerdict{}
}

// childRefusalKind returns the "kind" of the first refusal envelope in a child bootstrap's output that carries one.
// It returns "" when no line does.
// Each output line is tried as an envelope, since the child's stdout and stderr share one buffer.
func childRefusalKind(childOutput string) string {
	for line := range strings.Lines(childOutput) {
		var envelope struct {
			OK   bool   `json:"ok"`
			Kind string `json:"kind"`
		}
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if !envelope.OK && envelope.Kind != "" {
			return envelope.Kind
		}
	}
	return ""
}

// boardRecipe returns the recipe that Seed-Child seeds for task.
// A project entry gets pawl.
// Any other entry gets its own recipe field, or board's default_recipe when that field is empty.
func boardRecipe(board *boardengine.Board, task boardengine.Task) string {
	if task.Kind == boardengine.KindProject {
		return shedrun.RecipePawl
	}
	return board.RecipeOf(task)
}

// childRecipeSupported reports whether a task worktree's bootstrap verb can run recipe as the child's own run.
// The admitted recipes are the ones `lym loom start` serves.
func childRecipeSupported(recipe string) bool {
	return recipe == shedrun.RecipeTapestry || recipe == shedrun.RecipeMotif || recipe == shedrun.RecipeDarn || recipe == shedrun.RecipePawl || recipe == shedrun.RecipeSampler || recipe == shedrun.RecipeRaddle
}

// unsupportedChildRecipeRefusal returns the refusal for a recipe that a task worktree's bootstrap verb cannot run.
// It wraps battenshed.ErrUnsupportedChildRecipe and names the admitted recipes.
func unsupportedChildRecipeRefusal(recipe string) error {
	return fmt.Errorf("%w: only %q, %q, %q, %q, %q and %q have a bootstrap verb Run-Shed can start in the task worktree, so a Board task's type must be %q, %q, %q, %q, %q or empty, and a project entry seeds %q; got %q", battenshed.ErrUnsupportedChildRecipe, shedrun.RecipeTapestry, shedrun.RecipeMotif, shedrun.RecipeDarn, shedrun.RecipePawl, shedrun.RecipeSampler, shedrun.RecipeRaddle, shedrun.RecipeTapestry, shedrun.RecipeMotif, shedrun.RecipeDarn, shedrun.RecipeSampler, shedrun.RecipeRaddle, shedrun.RecipePawl, recipe)
}

// childSeedParams returns the params of a child's seed.
// For a recipe that childRecipeSupported accepts, it carries params.parent, taken from the pair's recorded origin;
// for a beam that is the beam's parent branch.
// An absent or empty recorded parent yields no param,
// so the bootstrap's own "pass --parent once" refusal is the one an operator sees.
//
// shedrun.WriteSeed is idempotent only against a seed agreeing on recipe, driver and params,
// so a child seeded without a param its own bootstrap writes makes that bootstrap refuse.
func childSeedParams(recipe string, childLocation *lymcwd.Location) (map[string]string, error) {
	if !childRecipeSupported(recipe) {
		return nil, nil
	}
	origin, found, err := fabricengine.ReadOrigin(childLocation)
	if err != nil {
		return nil, err
	}
	if !found || origin.ParentBranch == "" {
		return nil, nil
	}
	return map[string]string{"parent": origin.ParentBranch}, nil
}

// childSeedFor builds the seed written into the task worktree from the given recipe, driver and params.
// It records no parent: the pair's parent is resolved from fabric's origin record when the seed is used.
func childSeedFor(recipe, driver string, params map[string]string) shedrun.Seed {
	return shedrun.Seed{Recipe: recipe, Driver: driver, Params: params}
}

// noticesReachDriversParent reports whether batten's notices reach the task worktree driver's parent.
// It is true only when that destination records an orch strand.
// The task worktree's driver messages the nearest live orch from the pair's recorded parent.
// Batten's notices take the walk from the same pair, so the two destinations agree whenever the pair is present;
// a driver and batten under a live beam orch both reach it.
func noticesReachDriversParent(prime, taskLocation *lymcwd.Location) (bool, error) {
	return runParentRecorded(prime, taskLocation.WorktreeName)
}

// battenHistoryLen returns the length of batten's own status history for slug, 0 when the status file is absent.
func battenHistoryLen(location *lymcwd.Location, slug string) (int, error) {
	status, found, err := state.ReadJSON[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
	if err != nil || !found {
		return 0, err
	}
	return len(status.History), nil
}

// rowNotices returns the notice seams of the rows after Run-Shed: notices go to the nearest live orch above the run,
// and an episode is told apart by batten's own history length.
func rowNotices(location *lymcwd.Location, slug string) battenshed.RowNoticeDeps {
	return battenshed.RowNoticeDeps{
		Notify: func(ctx context.Context, line string) (bool, error) {
			return notifyRunParent(location, slug, line)
		},
		OrchStrandRecorded: func() (bool, error) { return runParentRecorded(location, slug) },
		BattenHistoryLen:   func() (int, error) { return battenHistoryLen(location, slug) },
	}
}

// readStopReport reads the driver park marker of the task worktree's run.
// The marker's content is the path of the stop report the driver parked on,
// and its modification time is the time the marker was written.
// An absent marker reports found == false.
func readStopReport(taskLocation *lymcwd.Location) (path string, at time.Time, found bool, err error) {
	marker := shedrun.ParkMarker(taskLocation, shedrun.SelfRunID)
	info, err := os.Stat(marker)
	if os.IsNotExist(err) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		return "", time.Time{}, false, err
	}
	return strings.TrimSpace(string(raw)), info.ModTime(), true, nil
}

// readAgentActivity reads the running agent runs of the task worktree.
// Those include runs whose waiting process is gone.
// It reads them through the shuttle config and its registered engines, and maps them onto dobbyengine's neutral type.
// It also reports whether a Go-side wait is live.
// A running verify, a parent lock wait, or a child at a row of loomshed.OperatorWaitRows is such a wait.
// A live gate runner or a fixer process of the pair younger than its timeout counts as well.
// A shuttle wait that is not a held turn end counts too.
// A held run is idle at its turn end,
// so it does not suppress the quiet notice, and neither does a verify held by a pause.
// A lock wait record whose writer is dead is ignored.
// The waits are read from their marker and record files, never from the pane options that display them.
func readAgentActivity(taskLocation *lymcwd.Location) (runs []dobbyengine.AgentRun, waitLive bool, err error) {
	anchor := taskLocation.AnchorPath()
	cfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "shuttle")
	if err != nil {
		return nil, false, err
	}
	engines, err := shuttleengines.New(cfg, fabricengine.ConfigDirs(taskLocation))
	if err != nil {
		return nil, false, err
	}
	readings, err := shuttleengine.ReadAgentActivity(cfg, anchor, engines)
	if err != nil {
		return nil, false, err
	}
	states := readSessionStatesByStrand(cfg, anchor, engines)
	for _, reading := range readings {
		run := dobbyengine.AgentRun{Producer: reading.StrandName, LastActivity: reading.LastActivity, APIError: reading.APIError, APIErrorText: reading.APIErrorText,
			UsageLimited: reading.UsageLimit.Limited, UsageLimitResetsAt: reading.UsageLimit.ResetsAt}
		if state, ok := states[reading.StrandName]; ok {
			run.SessionState, run.SessionCause = string(state.Name), state.Cause
			if !state.Since.IsZero() {
				run.SessionSince = state.Since.Format(time.RFC3339)
			}
		}
		runs = append(runs, run)
	}
	verifyMarker, verifyLive, err := verifytree.ReadMarker(verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(anchor)).Marker)
	if err != nil {
		return nil, false, err
	}
	if verifyLive && verifyMarker.State != verifytree.MarkerStatePaused {
		return runs, true, nil
	}
	_, lockWaiting, err := parentlock.ReadWait(parentlock.WaitRecordPath(loomengine.LoomScratchDir(taskLocation)), parentlock.ProcessAlive)
	if err != nil {
		return nil, false, err
	}
	if lockWaiting {
		return runs, true, nil
	}
	if childAtWaitingRow(taskLocation) {
		return runs, true, nil
	}
	gateLive, err := gateProcessLive(taskLocation, time.Now())
	if err != nil {
		return nil, false, err
	}
	if gateLive {
		return runs, true, nil
	}
	markers, err := shuttleengine.ReadWaitMarkers(cfg, anchor)
	if err != nil {
		return nil, false, err
	}
	for _, marker := range markers {
		if !marker.Held() {
			return runs, true, nil
		}
	}
	return runs, false, nil
}

// gateProcessLive reports whether the pair of taskLocation has a live registered gate process:
// a detached `lym gate` runner, or the pane process of a cold fixer strand.
// A fixer works in its own session while the writer's strand sits idle,
// and a gate run shows no agent activity at all, so either one is the run at work.
// A fixer record counts only while it is younger than the fixer's own timeout at now.
// The age bound matters: a fixer that ended without its strand removed leaves a pane process outliving its work.
func gateProcessLive(taskLocation *lymcwd.Location, now time.Time) (bool, error) {
	records, err := runproc.List(fabricengine.RunProcDir(taskLocation.HubPath), taskLocation.WorktreeName)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.Role == runproc.RoleGate {
			return true, nil
		}
		if !gatefixer.IsFixerRole(record.Role) {
			continue
		}
		bound, bounded, err := fixerTimeout(taskLocation, record.Role)
		if err != nil {
			return false, err
		}
		if !bounded || now.Sub(record.RegisteredAt) <= bound {
			return true, nil
		}
	}
	return false, nil
}

// fixerTimeout returns how long a fixer under role may run:
// webster's recovery timeout for a verify fixer, the gate config's fixer timeout for any other.
// It reports bounded == false when the gate config is absent, which leaves a gate fixer with no known bound.
func fixerTimeout(taskLocation *lymcwd.Location, role string) (timeout time.Duration, bounded bool, err error) {
	dirs := fabricengine.ConfigDirs(taskLocation)
	if agentname.MatchesRole(role, gatefixer.RoleVerifyFixer) {
		cfg, err := websterengine.LoadConfig(dirs, "webster")
		if err != nil {
			return 0, false, err
		}
		return time.Duration(cfg.RecoveryTimeoutMin) * time.Minute, true, nil
	}
	cfg, err := gateslot.LoadConfig(dirs)
	if errors.Is(err, gateslot.ErrConfigAbsent) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return cfg.FixerTimeout(), true, nil
}

// readNewestAgentEnd reads when the task worktree's newest ended shuttle run ended, or the zero time when none has.
func readNewestAgentEnd(taskLocation *lymcwd.Location) (time.Time, error) {
	cfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "shuttle")
	if err != nil {
		return time.Time{}, err
	}
	return shuttleengine.ReadNewestRunEnd(cfg, taskLocation.AnchorPath())
}

// childAtWaitingRow reports whether the child's own status file reads running at a row of loomshed.OperatorWaitRows.
// Those rows wait with no agent running.
// The wait is live for as long as the status reads running at such a row.
// An unreadable child status reads as no such wait, as an absent one does.
func childAtWaitingRow(taskLocation *lymcwd.Location) bool {
	statusPath := shedrun.StatusFile(taskLocation, shedrun.SelfRunID)
	if _, err := os.Stat(statusPath); err != nil {
		return false
	}
	statusLockPath := shedrun.StatusLock(taskLocation, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		logger.Debug("battencli: the child's status lock directory could not be made; it reads as no waiting row", "path", statusLockPath, "cause", err)
		return false
	}
	status, found, err := state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
	if err != nil {
		logger.Debug("battencli: the child's status is unreadable; it reads as no waiting row", "path", statusPath, "cause", err)
		return false
	}
	if !found || status.State != shedengine.StateRunning {
		return false
	}
	return loomshed.WaitsOnOperator(status.CurrentProducer)
}

// readSessionStatesByStrand reads the session state of every running shuttle run under anchor, keyed by strand name.
// The states are only logged beside run notices, so a failed read is logged at Warn once and yields none.
func readSessionStatesByStrand(cfg shuttleengine.Config, anchor string, engines shuttleengine.EngineSet) map[string]shuttleengine.SessionState {
	readings, err := shuttleengine.ReadSessionStates(cfg, anchor, engines, time.Now())
	if err != nil {
		logger.Warn("batten: session states unreadable; the notice logs carry none", "anchor", anchor, "cause", err)
		return nil
	}
	states := make(map[string]shuttleengine.SessionState, len(readings))
	for _, reading := range readings {
		states[reading.StrandName] = reading.State
	}
	return states
}

// markBattenWatched writes the batten-watched marker of the task worktree's run while notices reach the drivers' parent.
// It removes the marker otherwise.
// The marker holds this process's pid and names the batten scratch directory and the Run-Shed row's name,
// so a Go driver can tell whether this batten has died.
// It reports whether the marker is held.
func markBattenWatched(prime, taskLocation *lymcwd.Location, scratchDir, producer string, pid int) (bool, error) {
	watched, err := noticesReachDriversParent(prime, taskLocation)
	if err != nil {
		return false, err
	}
	return syncWatchedMarker(shedrun.BattenWatchedMarker(taskLocation, shedrun.SelfRunID), watched, shedrun.BattenWatchedContent(pid, scratchDir, producer))
}

// dobbyReading reads the state record of the dobby of the run's parent worktree: its pid and the machine sleeps it recorded.
// A dead dobby, a missing record and a worktree absent here read as this process's pid and no sleeps,
// so a hand pass with no live dobby marks itself and leaves the child's driver to notice for itself.
// Any other read error is returned.
func dobbyReading(prime *lymcwd.Location, slug string) (pid int, sleeps []pauserecord.Interval, err error) {
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return 0, nil, err
	}
	if parent == "" {
		parent = prime.WorktreeName
	}
	if info, statErr := os.Stat(filepath.Join(prime.HubPath, parent)); statErr != nil || !info.IsDir() {
		return os.Getpid(), nil, nil
	}
	geometry, err := hubgeom.DobbyGeometry(prime, parent)
	if err != nil {
		return 0, nil, err
	}
	return dobbyStateReading(geometry.StatePath, func() (bool, error) { return dobbyengine.Live(geometry) })
}

// dobbyStateReading reads a dobby's pid and sleeps from its state record at statePath.
// The liveness of the dobby is probed with live.
func dobbyStateReading(statePath string, live func() (bool, error)) (int, []pauserecord.Interval, error) {
	running, err := live()
	if err != nil {
		return 0, nil, err
	}
	if !running {
		return os.Getpid(), nil, nil
	}
	record, found, err := dobbyengine.ReadState(statePath)
	if err != nil {
		return 0, nil, err
	}
	if !found || record.PID == 0 {
		return os.Getpid(), nil, nil
	}
	return record.PID, record.Sleeps, nil
}

// awakeReading is the clock every pass shares:
// the wall time since the Unix epoch less the machine sleeps the scheduling dobby recorded.
// It stops while the machine sleeps and holds one origin across processes,
// so the idle watches the memory record restores keep measuring awake time.
func awakeReading(now time.Time, sleeps []pauserecord.Interval) time.Duration {
	return time.Duration(now.UnixNano()) - pauserecord.Excluded(sleeps, time.Unix(0, 0), now)
}

// syncWatchedMarker makes the marker at path match watched: absent when not watched, and holding content when watched.
// A marker already holding content is not rewritten.
// Removing an absent marker is not an error.
// A write is atomic, so a driver's render never reads a torn marker.
// It reports whether the marker is held.
func syncWatchedMarker(marker string, watched bool, content []byte) (bool, error) {
	if !watched {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return false, nil
	}
	if existing, err := os.ReadFile(marker); err == nil && bytes.Equal(existing, content) {
		return true, nil
	}
	if err := fsx.AtomicWriteBytes(marker, content); err != nil {
		return false, err
	}
	return true, nil
}

// readTacklerAsArbiterMarker reads the tackler marker at path as the arbiter marker of the same attempt, strand and deadline, so one reader serves both in-flight seams.
func readTacklerAsArbiterMarker(path string) (loomengine.ArbiterMarker, bool, error) {
	marker, found, err := sheddriver.ReadTacklerMarker(path)
	if err != nil || !found {
		return loomengine.ArbiterMarker{}, false, err
	}
	return loomengine.ArbiterMarker{Attempt: marker.Attempt, StrandGUID: marker.StrandGUID, Deadline: marker.Deadline}, true, nil
}

// dirtyPathsOutsideRunRecords lists the task pair's uncommitted or untracked paths outside the run's own records directories.
// Each path is joined onto the task worktree, since the records side reaches it through the `_lym` junction.
func dirtyPathsOutsideRunRecords(taskLocation *lymcwd.Location) ([]string, error) {
	fab, err := fabricengine.Open(taskLocation)
	if err != nil {
		return nil, err
	}
	entries, err := fab.Status()
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(taskLocation.WorktreePath(), filepath.FromSlash(entry.Path)))
	}
	return loomengine.PathsOutsideRunRecords(taskLocation, shedrun.SelfRunID, paths), nil
}

// commandDeps returns the Env.Command facts and runner of the Open-Window, Seed-IDE and Open-IDE rows.
// The commands run in the prime, and {task_dir} is the task worktree's anchor path, located when a command holds it.
// {title} is the slug opened with the task's board handle, `T4: <slug>`, read from the board when a command holds it.
// When the board names no handle, the title is the slug alone.
// Each command's output goes to a command-<row>.log under the run's scratch directory.
func (c *battenCLI) commandDeps(location *lymcwd.Location, slug, executable string, locateTask func() (*lymcwd.Location, error)) shedadapters.CommandDeps {
	return shedadapters.CommandDeps{
		Lym:  executable,
		Slug: slug,
		TaskDir: func() (string, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return "", err
			}
			return taskLocation.AnchorPath(), nil
		},
		Title: func() string {
			return boardengine.TitleWithHandle(fabricengine.BoardDir(location.HubPath), slug, slug)
		},
		Dir: location.WorktreePath(),
		LogPath: func(row string) string {
			return filepath.Join(shedrun.ScratchDir(location, slug), "command-"+row+".log")
		},
		Run:   runRowCommand,
		Rerun: fmt.Sprintf("lym batten run %s", slug),
	}
}

// wire builds and stores the shedrecipe.Env and shedbuild.ShedPaths the run and status verbs
// need, over the resolved prime location and slug.
func (c *battenCLI) wire(location *lymcwd.Location, slug string) error {
	// Batten runs in the prime on behalf of one pair, so its network git and GitHub calls wait under that pair's pause entry.
	pauserecord.SetGateSlug(slug)
	primeRunLockPath := PrimeRunLock(location)
	primeLock := battenshed.PrimeLock{
		Path: primeRunLockPath,
		Acquire: func() (release func() error, ok bool, err error) {
			// The prime lock's own directory is created first, since it is the lock being acquired.
			// The per-slug scratch directory comes next: the producers holding this lock write their stuck-reason files there.
			// The two shedrun constructors state no relationship, so neither may be left to the other's MkdirAll.
			if err := os.MkdirAll(filepath.Dir(primeRunLockPath), 0o755); err != nil {
				return nil, false, err
			}
			if err := os.MkdirAll(shedrun.ScratchDir(location, slug), 0o755); err != nil {
				return nil, false, err
			}
			fl, acquired, err := lock.TryAcquireWriteLock(primeRunLockPath)
			if err != nil {
				return nil, false, err
			}
			if !acquired {
				return nil, false, nil
			}
			return fl.Release, true, nil
		},
	}

	// Every seam below that reads the task worktree locates it through this one locator, so git runs only until the first resolution succeeds.
	locateTask := taskWorktreeLocator(location, slug)

	// The on-disk lym path is resolved once, here: after a deploy replaces the file under this running batten, os.Executable reads a path that no longer names it.
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("batten: resolve the lym executable: %w", err)
	}

	// One closure keeps the marker for both rows that ask: Seed-Child once the seed is committed, and Run-Shed through the whole watch.
	markWatched := func(ctx context.Context) (bool, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return false, err
		}
		pid, _, err := dobbyReading(location, slug)
		if err != nil {
			return false, err
		}
		return markBattenWatched(location, taskLocation, BattenDir(location, slug), battenrecipe.NameRunShed, pid)
	}

	// readReedDirectory reads the task worktree's reed directory, for the seams that tell a marker's strand live.
	readReedDirectory := func() ([]reedengine.DirectoryRow, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return nil, err
		}
		reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "reed")
		if err != nil {
			return nil, err
		}
		reedGeom, err := hubgeom.ReedGeometry(taskLocation)
		if err != nil {
			return nil, err
		}
		return reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub()).Directory()
	}

	env := shedrecipe.Env{
		Slug:       slug,
		ScratchDir: BattenDir(location, slug),
		PrimeLock:  primeLock,
		// The closure takes holdPrime only for the fork-point read, the registry write and a held pair's removal.
		// Claim, reconcile and push run outside it, and origin probes start before it; a waiting create holds up no other run.
		CreateWorktree: func(ctx context.Context, holdPrime battenshed.HoldPrimeLock) error {
			// The claim comes first,
			// so the run lock on the board entry starts before the pair exists and survives this row failing.
			// It records this machine as the entry's owner in the same commit,
			// so a failed write or push refuses the row and the stopped run resumes with the same verb.
			// Its push stays synchronous: it is what tells another machine this one owns the entry.
			if err := c.claimBoardEntry(slug); err != nil {
				return err
			}

			// The already-complete probe runs before Add,
			// so a row re-entered after a crash is idempotent against its own post-condition.
			present, complete, incompleteReason, err := taskWorktreeComplete(location, slug)
			if err != nil {
				return err
			}
			if complete {
				logger.Info("battencli: create worktree skipped, the task worktree is already present", "slug", slug)
				// A member that crashed between Add and its hold read is judged here when it is resumed.
				if err := c.refuseHeldMember(ctx, location, slug, holdPrime); err != nil {
					return err
				}
				return reconcileTaskPair(location, slug)
			}
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return err
			}
			if present {
				branch, err := fabricengine.NewTopology(cfg).PairTaskBranch(location, slug)
				if err != nil {
					return err
				}
				// A worktree exists but Add never finished it.
				// Add refuses an existing worktree directory the same way it refuses a leftover branch,
				// so the remedy is a cleanup followed by a resume.
				return fmt.Errorf(
					"the task worktree for %q exists but %s; %s, then resume this run to create it fresh",
					slug, incompleteReason, incompletePairRemedy(slug, branch),
				)
			}

			top := fabricengine.NewTopology(cfg)
			// A member of a beam forks from the beam's pair, whichever worktree started the run;
			// every other task forks from the prime.
			parent, err := beamForkParent(location, slug, func(beam string) (bool, error) { return top.TaskBranchOnOrigin(location, beam) })
			if err != nil {
				return err
			}
			// A beam project's pair takes the anchor its first pass recorded in the seed.
			// The pair stays unmarked until the pawl run's Beam-Up marks it.
			var anchor string
			project, err := c.entryIsProject()
			if err != nil {
				return err
			}
			if project {
				if anchor, err = anchorFromSeed(location, slug); err != nil {
					return err
				}
			}
			_, err = top.Add(location, slug, fabricengine.AddOptions{Parent: parent, Anchor: anchor, Hold: holdPrime})
			logger.Info("battencli: create worktree", "slug", slug)
			if err != nil {
				return createRefusal(err, slug, cfg.BranchPrefix+slug, func() bool { return fabricengine.PairWorktreeStands(location, slug) })
			}
			// The beam's accept may have written its hold while the pair was being made,
			// so the member's beam is read again before anything more is done to the pair.
			if err := c.refuseHeldMember(ctx, location, slug, holdPrime); err != nil {
				return err
			}
			if err := reconcileTaskPair(location, slug); err != nil {
				return err
			}
			// Add pushes nothing; the new pair goes out through a detached push the run never waits on.
			// A failed spawn leaves the branches local for the pair's next detached push, so it only warns.
			if taskLocation, err := locateTask(); err != nil {
				logger.Warn("battencli: locate the new pair to push it failed; its next push carries the branches", "slug", slug, "error", err)
			} else if err := spawnPairPush(taskLocation); err != nil {
				logger.Warn("battencli: spawn the new pair's push failed; its next push carries the branches", "slug", slug, "error", err)
			}
			return nil
		},
		// Both teardown halves run through the pair-teardown composite,
		// and both are idempotent against their shared post-condition: the pair is gone.
		// shedengine persists the row's transition only after the producer returns,
		// so a process killed right after Remove succeeds re-enters the row with no pair on disk.
		// EndSession's refusal probe then reports ErrPairNotFound, which Shutdown takes as nothing left to shut down.
		// RemovePair reports it again as done.
		// The composite finishes a half-removed pair itself (sibling worktree, launcher entries, both branches),
		// so no such state is refused by name here.
		// EndSession ends the session by name when the task worktree is gone, and runs the refusal probe,
		// so a refusal reaches the row before Remove and leaves the pair in place.
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(ctx context.Context) (abandonedSession string, err error) {
				td, err := pairteardown.New(location)
				if err != nil {
					return "", err
				}
				res, err := td.EndSession(ctx, teardownRequest(slug))
				if errors.Is(err, fabricengine.ErrPairNotFound) {
					logger.Info("battencli: session shutdown found nothing of the pair left", "slug", slug)
					return "", nil
				}
				if err != nil {
					return "", teardownRefusal(err, slug)
				}
				return res.AbandonedSession, nil
			},
			Remove: func(ctx context.Context) error {
				td, err := pairteardown.New(location)
				if err != nil {
					return err
				}
				res, err := td.RemovePair(teardownRequest(slug))
				return finishRemoval(res, err, slug)
			},
		},
		Land: landDeps(location, slug, locateTask),
		InnerRun: battenshed.InnerRunDeps{
			// Notify posts the notice as creel mail to the nearest live orch above the run, and reports whether it was posted.
			Notify: func(ctx context.Context, line string) (bool, error) {
				return notifyRunParent(location, slug, line)
			},
			// OrchStrandRecorded reads whether the nearest live orch above the run records a strand for this run.
			// The same fact keeps the run marked as watched.
			OrchStrandRecorded: func() (bool, error) { return runParentRecorded(location, slug) },
			// StopReport reads the driver's park marker in the task worktree.
			// It returns the stop report path the marker holds and the time it was written.
			StopReport: func() (string, time.Time, bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return "", time.Time{}, false, err
				}
				return readStopReport(taskLocation)
			},
			// ArbiterStop reads, in the task worktree, the facts the eligible-stop hold decides on at the child's halted stop.
			ArbiterStop: func(ctx context.Context, status shedengine.Status) (battenshed.ChildArbiterStop, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return battenshed.ChildArbiterStop{}, err
				}
				return arbiterstop.HoldReading(taskLocation, status)
			},
			MarkWatched: markWatched,
			// Awake is the wall clock less those sleeps, so every pass measures awake time from the same origin.
			// An unreadable record excludes none, and the error is logged.
			Awake: func() time.Duration {
				_, sleeps, err := dobbyReading(location, slug)
				if err != nil {
					logger.Debug("battencli: the dobby's recorded sleeps are unreadable; the awake clock excludes none", "slug", slug, "error", err)
				}
				return awakeReading(time.Now(), sleeps)
			},
			ParentLock: childParentLock(slug, locateTask, fabricengine.BoltDir(location.HubPath), lockIdleBound(location)),
			// OwnerCheck judges, without moving a ref, the slug's run-scope reconcile from the prime and reads its owner verdict.
			OwnerCheck: func(ctx context.Context) (battenshed.OwnerVerdict, error) {
				teardown, err := pairteardown.New(location)
				if err != nil {
					return battenshed.OwnerVerdict{}, err
				}
				report, err := fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{Run: slug, JudgeOnly: true}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), fabricengine.ReconcileOptions{})
				if err != nil {
					return battenshed.OwnerVerdict{}, err
				}
				return ownerVerdictOf(report, slug), nil
			},
			PauseOwned: func(ctx context.Context, owner string) error {
				return tendercli.PauseOwnedElsewhere(location, slug)
			},
			// ParentHoldsTaskWork marks a Finalize past its parent-side merge, read over the prime and the located task worktree.
			ParentHoldsTaskWork: func() (bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return false, err
				}
				return fabricengine.ParentHoldsTaskWork(location, taskLocation, slug)
			},
			// Restepped reads the child's settle records for a re-step of the status's stop.
			Restepped: func(status shedengine.Status) (bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return false, err
				}
				records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(taskLocation, shedrun.SelfRunID))
				if err != nil {
					return false, err
				}
				return sheddriver.RestepRecorded(records, status), nil
			},
			// HandBack marks a blocked landing halt, decision halt or tapestry, darn or sampler escalate halt,
			// so the binary re-step leaves it alone.
			HandBack: restepSkips,
			// PauseRequested reads batten's own status file, the one `lym batten pause` writes,
			// so the in-call wait notices a pause within one check.
			PauseRequested: pauseRequested(location, slug),
			// Pause reads the hub's pause record at the prime's geometry, for the slug this run drives.
			Pause: func() (pauserecord.Coverage, error) {
				geometry, err := hubgeom.PauseGeometry(location)
				if err != nil {
					return pauserecord.Coverage{}, err
				}
				return pauserecord.ReadCoverage(geometry.RecordPath, slug, time.Now())
			},
			PairDir: func() (string, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return "", err
				}
				return taskLocation.WorktreePath(), nil
			},
			// DriverAlive resolves the task worktree at each call, never at wiring time.
			// ReadDecision reports no decision: no verb writes an approval or rejection record,
			// so an awaiting child is never resumed on an operator decision.
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				return battenshed.ChildDecision{}, false, nil
			},
			DriverAlive: func(ctx context.Context) (bool, error) {
				return childDriverAlive(location, slug, locateTask)
			},
			DriverStrand: func(ctx context.Context) (battenshed.ChildDriverStrand, error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return battenshed.ChildDriverNone, err
				}
				return driverStrandFrom(present, func() ([]reedengine.DirectoryRow, error) {
					taskLocation, err := locateTask()
					if err != nil {
						return nil, err
					}
					reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "reed")
					if err != nil {
						return nil, err
					}
					reedGeom, err := hubgeom.ReedGeometry(taskLocation)
					if err != nil {
						return nil, err
					}
					return reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub()).Directory()
				})
			},
			// ArbiterInFlight reads the task worktree's in-flight arbiter marker against the pair's reed directory.
			ArbiterInFlight: func(ctx context.Context) (battenshed.ChildArbiter, bool, error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return battenshed.ChildArbiter{}, false, err
				}
				return arbiterInFlightFrom(present,
					func() (loomengine.ArbiterMarker, bool, error) {
						taskLocation, err := locateTask()
						if err != nil {
							return loomengine.ArbiterMarker{}, false, err
						}
						return loomengine.ReadArbiterMarker(shedrun.ArbiterMarker(taskLocation, shedrun.SelfRunID))
					},
					readReedDirectory)
			},
			// TacklerInFlight reads the task worktree's in-flight tackler marker against the pair's reed directory.
			// A live row carrying the tackler's strand reads alive.
			TacklerInFlight: func(ctx context.Context) (battenshed.ChildArbiter, bool, error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return battenshed.ChildArbiter{}, false, err
				}
				return arbiterInFlightFrom(present,
					func() (loomengine.ArbiterMarker, bool, error) {
						taskLocation, err := locateTask()
						if err != nil {
							return loomengine.ArbiterMarker{}, false, err
						}
						return readTacklerAsArbiterMarker(shedrun.TacklerMarker(taskLocation, shedrun.SelfRunID))
					},
					readReedDirectory)
			},
			// ReviveStrands ends the pair's dead arbiter strands, then resumes the task worktree's reed session.
			// The resume relaunches the dead driver strand and every other non-live strand of the pair.
			// A dead arbiter is stopped rather than relaunched: its attempt reads as hand-on.
			ReviveStrands: func(ctx context.Context) error {
				taskLocation, err := locateTask()
				if err != nil {
					return err
				}
				reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "reed")
				if err != nil {
					return err
				}
				reedGeom, err := hubgeom.ReedGeometry(taskLocation)
				if err != nil {
					return err
				}
				shuttleCfg, err := shuttleengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "shuttle")
				if err != nil {
					return err
				}
				reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
				rows, err := reedEngine.Directory()
				if err != nil {
					return err
				}
				engines, err := shuttleengines.New(shuttleCfg, fabricengine.ConfigDirs(taskLocation))
				if err != nil {
					return err
				}
				runner := shuttleengine.NewRunner(reedEngine, engines, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
				pauseGeom, err := hubgeom.PauseGeometry(taskLocation)
				if err != nil {
					return err
				}
				runner.SetPause(pauseGeom)
				runner.SetSessionEnd(shedadapters.UsageRecorder(taskLocation, shuttleengines.RecordReaders()))
				if err := stopDeadArbiterStrands(rows, runner.StopStrand); err != nil {
					logger.Warn("battencli: stopping the task worktree's dead arbiter strands failed", "slug", slug, "error", err)
					return err
				}
				logger.Info("battencli: reviving the task worktree's strands through reed resume", "slug", slug)
				res, err := reedEngine.Resume()
				if err != nil {
					logger.Warn("battencli: reed resume of the task worktree failed", "slug", slug, "error", err)
					return err
				}
				logger.Info("battencli: reed resume of the task worktree finished", "slug", slug, "resumed", res.Resumed, "dropped", res.Dropped, "held", res.Held)
				return nil
			},
			ChildRunLockHeld: func() (bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return false, err
				}
				return runLockHeld(shedrun.RunLock(taskLocation, shedrun.SelfRunID))
			},
			// ChildDriverLockHeld probes the Go driver's lock, which the driver holds for life.
			// The run lock is free between the child's steps.
			ChildDriverLockHeld: func() (bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return false, err
				}
				return runLockHeld(shedrun.DriverLock(taskLocation, shedrun.SelfRunID))
			},
			// ChildBuildChanged compares the lym on disk with the build the child's last step recorded.
			// A deploy replaces that lym under this running batten.
			ChildBuildChanged: func() (string, bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return "", false, err
				}
				onDisk, err := buildvcs.OfFile(executable)
				if err != nil {
					return "", false, err
				}
				return onDisk.Label(), shedverbs.LastStepBuildChanged(shedrun.StepsDir(taskLocation, shedrun.SelfRunID), onDisk), nil
			},
			// ChildDirtyPaths lists the task pair's uncommitted or untracked paths outside the run's records directories.
			ChildDirtyPaths: func() ([]string, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return nil, err
				}
				return dirtyPathsOutsideRunRecords(taskLocation)
			},
			// ResolveStatus also creates the child's ephemeral status-lock directory.
			// The child's status file is durable, but its lock is not.
			// The caller reads through that lock next, and nothing else on the Run-Shed path creates the directory.
			ResolveStatus: func() (statusPath, statusLockPath string, err error) {
				taskLocation, err := locateTask()
				if err != nil {
					return "", "", err
				}
				statusPath = shedrun.StatusFile(taskLocation, shedrun.SelfRunID)
				statusLockPath = shedrun.StatusLock(taskLocation, shedrun.SelfRunID)
				if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
					return "", "", err
				}
				return statusPath, statusLockPath, nil
			},
			Spawn: func(ctx context.Context) error {
				taskLocation, err := locateTask()
				if err != nil {
					return err
				}
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				// The bootstrap verb of tapestry and darn alike, not a per-recipe dispatch:
				// the WriteSeed seam below admits no other child recipe, so the two stay consistent by construction.
				// Dir is the task worktree's AnchorPath(), never its bare worktree root.
				// The resolver gates a child's working directory to the anchor, and a bare root fails on any subpath-anchored hub.
				cmd := exec.Command(exe, "loom", "start", "--no-attach")
				cmd.Dir = taskLocation.AnchorPath()
				// Captured rather than discarded: the child's own envelope is the only
				// diagnosis this seam can offer for a failed bootstrap.
				var childOutput bytes.Buffer
				cmd.Stdout = &childOutput
				cmd.Stderr = &childOutput
				logger.Debug("battencli: spawning loom session", "slug", slug, "dir", cmd.Dir)
				runErr := cmd.Run()
				logger.Debug("battencli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
				return childSpawnError(runErr, childOutput.String())
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return state.ReadJSON[shedengine.Status](statusPath, statusLockPath)
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			// ReadBoardType opens the hub's Board with its board.yaml fresh on every call and returns the recipe that boardRecipe gives the entry.
			// It never returns a value captured at wiring time,
			// so a recipe or default_recipe corrected after prime was seeded is still honoured.
			ReadBoardType: func(ctx context.Context) (string, error) {
				b, err := boardengine.OpenHub(location.HubPath)
				if err != nil {
					return "", err
				}
				task, found, err := b.GetTask(slug)
				if err != nil {
					return "", err
				}
				if !found {
					return "", fmt.Errorf("battencli: board task %q not found", slug)
				}
				return boardRecipe(b, task), nil
			},
			// ChildDriver returns the "child_driver" param of prime's own seed when the operator typed one.
			// Otherwise it returns the child recipe's resolved driver_kind.
			// The kind is read from the task worktree's config layers.
			// The seeding step calls this after it writes that worktree's local layer, so a driver_kind from the run settings is seen.
			ChildDriver: func(recipe string) (string, error) {
				seed, found, err := shedrun.ReadSeed(location, slug)
				if err != nil {
					return "", err
				}
				return childDriverFrom(seed, found, func() (string, error) {
					taskLocation, err := locateTask()
					if err != nil {
						return "", err
					}
					return childDriverKind(fabricengine.ConfigDirs(taskLocation), recipe)
				})
			},
			// ReadRunSettings opens the Board fresh on every call and returns the task's profile and run_settings.
			ReadRunSettings: func(ctx context.Context) (battenshed.RunSettings, error) {
				b := boardengine.New(boardengine.Config{Path: fabricengine.BoardDir(location.HubPath)})
				task, found, err := b.GetTask(slug)
				if err != nil {
					return battenshed.RunSettings{}, err
				}
				if !found {
					return battenshed.RunSettings{}, fmt.Errorf("battencli: board task %q not found", slug)
				}
				return battenshed.RunSettings{Profile: task.Profile, Settings: task.RunSettings}, nil
			},
			// PrepareRunSettings reads the named profile from the hub's profiles.yaml.
			// It lays the task's settings over that profile through configreg.
			// An unknown profile or a refusal of the settings is wrapped in battenshed.ErrInvalidRunSettings,
			// so the seeding step routes it to Stuck.
			PrepareRunSettings: func(settings battenshed.RunSettings) (map[string][]byte, error) {
				return prepareRunSettings(configengine.Dirs{Board: fabricengine.BoltDir(location.HubPath)}, settings)
			},
			// WriteLocalOverride writes one module's local file in the task worktree, creating its directory.
			WriteLocalOverride: func(ctx context.Context, module string, document []byte) error {
				taskLocation, err := locateTask()
				if err != nil {
					return err
				}
				return fsx.AtomicWriteBytes(configengine.LocalFile(taskLocation.AnchorPath(), module), document)
			},
			// RemoveLocalOverrides removes the local file of every registered module outside keep and returns the paths that existed.
			RemoveLocalOverrides: func(ctx context.Context, keep []string) ([]string, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return nil, err
				}
				var removed []string
				for _, module := range configreg.Names() {
					if slices.Contains(keep, module) {
						continue
					}
					path := configengine.LocalFile(taskLocation.AnchorPath(), module)
					if err := os.Remove(path); err != nil {
						if errors.Is(err, os.ErrNotExist) {
							continue
						}
						return removed, err
					}
					removed = append(removed, path)
				}
				return removed, nil
			},
			// WriteSeed is the only place in the batten path that encodes a seed.
			// It validates recipe itself: an unknown name is wrapped in battenshed.ErrUnknownRecipe,
			// and a known recipe that cannot be bootstrapped is wrapped in battenshed.ErrUnsupportedChildRecipe.
			// A shedrun.ErrDisagreeingSeed from shedrun.WriteSeed means a pre-existing child seed disagrees with this one.
			// Such an error is re-wrapped as battenshed.ErrDisagreeingChildSeed.
			// Callers that match these errors route all three to the Stuck state rather than a hard error.
			//
			// The params come from childSeedParams.
			// A seed missing a param that the child's own bootstrap writes is refused by that bootstrap,
			// so the seed must carry them all.
			WriteSeed: func(ctx context.Context, recipe, driver string) error {
				if err := shedrun.ValidateRecipe(recipe); err != nil {
					return fmt.Errorf("%w: %s", battenshed.ErrUnknownRecipe, err.Error())
				}
				// Refused here, ahead of any write, because spawning the child runs loom's bootstrap verb unconditionally.
				// A child seeded with any other recipe would carry a committed, pushed seed.
				// Its own bootstrap then refuses that seed as a disagreement;
				// the child reports it as a shedrun fault rather than as this choice.
				if !childRecipeSupported(recipe) {
					return unsupportedChildRecipeRefusal(recipe)
				}
				childLocation, err := locateTask()
				if err != nil {
					return err
				}
				params, err := childSeedParams(recipe, childLocation)
				if err != nil {
					return err
				}
				if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, childSeedFor(recipe, driver, params)); err != nil {
					if errors.Is(err, shedrun.ErrDisagreeingSeed) {
						return fmt.Errorf("%w: %s", battenshed.ErrDisagreeingChildSeed, err.Error())
					}
					return err
				}
				// Batten's own run directory keeps the child's recipe, which the next status commit carries.
				// A done run then still identifies a motif child after its pair is gone.
				return shedrun.WriteChildRecipe(location, slug, recipe)
			},
			// CommitSeed commits the child's seed onto the child's fabric pair as a one-off write.
			// Batten's status commits to prime's own pair are separate, and they run on every non-no-op transition.
			CommitSeed: func(ctx context.Context) error {
				childLocation, err := locateTask()
				if err != nil {
					return err
				}
				_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation, []string{shedrun.SeedRel(childLocation, shedrun.SelfRunID)}, fmt.Sprintf("batten: seed child %s", slug), fabricengine.EnvSyncOptions())
				return err
			},
			MarkWatched: markWatched,
			// PushSeed spawns the detached push of the child's fabric pair and does not wait for it.
			// The child reads its seed locally, so the push need not finish first.
			PushSeed: func(ctx context.Context) error {
				childLocation, err := locateTask()
				if err != nil {
					return err
				}
				return spawnPairPush(childLocation)
			},
		},
	}

	// Each setting is read when its row is reached, over the task worktree located then,
	// so a task's run_settings in that worktree's local layer switch a row for that task alone.
	env.Setting = func(module, key string) (any, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return nil, err
		}
		return configreg.SettingLookup(fabricengine.ConfigDirs(taskLocation))(module, key)
	}
	// A Command row's command is read from the template and the board default layer only, unlike deploy and darn's verify.
	env.ExecutableSetting = configreg.SettingLookup(configengine.Dirs{Board: fabricengine.ConfigDirs(location).Board})
	// A recipe row's entry gate checks its setting operands at build.
	// Build needs no task worktree, and the gate's evaluation errors name the resume command.
	env.SettingKnown = configreg.CheckSettingKey
	env.Resume = fmt.Sprintf("lym batten run %s", slug)
	env.Command = c.commandDeps(location, slug, executable, locateTask)

	c.beam = hubLandingSeams(location, slug)
	c.clientAttached = func() (bool, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return false, err
		}
		reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(taskLocation), "reed")
		if err != nil {
			return false, err
		}
		reedGeom, err := hubgeom.ReedGeometry(taskLocation)
		if err != nil {
			return false, err
		}
		return reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub()).ClientAttached()
	}

	c.env = env
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     StatusFile(location, slug),
		LockPath:       RunLock(location, slug),
		StatusLockPath: StatusLock(location, slug),
		// CommitStatus commits prime's batten status onto prime's fabric pair on every non-no-op transition.
		// The status file is durable, fabric-synced state, so the seam is required here rather than nil.
		CommitStatus: battenCommitStatusSeam(location, slug),

		RunID:                   slug,
		MissingStatusWayForward: fmt.Sprintf("way forward: \"lym batten run %s\" creates the lifecycle's status file", slug),
		PauseWayForward:         fmt.Sprintf("lym batten pause %s", slug),
		BudgetWayForward:        fmt.Sprintf("run \"lym batten run %s\": the resume grants the row one more budget", slug),
	}
	return nil
}

// restepSkips reports whether a child's status is a blocked halt the binary re-step leaves alone:
// a landing halt or a decision halt for the orch, or a tapestry, darn or sampler fixer's escalate halt for the arbiter.
// The halt is read from the status's current producer and error.
func restepSkips(status shedengine.Status) bool {
	if status.State != shedengine.StateBlocked {
		return false
	}
	return arbiterstop.HandsBack(status.CurrentProducer, status.Error) || landingshed.EscalateHalt(status.CurrentProducer, status.Error)
}
