// ctx.go implements the two context checks this package's four producers share: entryErr,
// consulted before Call starts anything, and cancelErr, consulted by every return path that
// follows a seam call, whether that call went on to return Stuck or a hard error.
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
// told producer name), stating ctx was cancelled during the run. Every return path in Call that
// follows a seam call -- a closure invocation that may itself block or do I/O, whether it went on
// to return Stuck or a hard error -- consults cancelErr first, replacing its own verdict with this
// error when the context is cancelled. This discharges two obligations Shed cannot enforce itself:
// a Stuck returned under a cancelled context is indistinguishable to Shed from a genuine verdict
// and would silently consume bounce budget for what was actually an operator stop, and a hard
// error returned under a cancelled context would otherwise surface the seam's own raw error text
// instead of the honest "cancelled during run" diagnosis every other exit path in this package
// gives. A return that follows no seam call of its own -- a plain switch on an already-read value
// -- needs no separate check: nothing between the read and the return can observe a cancellation
// the immediately preceding check did not already observe.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("battenshed: %s: context cancelled during run: %w", name, ctx.Err())
}
