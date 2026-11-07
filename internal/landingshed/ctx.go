// ctx.go implements the two context checks the producers in this package share.
// entryErr runs before a producer's Call starts, and cancelErr runs on every non-success exit path.
// Producers must return exactly Done, Stuck or Awaiting, and surface cancellation as a non-nil error, never as Stuck.

package landingshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedtransient"
)

// entryErr returns nil when ctx is not cancelled, and otherwise a wrapped error naming the producer given as name.
// The error states that the run never started because ctx was already cancelled.
func entryErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("landingshed: %s: context cancelled before run started: %w", name, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise a wrapped error naming the producer given as name.
// The error states that ctx was cancelled during the run.
// Every non-Done return path of a producer's Call consults cancelErr first.
// When ctx is cancelled, that path returns this error in place of its own verdict.
// A Stuck returned under a cancelled context is indistinguishable from a genuine verdict to the caller that reads it.
// Acting on such a verdict would silently consume bounce budget for an operator stop.
func cancelErr(ctx context.Context, name string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("landingshed: %s: context cancelled during run: %w", name, ctx.Err())
}

// transientFailure decides whether a failed remote call ends the call as a hard error.
// When ctx is cancelled, it returns the cancellation error and does not look at err.
// Otherwise, when shedtransient.Class marks err transient, it wraps err as `landingshed: <name>: <action>: %w`.
// A classifier that reads the error then persists the step as `failed`, and the driver re-steps it once.
// It returns nil for every other failure, which the caller keeps as a Stuck verdict for a human.
func transientFailure(ctx context.Context, name, action string, err error) error {
	if cerr := cancelErr(ctx, name); cerr != nil {
		return cerr
	}
	if shedtransient.Class(err) == "" {
		return nil
	}
	return fmt.Errorf("landingshed: %s: %s: %w", name, action, err)
}
