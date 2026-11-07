// carrier.go implements the Carrier that takes up and runs the hub's hibernate request in the prime's dobby.

package hibernateengine

import (
	"fmt"
	"sync"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Carrier carries the hub's hibernate request for the running process, the prime's dobby.
// It takes up a pending request no live process carries, and runs the request's sequence in a goroutine of its own,
// so the dobby's ticks never wait on it.
// It is safe for concurrent use.
type Carrier struct {
	deps Deps
	mu   sync.Mutex
	// running reports whether a sequence of this carrier is in flight.
	// The phase field is the last phase that sequence wrote.
	running bool
	phase   Phase
}

// NewCarrier returns the Carrier for deps, carrying no request yet.
func NewCarrier(deps Deps) *Carrier {
	c := &Carrier{deps: deps}
	c.deps.phased = c.setPhase
	return c
}

// setPhase records the phase the sequence in flight wrote.
func (c *Carrier) setPhase(phase Phase) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase = phase
}

// Tick takes up the pending request when no sequence of this carrier is in flight.
// It starts the sequence in a goroutine and returns at once.
// No request, a terminal one and one another live process carries are left alone.
// The error is a failed read or write of the request file, or a failed lift of the pause entry a takeover clears;
// the sequence's own errors are logged when it ends.
func (c *Carrier) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	r, taken, err := takeUp(c.deps)
	if err != nil || !taken {
		return err
	}
	c.running, c.phase = true, r.Phase
	go func() {
		ended, err := carry(c.deps, r)
		if err != nil {
			logger.Warn("hibernate: the request's sequence ended on an error", "phase", ended.Phase, "error", err)
		} else {
			logger.Info("hibernate: the request's sequence ended", "phase", ended.Phase, "reason", ended.Reason)
		}
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		c.wakeWatch()
	}()
	return nil
}

// wakeWatch rewrites the request file as it stands, so the watch on it runs the next tick.
// A request written during a cancelled sequence fired its watch event while Tick could take up nothing.
func (c *Carrier) wakeWatch() {
	err := updateRequest(c.deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found {
			return Request{}, errUnchanged
		}
		return cur, nil
	})
	if err != nil {
		logger.Warn("hibernate: the request file could not be touched to wake the next tick", "error", err)
	}
}

// HoldsReexec reports whether a sequence is in flight past its trigger wait.
// A re-exec of the process would cut a pause, a confirmation, a hibernate or a wake in the middle.
// The hold lasts until the sequence has ended, which is after it has lifted its entry and sent its notice,
// so a terminal request still being closed out holds too.
// A sequence still waiting for its trigger holds nothing:
// the process takes it up again from the same wait after the re-exec.
func (c *Carrier) HoldsReexec() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running && c.phase != PhaseWaiting
}

// takeUp records the running process as the carrier of the pending request under the request lock.
// It returns the request to carry.
// It takes up a request no live process carries, and one naming the running process itself.
// A re-exec of the process leaves such a request behind.
// No request, a terminal one, and one another live process carries are left alone.
// A request taken up in the pausing or confirming phase has the pause entry it owns lifted and goes back to waiting,
// so its sequence starts again from the trigger.
// A failed lift writes nothing and is returned.
func takeUp(deps Deps) (Request, bool, error) {
	var taken Request
	ok := false
	err := updateRequest(deps.RequestPath, func(cur Request, found bool) (Request, error) {
		if !found || cur.Terminal() || (!ours(deps, cur) && daemonLive(deps, cur)) {
			return Request{}, errUnchanged
		}
		if cur.Phase == PhasePausing || cur.Phase == PhaseConfirming {
			if _, err := liftOwned(deps, cur); err != nil {
				return Request{}, err
			}
			cur.Owned = nil
			cur.Phase, cur.Reason = PhaseWaiting, fmt.Sprintf("taken up again after the sequence that carried it ended in phase %s", cur.Phase)
		}
		cur.DaemonPID, cur.DaemonStart = deps.SelfPID, deps.SelfStart
		taken, ok = cur, true
		return cur, nil
	})
	if err != nil || !ok {
		return Request{}, false, err
	}
	logger.Info("hibernate: the request is taken up", "phase", taken.Phase, "pid", deps.SelfPID)
	return taken, true, nil
}
