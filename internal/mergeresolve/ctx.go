// ctx.go implements the two context checks Resolve consults.
// entryErr is checked before Resolve starts anything; cancelErr is checked on every non-success exit path.

package mergeresolve

import (
	"context"
	"fmt"
)

// entryErr returns nil when ctx is not cancelled.
// Otherwise it returns an error stating that Resolve never started because ctx was already cancelled.
func entryErr(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("mergeresolve: context cancelled before Resolve started: %w", ctx.Err())
}

// cancelErr returns nil when ctx is not cancelled.
// Otherwise it returns an error stating that ctx was cancelled during Resolve's run.
// Every non-success return path in Resolve consults cancelErr first,
// and it returns this error in place of its own verdict when ctx is cancelled.
// A stuck verdict returned under a cancelled context is indistinguishable from a genuine one to the caller.
func cancelErr(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("mergeresolve: context cancelled during Resolve: %w", ctx.Err())
}
