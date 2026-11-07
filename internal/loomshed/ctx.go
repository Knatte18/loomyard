// ctx.go implements the two context checks every producer in this package shares:
// entryErr, consulted before a producer's Call starts anything, and cancelErr, consulted by every non-success exit path.

package loomshed

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not cancelled, and otherwise an error that includes name.
// The error means the run never started, because ctx was already cancelled.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("loomshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise an error that includes name.
// The error means ctx was cancelled during the run.
// Every non-Done return path in a producer's Call consults cancelErr first, replacing its own verdict with this error.
// Shed cannot enforce that obligation on its own.
// A Stuck returned under a cancelled context looks like a genuine verdict;
// it would silently consume bounce budget for what was an operator stop.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("loomshed: %s: context cancelled during run: %w", name, ctx.Err())
}
