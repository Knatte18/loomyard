// wire.go implements wire, the assembly seam that builds the shedrecipe.Env and
// shedbuild.ShedPaths the run and status verbs need, plus the receiver field the abandoned
// session value is recorded into.
//
// Every seam that touches the managed task worktree resolves inside its own closure body on Call,
// never here at wiring time: the task worktree does not exist until WorktreeCreate has already run,
// so resolving any of its paths any earlier would resolve a path that is not there yet. That applies
// to the status-path pair, the spawn directory, and both teardown halves alike -- not only to the one
// whose laziness (ResolveStatus's own signature) makes it visible.

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
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
)

// spawnPairPush launches the detached push of a task pair; tests replace it, since a detached push forks the running binary.
var spawnPairPush = fabricengine.SpawnPairPush

// driverAliveFrom answers whether the child's driver strand is live, over an injected status reader so its answers are testable without tmux.
// An absent task worktree is false without reading status, and so is an absent reed session -- no session means no driver -- while every other status error is returned unchanged.
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

// driverStrandFrom reports the child's driver strand from reed's sessionless directory, over an injected reader so its answers are testable without tmux.
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

// arbiterInFlightFrom reads the child's in-flight arbiter marker through readMarker and reports its deadline, with Alive true only when a live row of the pair's reed directory carries the marker's strand guid.
// An absent task worktree and an absent marker are not found; a marker read error and a directory read error are returned unchanged.
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

// taskWorktreeLocation resolves the managed task worktree's own *lyxcwd.Location, for slug, from
// the prime *lyxcwd.Location. It is the shared body every lazily-resolved seam below calls, so a
// caller reading this file only once still sees every "resolved lazily" claim in one place.
//
// It joins the task worktree's root via fabricengine.WorktreePath -- the topology package's own
// sibling-path helper -- and resolves that root through lyxcwd.ResolveWorktree, which applies no
// cwd gate: the caller here holds a worktree root, not an acting cwd, so the gate would spuriously
// fire.
//
// An absent pair is refused by name rather than left to the resolver's generic "not a git
// repository": batten's status is durable, so a run resumed on another machine reaches every row
// past Worktree-Create with no pair here.
// Recreating it is not attempted, since fabric's Add refuses a pre-existing branch by design -- and
// the refusal below names no fabric command as a substitute: "lyx fabric checkout" switches the
// CALLER's own worktree onto the named branch, so run from prime, as every batten verb must be, it
// would mutate prime itself rather than restore anything.
// Deleting the pair's branches alone rewinds nothing: the resumed run re-enters its persisted row,
// not Worktree-Create, so the abandon path names batten's own run directory as well, and both
// branch copies, since the ones on the remote make a fresh create's push refuse.
func taskWorktreeLocation(prime *lyxcwd.Location, slug string) (*lyxcwd.Location, error) {
	worktreePath, err := presentTaskWorktreePath(prime, slug)
	if err != nil {
		return nil, err
	}
	return lyxcwd.ResolveWorktree(worktreePath)
}

// presentTaskWorktreePath returns the task worktree's root for slug, refusing an absent one by name (see taskWorktreeLocation).
func presentTaskWorktreePath(prime *lyxcwd.Location, slug string) (string, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf(
				"battencli: the task worktree for %q is not present at %s; this run's durable status says it was already created, so it is either on another machine or was removed by hand -- batten does not recreate a pair from its branch, and no \"lyx fabric\" command currently does either (creating one refuses when its branch already exists). Resolve it by hand, one of two ways: restore the worktree pair yourself from its branches, outside lyx's own automation, which keeps the task's work, then resume this run; or abandon this run by deleting its run directory %s (a change on the pair's fabric sibling) and the pair's branches, local and remote, after which \"lyx batten run %s\" starts over from a fresh create -- discarding any of the task's work not already merged",
				slug, worktreePath, shedrun.RunDir(prime, slug), slug,
			)
		}
		return "", err
	}
	return worktreePath, nil
}

// taskWorktreeLocator returns a taskWorktreeLocation for slug that spawns git only until a resolution succeeds.
// Run-Shed's wait reads the task worktree's files every poll check, and resolving the location runs git;
// a present pair's location never changes, so the first success is kept, while every call still stats the pair and refuses an absent one by name.
func taskWorktreeLocator(prime *lyxcwd.Location, slug string) func() (*lyxcwd.Location, error) {
	var resolved *lyxcwd.Location
	return func() (*lyxcwd.Location, error) {
		if resolved == nil {
			taskLocation, err := taskWorktreeLocation(prime, slug)
			if err != nil {
				return nil, err
			}
			resolved = taskLocation
			return resolved, nil
		}
		if _, err := presentTaskWorktreePath(prime, slug); err != nil {
			return nil, err
		}
		return resolved, nil
	}
}

