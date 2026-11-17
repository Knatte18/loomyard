// testmapyield.go holds the slot and lock waits of `lym gate testmap`:
// a pass's take of a slot only while another stays free, its wait for one, and its watch for a gate that needs it back.
// It also holds a named call's wait for a package's map build lock.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// errSlotYielded ends a pass's job once the pass gave its slot back, or could take none while another stayed free.
var errSlotYielded = errors.New("the upkeep pass gave its gate slot back")

// The pauses of `lym gate testmap`'s waits in production.
const (
	// testMapWaitBase is the first pause of a pass's slot wait and of a named call's build lock wait.
	// Each later pause doubles, up to testMapWaitCeiling.
	testMapWaitBase    = time.Second
	testMapWaitCeiling = 30 * time.Second
	// testMapYieldPoll is the pause between a pass's checks, while it holds its slot, of whether a gate needs it.
	testMapYieldPoll = time.Second
)

// jobSlot makes sure the call holds a slot for a job's build and runs.
// A call that is no pass waits for one, and a call inside an inherited slot takes none.
// A pass takes one only while another stays free, and returns errSlotYielded when it can take none.
// It returns errSlotYielded too when a gate needs the slot it holds; it releases nothing itself.
func (r *testMapRun) jobSlot() error {
	if !r.spare {
		return r.holdSlot()
	}
	if r.lease == nil {
		held, err := r.takeSlot(r.pool.TryAcquireSpare)
		if err != nil {
			return err
		}
		if !held {
			return errSlotYielded
		}
		return nil
	}
	contended, err := r.pool.Contended(r.lease)
	if err != nil {
		return fmt.Errorf("read the hub's gate slots: %w", err)
	}
	if contended {
		return errSlotYielded
	}
	return nil
}

// waitForSpareSlot waits until the pass takes a slot while another stays free.
// It pauses waitBase at first and doubles each pause up to waitCeiling while it can take none.
// On each wake it reads HEAD, and it ends the wait with headMoved set once HEAD moved.
// The error reports a slot or read failure, or the call context's error after a terminating signal.
func (r *testMapRun) waitForSpareSlot() error {
	pause := r.request.waitBase
	waitStart := time.Now()
	logger.Info("gate testmap: the pass waits for a gate slot it may take", "worktree", r.location.WorktreePath())
	for {
		held, err := r.takeSlot(r.pool.TryAcquireSpare)
		if err != nil {
			return err
		}
		if held {
			logger.Info("gate testmap: the pass took a gate slot again", "waited", time.Since(waitStart).String())
			return nil
		}
		if err := pauseFor(r.ctx, pause); err != nil {
			return err
		}
		pause = min(2*pause, r.request.waitCeiling)
		if err := r.readHead(); err != nil || r.headMoved {
			return err
		}
	}
}

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

// watchForYield returns the context a job runs under, and the function that ends the watch once the job ends.
// A pass holding its slot gets a context that ends once a check finds a gate needs the slot.
// The check runs every yieldPoll, and finds that when every other slot is held or a multi-slot acquire drains.
// Any other call gets the call's own context.
func (r *testMapRun) watchForYield() (context.Context, func()) {
	if !r.spare || r.lease == nil {
		return r.ctx, func() {}
	}
	lease := r.lease
	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		warned := false
		for pauseFor(ctx, r.request.yieldPoll) == nil {
			contended, err := r.pool.Contended(lease)
			if err != nil {
				if !warned {
					logger.Warn("gate testmap: could not read the hub's gate slots, so the pass keeps its slot", "cause", err)
					warned = true
				}
				continue
			}
			if contended {
				logger.Info("gate testmap: the pass gives its gate slot back to a gate run", "worktree", r.location.WorktreePath())
				cancel()
				return
			}
		}
	}()
	return ctx, func() {
		cancel()
		<-done
	}
}

// interruption returns why a job's work under ctx ended early, or nil while ctx is live.
// It is the call context's error after a terminating signal, and errSlotYielded when the pass's yield watch ended ctx.
func (r *testMapRun) interruption(ctx context.Context) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errSlotYielded
	}
	return nil
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
