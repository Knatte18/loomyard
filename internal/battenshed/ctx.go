// ctx.go implements the two context checks that battenshed's producers share.
// entryErr runs before a producer starts anything, and cancelErr runs on every return path that follows a seam call.

package battenshed

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not cancelled.
// Otherwise it returns a wrapped error that names the producer and states the run never started.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("battenshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled.
// Otherwise it returns a wrapped error that names the producer and states the cancellation came during the run.
// A producer consults it after every seam call, even when that call returns Stuck or a hard error.
// It then returns this error in place of that verdict.
// A Stuck returned under a cancelled context looks like a real verdict and would spend bounce budget,
// and a hard error would surface the seam's raw text instead of the cancellation.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("battenshed: %s: context cancelled during run: %w", name, ctx.Err())
}
