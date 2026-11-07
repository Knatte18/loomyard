// ctx.go holds the two context checks the Preflight producer runs.
// entryErr is checked before a run starts.
// cancelErr is checked on every exit path that does not succeed.

package preflightshed

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not cancelled yet.
// Otherwise it returns an error that wraps ctx.Err() and includes name, the told producer name.
// The error says the run never started.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("preflightshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled.
// Otherwise it returns an error that wraps ctx.Err() and includes name, the told producer name.
// The error says ctx was cancelled during the run.
// A producer runs this check on every path that does not finish done.
// On such a path it returns the error in place of the path's own verdict.
// Shed cannot tell a stuck verdict reached under a cancelled context from a genuine one,
// and that verdict would silently consume bounce budget for what was an operator stop.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("preflightshed: %s: context cancelled during run: %w", name, ctx.Err())
}
