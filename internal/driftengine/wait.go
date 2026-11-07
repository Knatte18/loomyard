// wait.go holds the wait for a running capture to end.
// The Drift rows that hold a run until the baseline exists share it.

package driftengine

import (
	"context"
	"time"
)

// Bounds of the sleep between probes of the capture lock:
// the wait starts at `captureWaitFloor`, doubles while the lock stays held, and stops growing at `captureWaitCeiling`.
const (
	captureWaitFloor   = time.Second
	captureWaitCeiling = 30 * time.Second
)

// awaitCaptureEnd waits until the capture lock reads free.
// It calls sleep between probes.
// The interval starts at `captureWaitFloor`, doubles while the lock stays held, and caps at `captureWaitCeiling`.
// The wait has no bound of its own, since a capture enforces its timeout and releases the lock on exit,
// and it returns when the lock is free or ctx ends.
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
