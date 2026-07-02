// reconcile.go implements the reconcile-against-live-panes engine op: the
// pure planning function planReconcile decides which strand pane bindings
// to clear and which dead panes to kill, and reconcileLocked composes that
// plan with the psmux kill I/O. Every public engine op runs reconcile
// first, under the op lock, so the persisted table never drifts from what
// psmux's list-panes actually reports.

package muxengine

import "fmt"

// planReconcile is the pure planning half of reconcile: given the current
// strand table and the live pane set list-panes just reported, it decides
// which strands' pane bindings must be cleared and which dead panes must be
// killed before the layout is re-applied. It never touches psmux itself, so
// the decision logic is unit-testable without a running server.
//
// The kill-before-apply rule keeps the rendered window_layout string
// consistent with psmux's actual pane set: a pane_dead=1 pane still
// occupies a slot in list-panes' output, so leaving it un-killed while
// excluding its strand from the layout would make the layout string
// enumerate fewer panes than psmux still holds (GAP2). The one exception is
// a sole-remaining dead pane — killing the session's last pane ends the
// session outright — so that pane is deliberately left un-killed, and its
// strand's binding is NOT cleared: it is still present, so render must
// still place it. solePane (a pane id, non-empty only in that one case)
// names it, so callers/tests can assert on the exception explicitly rather
// than inferring it from an empty deadToKill.
//
// For every other strand: (a) a strand whose PaneID is absent from live, or
// whose pane was just scheduled for killing, has its GUID returned in
// clearedGUIDs — the record survives (only RemoveStrand deletes one), but
// its binding is gone so it renders as not-live; (b) a strand whose pane is
// present and not being killed keeps its binding untouched — Live derives
// true for it downstream, via toRenderStrands' liveIDs lookup.
func planReconcile(strands []Strand, live []LivePane) (clearedGUIDs []string, deadToKill []string, solePane string) {
	liveByID := make(map[string]LivePane, len(live))
	for _, p := range live {
		liveByID[p.ID] = p
	}

	// A dead pane is "sole-remaining" only when it is the single pane in
	// the whole window — killing it would end the session. A dead pane
	// alongside any other pane (dead or alive) is not sole-remaining and
	// gets killed normally.
	solePaneID := ""
	if len(live) == 1 && live[0].Dead {
		solePaneID = live[0].ID
	}

	killSet := make(map[string]bool, len(live))
	for _, p := range live {
		if p.Dead && p.ID != solePaneID {
			killSet[p.ID] = true
			deadToKill = append(deadToKill, p.ID)
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
		// present and not being killed (including the sole-remaining dead
		// pane): binding stays, Live derives true downstream.
	}

	return clearedGUIDs, deadToKill, solePaneID
}

// reconcileLocked reconciles the persisted table against psmux's live pane
// set: it kills each dead-but-not-sole pane planReconcile identifies, then
// clears the PaneID of every strand whose pane is gone or was just killed
// (keeping the record — only RemoveStrand deletes one). It assumes the op
// lock is already held by the caller, mutates st in place, and returns the
// pane ids actually killed so the caller can re-derive the post-kill live
// set without a second list-panes round trip.
func (e *Engine) reconcileLocked(st *MuxState, live []LivePane) (killed []string, err error) {
	clearedGUIDs, deadToKill, _ := planReconcile(st.Strands, live)

	for _, id := range deadToKill {
		if err := e.psmux.run("kill-pane", "-t", id); err != nil {
			return killed, fmt.Errorf("kill dead pane %s: %w", id, err)
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
