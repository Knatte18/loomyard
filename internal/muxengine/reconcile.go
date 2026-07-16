// reconcile.go implements the reconcile-against-live-panes engine op: the
// pure planning function planReconcile decides which strand pane bindings
// to clear and which dead panes to kill, and reconcileLocked composes that
// plan with the tmux kill I/O. Every public engine op runs reconcile
// first, under the op lock, so the persisted table never drifts from what
// tmux's list-panes actually reports.

package muxengine

import "fmt"

// planReconcile is the pure planning half of reconcile: given the current
// strand table and the live pane set list-panes just reported, it decides
// which strands' pane bindings must be cleared and which panes must be
// killed before the layout is re-applied. It never touches tmux itself, so
// the decision logic is unit-testable without a running server.
//
// The kill-before-apply rule keeps the rendered window_layout string
// consistent with tmux's actual pane set: a pane_dead=1 pane still
// occupies a slot in list-panes' output, so leaving it un-killed while
// excluding its strand from the layout would make the layout string
// enumerate fewer panes than tmux still holds (GAP2). The one exception is
// enforced by the session-survival rule: at least one pane must always
// survive, since tmux offers no clean way to empty a window (under
// remain-on-exit a last-pane kill corpses an alive pane rather than
// refusing, and can end the session for a dead one — mux never asks). When
// any pane is still alive, every dead pane is killable. When every pane is
// dead, exactly one dead pane is kept (keptDeadPane) so the session — and
// that strand's still-rebuildable record — survives until resume/remove; its
// strand's binding is NOT cleared, since the pane is still present and render
// must still place it. keptDeadPane (a pane id, non-empty only when a dead
// pane is deliberately spared) names it, so callers/tests can assert on the
// exception explicitly rather than inferring it from panesToKill.
//
// UNTRACKED panes — live panes no strand owns (an operator's raw
// split-window into the mux session, or an orphan from a mid-op crash
// before persist) — are also scheduled for killing, but only while mux owns
// content in the window (>=1 strand bound to a present pane). Killing them
// here, deterministically, replaces relying on select-layout's positional
// reaping: tmux assigns layout cells to panes in window order and destroys
// whichever panes sit BEYOND the emitted cell count, so with a foreign pane
// present the reaped victim is positional — observed live to destroy a
// TRACKED strand's pane while the foreign pane survived. With no bound
// content at all, foreign panes are left untouched (mux has nothing to lay
// out — the apply is skipped too, see anyPlacedStrand).
//
// For every other strand: (a) a strand whose PaneID is absent from live, or
// whose pane was just scheduled for killing, has its GUID returned in
// clearedGUIDs — the record survives (only RemoveStrand deletes one), but
// its binding is gone so it renders as not-live; (b) a strand whose pane is
// present and not being killed keeps its binding untouched — Live derives
// true for it downstream, via toRenderStrands' liveIDs lookup.
//
// headerPaneID names the always-present header pane (empty when none is
// tracked yet). It is deliberately NEVER folded into boundPaneIDs — that map
// also gates anyBoundPresent below, and folding the header in would make
// anyBoundPresent true whenever the header is merely live, reaping
// operator/foreign panes even with zero strands bound and breaking the
// documented "no bound content, foreign panes untouched" invariant. Instead
// it is added ONLY to a separate exemptPaneIDs set that the untracked-pane
// reap loop below checks, so the header pane itself is never killed as an
// "untracked" pane while still never counting toward "mux owns bound
// content" (Shared Decision header-is-not-a-strand).
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

	killSet := make(map[string]bool, len(live))
	for _, p := range live {
		if p.Dead && p.ID != keptDeadPaneID {
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

// clearAllPaneBindings drops every strand's PaneID. It is used after a
// session is freshly booted (server rebirth): tmux restarts pane numbering
// from %0/%1, so a persisted binding can collide with a reborn pane id and be
// mistaken for a live strand by reconcile. A just-booted session hosts none
// of the prior strands, so every binding is stale by definition.
func clearAllPaneBindings(st *MuxState) {
	for i := range st.Strands {
		st.Strands[i].PaneID = ""
	}
}

// reconcileLocked reconciles the persisted table against tmux's live pane
// set: it kills each pane planReconcile schedules (dead-but-not-sole panes,
// plus untracked panes while mux owns bound content), then clears the
// PaneID of every strand whose pane is gone or was just killed (keeping the
// record — only RemoveStrand deletes one). It assumes the op lock is
// already held by the caller, mutates st in place, and returns the pane ids
// actually killed so the caller can re-derive the post-kill live set
// without a second list-panes round trip.
func (e *Engine) reconcileLocked(st *MuxState, live []LivePane) (killed []string, err error) {
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
