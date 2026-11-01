// await.go implements the capped polls the keep-cache-warm wait verbs run on: one over files, one over a run's outstanding forks.

package shuttleengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// DefaultAwaitCap is the longest one call of a keep-cache-warm wait blocks: under the 5-minute prompt-cache lifetime.
// It is the fallback when the keep-warm resolver cannot answer; a caller runs a keep-warm wait with a Bash tool timeout of 3600000 ms so a long or raised cap fits, and re-runs it to keep its cache warm.
// A wait verb's --cap flag declares it as its flag default.
const DefaultAwaitCap = 250 * time.Second

// AwaitAny stats the told absolute paths once a second until one exists or cap elapses, on the real clock.
// It returns the first existing path, in the order told, with true; "", false and no error when cap elapses first.
// A stat error other than not-exist, and a cancelled ctx, are returned.
// It watches no directory, since a watched path's parent may not exist yet.
func AwaitAny(ctx context.Context, paths []string, cap time.Duration) (string, bool, error) {
	return awaitAny(ctx, realClock{}, paths, cap)
}

// awaitAny is AwaitAny over a told clock: it sleeps min(pollFloor, remaining) between two rounds of stats,
// so it wakes at most once a second and never past the cap.
func awaitAny(ctx context.Context, clock Clock, paths []string, cap time.Duration) (string, bool, error) {
	deadline := clock.Now().Add(cap)
	for {
		for _, path := range paths {
			_, err := os.Stat(path)
			if err == nil {
				return path, true, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return "", false, err
			}
		}

		remaining := deadline.Sub(clock.Now())
		if remaining <= 0 {
			return "", false, nil
		}
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		clock.Sleep(min(pollFloor, remaining))
	}
}

// AwaitForks waits, capped, until the session of the run identified by guid has no fork outstanding, polling once a second on the runner's clock.
// A session that spawns forks and then waits in this call takes every fork's result in the one turn after it returns, instead of waking once per fork.
// It returns true once no fork is outstanding, and the forks still outstanding with false when cap elapses first.
// An unknown guid, an engine without the OutstandingForkReader capability, a read error and a cancelled ctx are errors.
func (r *Runner) AwaitForks(ctx context.Context, guid string, cap time.Duration) ([]BackgroundTask, bool, error) {
	if r.toldErr != nil {
		return nil, false, r.toldErr
	}
	state, _, err := FindRun(r.cfg, r.anchorPath, guid)
	if err != nil {
		return nil, false, fmt.Errorf("shuttle: find the run of strand %q: %w", guid, err)
	}
	engine, name, err := r.engineFor(state)
	if err != nil {
		return nil, false, err
	}
	reader, ok := engine.(OutstandingForkReader)
	if !ok {
		return nil, false, fmt.Errorf("shuttle: engine %q does not implement the OutstandingForkReader capability, so a session cannot wait on its forks", name)
	}
	return awaitForks(ctx, r.clock, func() ([]BackgroundTask, error) {
		return reader.OutstandingForks(state.SessionID, r.paneCwd)
	}, cap)
}

// awaitForks is AwaitForks over a told clock and outstanding-fork read:
// it reads once, then sleeps min(pollFloor, remaining) between reads, so it wakes at most once a second and never past the cap.
func awaitForks(ctx context.Context, clock Clock, readOutstanding func() ([]BackgroundTask, error), cap time.Duration) ([]BackgroundTask, bool, error) {
	deadline := clock.Now().Add(cap)
	for {
		outstanding, err := readOutstanding()
		if err != nil {
			return nil, false, err
		}
		if len(outstanding) == 0 {
			return nil, true, nil
		}

		remaining := deadline.Sub(clock.Now())
		if remaining <= 0 {
			return outstanding, false, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		clock.Sleep(min(pollFloor, remaining))
	}
}
