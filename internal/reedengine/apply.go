// apply.go holds the two pane-id sets the engine's reconcile and resume paths test membership against:
// the panes present in the window and the panes still running.

package reedengine

// liveIDSet builds the set of pane ids present in live, alive or dead-but-remain-on-exit.
// That is "present in the window", not "still running".
func liveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		ids[p.ID] = true
	}
	return ids
}

// aliveIDSet builds the set of pane ids that are present AND not dead — the
// "still running" set, as distinct from liveIDSet's "present in the window"
// set. Resume-planning and Status use this so a dead-but-remain-on-exit pane
// counts as not-live: a strand bound to such a pane must be relaunched by
// Resume and reported dead by Status, rather than being mistaken for a live
// strand just because tmux still lists its (dead) pane.
func aliveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		if !p.Dead {
			ids[p.ID] = true
		}
	}
	return ids
}
