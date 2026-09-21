// innerrun.go implements NewInnerRun, the producer that spawns the inner shed run inside the task
// worktree, if it has not spawned yet, and checks its persisted status exactly once per Call --
// re-entered via the recipe row's own on_stuck self-route rather than looping internally.

package battenshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// waitOrCancel pauses for d, returning as soon as ctx is cancelled if that happens first. It is the
// production value a nil InnerRunDeps.Sleep resolves to.
//
// A bare time.Sleep here held an operator's stop for the whole poll interval -- 30 seconds by the
// recipe's own config -- before Call's cancellation check, which sits on the very next line, could
// run. The timer is stopped on either exit so a cancelled wait leaves nothing behind.
func waitOrCancel(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

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
// A nil deps.Now resolves to time.Now and a nil deps.Sleep resolves to waitOrCancel, both resolved
// once here rather than on every Call, so a test's fake clock and no-op sleep are the only values
// ever substituted.
func NewInnerRun(name, slug string, deps InnerRunDeps, pollInterval time.Duration, scratchDir string) shedengine.ShedProducer {
	if deps.Now == nil {
		deps.Now = time.Now
	}
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
// It reads the child's status before doing anything else -- the read-before-spawn ordering is the
// re-entry-safety mechanism: the child's own status file is the durable record of whether the spawn
// already happened, so a resumed Call never double-spawns and needs no separate marker of its own.
//
// The full disposition table, evaluated top to bottom: no status file yet, spawn the inner shed run
// (logging both Live-Substrate Spawn Observability lines around deps.Spawn) and read once more;
// deps.Spawn returning an error is a hard error, not Stuck, since a failed spawn is mechanism
// failure, not an ordinary wait; still no status file after a successful spawn is a hard error
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
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: resolve status path: %w", p.name, err)
	}

	status, found, err := p.deps.ReadStatus(statusPath, statusLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: read status: %w", p.name, err)
	}

	if !found {
		logger.Info("battenshed: spawning inner shed run", "producer", p.name, "slug", p.slug)
		spawnErr := p.deps.Spawn(ctx)
		logger.Info("battenshed: inner shed run wait complete", "producer", p.name, "slug", p.slug)
		if spawnErr != nil {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: spawn inner shed run: %w", p.name, spawnErr)
		}

		status, found, err = p.deps.ReadStatus(statusPath, statusLockPath)
		if err != nil {
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
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: inner shed run reached state %q: error=%q current_producer=%q", p.name, status.State, status.Error, status.CurrentProducer)
	default:
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: unrecognized status state %q", p.name, status.State)
	}
}
