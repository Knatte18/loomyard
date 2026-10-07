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
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

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
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"battencli: the task worktree for %q is not present at %s; this run's durable status says it was already created, so it is either on another machine or was removed by hand -- batten does not recreate a pair from its branch, and no \"lyx fabric\" command currently does either (creating one refuses when its branch already exists). Resolve it by hand, one of two ways: restore the worktree pair yourself from its branches, outside lyx's own automation, which keeps the task's work, then resume this run; or abandon this run by deleting its run directory %s (a change on the pair's fabric sibling) and the pair's branches, local and remote, after which \"lyx batten run %s\" starts over from a fresh create -- discarding any of the task's work not already merged",
				slug, worktreePath, shedrun.RunDir(prime, slug), slug,
			)
		}
		return nil, err
	}
	return lyxcwd.ResolveWorktree(worktreePath)
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
// "lyx fabric remove --force" run from prime removes whatever part of the pair Add got to -- the task worktree, its sibling, their junctions, portal and launcher entries, and the sibling's branch -- in one command, which no pair of plain git commands can do from here: the sibling is a worktree of another repository.
// Remove also deletes the task branch when its work is already on another ref -- always so for a create interrupted before any work -- and keeps it otherwise, so the branch deletion is named as conditional on the branch surviving the removal: an unconditional "git branch -D" fails on the branch Remove already deleted.
// The task branch is named with fabric's branch prefix, since Add refuses a leftover one.
func incompletePairRemedy(slug, branch string) string {
	return fmt.Sprintf("remove it by hand from here (\"lyx fabric remove --force %s\"), then, only if its branch %s is still present afterwards, delete that branch too (\"git branch -D %s\")", slug, branch, branch)
}

