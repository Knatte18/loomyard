// run.go holds the friction-reflection calls: reflectFriction, the call itself, and
// reflectFrictionRow, the closure the terminal Friction-Reflect row runs.
// The halt decision lives in halt.go, in loomAfterStep (step) and loomPostRun (run).

package loomcli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
)

// reflectFriction builds Deps from c's own already-resolved fields and calls frictionengine.Reflect,
// returning the envelope's "friction" status. A non-nil error from Reflect is a Deps-validation
// failure -- a wiring bug -- and is logged rather than surfaced: failing a successful, already-merged
// run because an optional bookkeeping agent could not run is strictly worse than filing nothing, and
// RunBlocked is worse still -- an operator staring at a blocked run does not need a second, unrelated
// failure layered on top.
//
// The whole call is wrapped in a lock on loomengine.LoomFrictionLock, which guards against two
// reflections over one friction directory. They can overlap because the blocked-path call (from
// loomPostRun, and loomAfterStep under step) runs after shed.Run or shed.Step has returned and released the run lock, so an operator can resume
// the task and reach a new driver's Friction-Reflect row while that earlier reflection still runs;
// the second would archive the directory out from under the first one's live agent while both held
// the same reflection-report.md as a declared output.
// The blocked path (wait false) skips on a held lock: the other reflection already covers these
// notes. The row path (wait true) waits, because its purpose is to hold done back until every
// reflection over these notes has finished. The wait is bounded by the holder's friction_timeout_min,
// an advisory lock is released on process death, and the holder never waits on anything the row
// holds, so it cannot deadlock. The wait is not cancellable, matching frictionengine.Reflect itself.
func (c *loomCLI) reflectFriction(wait bool) string {
	// The logged "dir" is the lock's own parent — the directory this MkdirAll actually creates —
	// not c.frictionDir, which is a sibling this call never touches (crucible round 2, R2-F4).
	lockDir := filepath.Dir(loomengine.LoomFrictionLock(c.location))
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		logger.Warn("loom: could not create the friction lock's directory; skipping the reflection", "dir", lockDir, "error", err)
		return frictionengine.StatusFailed
	}
	var reflectionLock *lock.FileLock
	if wait {
		l, err := lock.AcquireWriteLock(loomengine.LoomFrictionLock(c.location))
		if err != nil {
			logger.Warn("loom: could not take the friction reflection lock; skipping the reflection", "dir", c.frictionDir, "error", err)
			return frictionengine.StatusFailed
		}
		reflectionLock = l
	} else {
		l, free, err := lock.TryAcquireWriteLock(loomengine.LoomFrictionLock(c.location))
		if err != nil {
			logger.Warn("loom: could not probe the friction reflection lock; skipping the reflection", "dir", c.frictionDir, "error", err)
			return frictionengine.StatusFailed
		}
		if !free {
			logger.Warn("loom: another driver is already reflecting over this task's friction notes; skipping", "dir", c.frictionDir)
			return frictionengine.StatusSkipped
		}
		reflectionLock = l
	}
	defer func() { _ = reflectionLock.Release() }()

	report, err := frictionengine.Reflect(frictionengine.Deps{
		Shuttle:       c.reflectionShuttle,
		FrictionDir:   c.frictionDir,
		ArchivePrefix: loomengine.LoomFrictionArchivePrefix(c.location),
		StencilsDir:   c.runDeps.Geom.StencilsDir,
		FrictionSpec:  c.cfg.Friction,
		Registry:      c.registry,
		Timeout:       time.Duration(c.cfg.FrictionTimeoutMin) * time.Minute,
	})
	if err != nil {
		logger.Warn("loom: friction reflection failed", "dir", c.frictionDir, "error", err)
		return frictionengine.StatusFailed
	}
	return report.Status
}

// reflectFrictionRow is the shedrecipe.Env.ReflectFriction closure wire installs. Its status is
// frictionengine.StatusSkipped unless Tier 2 is on, in which case it is reflectFriction(true); it
// records the status on c.rowFrictionStatus and returns it.
// It reflects under step ("lyx loom step", "lyx shed step --recipe loom") as under run, since every
// driven run uses step.
// The reflection agent files public issues during the step with no operator in the loop; the step
// blocks for at most friction_timeout_min.
func (c *loomCLI) reflectFrictionRow() string {
	status := frictionengine.StatusSkipped
	if c.frictionDir != "" {
		status = c.reflectFriction(true)
	}
	c.rowFrictionStatus = status
	return status
}
