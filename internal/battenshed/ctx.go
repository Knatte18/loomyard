// ctx.go implements the two context checks this package's three producers share: entryErr,
// consulted before Call starts anything, and cancelErr, consulted by every non-Done exit path.
// This is battenshed's own copy of preflightshed's and landingshed's identically-shaped
// helpers -- see doc.go for why the duplication is deliberate.

package battenshed

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
	return fmt.Errorf("battenshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise a wrapped error naming name (the
// told producer name), stating ctx was cancelled during the run. Every non-Done return path in
// Call consults cancelErr first, replacing its own verdict with this error when the context is
// cancelled -- this is what discharges the obligation Shed cannot enforce: a Stuck returned under
// a cancelled context is indistinguishable to Shed from a genuine verdict and would silently
// consume bounce budget for what was actually an operator stop.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("battenshed: %s: context cancelled during run: %w", name, ctx.Err())
}
