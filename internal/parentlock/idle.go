// idle.go holds the idle rule for a lock holder.
// A holder is idle once no agent is busy and at least one agent has waited for a bound.

package parentlock

import "time"

// AgentState is one agent's reading: whether it is busy and whether it is waiting.
type AgentState struct {
	Name    string
	Busy    bool
	Waiting bool
}

// IdleWatch tracks how long each agent has been waiting, on the caller's awake clock.
type IdleWatch struct {
	// Bound is how long an agent must wait, with nothing busy, before the watch reports idle.
	Bound time.Duration

	firstSeenWaiting map[string]time.Duration
}

// Waiting returns a copy of the awake time each waiting agent was first seen waiting at, by agent name.
// The readings mean something only against the same awake clock, which a caller restoring them keeps across processes.
func (w *IdleWatch) Waiting() map[string]time.Duration {
	waiting := make(map[string]time.Duration, len(w.firstSeenWaiting))
	for name, first := range w.firstSeenWaiting {
		waiting[name] = first
	}
	return waiting
}

// RestoreIdleWatch returns a watch with bound and the first-seen-waiting readings of waiting.
// Its next Observe treats those readings as the earlier sightings.
// The readings are copied, and are on the same awake clock as the Observe calls that follow.
func RestoreIdleWatch(bound time.Duration, waiting map[string]time.Duration) *IdleWatch {
	restored := make(map[string]time.Duration, len(waiting))
	for name, first := range waiting {
		restored[name] = first
	}
	return &IdleWatch{Bound: bound, firstSeenWaiting: restored}
}

// Observe records one reading taken at awake and reports whether the lock holder is idle.
// It is idle when no agent is busy and at least one agent has waited for at least Bound.
// The wait is counted from the reading at which Observe first saw the agent waiting.
// An agent that is no longer waiting, or is absent, drops its first-seen time.
// A false read, an unreadable state, counts every agent as not waiting.
// Awake is the caller's reading of a clock that does not run while the host is suspended, so suspended time never counts as waiting.
func (w *IdleWatch) Observe(agents []AgentState, read bool, awake time.Duration) bool {
	seen := map[string]time.Duration{}
	if read {
		for _, agent := range agents {
			if !agent.Waiting {
				continue
			}
			first, known := w.firstSeenWaiting[agent.Name]
			if !known {
				first = awake
			}
			seen[agent.Name] = first
		}
	}
	w.firstSeenWaiting = seen
	if !read {
		return false
	}
	idle := false
	for _, agent := range agents {
		if agent.Busy {
			return false
		}
		if first, waiting := seen[agent.Name]; waiting && awake-first >= w.Bound {
			idle = true
		}
	}
	return idle
}
