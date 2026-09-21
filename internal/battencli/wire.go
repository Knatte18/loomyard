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
// It probes the worktree's existence before resolving, so an absent pair is reported as the state
// it actually is rather than as whatever the resolver happens to say about a directory that is not
// there. That case is not hypothetical and not a corrupt hub: batten's status file is durable and
// fabric-synced, so a run resumed on a second machine legitimately reaches every row past
// Worktree-Create with the pair unmaterialized locally, and a pair removed by hand mid-run lands in
// the same place. Left to the resolver it surfaced as a bare
// "not a git repository: chdir <path>: no such file or directory", which names neither the run, nor
// the reason, nor a remedy.
//
// Recreating the pair here is deliberately NOT attempted: fabric's own Add refuses a pre-existing
// branch by design, so materializing a pair from branches that already exist needs a fabric
// capability batten does not have, and inventing one behind a path resolver would be the wrong
// place for it regardless.
func taskWorktreeLocation(prime *lyxcwd.Location, slug string) (*lyxcwd.Location, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"battencli: the task worktree for %q is not present at %s; this run's durable status says it was already created, so it is either on another machine or was removed by hand -- batten does not recreate a pair from its branch, so restore it with \"lyx fabric checkout %s\" before resuming",
				slug, worktreePath, slug,
			)
		}
		return nil, err
	}
	return lyxcwd.ResolveWorktree(worktreePath)
}

// maxChildOutputInError caps how much of a failed child bootstrap's own output is folded into the
// returned error. The child writes a single JSON envelope, so the cap is never reached in practice;
// it exists so a child that misbehaves cannot push an unbounded string into a status file that is
// committed onto prime's own pair.
const maxChildOutputInError = 2000

// childSpawnError composes the error the Spawn seam returns for a child bootstrap that exited
// non-zero, folding the child's own captured output into runErr's text.
//
// It returns nil for a nil runErr, and runErr unchanged when the child said nothing -- there is no
// value in appending an empty quote. Output longer than maxChildOutputInError is truncated with an
// explicit marker, never silently.
func childSpawnError(runErr error, childOutput string) error {
	if runErr == nil {
		return nil
	}
	trimmed := strings.TrimSpace(childOutput)
	if trimmed == "" {
		return runErr
	}
	if len(trimmed) > maxChildOutputInError {
		trimmed = trimmed[:maxChildOutputInError] + " ... (truncated)"
	}
	return fmt.Errorf("%w: %s", runErr, trimmed)
}

// childSeedParams returns the seed params the child's own bootstrap verb will itself write for
// recipe, read from childLocation, so the seed Seed-Child writes is one that bootstrap AGREES with
// rather than one it refuses.
//
// The coupling is not optional and not defensive. shedrun.WriteSeed is idempotent only against a
// seed that agrees on recipe, driver AND params; a disagreeing seed is refused outright with no
// self-healing. loom's own bootstrap re-writes its seed on every "lyx loom start" carrying
// params.parent, so a child seeded here without that param makes every subsequent bootstrap in that
// worktree refuse permanently -- which is exactly what Run-Shed's spawn hit before this existed.
//
// Params are per-recipe by definition, so the branch on recipe is the contract, not a special case.
// Only shedrun.RecipeLoom declares one today; any other recipe seeds no params, which is what its
// own bootstrap will write.
//
// A recorded parent branch that is absent or empty yields no param at all, deliberately: loom's own
// bootstrap refuses an unrecorded parent with a message naming --parent as the remedy, and that
// refusal is far more useful to an operator than a seed disagreement manufactured here.
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
			// Both directories are ensured, and the prime lock's own comes first, because it is
			// the one this closure is about to take a lock in. The per-slug scratch directory is
			// ensured too because the producers that hold this lock write their stuck-reason files
			// there. Relying on the slug directory's MkdirAll to create the prime lock's parent as
			// a side effect of creating a deeper path under it worked only as long as the two
			// shedrun constructors happened to nest, a coupling neither one states.
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
			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return err
			}
			top := fabricengine.NewTopology(cfg)
			res, err := top.Add(location, slug, fabricengine.AddOptions{})
			logger.Info("battencli: create worktree", "slug", slug, "mutations", res.Mutated())
			return err
		},
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(ctx context.Context) (abandonedSession string, err error) {
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
			// ResolveStatus also creates the child's own ephemeral status-lock directory before
			// returning, because the very next thing its caller does is read through that lock.
			// The child's status file is durable (_lyx/shed/self/) while its lock is ephemeral
			// (.lyx/shed/self/), and nothing else creates the ephemeral half on the Run-Shed path:
			// Worktree-Create creates the pair, and Seed-Child's shedrun.WriteSeed MkdirAlls the
			// DURABLE run directory only. Without this, Run-Shed's own read-before-spawn check --
			// the producer's re-entry-safety mechanism -- fails on a bare "no such file or
			// directory" before deps.Spawn is ever reached, on every freshly created task worktree.
			// This mirrors battenPreRun's and battenPreStep's identical MkdirAll for PRIME's own
			// status lock (arm.go); that guard was never carried down to the child's.
			// Making a told path usable is legal here where deriving one would not be, the same
			// licence battenshed's own reportStuck takes with its told scratch directory.
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
				// Dir is the task worktree's AnchorPath(), never its bare worktree root: the resolver
				// gates a child's working directory to the anchor, and a bare root fails on any
				// subpath-anchored hub.
				cmd := exec.Command(exe, "loom", "start", "--no-attach")
				cmd.Dir = taskLocation.AnchorPath()
				// The child's own output is captured rather than discarded: it is the ONLY
				// diagnosis this seam can offer. Without it every child-bootstrap failure -- a
				// refused seed, an unrecorded parent branch, an unparseable module config, a
				// provider binary that will not boot -- reaches the operator, and the persisted
				// status.error, as the identical bare "exit status 1".
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
			// seed is absent or the param is absent or empty.
			ChildDriver: func() (string, error) {
				seed, found, err := shedrun.ReadSeed(location, slug)
				if err != nil {
					return "", err
				}
				if !found {
					return shedrun.DriverGo, nil
				}
				if driver, ok := seed.Params["child_driver"]; ok && driver != "" {
					return driver, nil
				}
				return shedrun.DriverGo, nil
			},
			// WriteSeed is the only place in the batten path that encodes a seed, per the
			// seed-encoding-stays-behind-a-seam-in-battenshed Shared Decision. It validates recipe
			// itself, wrapping a failure in battenshed.ErrUnknownRecipe so seedChildProducer's own
			// errors.Is check routes it to Stuck rather than a hard error.
			//
			// The params it writes come from childSeedParams: a seed missing a param the child's
			// own bootstrap will write is not a smaller seed, it is a seed that bootstrap refuses.
			WriteSeed: func(ctx context.Context, recipe, driver string) error {
				if err := shedrun.ValidateRecipe(recipe); err != nil {
					return fmt.Errorf("%w: %s", battenshed.ErrUnknownRecipe, err.Error())
				}
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				params, err := childSeedParams(recipe, childLocation)
				if err != nil {
					return err
				}
				return shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: driver, Params: params})
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
