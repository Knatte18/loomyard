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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
func taskWorktreeLocation(prime *lyxcwd.Location, slug string) (*lyxcwd.Location, error) {
	return lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
}

// wire builds and stores the shedrecipe.Env and shedbuild.ShedPaths the run and status verbs
// need, over the resolved prime location and slug.
func (c *battenCLI) wire(location *lyxcwd.Location, slug string) error {
	primeRunLockPath := PrimeRunLock(location)
	primeLock := battenshed.PrimeLock{
		Path: primeRunLockPath,
		Acquire: func() (release func() error, ok bool, err error) {
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
				logger.Info("battencli: spawning loom session", "slug", slug, "dir", cmd.Dir)
				err = cmd.Run()
				logger.Info("battencli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
				return err
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
			WriteSeed: func(ctx context.Context, recipe, driver string) error {
				if err := shedrun.ValidateRecipe(recipe); err != nil {
					return fmt.Errorf("%w: %s", battenshed.ErrUnknownRecipe, err.Error())
				}
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				return shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: driver})
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
