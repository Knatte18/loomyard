// ctx.go holds the two context checks every shedadapters adapter shares.
// entryErr is consulted before an adapter's Call starts anything;
// cancelErr is consulted by every non-success exit path.
// The exit rule's one exception is a genuine success verdict surviving cancellation.
// It lives in each adapter's own Call, not here.

package shedadapters

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not yet cancelled.
// Otherwise it returns a wrapped error naming name (the told producer name) and engine (a short engine label).
// The error states that the run never started because ctx was already cancelled.
func entryErr(ctx context.Context, name, engine string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("shedadapters: %s (%s): context cancelled before run started: %w", name, engine, ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled.
// Otherwise it returns a wrapped error naming name (the told producer name) and engine (a short engine label).
// The error states that ctx was cancelled during the run.
// Every non-success return path in an adapter's Call consults cancelErr first.
// It replaces the path's own verdict with this error when ctx is cancelled.
func cancelErr(ctx context.Context, name, engine string) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("shedadapters: %s (%s): context cancelled during run: %w", name, engine, ctx.Err())
}
