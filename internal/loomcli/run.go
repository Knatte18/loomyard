// run.go holds the friction-reflection calls.
// reflectFriction is the call itself, and reflectFrictionRow is the closure the terminal Friction-Reflect row runs.
// The halt hooks in halt.go write a note and spawn no reflection.

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

// reflectFriction builds Deps from c's resolved fields and calls frictionengine.Reflect.
// It returns the envelope's "friction" status.
// A non-nil error from Reflect is a Deps-validation failure, which is a wiring bug.
// It is logged rather than surfaced:
// failing an already-merged run over an optional bookkeeping agent is worse than filing nothing.
//
// The whole call runs under a write lock on loomengine.LoomFrictionLock.
// The lock orders the reflections of two drivers over one friction directory:
// an operator can resume a halted task and reach a new driver's row while an earlier driver's row still reflects.
// Otherwise the second would archive notes out from under the first one's live agent,
// and both would hold reflection-report.md as a declared output.
// The call waits for the lock: the row holds done back until every reflection over these notes has finished.
// The wait is bounded by the holder's friction_timeout_min.
// An advisory lock is released on process death.
// The holder never waits on anything the row holds, so the wait cannot deadlock.
// The wait is not cancellable.
func (c *loomCLI) reflectFriction() string {
	// The logged "dir" is the lock's own parent: the directory this MkdirAll creates.
	// It is not c.frictionDir, which this call never touches.
	lockDir := filepath.Dir(loomengine.LoomFrictionLock(c.location))
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		logger.Warn("loom: could not create the friction lock's directory; skipping the reflection", "dir", lockDir, "error", err)
		return frictionengine.StatusFailed
	}
	reflectionLock, err := lock.AcquireWriteLock(loomengine.LoomFrictionLock(c.location))
	if err != nil {
		logger.Warn("loom: could not take the friction reflection lock; skipping the reflection", "dir", c.frictionDir, "error", err)
		return frictionengine.StatusFailed
	}
	defer func() { _ = reflectionLock.Release() }()

	report, err := frictionengine.Reflect(frictionengine.Deps{
		Shuttle:       c.reflectionShuttle,
		FrictionDir:   c.frictionDir,
		ArchivePrefix: loomengine.LoomFrictionArchivePrefix(c.location),
		StencilsDir:   c.runDeps.Geom.StencilsDir,
		ParentName:    c.parentName,
		FrictionSpec:  c.cfg.Friction,
		Registry:      c.registry,
		TaskSlug:      seedSlug(c.location.WorktreeName),
		Timeout:       time.Duration(c.cfg.FrictionTimeoutMin) * time.Minute,
	})
	if err != nil {
		logger.Warn("loom: friction reflection failed", "dir", c.frictionDir, "error", err)
		return frictionengine.StatusFailed
	}
	return report.Status
}

// reflectFrictionRow is the closure the terminal Friction-Reflect row runs.
// Its status is frictionengine.StatusSkipped unless Tier 2 is on; otherwise it is the status of reflectFriction.
// It records the status on c.rowFrictionStatus and returns it.
// The row reflects under step (`lym loom step`, `lym shed step --recipe tapestry`) as under run:
// every driven run uses step.
// The reflection agent files public issues during the step with no operator in the loop.
// The step blocks for at most friction_timeout_min.
func (c *loomCLI) reflectFrictionRow() string {
	status := frictionengine.StatusSkipped
	if c.frictionDir != "" {
		status = c.reflectFriction()
	}
	c.rowFrictionStatus = status
	return status
}
