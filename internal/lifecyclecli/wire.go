// wire.go implements wire, the assembly seam that builds the shedrecipe.Env and
// shedbuild.ShedPaths the run and status verbs need, plus the receiver field the abandoned
// session value is recorded into.
//
// Every seam that touches the managed task worktree resolves inside its own closure body on Call,
// never here at wiring time: the task worktree does not exist until WorktreeCreate has already run,
// so resolving any of its paths any earlier would resolve a path that is not there yet. That applies
// to the status-path pair, the spawn directory, and both teardown halves alike -- not only to the one
// whose laziness (ResolveStatus's own signature) makes it visible.

package lifecyclecli

import (
	"context"
	"os"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lifecycleshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
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
func (c *lifecycleCLI) wire(location *lyxcwd.Location, slug string) error {
	primeRunLockPath := PrimeRunLock(location)
	primeLock := lifecycleshed.PrimeLock{
		Path: primeRunLockPath,
		Acquire: func() (release func() error, ok bool, err error) {
			if err := os.MkdirAll(LifecycleDir(location, slug), 0o755); err != nil {
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
		ScratchDir: LifecycleDir(location, slug),
		PrimeLock:  primeLock,
		CreateWorktree: func(ctx context.Context) error {
			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return err
			}
			top := fabricengine.NewTopology(cfg)
			res, err := top.Add(location, slug, fabricengine.AddOptions{})
			logger.Info("lifecyclecli: create worktree", "slug", slug, "mutations", res.Mutated())
			return err
		},
		Teardown: lifecycleshed.TeardownDeps{
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
				logger.Info("lifecyclecli: teardown worktree", "slug", slug, "mutations", res.Mutated())
				return err
			},
		},
		LoomRun: lifecycleshed.LoomRunDeps{
			ResolveStatus: func() (statusPath, statusLockPath string, err error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", "", err
				}
				return loomengine.LoomStatusFile(taskLocation), loomengine.LoomStatusLock(taskLocation), nil
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
				logger.Info("lifecyclecli: spawning loom session", "slug", slug, "dir", cmd.Dir)
				err = cmd.Run()
				logger.Info("lifecyclecli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
				return err
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
			},
		},
	}

	c.env = env
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     StatusFile(location, slug),
		LockPath:       RunLock(location, slug),
		StatusLockPath: StatusLock(location, slug),
		// CommitStatus is left nil: nil is the documented absent value meaning "commit nothing",
		// which is exactly right for this package's per-machine, never-committed state.
		CommitStatus: nil,
	}
	return nil
}
