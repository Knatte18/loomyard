// await.go implements the capped file poll every keep-cache-warm wait verb runs on.

package shuttleengine

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"
)

// DefaultAwaitCap is the cap a wait verb's --cap flag defaults to: under the 5-minute prompt-cache lifetime,
// so a caller that runs the wait with a Bash tool timeout above the cap (300000 ms) and re-runs it keeps its cache warm.
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
