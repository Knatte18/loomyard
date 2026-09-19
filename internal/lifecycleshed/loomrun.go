// loomrun.go implements NewLoomRun, the producer that spawns the loom session inside the task
// worktree and polls its persisted status to a terminal verdict.

package lifecycleshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// loomRunProducer spawns the loom session for a task worktree and waits for it to reach a
// terminal state, polling its persisted status up to a bounded number of attempts.
type loomRunProducer struct {
	name         string
	slug         string
	deps         LoomRunDeps
	pollInterval time.Duration
	pollAttempts int
	scratchDir   string
}

var _ shedengine.ShedProducer = (*loomRunProducer)(nil)

// NewLoomRun returns a shedengine.ShedProducer that resolves the task worktree's status path,
// spawns the loom session via deps.Spawn, and polls deps.ReadStatus up to pollAttempts times,
// waiting pollInterval between attempts, to reach a terminal shedengine.State.
//
// A nil deps.Now resolves to time.Now and a nil deps.Sleep resolves to time.Sleep, both resolved
// once here rather than on every Call, so a test's fake clock and no-op sleep are the only values
// ever substituted.
func NewLoomRun(name, slug string, deps LoomRunDeps, pollInterval time.Duration, pollAttempts int, scratchDir string) shedengine.ShedProducer {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Sleep == nil {
		deps.Sleep = time.Sleep
	}
	return &loomRunProducer{
		name:         name,
		slug:         slug,
		deps:         deps,
		pollInterval: pollInterval,
		pollAttempts: pollAttempts,
		scratchDir:   scratchDir,
	}
}

// Call implements shedengine.ShedProducer.
//
// It resolves the status path, logs the spawn, calls deps.Spawn, logs the completed wait (both log
// lines are required by the Live-Substrate Spawn Observability invariant, since this producer
// waits for its child rather than detaching), and then polls deps.ReadStatus up to pollAttempts
// times.
//
// A ResolveStatus error and a ReadStatus error are both returned hard errors, not verdicts: the
// task worktree is required to exist by the time this row runs, and a status file that exists but
// does not decode, or a status lock that cannot be taken, is mechanism failure. A Spawn error is
// Stuck. found == false from ReadStatus is Stuck: the child's own handshake already confirmed a
// driver took the run lock, so a missing seed at this point is a real inconsistency, not an
// ordinary not-yet-written race.
//
// The verdict table over Status.State is exhaustive: StateDone is Done; StateBlocked, StatePaused
// and StateFailed are each Stuck, naming the state, the status's own Error field and its
// CurrentProducer; StateRunning consumes one poll attempt and continues. Any other value is a
// returned hard error. Exhausting pollAttempts while still StateRunning is Stuck, naming the
// interval, the attempt count, and the elapsed wall clock computed from deps.Now() readings taken
// before the first attempt and at exhaustion.
func (p *loomRunProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	statusPath, statusLockPath, err := p.deps.ResolveStatus()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: resolve status path: %w", p.name, err)
	}

	logger.Info("lifecycleshed: spawning loom session", "producer", p.name, "slug", p.slug)
	spawnErr := p.deps.Spawn(ctx)
	logger.Info("lifecycleshed: loom session wait complete", "producer", p.name, "slug", p.slug)
	if spawnErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("spawn loom session failed: %s", spawnErr.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	start := p.deps.Now()
	for attempt := 1; attempt <= p.pollAttempts; attempt++ {
		status, found, err := p.deps.ReadStatus(statusPath, statusLockPath)
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: read status: %w", p.name, err)
		}
		if !found {
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			reason := "no status file found after the loom session's own handshake confirmed a driver took the run lock"
			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		}

		switch status.State {
		case shedengine.StateDone:
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			return shedengine.Done, shedengine.OutputPointer{}, nil
		case shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			reason := fmt.Sprintf("loom session reached state %q: error=%q current_producer=%q", status.State, status.Error, status.CurrentProducer)
			reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
			return shedengine.Stuck, shedengine.OutputPointer{}, nil
		case shedengine.StateRunning:
			if attempt == p.pollAttempts {
				break
			}
			if cerr := cancelErr(ctx, p.name); cerr != nil {
				return "", shedengine.OutputPointer{}, cerr
			}
			p.deps.Sleep(p.pollInterval)
			continue
		default:
			return "", shedengine.OutputPointer{}, fmt.Errorf("lifecycleshed: %s: unrecognized status state %q", p.name, status.State)
		}
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	elapsed := p.deps.Now().Sub(start)
	reason := fmt.Sprintf("loom session still running after %d attempts at interval %s (elapsed %s)", p.pollAttempts, p.pollInterval, elapsed)
	reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
	return shedengine.Stuck, shedengine.OutputPointer{}, nil
}
