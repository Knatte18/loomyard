// run.go holds the friction-reflection decision and its call: shouldReflectFriction, the pure
// decision loomPostRun (arm.go) gates the reflection call on, and reflectFriction, the call
// itself. Both are loom's own and were run's own before run's body moved into arm.go's PreRun and
// PostRun hooks.

package loomcli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// shouldReflectFriction reports whether loomPostRun should fire the friction reflection step: a
// non-empty friction directory and an outcome of shedengine.RunDone or shedengine.RunBlocked.
// It is the pure decision the reflection call site gates on, factored out so a test can drive every
// outcome without a real Shed.
func shouldReflectFriction(frictionDir string, outcome shedengine.RunOutcome) bool {
	if frictionDir == "" {
		return false
	}
	return outcome == shedengine.RunDone || outcome == shedengine.RunBlocked
}

// reflectFriction builds Deps from c's own already-resolved fields and calls frictionengine.Reflect,
// returning the envelope's "friction" status. A non-nil error from Reflect is a Deps-validation
// failure -- a wiring bug -- and is logged rather than surfaced: failing a successful, already-merged
// run because an optional bookkeeping agent could not run is strictly worse than filing nothing, and
// RunBlocked is worse still -- an operator staring at a blocked run does not need a second, unrelated
// failure layered on top.
//
// The whole call is wrapped in a non-blocking lock on loomengine.LoomFrictionLock, and a lock already
// held skips the step rather than waiting for it. This step runs AFTER shed.Run has returned, and
// shed.Run releases the run lock on return -- so for the whole of the reflection agent's life (up to
// friction_timeout_min, thirty minutes in the shipped template) the run lock reads as free and a
// second "lyx loom start" spawns a second driver. That second driver is legitimate, but its own
// reflection would archive the friction directory out from under the first one's live agent while
// both held the same reflection-report.md as a declared output. Skipping rather than waiting is
// correct here: the other reflection is already covering these very notes, so there is nothing left
// for this one to do, and blocking would hold a driver open for another agent's whole deadline.
func (c *loomCLI) reflectFriction() string {
	// The logged "dir" is the lock's own parent — the directory this MkdirAll actually creates —
	// not c.frictionDir, which is a sibling this call never touches (crucible round 2, R2-F4).
	lockDir := filepath.Dir(loomengine.LoomFrictionLock(c.location))
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		logger.Warn("loom: could not create the friction lock's directory; skipping the reflection", "dir", lockDir, "error", err)
		return frictionengine.StatusFailed
	}
	reflectionLock, free, err := lock.TryAcquireWriteLock(loomengine.LoomFrictionLock(c.location))
	if err != nil {
		logger.Warn("loom: could not probe the friction reflection lock; skipping the reflection", "dir", c.frictionDir, "error", err)
		return frictionengine.StatusFailed
	}
	if !free {
		logger.Warn("loom: another driver is already reflecting over this task's friction notes; skipping", "dir", c.frictionDir)
		return frictionengine.StatusSkipped
	}
	defer func() { _ = reflectionLock.Release() }()

	report, err := frictionengine.Reflect(frictionengine.Deps{
		Shuttle:       c.runner,
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
