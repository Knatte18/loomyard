// reconcile.go implements the reconcile-against-live-panes engine op: the pure planning function
// planReconcile decides which strand pane bindings to clear and which dead panes to kill,
// and reconcileLocked composes that plan with the tmux kill I/O.
// Every public engine op runs reconcile first, under the op lock, so the persisted table never
// drifts from what tmux's list-panes actually reports.

package reedengine

import "fmt"

// planReconcile decides which pane bindings to clear and which panes to kill.
// Pure logic; unit-testable without a running server.
// Keeps at least one pane alive (session-survival rule); spares header pane.
func planReconcile(strands []Strand, live []LivePane, headerPaneID string) (clearedGUIDs []string, panesToKill []string, keptDeadPane string) {
	liveByID := make(map[string]LivePane, len(live))
	for _, p := range live {
		liveByID[p.ID] = p
	}

	// If any pane is still alive, killing every dead pane leaves the session
	// with at least that live pane, so all dead panes are killable. If every
	// pane in the window is dead, one dead pane must be spared — killing the
	// last pane ends the session — so keptDeadPane names the first dead pane
	// to keep.
	anyAlive := false
	for _, p := range live {
		if !p.Dead {
			anyAlive = true
			break
		}
	}
	keptDeadPaneID := ""
	if !anyAlive {
		for _, p := range live {
			if p.Dead {
				keptDeadPaneID = p.ID
				break
			}
		}
	}

	// The header pane is exempt from the dead-pane kill too, not only from
	// the untracked reap below: nothing outside up/resume ever rebuilds the
	// header, so killing a pane_dead=1 header here would leave the session
	// headerless (breaking the always-on keepalive the header exists for)
	// with a stale HeaderPaneID until the next up/resume — and, before the
	// planLayout presence filter existed, that stale id was still emitted as
	// a layout cell, which a real tmux ACCEPTS (exit 0) and assigns
	// positionally, scrambling every strand's height (observed live,
	// tmux 3.6). A kept header corpse instead stays enumerable, keeps the
	// cell/pane count consistent, and is healed — killed and re-split — by
	// ensureHeaderPaneLocked on the next up/resume.
	killSet := make(map[string]bool, len(live))
	for _, p := range live {
		if p.Dead && p.ID != keptDeadPaneID && p.ID != headerPaneID {
			killSet[p.ID] = true
			panesToKill = append(panesToKill, p.ID)
		}
	}

	// Deterministic untracked-pane reaping (see the doc comment): kill every
	// live pane no strand owns, but only while some strand is bound to a
	// present pane — killing an alive pane at worst corpses it under
	// remain-on-exit, so the bound pane always keeps the session alive.
	boundPaneIDs := make(map[string]bool, len(strands))
	for _, s := range strands {
		if s.PaneID != "" {
			boundPaneIDs[s.PaneID] = true
		}
	}
	anyBoundPresent := false
	for _, p := range live {
		if boundPaneIDs[p.ID] {
			anyBoundPresent = true
			break
		}
	}

	// exemptPaneIDs gates ONLY which untracked panes escape the deterministic
	// reap below; anyBoundPresent above stays computed from real strand
	// bindings alone (see this function's doc comment).
	exemptPaneIDs := make(map[string]bool, len(boundPaneIDs)+1)
	for id := range boundPaneIDs {
		exemptPaneIDs[id] = true
	}
	if headerPaneID != "" {
		exemptPaneIDs[headerPaneID] = true
	}

	if anyBoundPresent {
		for _, p := range live {
			if !exemptPaneIDs[p.ID] && !killSet[p.ID] && p.ID != keptDeadPaneID {
				killSet[p.ID] = true
				panesToKill = append(panesToKill, p.ID)
			}
		}
	}

	for _, s := range strands {
		if s.PaneID == "" {
			continue
		}
		p, present := liveByID[s.PaneID]
		if !present || killSet[p.ID] {
			clearedGUIDs = append(clearedGUIDs, s.GUID)
		}
		// present and not being killed (including the kept dead pane):
		// binding stays, so render still places it.
	}

	return clearedGUIDs, panesToKill, keptDeadPaneID
}

// clearAllPaneBindings clears every strand's PaneID after server rebirth.
// Prevents stale bindings from colliding with reborn pane ids.
func clearAllPaneBindings(st *ReedState) {
	for i := range st.Strands {
		st.Strands[i].PaneID = ""
	}
}

// reconcileLocked reconciles the persisted table against live panes.
// Kills panes per planReconcile's schedule; clears bindings for gone panes.
func (e *Engine) reconcileLocked(st *ReedState, live []LivePane) (killed []string, err error) {
	clearedGUIDs, panesToKill, _ := planReconcile(st.Strands, live, st.HeaderPaneID)

	for _, id := range panesToKill {
		if err := e.tmux.run("kill-pane", "-t", id); err != nil {
			return killed, fmt.Errorf("kill pane %s: %w", id, err)
		}
		killed = append(killed, id)
	}

	clearSet := make(map[string]bool, len(clearedGUIDs))
	for _, g := range clearedGUIDs {
		clearSet[g] = true
	}
	for i := range st.Strands {
		if clearSet[st.Strands[i].GUID] {
			st.Strands[i].PaneID = ""
		}
	}

	return killed, nil
}
