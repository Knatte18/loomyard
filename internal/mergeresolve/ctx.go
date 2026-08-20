// ctx.go implements the two context checks Resolve consults: entryErr, checked before Resolve
// starts anything, and cancelErr, checked by every non-success exit path. Shaped exactly like
// internal/loomshed/ctx.go's identically-purposed pair, naming this package in their own message
// prefix rather than reusing loomshed's or shedadapters' unexported copies.

package mergeresolve

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not yet cancelled, and otherwise a wrapped error stating that
// Resolve never started because ctx was already cancelled.
func entryErr(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("mergeresolve: context cancelled before Resolve started: %w", ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled, and otherwise a wrapped error stating that ctx
// was cancelled during Resolve's run. Every non-success return path in Resolve consults cancelErr
// first, replacing its own verdict with this error when the context is cancelled — a stuck verdict
// returned under a cancelled context is indistinguishable from a genuine one to the caller.
func cancelErr(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("mergeresolve: context cancelled during Resolve: %w", ctx.Err())
}