// createRefusal rewords the one create refusal whose fabric remedy is wrong from prime, and passes
// every other create error through verbatim (fabric's own refusals otherwise name remedies that hold
// from here, and rewording them would only drop information).
//
// fabric words its leftover-branch refusal for a caller inside a pair: "switch a pair onto it with
// lyx fabric checkout". Every batten verb runs from prime, where that command switches prime's own
// pair onto the task's branches. The branch is a leftover in the everyday flow -- a torn-down pair
// keeps its task branch locally and on the remote, and a rolled-back create keeps the branch it
// made -- so the remedy named here is the abandon path the absent-worktree refusal already names:
// delete the leftover locally and on the remote, and any orphaned other-side branch, then resume.
func createRefusal(err error) error {
	var branchExists *fabricengine.ErrBranchExists
	if !errors.As(err, &branchExists) {
		return err
	}
	return fmt.Errorf(
		"branch %q already exists, left behind by an earlier pair for this slug (a torn-down pair keeps its task branch locally and on the remote, and a rolled-back create keeps the branch it made); delete it locally (\"git branch -D %s\") and on the remote (\"git push origin --delete %s\"), remove any leftover of the pair's sibling branch (\"lyx fabric cleanup --apply --remote\" removes an orphaned one), then resume this run -- never \"lyx fabric checkout\" from here, which would switch this worktree itself onto that branch",
		branchExists.Branch, branchExists.Branch, branchExists.Branch,
	)
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
func teardownRequest(slug string) pairteardown.Request {
	return pairteardown.Request{Slug: slug, Force: false, Remote: true, QuietWait: 0, RefuseWhenBusy: false}
}

// teardownRefusal rewords the teardown refusal for content in the pair's sibling the teardown could not commit, names the resume for a failed archive of the run records, and passes every other error through unchanged.
//
// fabric's own refusal for a dirty pair sibling leaves --force as the way out, and --force would discard exactly the records this task exists to keep.
// The teardown commits the run records itself, so this refusal means content outside the record paths in the pair's sibling worktree under the hub;
// the remedy names inspecting it, committing or removing it by hand, and resuming.
// A failed archive runs before any teardown mutation, so the pair is still whole and a plain resume retries it once the failure is fixed.
// That failure is most often an unreachable remote, but not always, so the remedy points at the wrapped cause rather than naming one.
func teardownRefusal(err error, slug string) error {
	if errors.Is(err, fabricengine.ErrArchiveFailed) {
		return fmt.Errorf("the pair for %q was left in place because archiving its run records to the remote failed: %w; fix the failure named here (most often an unreachable remote), then resume this run with \"lyx batten run %s\"", slug, err, slug)
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

// childSpawnError folds a failed child bootstrap's own output into runErr, so the returned error
// carries the child's diagnosis rather than only its exit status.
// It returns nil for a nil runErr, runErr unchanged when the child said nothing, and truncates
// output past maxChildOutputInError with an explicit marker.
// An output carrying the not-parked refusal (childNotParked) also wraps battenshed.ErrChildNotParked, so InnerRun retries rather than failing.
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
	notParked := childNotParked(trimmed)
	if len(trimmed) > maxChildOutputInError {
		trimmed = strings.ToValidUTF8(trimmed[:maxChildOutputInError], "") + " ... (truncated)"
	}
	if notParked {
		return fmt.Errorf("%w: %w: %s", battenshed.ErrChildNotParked, runErr, trimmed)
	}
	return fmt.Errorf("%w: %s", runErr, trimmed)
}

// childNotParked reports whether a child bootstrap's output carries the refusal envelope whose "kind" is shedrun.StartNotParkedKind.
// Each output line is tried as an envelope, since the child's stdout and stderr share one buffer.
func childNotParked(childOutput string) bool {
	for line := range strings.Lines(childOutput) {
		var envelope struct {
			OK   bool   `json:"ok"`
			Kind string `json:"kind"`
		}
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if !envelope.OK && envelope.Kind == shedrun.StartNotParkedKind {
			return true
		}
	}
	return false
}

// childSeedParams returns the seed params recipe's own bootstrap verb will itself write, read from
// childLocation.
//
// shedrun.WriteSeed is idempotent only against a seed agreeing on recipe, driver and params, so a
// child seeded without a param its own bootstrap writes makes that bootstrap refuse.
// Params are per-recipe: only loom declares one, params.parent, taken from the pair's recorded
// origin.
// An absent or empty recorded parent yields no param, leaving loom's own "pass --parent once"
// refusal as the one an operator sees.
func childSeedParams(recipe string, childLocation *lyxcwd.Location) (map[string]string, error) {
	if recipe != shedrun.RecipeLoom {
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

// noticesReachDriversParent reports whether this batten's notices have a destination that is also the task worktree driver's parent:
// the pair's parent worktree is the prime batten runs in, and the prime's orch state records a strand.
// A pair created from another worktree, or with no resolvable parent, has a different parent to message.
func noticesReachDriversParent(prime, taskLocation *lyxcwd.Location) (bool, error) {
	parent, err := hubgeom.ResolveParent(taskLocation)
	if err != nil {
		return false, err
	}
	if parent.Worktree != prime.WorktreeName {
		return false, nil
	}
	orch, err := orchengine.LoadState(orchcli.PrimePaths(prime))
	if err != nil {
		return false, err
	}
	return orch.Strand != "", nil
}

// markBattenWatched writes the batten-watched marker of the task worktree's run, holding this process's pid, while noticesReachDriversParent holds, and removes it otherwise.
// The write is atomic, so the driver's render never reads a torn pid, and an absent marker is fine to remove.
// An absent task worktree is an error.
// It reports whether the marker is held.
func markBattenWatched(prime *lyxcwd.Location, slug string) (bool, error) {
	taskLocation, err := taskWorktreeLocation(prime, slug)
	if err != nil {
		return false, err
	}
	marker := shedrun.BattenWatchedMarker(taskLocation, shedrun.SelfRunID)
	watched, err := noticesReachDriversParent(prime, taskLocation)
	if err != nil {
		return false, err
	}
	if !watched {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return false, err
		}
		return false, nil
	}
	content := []byte(strconv.Itoa(os.Getpid()) + "\n")
	if existing, err := os.ReadFile(marker); err == nil && bytes.Equal(existing, content) {
		return true, nil
	}
	if err := fsx.AtomicWriteBytes(marker, content); err != nil {
		return false, err
	}
	return true, nil
}

// wire builds and stores the shedrecipe.Env and shedbuild.ShedPaths the run and status verbs
// need, over the resolved prime location and slug.
func (c *battenCLI) wire(location *lyxcwd.Location, slug string) error {
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

	// One closure keeps the marker for both rows that ask: Seed-Child once the seed is committed, and Run-Shed through the whole watch.
	markWatched := func(ctx context.Context) (bool, error) { return markBattenWatched(location, slug) }

	env := shedrecipe.Env{
		Slug:       slug,
		ScratchDir: BattenDir(location, slug),
		PrimeLock:  primeLock,
		CreateWorktree: func(ctx context.Context) error {
			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return err
			}
			// The already-complete probe before Add, so the row is idempotent against its own
			// post-condition -- see taskWorktreeComplete for the crash window this closes.
			present, complete, incompleteReason, err := taskWorktreeComplete(location, slug)
			if err != nil {
				return err
			}
			if complete {
				logger.Info("battencli: create worktree skipped, the task worktree is already present", "slug", slug)
				return nil
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
			res, err := top.Add(location, slug, fabricengine.AddOptions{})
			logger.Info("battencli: create worktree", "slug", slug, "mutations", res.Mutated())
			return createRefusal(err)
		},
		// Both teardown halves go through the pair-teardown composite and are idempotent against their shared post-condition, "the pair is gone".
		// shedengine persists the row's transition only after the producer returns, so a process killed right after Remove succeeded re-enters this row with no pair on disk.
		// EndSession's refusal probe then reports ErrPairNotFound, which Shutdown takes as nothing left to shut down, and RemovePair reports it again as done.
		// The composite finishes a half-removed pair itself -- sibling worktree, portal and launcher entries, both branches -- so no such state is refused by name here.
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
				// Recorded onto the receiver, not only returned: the run verb reads it back after
				// the Shed's own Run has returned, to surface it on the success envelope.
				c.abandonedSession = res.AbandonedSession
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
		InnerRun: battenshed.InnerRunDeps{
			// Notify queues the notice on the prime's orch, the parent of a batten run started from the prime; a notice the orch has no strand to receive is logged by QueueNotice.
			Notify: func(ctx context.Context, line string) error {
				_, err := orchengine.QueueNotice(orchcli.PrimePaths(location), line, time.Now())
				return err
			},
			MarkWatched: markWatched,
			// PauseRequested reads batten's own status file, the one `lyx batten pause` writes, so the in-call wait notices a pause within one check.
			PauseRequested: func() (bool, error) {
				status, found, err := state.ReadJSONStrict[shedengine.Status](StatusFile(location, slug), StatusLock(location, slug))
				if err != nil || !found {
					return false, err
				}
				return status.PauseRequested, nil
			},
			AttachDir: func() (string, error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", err
				}
				return taskLocation.AnchorPath(), nil
			},
			// ReadDecision, DriverAlive and ReviewWait resolve the task worktree on Call like every seam here, never at wiring time.
			ReviewWait: func() (string, error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", err
				}
				store := parentreview.Store{Root: loomengine.LoomParentReviewDir(taskLocation), LockDir: loomengine.LoomParentReviewLockDir(taskLocation)}
				note, err := store.WaitNote()
				if err != nil || note == "" {
					return "", err
				}
				return "parent review: " + note, nil
			},
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return battenshed.ChildDecision{}, false, err
				}
				// approve and reject each remove the other's record first, so both are present only when the two verbs race.
				// The approval wins then, as it does in the gate when both records match the pull request.
				a, found, err := landingshed.ReadApproval(loomengine.LoomApprovalPath(taskLocation))
				if err != nil {
					return battenshed.ChildDecision{}, false, err
				}
				if found {
					return battenshed.ChildDecision{Kind: battenshed.DecisionApprove, At: a.ApprovedAt, HeadSHA: a.HeadSHA}, true, nil
				}
				r, found, err := landingshed.ReadRejection(loomengine.LoomRejectionPath(taskLocation))
				if err != nil || !found {
					return battenshed.ChildDecision{}, false, err
				}
				return battenshed.ChildDecision{Kind: battenshed.DecisionReject, At: r.RejectedAt, HeadSHA: r.HeadSHA}, true, nil
			},
			DriverAlive: func(ctx context.Context) (bool, error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return false, err
				}
				return driverAliveFrom(present, func() (reedengine.StatusResult, error) {
					taskLocation, err := taskWorktreeLocation(location, slug)
					if err != nil {
						return reedengine.StatusResult{}, err
					}
					reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
					if err != nil {
						return reedengine.StatusResult{}, err
					}
					reedGeom, err := hubgeom.ReedGeometry(taskLocation)
					if err != nil {
						return reedengine.StatusResult{}, err
					}
					return reedengine.New(reedCfg, reedGeom).Status()
				})
			},
			DriverStrand: func(ctx context.Context) (battenshed.ChildDriverStrand, error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return battenshed.ChildDriverNone, err
				}
				return driverStrandFrom(present, func() ([]reedengine.DirectoryRow, error) {
					taskLocation, err := taskWorktreeLocation(location, slug)
					if err != nil {
						return nil, err
					}
					reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
					if err != nil {
						return nil, err
					}
					reedGeom, err := hubgeom.ReedGeometry(taskLocation)
					if err != nil {
						return nil, err
					}
					return reedengine.New(reedCfg, reedGeom).Directory()
				})
			},
			// ReviveStrands resumes the task worktree's reed session, which relaunches the dead driver strand and every other non-live strand of the pair.
			ReviveStrands: func(ctx context.Context) error {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
				if err != nil {
					return err
				}
				reedGeom, err := hubgeom.ReedGeometry(taskLocation)
				if err != nil {
					return err
				}
				logger.Info("battencli: reviving the task worktree's strands through reed resume", "slug", slug)
				res, err := reedengine.New(reedCfg, reedGeom).Resume()
				if err != nil {
					logger.Warn("battencli: reed resume of the task worktree failed", "slug", slug, "error", err)
					return err
				}
				logger.Info("battencli: reed resume of the task worktree finished", "slug", slug, "resumed", res.Resumed, "dropped", res.Dropped)
				return nil
			},
			ChildRunLockHeld: func() (bool, error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return false, err
				}
				return runLockHeld(shedrun.RunLock(taskLocation, shedrun.SelfRunID))
			},
			// ResolveStatus also creates the child's ephemeral status-lock directory, since its
			// caller reads through that lock next and nothing else on the Run-Shed path creates
			// it: the child's status file is durable while its lock is not. This mirrors
			// battenPreRun's own MkdirAll for prime's status lock (arm.go).
			ResolveStatus: func() (statusPath, statusLockPath string, err error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
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
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				// loom's bootstrap verb, not a per-recipe dispatch: the WriteSeed seam below admits
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
				logger.Info("battencli: spawning loom session", "slug", slug, "dir", cmd.Dir)
				runErr := cmd.Run()
				logger.Info("battencli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
				return childSpawnError(runErr, childOutput.String())
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
			},
			// OpenIDE confirms the pair first so an absent one is named in the warning, then hands the prime location to the driven open.
			OpenIDE: func(ctx context.Context) error {
				if _, err := taskWorktreeLocation(location, slug); err != nil {
					return err
				}
				return ideengine.SpawnDriven(location, slug)
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			// ReadBoardType opens the Board fresh on every Call, over fabricengine.BoardDir(location.HubPath), and returns the task's own Recipe field -- never a value captured at wiring time, so a recipe corrected after prime was seeded is still honoured.
			ReadBoardType: func(ctx context.Context) (string, error) {
				b := boardengine.New(boardengine.Config{Path: fabricengine.BoardDir(location.HubPath)})
				task, found, err := b.GetTask(slug)
				if err != nil {
					return "", err
				}
				if !found {
					return "", fmt.Errorf("battencli: board task %q not found", slug)
				}
				return task.Recipe, nil
			},
			// ChildDriver reads the "child_driver" param from prime's own seed -- the seed
			// card 24's auto-seed writes at this run-id -- defaulting to shedrun.DriverGo when the
			// seed is absent or the param is absent or empty. The defaulting itself is childDriverOf
			// (arm.go), shared with refuseAdoptedSeed's own comparison so the two can never read the
			// same seed's child driver differently.
			ChildDriver: func() (string, error) {
				seed, found, err := shedrun.ReadSeed(location, slug)
				if err != nil {
					return "", err
				}
				if !found {
					return shedrun.DriverGo, nil
				}
				return childDriverOf(seed), nil
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
				if recipe != shedrun.RecipeLoom {
					return fmt.Errorf("%w: only %q has a bootstrap verb Run-Shed can start in the task worktree, so a Board task's type must be %q or empty; got %q", battenshed.ErrUnsupportedChildRecipe, shedrun.RecipeLoom, shedrun.RecipeLoom, recipe)
				}
				childLocation, err := taskWorktreeLocation(location, slug)
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
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation, []string{shedrun.SeedRel(childLocation, shedrun.SelfRunID)}, fmt.Sprintf("batten: seed child %s", slug), fabricengine.EnvSyncOptions())
				return err
			},
			MarkWatched: markWatched,
			// PushSeed pushes the child's own fabric pair, the same location CommitSeed just
			// committed onto.
			PushSeed: func(ctx context.Context) error {
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				_, err = fabricengine.PushAnchored(childLocation, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait)
				return err
			},
		},
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
	}
	return nil
}
