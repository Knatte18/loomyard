// apply.go implements the render -> select-layout/select-pane engine op:
// planLayout is the pure half that maps the persisted strand table down to
// render.Rules and computes the layout string + focus target, and
// applyLayoutLocked composes that plan with the psmux apply I/O. Reconcile
// (reconcile.go) must run before this — kill dead -> re-enumerate live ->
// compute layout -> apply — so live reflects psmux's actual pane set at
// render time; this file makes no reconcile decisions itself.

package muxengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// liveIDSet builds the set of pane ids present in live — alive or
// dead-but-remain-on-exit — matching toRenderStrands' Live derivation:
// "present in the window", not "still running". Render uses this set because
// select-layout must enumerate every pane psmux still holds, dead ones
// included.
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
// strand just because psmux still lists its (dead) pane.
func aliveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		if !p.Dead {
			ids[p.ID] = true
		}
	}
	return ids
}

// paneIDsByTop returns live's pane ids sorted by vertical position
// (pane_top), top first — the window's actual top-to-bottom pane order,
// which is the order psmux applies layout cells against (see render.Rules'
// paneOrder contract). The sort is stable so panes reporting the same top
// (which psmux does not produce for a vertical stack, but a corrupt
// snapshot might) keep list-panes order.
func paneIDsByTop(live []LivePane) []string {
	sorted := make([]LivePane, len(live))
	copy(sorted, live)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Top < sorted[j].Top })

	ids := make([]string, len(sorted))
	for i, p := range sorted {
		ids[i] = p.ID
	}
	return ids
}

// planLayout computes the tmux window_layout string and focus pane id that
// applyLayoutLocked would apply for st's current strand table against live,
// without touching psmux. It maps every strand into render's projection via
// toRenderStrands (render, not the engine, drops not-live/pane-less/hidden
// strands from placement — GAP B, card 6) and calls render.Rules with
// keyed struct fields, handing it live's actual top-to-bottom pane order so
// the emitted cells land on the panes they were sized for.
func (e *Engine) planLayout(st *MuxState, live []LivePane) (layout, focus string, err error) {
	strands := toRenderStrands(st.Strands, liveIDSet(live))
	return render.Rules(strands, render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}, render.Params{
		TopBandRows:        e.cfg.TopBandRows,
		CollapsedStripRows: e.cfg.CollapsedStripRows,
		MinFullRows:        e.cfg.MinFullRows,
	}, paneIDsByTop(live))
}

// anyPlacedStrand reports whether at least one strand would be placed by
// render.Rules against the given present-pane set: a non-hidden strand whose
// PaneID names a pane still present in the window (matching
// partitionByAnchor's filter). applyLayoutLocked uses this to refuse to apply
// a layout that enumerates ZERO panes: psmux accepts an empty-cell layout
// string (exit 0) and answers it by destroying every pane in the session,
// leaving a zero-pane zombie session no later add can host a strand in
// (verified live — an `up` in a session holding only foreign/operator panes
// wiped them all and wedged the session).
func anyPlacedStrand(strands []Strand, presentIDs map[string]bool) bool {
	for _, s := range strands {
		if s.Display.Anchor != render.AnchorHidden && s.PaneID != "" && presentIDs[s.PaneID] {
			return true
		}
	}
	return false
}

// applyLayoutLocked renders the current strand table into a tmux
// window_layout string and applies it via select-layout, then focuses the
// resolved focus pane via select-pane. It assumes the op lock is already
// held and that reconcile has already run against live (this function
// makes no reconcile decisions of its own). When live has fewer than two
// panes it skips both psmux calls entirely, since a single pane already
// fills the window and select-layout/select-pane would be a needless round
// trip. It also skips them when no strand owns a present pane: the layout
// string would then enumerate zero panes, which psmux answers by destroying
// the session's entire pane set (see anyPlacedStrand) — with nothing of
// mux's to lay out, there is nothing worth destroying foreign panes over.
func (e *Engine) applyLayoutLocked(st *MuxState, live []LivePane) error {
	layout, focus, err := e.planLayout(st, live)
	if err != nil {
		return fmt.Errorf("plan layout: %w", err)
	}

	if len(live) < 2 {
		return nil
	}
	if !anyPlacedStrand(st.Strands, liveIDSet(live)) {
		return nil
	}

	session := e.SessionName()
	if err := e.psmux.run("select-layout", "-t", session, layout); err != nil {
		return fmt.Errorf("select-layout: %w", err)
	}
	if focus == "" {
		return nil
	}
	if err := e.psmux.run("select-pane", "-t", focus); err != nil {
		return fmt.Errorf("select-pane: %w", err)
	}
	return nil
}
