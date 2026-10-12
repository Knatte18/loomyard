// idle.go — the idle rule: a lock holder whose agents all wait, none busy, for a bound has stopped making progress.

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
	// Bound is how long an agent must wait, with nothing busy, for Observe to report idle.
	Bound time.Duration

	firstSeenWaiting map[string]time.Duration
}

// Observe records one reading taken at awake and reports whether no agent is busy and at least one has been waiting for at least Bound since Observe first saw it waiting.
// An agent no longer waiting, or absent, drops its first-seen time; read false, an unreadable state, counts every agent as not waiting.
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