// taskWorktreePresent reports whether the task worktree for slug is already on disk under prime's
// hub and resolvable as a worktree of its own.
//
// DriverAlive and taskWorktreeComplete use it;
// Worktree-Teardown does not, since the pair-teardown composite decides for itself whether the task worktree is gone.
//
// Worktree-Create uses the stronger taskWorktreeComplete instead, not this function: a bare
// directory check cannot tell a pair Add finished from one a SIGKILL interrupted partway through
// (see taskWorktreeComplete's own doc comment).
//
// It answers false rather than an error when the worktree is absent, so a genuinely absent one
// still reaches Topology.Add or Topology.Remove and every real failure keeps its Stuck disposition.
// A stat error that is not "absent", and a path that is there but does not resolve as a worktree,
// are both returned: neither is an answer to the question asked.
func taskWorktreePresent(prime *lyxcwd.Location, slug string) (bool, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := lyxcwd.ResolveWorktree(worktreePath); err != nil {
		return false, err
	}
	return true, nil
}

// taskWorktreeComplete reports whether the task worktree for slug is not just present but fully
// materialised: Topology.Add's own full post-condition (fabricengine.PairComplete), which
// taskWorktreePresent's bare directory check cannot tell from a pair Add left half-built.
//
// Worktree-Create's post-condition is "the task worktree for this slug exists", so a row re-entered
// against an already-satisfied post-condition must report success rather than asking fabric to
// create the same worktree twice. The window is wider than "Add returned but the transition was not
// yet persisted": Add is a multi-step transaction whose own in-process rollback runs only when Add
// itself observes an error, never when the process running it is killed instead -- a bare "the
// worktree directory resolves" check is satisfied by that transaction's very first step alone.
//
// present is false only when the worktree does not exist at all -- the genuinely-absent case, which
// still reaches Topology.Add. incompleteReason is non-empty only when the worktree exists but is not
// yet a complete pair, worded for this package's own refusal text rather than repeating
// PairComplete's fabric-vocabulary reason verbatim.
func taskWorktreeComplete(prime *lyxcwd.Location, slug string) (present, complete bool, incompleteReason string, err error) {
	present, err = taskWorktreePresent(prime, slug)
	if err != nil || !present {
		return present, false, "", err
	}
	taskLocation, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
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

// incompletePairRemedy names the manual cleanup for a pair taskWorktreeComplete found incomplete.
// "lyx fabric remove --force" run from prime removes whatever part of the pair Add got to -- the task worktree, its sibling, their junctions, launcher entries, and the sibling's branch -- in one command, which no pair of plain git commands can do from here: the sibling is a worktree of another repository.
// Remove also deletes the task branch when its work is already on another ref -- always so for a create interrupted before any work -- and keeps it otherwise, so the branch deletion is named as conditional on the branch surviving the removal: an unconditional "git branch -D" fails on the branch Remove already deleted.
// The task branch is named with fabric's branch prefix, since Add refuses a leftover one.
func incompletePairRemedy(slug, branch string) string {
	return fmt.Sprintf("remove it by hand from here (\"lyx fabric remove --force %s\"), then, only if its branch %s is still present afterwards, delete that branch too (\"git branch -D %s\")", slug, branch, branch)
}

// reconcileTaskPair reconciles and commits the config of the task pair for slug, whatever the hub's build stamp says.
// The pair forked the prime's records branch, which an earlier walk may have skipped as mid-merge or another holder's walk may not have committed yet.
// A repository outside a hub has nothing to reconcile.
func reconcileTaskPair(prime *lyxcwd.Location, slug string) error {
	geometry, inHub := hubgeom.ReconcileGeometry(prime)
	if !inHub {
		return nil
	}
	return hubreconcile.Ensure(geometry, hubreconcile.Options{Pair: fabricengine.WorktreePath(prime, slug)})
}

// createRefusal maps the create refusals no tackler can settle to a *battenshed.PreconditionError, and passes every other create error through verbatim.
// A tracked-dirty source worktree is settled by committing or reverting in it.
// A task branch left from an earlier pair of the slug, locally or on origin, is settled by the cancel verb, which archives and deletes it.
// fabric words that leftover refusal for a caller inside a pair ("lyx fabric checkout"), which from prime would switch prime's own pair onto the task's branches, so its text is replaced.
// A remote leftover on any other branch, the records side included, passes through.
func createRefusal(err error, slug, taskBranch string) error {
	leftoverWayForward := fmt.Sprintf(`run "lyx batten cancel %s --remove" from the prime, which archives and deletes it, then "lyx batten run %s"`, slug, slug)

	var sourceDirty *fabricengine.ErrSourceDirty
	if errors.As(err, &sourceDirty) {
		return &battenshed.PreconditionError{
			Unmet:      "the source worktree has uncommitted tracked changes: " + strings.Join(sourceDirty.Paths, ", "),
			WayForward: fmt.Sprintf(`commit or revert them in the source worktree, then run "lyx batten run %s" again`, slug),
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
	return err
}

// finishRemoval turns the composite's RemovePair outcome into the Worktree-Teardown row's verdict.
// A pair of which nothing remains is the row's done post-condition, since shedengine persists the transition only after the producer returns and a process killed right after the removal re-enters the row.
// Remove reports a failed remote deletion without failing; it is returned here so the row halts resumable, and the resumed row retries the deletion.
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
		return fmt.Errorf("the pair for %q was removed but its other-side branch's remote copy was not deleted: %s; resume this run to retry the deletion", slug, res.RemoteBranchError)
	}
	return nil
}

// teardownRequest is the composite's request for a batten-driven teardown.
// Remote is true: a batten-driven teardown is the task's own final removal, never a step toward re-adopting the pair, so nothing will ever need the pair's other-side branch again, and a remote copy left behind makes a later create of this slug refuse its push.
// It never forces, and it neither waits for a quiet driver nor refuses a busy one, since the InnerRun done arm already waited its driver_exit_grace_s.
// Landed is true: the row runs only after a done child, so the task's board entry is marked done once the pair is removed.
func teardownRequest(slug string) pairteardown.Request {
	return pairteardown.Request{Slug: slug, Force: false, Remote: true, QuietWait: 0, RefuseWhenBusy: false, Landed: true}
}

// teardownRefusal rewords the teardown refusal for content in the pair's sibling the teardown could not commit, names the resume for a failed archive of the run records, and passes every other error through unchanged.
//
// fabric's own refusal for a dirty pair sibling leaves --force as the way out, and --force would discard exactly the records this task exists to keep.
// The teardown commits the run records itself, so this refusal means content outside the record paths in the pair's sibling worktree under the hub;
// the remedy names inspecting it, committing or removing it by hand, and resuming.
// A push of the pair still running past the teardown's wait is refused before any mutation too, and the remedy names the resume once the push finishes.
// A failed archive runs before any teardown mutation, so the pair is still whole and a plain resume retries it once the failure is fixed.
// That failure is most often an unreachable remote, but not always, so the remedy points at the wrapped cause rather than naming one.
func teardownRefusal(err error, slug string) error {
	if errors.Is(err, fabricengine.ErrArchiveFailed) {
		return fmt.Errorf("the pair for %q was left in place because archiving its run records to the remote failed: %w; fix the failure named here (most often an unreachable remote), then resume this run with \"lyx batten run %s\"", slug, err, slug)
	}
	if errors.Is(err, fabricengine.ErrPairPushRunning) {
		return fmt.Errorf("the pair for %q was left in place because a push of it is still running; once it finishes, resume this run with \"lyx batten run %s\": %w", slug, slug, err)
	}
	if !errors.Is(err, fabricengine.ErrPairSiblingDirty) {
		return err
	}
	return siblingDirtyRefusal{
		msg: fmt.Sprintf("the pair for %q was left in place because its sibling worktree under the hub holds content outside the run record paths, which the teardown cannot commit; inspect that sibling worktree, commit or remove that content by hand, then resume this run with \"lyx batten run %s\"", slug, slug),
		err: err,
	}
}

// siblingDirtyRefusal carries batten's own remedy text and unwraps to fabric's refusal without repeating its text, which names --force.
type siblingDirtyRefusal struct {
	msg string
	err error
}

func (r siblingDirtyRefusal) Error() string { return r.msg }

func (r siblingDirtyRefusal) Unwrap() error { return r.err }

// maxChildOutputInError caps the child output folded into a spawn error, so a misbehaving child
// cannot push an unbounded string into a status file committed onto prime's own pair.
const maxChildOutputInError = 2000

// battenProgram is the program name batten signs its notices to the orch with.
const battenProgram = "BATTEN"

// childSpawnError folds a failed child bootstrap's own output into runErr, so the returned error
// carries the child's diagnosis rather than only its exit status.
// It returns nil for a nil runErr, runErr unchanged when the child said nothing, and truncates
// output past maxChildOutputInError with an explicit marker.
// An output carrying the not-parked refusal (kind shedrun.StartNotParkedKind) also wraps battenshed.ErrChildNotParked, and one carrying the busy-pane refusal (kind shedrun.StartDriverBusyKind) also wraps battenshed.ErrChildDriverBusy, so InnerRun retries rather than failing.
// One carrying a held-run-lock refusal (kind shedengine.ShedBusyKind) also wraps battenshed.ErrChildRunBusy.
// One carrying the owned-elsewhere refusal (kind shedrun.StartOwnedElsewhereKind) also wraps a *battenshed.ChildOwnedElsewhereError naming the owner the refusal carries, so InnerRun pauses instead of failing the spawn.
//
// The byte slice lands on a rune boundary only by chance, so strings.ToValidUTF8 drops any partial
// rune it leaves dangling at the cut point rather than embedding invalid UTF-8 into the error text.
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

// childRefusalOwner returns the "owner" of the first line of a child bootstrap's output that is a refusal envelope carrying one, or "" when none is.
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

// ownerVerdictOf maps a run-scope origin report onto the owner verdict InnerRun reads.
// An unreachable report is Unreachable, the slug's owned-elsewhere entry is Elsewhere with its owner, and anything else, a report without the slug included, is owned here.
func ownerVerdictOf(report fabricengine.OriginReport, slug string) battenshed.OwnerVerdict {
	if report.Unreachable {
		return battenshed.OwnerVerdict{Unreachable: true}
	}
	for _, entry := range report.Entries {
		if entry.Slug == slug && entry.State == fabricengine.EntryOwnedElsewhere {
			return battenshed.OwnerVerdict{Owner: entry.Owner, Elsewhere: true}
		}
	}
	return battenshed.OwnerVerdict{}
}

// childRefusalKind returns the "kind" of the first line of a child bootstrap's output that is a refusal envelope carrying one, or "" when none is.
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

// boardRecipe returns the recipe Seed-Child seeds for task: beam for a project entry, the entry's own recipe field otherwise.
func boardRecipe(task boardengine.Task) string {
	if task.Kind == boardengine.KindProject {
		return shedrun.RecipeBeam
	}
	return task.Recipe
}

// childRecipeSupported reports whether recipe is one a task worktree's bootstrap verb can run as the child's own run:
// the three recipes `lyx loom start` serves.
func childRecipeSupported(recipe string) bool {
	return recipe == shedrun.RecipeLoom || recipe == shedrun.RecipeDarn || recipe == shedrun.RecipeBeam
}

// unsupportedChildRecipeRefusal is the refusal for a recipe childRecipeSupported rejects.
// It wraps battenshed.ErrUnsupportedChildRecipe and names the three admitted recipes.
func unsupportedChildRecipeRefusal(recipe string) error {
	return fmt.Errorf("%w: only %q, %q and %q have a bootstrap verb Run-Shed can start in the task worktree, so a Board task's type must be %q, %q or empty, and a project entry seeds %q; got %q", battenshed.ErrUnsupportedChildRecipe, shedrun.RecipeLoom, shedrun.RecipeDarn, shedrun.RecipeBeam, shedrun.RecipeLoom, shedrun.RecipeDarn, shedrun.RecipeBeam, recipe)
}

// childSeedParams returns the seed params recipe's own bootstrap verb will itself write, read from
// childLocation.
//
// shedrun.WriteSeed is idempotent only against a seed agreeing on recipe, driver and params, so a
// child seeded without a param its own bootstrap writes makes that bootstrap refuse.
// Params are per-recipe: loom, darn and beam declare one each, params.parent, taken from the pair's recorded origin, which for a beam is the beam's parent branch.
// An absent or empty recorded parent yields no param, leaving the bootstrap's own "pass --parent once" refusal as the one an operator sees.
func childSeedParams(recipe string, childLocation *lyxcwd.Location) (map[string]string, error) {
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

// childSeedFor builds the seed written into the task worktree: the given recipe, driver and params.
// It records no parent: the pair's parent is resolved from fabric's origin record at use.
func childSeedFor(recipe, driver string, params map[string]string) shedrun.Seed {
	return shedrun.Seed{Recipe: recipe, Driver: driver, Params: params}
}

// noticesReachDriversParent reports whether this batten's notices have a destination that is also the task worktree driver's parent, and that destination records an orch strand.
// The task worktree's driver messages the nearest live orch from the pair's recorded parent, and batten's notices take the walk from the same pair, so the two destinations agree whenever the pair is present;
// a driver and batten under a live beam orch both reach it.
func noticesReachDriversParent(prime, taskLocation *lyxcwd.Location) (bool, error) {
	return runParentRecorded(prime, taskLocation.WorktreeName)
}

// battenHistoryLen returns the length of batten's own status history for slug, 0 when the status file is absent.
func battenHistoryLen(location *lyxcwd.Location, slug string) (int, error) {
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
	if err != nil || !found {
		return 0, err
	}
	return len(status.History), nil
}

// rowNotices returns the notice seams of the rows after Run-Shed: notices go to the nearest live orch above the run, as Run-Shed's do, and an episode is told apart by batten's own history length.
func rowNotices(location *lyxcwd.Location, slug string) battenshed.RowNoticeDeps {
	return battenshed.RowNoticeDeps{
		Notify: func(ctx context.Context, line string) (bool, error) {
			return notifyRunParent(location, slug, line)
		},
		OrchStrandRecorded: func() (bool, error) { return runParentRecorded(location, slug) },
		BattenHistoryLen:   func() (int, error) { return battenHistoryLen(location, slug) },
	}
}

// readStopReport reads the driver park marker of the task worktree's run, whose content is the path of the stop report the driver parked on, and the time the marker was written.
// An absent marker reports found == false.
func readStopReport(taskLocation *lyxcwd.Location) (path string, at time.Time, found bool, err error) {
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

// readAgentActivity reads the live agent runs of the task worktree through the shuttle config and its registered engines, mapped onto battenshed's neutral type,
// and reports whether a Go-side wait is live: a running verify, a parent lock wait, or a shuttle wait that is not a held turn end.
// A held run is idle at its turn end, so it does not suppress the quiet notice, and neither does a verify held by a pause.
// A lock wait record whose writer is dead is ignored.
// The waits are read from their marker and record files, never from the pane options that display them.
func readAgentActivity(taskLocation *lyxcwd.Location) (runs []battenshed.AgentActivity, waitLive bool, err error) {
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
		run := battenshed.AgentActivity{Producer: reading.StrandName, LastActivity: reading.LastActivity, APIError: reading.APIError, APIErrorText: reading.APIErrorText}
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

// readSessionStatesByStrand reads the session state of every running shuttle run under anchor, keyed by strand name.
// The states are only logged beside batten's notices, so a failed read is logged at Warn once and yields none.
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

// markBattenWatched writes the batten-watched marker of the task worktree's run while noticesReachDriversParent holds, and removes it otherwise.
// The marker holds this process's pid and names the batten scratch directory and the Run-Shed row's name, so a Go driver can tell whether this batten has died.
// It reports whether the marker is held.
func markBattenWatched(prime, taskLocation *lyxcwd.Location, scratchDir, producer string, pid int) (bool, error) {
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
func dobbyReading(prime *lyxcwd.Location, slug string) (pid int, sleeps []pauserecord.Interval, err error) {
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

// dobbyStateReading is dobbyReading over the state record at statePath and a liveness probe of its dobby.
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

// awakeReading is the clock every pass shares: the wall time since the Unix epoch less the machine sleeps the scheduling dobby recorded.
// It stops while the machine sleeps and holds one origin across processes, so the idle watches the memory record restores keep measuring awake time.
func awakeReading(now time.Time, sleeps []pauserecord.Interval) time.Duration {
	return time.Duration(now.UnixNano()) - pauserecord.Excluded(sleeps, time.Unix(0, 0), now)
}

// syncWatchedMarker makes the marker at path match watched: absent when not watched, and holding content when watched.
// A marker already holding content is not rewritten, an absent marker is fine to remove, and a write is atomic so a driver's render never reads a torn marker.
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
// Both sides join onto the task worktree, since the records side reaches it through the `_lyx` junction.
func dirtyPathsOutsideRunRecords(taskLocation *lyxcwd.Location) ([]string, error) {
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
// Each command's output goes to a command-<row>.log under the run's scratch directory.
func (c *battenCLI) commandDeps(location *lyxcwd.Location, slug, executable string, locateTask func() (*lyxcwd.Location, error)) shedadapters.CommandDeps {
	return shedadapters.CommandDeps{
		Lyx:  executable,
		Slug: slug,
		TaskDir: func() (string, error) {
			taskLocation, err := locateTask()
			if err != nil {
				return "", err
			}
			return taskLocation.AnchorPath(), nil
		},
		Dir: location.WorktreePath(),
		LogPath: func(row string) string {
			return filepath.Join(shedrun.ScratchDir(location, slug), "command-"+row+".log")
		},
		Run:   runRowCommand,
		Rerun: fmt.Sprintf("lyx batten run %s", slug),
	}
}

// wire builds and stores the shedrecipe.Env and shedbuild.ShedPaths the run and status verbs
// need, over the resolved prime location and slug.
func (c *battenCLI) wire(location *lyxcwd.Location, slug string) error {
	// Batten runs in the prime on behalf of one pair, so its network git and GitHub calls wait under that pair's pause entry.
	pauserecord.SetGateSlug(slug)
	primeRunLockPath := PrimeRunLock(location)
	primeLock := battenshed.PrimeLock{
		Path: primeRunLockPath,
		Acquire: func() (release func() error, ok bool, err error) {
			// The prime lock's own directory first -- it is the one being locked in -- then the
			// per-slug scratch directory, where the producers holding this lock write their
			// stuck-reason files. The two shedrun constructors state no relationship, so neither
			// may be left to the other's MkdirAll.
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

	// The on-disk lyx path is resolved once, here: after a deploy replaces the file under this running batten, os.Executable reads a path that no longer names it.
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("batten: resolve the lyx executable: %w", err)
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
		CreateWorktree: func(ctx context.Context) error {
			// The claim comes first, so the run lock on the board entry starts before the pair exists and survives this row failing.
			// It records this machine as the entry's owner in the same commit, so a failed write or push refuses the row and the stopped run resumes with the same verb.
			if err := c.claimBoardEntry(slug); err != nil {
				return err
			}

			// A beam's landing forks no pair, and work started under the beam after the start guard read it would be lost with the pair, so the refusal runs again after the claim.
			project, err := c.entryIsProject()
			if err != nil {
				return err
			}
			if project && c.beam.Refusal != nil {
				if err := c.beam.Refusal(); err != nil {
					return err
				}
			}

			// The already-complete probe before Add, so the row is idempotent against its own
			// post-condition -- see taskWorktreeComplete for the crash window this closes.
			present, complete, incompleteReason, err := taskWorktreeComplete(location, slug)
			if err != nil {
				return err
			}
			if complete {
				logger.Info("battencli: create worktree skipped, the task worktree is already present", "slug", slug)
				return reconcileTaskPair(location, slug)
			}
			if project && !present {
				// A beam is made by `lyx beam create`, never by a landing, so no path forks an empty pair for it.
				return &battenshed.PreconditionError{
					Unmet:      fmt.Sprintf("the beam %s has no pair on this machine", slug),
					WayForward: fmt.Sprintf("run %q to finish the create, or %q when the beam's branch is on origin, then %q", "lyx beam create "+slug, "lyx adopt "+slug, "lyx batten run "+slug),
				}
			}
			cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return err
			}
			if present {
				// A worktree exists but Add never finished it -- see taskWorktreeComplete's own doc
				// comment. Add refuses an existing worktree directory the same way it refuses a
				// leftover branch, so the remedy is a cleanup followed by a resume.
				return fmt.Errorf(
					"the task worktree for %q exists but %s; %s, then resume this run to create it fresh",
					slug, incompleteReason, incompletePairRemedy(slug, cfg.BranchPrefix+slug),
				)
			}

			top := fabricengine.NewTopology(cfg)
			// A member of a beam forks from the beam's pair, whichever worktree started the run; every other task forks from the prime.
			parent, err := beamForkParent(location, slug, func(beam string) (bool, error) { return top.TaskBranchOnOrigin(location, beam) })
			if err != nil {
				return err
			}
			_, err = top.Add(location, slug, fabricengine.AddOptions{Parent: parent})
			logger.Info("battencli: create worktree", "slug", slug)
			if err != nil {
				return createRefusal(err, slug, cfg.BranchPrefix+slug)
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
		// Both teardown halves go through the pair-teardown composite and are idempotent against their shared post-condition, "the pair is gone".
		// shedengine persists the row's transition only after the producer returns, so a process killed right after Remove succeeded re-enters this row with no pair on disk.
		// EndSession's refusal probe then reports ErrPairNotFound, which Shutdown takes as nothing left to shut down, and RemovePair reports it again as done.
		// The composite finishes a half-removed pair itself -- sibling worktree, launcher entries, both branches -- so no such state is refused by name here.
		// EndSession ends the session by name when the task worktree is gone, and runs the refusal probe, so a refusal reaches the row before Remove and leaves the pair in place.
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
			// OrchStrandRecorded reads whether that orch records a strand, the same fact the watched marker is kept on.
			OrchStrandRecorded: func() (bool, error) { return runParentRecorded(location, slug) },
			// StopReport reads the driver's park marker in the task worktree: the stop report path it holds and the time it was written.
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
			// Sleeps reads the machine sleeps the parent worktree's dobby recorded.
			Sleeps: func() ([]pauserecord.Interval, error) {
				_, sleeps, err := dobbyReading(location, slug)
				return sleeps, err
			},
			// Awake is the wall clock less those sleeps, so every pass measures awake time from the same origin.
			// An unreadable record excludes none, as the Sleeps seam's own error does.
			Awake: func() time.Duration {
				_, sleeps, err := dobbyReading(location, slug)
				if err != nil {
					logger.Debug("battencli: the dobby's recorded sleeps are unreadable; the awake clock excludes none", "slug", slug, "error", err)
				}
				return awakeReading(time.Now(), sleeps)
			},
			// ReviveAPIError gives the pair's sessions stalled on an API error tender's revive line.
			ReviveAPIError: func(ctx context.Context) error {
				return tendercli.ReviveStalled(location, slug)
			},
			ParentLock: childParentLock(slug, locateTask, fabricengine.BoardDir(location.HubPath), lockIdleBound(location)),
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
			// HandBack marks a blocked landing halt or decision halt over the child status's current producer and error, so the binary re-step leaves it to the orch.
			HandBack: func(status shedengine.Status) bool {
				return status.State == shedengine.StateBlocked && arbiterstop.HandsBack(status.CurrentProducer, status.Error)
			},
			// PauseRequested reads batten's own status file, the one `lyx batten pause` writes, so the in-call wait notices a pause within one check.
			PauseRequested: pauseRequested(location, slug),
			// Pause reads the hub's pause record at the prime's geometry, for the slug this run drives.
			Pause: func() (pauserecord.Coverage, error) {
				geometry, err := hubgeom.PauseGeometry(location)
				if err != nil {
					return pauserecord.Coverage{}, err
				}
				return pauserecord.ReadCoverage(geometry.RecordPath, slug, time.Now())
			},
			// Activity resolves the task worktree on Call like every seam here.
			Activity: func(context.Context) ([]battenshed.AgentActivity, bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return nil, false, err
				}
				return readAgentActivity(taskLocation)
			},
			PairDir: func() (string, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return "", err
				}
				return taskLocation.AnchorPath(), nil
			},
			// ReadDecision and DriverAlive resolve the task worktree on Call like every seam here, never at wiring time.
			// No verb writes an approval or rejection record any more, so an awaiting child is never resumed on an operator decision.
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				return battenshed.ChildDecision{}, false, nil
			},
			DriverAlive: func(ctx context.Context) (bool, error) {
				present, err := taskWorktreePresent(location, slug)
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
			// TacklerInFlight reads the task worktree's in-flight tackler marker as the arbiter's is read: against the same reed directory, so a live row carrying the tackler's strand reads alive.
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
			// ReviveStrands ends the pair's dead arbiter strands, then resumes the task worktree's reed session, which relaunches the dead driver strand and every other non-live strand of the pair.
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
			// ChildDriverLockHeld probes the Go driver's lock, which the driver holds for life while the run lock is free between its steps.
			ChildDriverLockHeld: func() (bool, error) {
				taskLocation, err := locateTask()
				if err != nil {
					return false, err
				}
				return runLockHeld(shedrun.DriverLock(taskLocation, shedrun.SelfRunID))
			},
			// ChildBuildChanged reads the lyx on disk, which a deploy replaces under this running batten, and compares it with the build the child's last step recorded.
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
			// ResolveStatus also creates the child's ephemeral status-lock directory, since its
			// caller reads through that lock next and nothing else on the Run-Shed path creates
			// it: the child's status file is durable while its lock is not. This mirrors
			// battenPreRun's own MkdirAll for prime's status lock (arm.go).
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
				// The bootstrap verb of loom and darn alike, not a per-recipe dispatch: the WriteSeed seam below admits
				// no other child recipe, so the two stay consistent by construction.
				// Dir is the task worktree's AnchorPath(), never its bare worktree root: the resolver
				// gates a child's working directory to the anchor, and a bare root fails on any
				// subpath-anchored hub.
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
				return state.ReadJSONNoMkdir[shedengine.Status](statusPath, statusLockPath)
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			// ReadBoardType opens the Board fresh on every Call, over fabricengine.BoardDir(location.HubPath), and returns the recipe boardRecipe answers for the entry -- never a value captured at wiring time, so a recipe corrected after prime was seeded is still honoured.
			ReadBoardType: func(ctx context.Context) (string, error) {
				b := boardengine.New(boardengine.Config{Path: fabricengine.BoardDir(location.HubPath)})
				task, found, err := b.GetTask(slug)
				if err != nil {
					return "", err
				}
				if !found {
					return "", fmt.Errorf("battencli: board task %q not found", slug)
				}
				return boardRecipe(task), nil
			},
			// ChildDriver is the "child_driver" param of prime's own seed when the operator typed one, otherwise the child recipe's resolved driver_kind (childDriverFrom).
			// The kind is read from the task worktree's config layers, and SeedChild calls this after writing that worktree's local layer, so a driver_kind from the run settings is seen.
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
			// ReadRunSettings opens the Board fresh on every Call, as ReadBoardType does, and returns the task's profile and run_settings.
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
			// PrepareRunSettings reads the named profile from the hub's profiles.yaml and lays the task's settings over it through configreg;
			// an unknown profile or a refusal of the settings is wrapped in battenshed.ErrInvalidRunSettings so Seed-Child routes it to Stuck.
			PrepareRunSettings: func(settings battenshed.RunSettings) (map[string][]byte, error) {
				return prepareRunSettings(configengine.Dirs{Board: fabricengine.BoardDir(location.HubPath)}, settings)
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
			// WriteSeed is the only place in the batten path that encodes a seed, per the
			// seed-encoding-stays-behind-a-seam-in-battenshed Shared Decision. It validates recipe
			// itself, wrapping an unknown name in battenshed.ErrUnknownRecipe and a known but
			// unbootstrappable one in battenshed.ErrUnsupportedChildRecipe; a shedrun.WriteSeed
			// failure wrapping shedrun.ErrDisagreeingSeed (a pre-existing child seed that disagrees
			// with the one being written) is re-wrapped in battenshed.ErrDisagreeingChildSeed, this
			// package's own spelling -- so seedChildProducer's own errors.Is checks route all three
			// to Stuck rather than a hard error.
			//
			// Params come from childSeedParams: a seed missing one the child's own bootstrap
			// writes is not a smaller seed, it is a seed that bootstrap refuses.
			WriteSeed: func(ctx context.Context, recipe, driver string) error {
				if err := shedrun.ValidateRecipe(recipe); err != nil {
					return fmt.Errorf("%w: %s", battenshed.ErrUnknownRecipe, err.Error())
				}
				// Refused here, ahead of any write, because Spawn above runs loom's bootstrap verb
				// unconditionally: a child seeded with any other recipe would carry a committed,
				// pushed seed that its own bootstrap then refuses as a disagreement, reported from
				// inside the child as a shedrun fault rather than as this choice.
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
				return nil
			},
			// CommitSeed commits the child's own seed onto the child's own fabric pair -- a
			// one-off write, distinct from CommitStatus below, which commits prime's own batten
			// status onto prime's own pair on every non-no-op transition.
			CommitSeed: func(ctx context.Context) error {
				childLocation, err := locateTask()
				if err != nil {
					return err
				}
				_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation, []string{shedrun.SeedRel(childLocation, shedrun.SelfRunID)}, fmt.Sprintf("batten: seed child %s", slug), fabricengine.EnvSyncOptions())
				return err
			},
			MarkWatched: markWatched,
			// PushSeed spawns the detached push of the child's own fabric pair, the same location CommitSeed just
			// committed onto, and never waits on it: the child reads its seed locally.
			PushSeed: func(ctx context.Context) error {
				childLocation, err := locateTask()
				if err != nil {
					return err
				}
				return spawnPairPush(childLocation)
			},
		},
	}

	// Each setting is read when its row is reached, over the task worktree located then, so a task's run_settings in that worktree's local layer switch a row per task.
	env.Setting = func(module, key string) (any, error) {
		taskLocation, err := locateTask()
		if err != nil {
			return nil, err
		}
		return configreg.SettingLookup(fabricengine.ConfigDirs(taskLocation))(module, key)
	}
	// A Command row's command is read from the template and the board default layer only, the layers deploy and darn's verify are read from.
	env.ExecutableSetting = configreg.SettingLookup(configengine.Dirs{Board: fabricengine.ConfigDirs(location).Board})
	// A recipe row's entry gate checks its setting operands at build, which needs no task worktree, and names the resume command in its evaluation errors.
	env.SettingKnown = configreg.CheckSettingKey
	env.Resume = fmt.Sprintf("lyx batten run %s", slug)
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
		// CommitStatus is now batten's own seam, committing prime's own batten status onto prime's
		// own pair on every non-no-op transition: the status file is durable, fabric-synced state
		// now (see paths.go), so nil is no longer right here.
		CommitStatus: battenCommitStatusSeam(location, slug),

		RunID:                   slug,
		MissingStatusWayForward: fmt.Sprintf("way forward: \"lyx batten run %s\" creates the lifecycle's status file", slug),
		PauseWayForward:         fmt.Sprintf("lyx batten pause %s", slug),
		BudgetWayForward:        fmt.Sprintf("run \"lyx batten run %s\": the resume grants the row one more budget", slug),
	}
	return nil
}
