// wait.go — the wait for a running capture to end, shared by the Drift rows that hold a run until the baseline exists.

package driftengine

import (
	"context"
	"time"
)

// The capture wait polls the capture lock from the floor and doubles to the ceiling while the lock stays held.
const (
	captureWaitFloor   = time.Second
	captureWaitCeiling = 30 * time.Second
)

// awaitCaptureEnd waits until the capture lock reads free, sleeping through sleep from one second and doubling to thirty while the lock stays held.
// It has no bound of its own, since a capture enforces its timeout and releases the lock on exit, and it returns when the lock is free or ctx ends.
func awaitCaptureEnd(ctx context.Context, p Paths, sleep func(context.Context, time.Duration) error) error {
	wait := captureWaitFloor
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		held, err := ProbeLock(p.CaptureLock)
		if err != nil || !held {
			return err
		}
		if err := sleep(ctx, wait); err != nil {
			return err
		}
		wait = min(wait*2, captureWaitCeiling)
	}
}
