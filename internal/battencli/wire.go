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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

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
// Worktree-Create's post-condition is "the task worktree for this slug exists", so a row re-entered
// against an already-satisfied post-condition must report success rather than asking fabric to
// create the same worktree twice. The window that makes this reachable is real: a create does
// seconds of git work, and shedengine persists the row's transition only after the producer
// returns, so a process killed in between leaves the worktree on disk with nothing recording it.
// Without this probe the resumed row takes fabric's pre-existing-branch refusal, whose two named
// remedies -- switching a pair onto the branch, and deleting the branch -- both refuse in exactly
// that state, leaving the run unresumable by any documented action.
//
// Worktree-Teardown's two halves use the same probe for the mirror-image post-condition, "the task
// worktree is gone" (see the Teardown field below).
//
// It answers false rather than an error when the worktree is absent, so a genuinely absent one
// still reaches Topology.Add and every real create failure keeps its Stuck disposition. A stat
// error that is not "absent", and a path that is there but does not resolve as a worktree, are both
// returned: neither is an answer to the question asked.
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

// maxChildOutputInError caps the child output folded into a spawn error, so a misbehaving child
// cannot push an unbounded string into a status file committed onto prime's own pair.
const maxChildOutputInError = 2000

// childSpawnError folds a failed child bootstrap's own output into runErr, so the returned error
// carries the child's diagnosis rather than only its exit status.
// It returns nil for a nil runErr, runErr unchanged when the child said nothing, and truncates
// output past maxChildOutputInError with an explicit marker.
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
	if len(trimmed) > maxChildOutputInError {
		trimmed = strings.ToValidUTF8(trimmed[:maxChildOutputInError], "") + " ... (truncated)"
	}
	return fmt.Errorf("%w: %s", runErr, trimmed)
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

	env := shedrecipe.Env{
		Slug:       slug,
		ScratchDir: BattenDir(location, slug),
		PrimeLock:  primeLock,
		CreateWorktree: func(ctx context.Context) error {
			// The already-present probe first, so the row is idempotent against its own
			// post-condition -- see taskWorktreePresent for the crash window this closes.
			present, err := taskWorktreePresent(location, slug)
			if err != nil {
				return err
			}
			if present {
				logger.Info("battencli: create worktree skipped, the task worktree is already present", "slug", slug)
				return nil
			}

			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return err
			}
			top := fabricengine.NewTopology(cfg)
			res, err := top.Add(location, slug, fabricengine.AddOptions{})
			logger.Info("battencli: create worktree", "slug", slug, "mutations", res.Mutated())
			return err
		},
		// Both teardown halves are idempotent against their shared post-condition, "the task
		// worktree is gone", mirroring CreateWorktree's already-present probe: shedengine persists
		// the row's transition only after the producer returns, so a process killed right after
		// Remove succeeded re-enters this row with no pair on disk. Without the probe Shutdown's
		// own location resolution refuses the absence and the run halts at teardown for good.
		// An absent pair means its session was shut down by the earlier pass, or its worktree was
		// removed by hand; either way there is nothing left here for either half to act on.
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(ctx context.Context) (abandonedSession string, err error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return "", err
				}
				if !present {
					logger.Info("battencli: session shutdown skipped, the task worktree is already gone", "slug", slug)
					return "", nil
				}
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", err
				}
				reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
				if err != nil {
					return "", err
				}
				reedGeom := hubgeom.ReedGeometry(taskLocation)
				reedEngine := reedengine.New(reedCfg, reedGeom)
				res, err := reedEngine.Down()
				if err != nil {
					return "", err
				}
				// Recorded onto the receiver, not only returned: the run verb reads it back after
				// the Shed's own Run has returned, to surface it on the success envelope.
				c.abandonedSession = res.AbandonedSession
				return res.AbandonedSession, nil
			},
			Remove: func(ctx context.Context) error {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return err
				}
				if !present {
					logger.Info("battencli: teardown worktree skipped, the task worktree is already gone", "slug", slug)
					return nil
				}
				cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
				if err != nil {
					return err
				}
				top := fabricengine.NewTopology(cfg)
				res, err := top.Remove(location, slug, false, false)
				logger.Info("battencli: teardown worktree", "slug", slug, "mutations", res.Mutated())
				return err
			},
		},
		InnerRun: battenshed.InnerRunDeps{
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
		},
		SeedChild: battenshed.SeedChildDeps{
			// ReadBoardType opens the Board fresh on every Call, over fabricengine.BoardDir(
			// location.HubPath), and returns the task's own Type field -- never a value captured at
			// wiring time, so a type corrected after prime was seeded is still honoured.
			ReadBoardType: func(ctx context.Context) (string, error) {
				b := boardengine.New(boardengine.Config{Path: fabricengine.BoardDir(location.HubPath)})
				task, found, err := b.GetTask(slug)
				if err != nil {
					return "", err
				}
				if !found {
					return "", fmt.Errorf("battencli: board task %q not found", slug)
				}
				return task.Type, nil
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
				if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: driver, Params: params}); err != nil {
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
				_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation, []string{shedrun.SeedRel(shedrun.SelfRunID)}, fmt.Sprintf("batten: seed child %s", slug), fabricengine.EnvSyncOptions())
				return err
			},
			// PushSeed pushes the child's own fabric pair, the same location CommitSeed just
			// committed onto.
			PushSeed: func(ctx context.Context) error {
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				_, err = fabricengine.PushAnchored(childLocation, fabricengine.EnvSyncOptions())
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
	}
	return nil
}
