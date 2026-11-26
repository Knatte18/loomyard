// pushuntillanded.go retries an owed push without end, backing off while the push stays owed.

package runpusher

import (
	"context"
	"time"
)

const (
	// owedRetryInitial is the wait before the first retry of an owed push.
	owedRetryInitial = time.Second
	// owedRetryCeiling is the longest wait between two retries of an owed push.
	owedRetryCeiling = time.Minute
)

// PushUntilLanded runs push until it lands or fails with an error owed does not read as owed, and returns that result.
// A push owed is retried without end, after sleeping from one second and doubling to a minute.
// The first time the wait reaches a minute it calls atCeiling with the last error, and not again in this call.
// A cancelled ctx ends the wait with the context's error.
// The caller holds no lock but its own while the wait runs, since a push releases the locks it takes on return.
func PushUntilLanded(ctx context.Context, push func() error, owed func(error) bool, sleep func(time.Duration), atCeiling func(error)) error {
	wait := owedRetryInitial
	notified := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := push()
		if err == nil || !owed(err) {
			return err
		}
		if wait >= owedRetryCeiling && !notified {
			notified = true
			atCeiling(err)
		}
		sleep(wait)
		wait = min(wait*2, owedRetryCeiling)
	}
}
