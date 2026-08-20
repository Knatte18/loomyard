// ctx.go implements the two context checks every real producer in this package shares: entryErr,
// consulted before a producer's Call starts anything, and cancelErr, consulted by every non-success
// exit path. This is loomshed's own copy of shedadapters' identically-shaped helpers -- see doc.go
// for why the duplication is deliberate.

package loomshed

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not yet cancelled, and otherwise a wrapped error naming name
// (the told producer name), stating the run never started because ctx was already cancelled.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("loomshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise a wrapped error naming name (the
// told producer name), stating ctx was cancelled during the run. Every non-Done return path in a
// producer's Call consults cancelErr first, replacing its own verdict with this error when the
// context is cancelled -- this is what discharges the obligation Shed cannot enforce: a Stuck
// returned under a cancelled context is indistinguishable to Shed from a genuine verdict and would
// silently consume bounce budget for what was actually an operator stop.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("loomshed: %s: context cancelled during run: %w", name, ctx.Err())
}
