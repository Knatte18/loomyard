// primelockwait.go implements acquirePrimeLock, the bounded, cancellable wait both prime-lock
// producers (Worktree-Create and Worktree-Teardown) call in place of a single Acquire.

package battenshed

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// primeLockPollInterval is the pause between two Acquire attempts on a contended prime lock.
	primeLockPollInterval = 2 * time.Second
	// primeLockWaitBound is how long a producer waits for a contended prime lock before giving up with Stuck.
	// It is counted in attempts (primeLockWaitBound / primeLockPollInterval sleeps), never read off a wall clock, so a test with a no-op sleep proves it deterministically.
	primeLockWaitBound = 10 * time.Minute
)

// acquirePrimeLock acquires lock for producer, retrying a contended lock every primeLockPollInterval until it is acquired, ctx is cancelled, or primeLockWaitBound is spent.
//
// A non-nil release means the lock is held and the caller must call it exactly once.
// A non-empty stuckReason means the bound was spent with the lock still held elsewhere, and the caller returns Stuck with it.
// A non-nil err is a hard error: an Acquire failure at any attempt, or the cancelled-during-run diagnosis when ctx ended first.
// The first contended attempt logs one info line, and no later attempt of the same call does.
func acquirePrimeLock(ctx context.Context, producer, slug string, lock PrimeLock) (release func() error, stuckReason string, err error) {
	sleep := lock.Sleep
	if sleep == nil {
		sleep = waitOrCancel
	}
	maxSleeps := int(primeLockWaitBound / primeLockPollInterval)

	for attempt := 0; ; attempt++ {
		rel, ok, acqErr := lock.Acquire()
		if acqErr != nil {
			if cerr := cancelErr(ctx, producer); cerr != nil {
				return nil, "", cerr
			}
			return nil, "", fmt.Errorf("battenshed: %s: acquire prime lock %q: %w", producer, lock.Path, acqErr)
		}
		if ok {
			return rel, "", nil
		}
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return nil, "", cerr
		}
		if attempt == 0 {
			logger.Info("battenshed: prime lock contended, waiting", "producer", producer, "path", lock.Path, "slug", slug)
		}
		if attempt >= maxSleeps {
			return nil, fmt.Sprintf("prime lock %q is still held after waiting %s; another batten producer is creating or tearing down a task worktree", lock.Path, primeLockWaitBound), nil
		}
		sleep(ctx, primeLockPollInterval)
		if cerr := cancelErr(ctx, producer); cerr != nil {
			return nil, "", cerr
		}
	}
}
