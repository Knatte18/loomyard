// innerrun.go implements NewInnerRun, the producer that spawns the inner shed run inside the task
// worktree, if it has not spawned yet, and checks its persisted status exactly once per Call --
// re-entered via the recipe row's own on_stuck self-route rather than looping internally.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// waitOrCancel pauses for d, returning as soon as ctx is cancelled if that happens first.
// It is the production value a nil InnerRunDeps.Sleep resolves to, so an operator's stop is not
// held for the whole poll interval.
func waitOrCancel(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// spawnConfirmedFileSuffix is the fixed suffix of the marker a producer writes under its scratch
// directory once its spawn has returned success, joined onto the producer's own name.
const spawnConfirmedFileSuffix = "-spawned"

// SpawnConfirmedFile returns the path of the marker innerRunProducer writes under scratchDir once
// producer's spawn has returned success.
// It is exported for the same reason StuckReasonFile is: the file's reader and writer share one
// declarer of its name.
func SpawnConfirmedFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+spawnConfirmedFileSuffix)
}

// haltedChildRemedy is the operator instruction every halted-child error carries: the outer run
// cannot restart the task worktree's own driver, only watch it.
const haltedChildRemedy = "the task worktree's own run must be resumed from inside that worktree (its recipe's bootstrap verb, e.g. \"lyx loom start\") before this run is resumed; resuming this run alone only resumes the watch"

// innerRunProducer spawns the inner shed run for a task worktree, once, and checks its persisted
// status once per Call, reporting Stuck while the child is still running so shedengine's own
// on_stuck self-route re-enters this producer rather than this type looping internally.
type innerRunProducer struct {
	name         string
	slug         string
	deps         InnerRunDeps
	pollInterval time.Duration
	scratchDir   string
}

var _ shedengine.ShedProducer = (*innerRunProducer)(nil)

// NewInnerRun returns a shedengine.ShedProducer that resolves the task worktree's status path,
// spawns the inner shed run via deps.Spawn the first time Call finds no status file, and checks
// deps.ReadStatus exactly once per Call thereafter. The bounded wait lives on the recipe row's own
// max_bounces and on_stuck self-route, one shedengine bounce per Call, not inside this producer.
//
// A nil deps.Sleep resolves to waitOrCancel, once here rather than on every Call, so a test's
// no-op sleep is the only value ever substituted.
func NewInnerRun(name, slug string, deps InnerRunDeps, pollInterval time.Duration, scratchDir string) shedengine.ShedProducer {
	if deps.Sleep == nil {
		deps.Sleep = waitOrCancel
	}
	return &innerRunProducer{
		name:         name,
		slug:         slug,
		deps:         deps,
		pollInterval: pollInterval,
		scratchDir:   scratchDir,
	}
}

// Call implements shedengine.ShedProducer.
//
// It reads the child's status before doing anything else, and spawns only when that status is
// absent, or is still running with no spawn confirmed on this machine. The child's status file
// alone cannot say whether a spawn happened: the child's bootstrap seeds it as running before it
// starts the driver, so a bootstrap that failed or was killed after seeding leaves a running status
// with no driver behind it. The confirmation is a marker under scratchDir (SpawnConfirmedFile),
// cleared before every spawn attempt and written only once deps.Spawn returns success. Re-spawning a
// running child is safe because the bootstrap it runs is idempotent against a driver that is
// already alive. A halted or done child is never re-spawned, marker or not: the outer run watches
// the child's own run and never restarts it.
//
// The full disposition table, evaluated top to bottom: a spawn as above (logging both Live-Substrate
// Spawn Observability lines around deps.Spawn), then one more read;
// deps.Spawn returning an error is a hard error, not Stuck, since a failed spawn is mechanism
// failure, not an ordinary wait, and the next Call retries it; still no status file after a
// successful spawn is a hard error
// naming the spawn that returned success without producing one; a resolved status with
// StateDone is Done; StateRunning sleeps p.pollInterval and returns Stuck, the sole Stuck arm --
// ProducerDef.OnStuck is a static per-producer value, so every Stuck this row ever returns routes
// back to the same self-route target, and a second arm returning Stuck for a genuinely stuck child
// would burn the whole bounce budget in a tight loop before ever reaching StateBlocked;
// StateBlocked, StatePaused or StateFailed is a hard error whose message carries the child's State,
// Error and CurrentProducer; any other value is a hard error naming the unrecognised state.
//
// A ResolveStatus error and a ReadStatus error are both returned hard errors, not verdicts: the
// task worktree is required to exist by the time this row runs, and a status file that exists but
// does not decode, or a status lock that cannot be taken, is mechanism failure.
func (p *innerRunProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	statusPath, statusLockPath, err := p.deps.ResolveStatus()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: resolve status path: %w", p.name, err)
	}

	status, found, err := p.deps.ReadStatus(statusPath, statusLockPath)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read status: %w", p.name, err)
	}

	confirmedPath := SpawnConfirmedFile(p.scratchDir, p.name)
	if !found || (status.State == shedengine.StateRunning && !spawnConfirmed(confirmedPath)) {
		if err := os.Remove(confirmedPath); err != nil && !os.IsNotExist(err) {
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: clear spawn confirmation: %w", p.name, err)
		}
		logger.Info("battenshed: spawning inner shed run", "producer", p.name, "slug", p.slug, "status_found", found)
		spawnErr := p.deps.Spawn(ctx)
		logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
		if spawnErr != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run (resuming this run retries the spawn): %w", p.name, spawnErr)
		}
		recordSpawnConfirmed(p.name, p.slug, p.scratchDir, confirmedPath)

		status, found, err = p.deps.ReadStatus(statusPath, statusLockPath)
		if err != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read status: %w", p.name, err)
		}
		if !found {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run returned success but no status file was found", p.name)
		}
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}

	switch status.State {
	case shedengine.StateDone:
		return shedengine.Done, shedengine.OutputPointer{}, nil
	case shedengine.StateRunning:
		p.deps.Sleep(ctx, p.pollInterval)
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("inner shed run still running; sleeping %s before the next bounce", p.pollInterval)
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
		// The remedy is named here because nothing on the prime side can perform it: this row never
		// spawns against a halted child, and resuming the outer run resumes the watch, never the
		// child's own driver.
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: inner shed run reached state %q: error=%q current_producer=%q; %s", p.name, status.State, status.Error, status.CurrentProducer, haltedChildRemedy)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: unrecognized status state %q", p.name, status.State)
	}
}

// spawnConfirmed reports whether the spawn-confirmation marker at path exists. Any stat failure
// other than "absent" also reports false: the cost of a wrong false is one idempotent re-spawn, the
// cost of a wrong true is a driverless child watched as running for the whole bounce budget.
func spawnConfirmed(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// recordSpawnConfirmed writes the spawn-confirmation marker at path. A write failure is logged
// rather than escalated: the spawn itself succeeded, and a missing marker costs only one
// idempotent re-spawn on the next Call.
func recordSpawnConfirmed(producer, slug, scratchDir, path string) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for spawn confirmation failed", "producer", producer, "slug", slug, "scratchDir", scratchDir, "error", err)
		return
	}
	if err := os.WriteFile(path, []byte("spawned\n"), 0o644); err != nil {
		logger.Warn("battenshed: write spawn confirmation failed", "producer", producer, "slug", slug, "path", path, "error", err)
	}
}
