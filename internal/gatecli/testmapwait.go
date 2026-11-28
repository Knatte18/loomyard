// testmapwait.go holds `lym gate testmap`'s wait for a package's map build lock, which a call naming packages takes.

package gatecli

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// The pauses of a named call's build lock wait in production.
const (
	// testMapWaitBase is the first pause; each later pause doubles, up to testMapWaitCeiling.
	testMapWaitBase    = time.Second
	testMapWaitCeiling = 30 * time.Second
)

// waitBuildLock takes the build lock of the map of pkg, waiting while another call holds it.
// It pauses waitBase at first and doubles each pause up to waitCeiling.
// The error reports a lock failure, or the call context's error after a terminating signal.
func (r *testMapRun) waitBuildLock(pkg string) (*lock.FileLock, error) {
	pause := r.request.waitBase
	for {
		buildLock, held, err := r.lockPackage(pkg)
		if err != nil || held {
			return buildLock, err
		}
		if err := pauseFor(r.ctx, pause); err != nil {
			return nil, err
		}
		pause = min(2*pause, r.request.waitCeiling)
	}
}

// pauseFor waits for d, or returns ctx's error once ctx is done first.
func pauseFor(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
