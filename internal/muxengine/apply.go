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

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// liveIDSet builds the set of pane ids present in live — alive or
// dead-but-remain-on-exit — matching toRenderStrands' Live derivation:
// "present in the window", not "still running".
func liveIDSet(live []LivePane) map[string]bool {
	ids := make(map[string]bool, len(live))
	for _, p := range live {
		ids[p.ID] = true
	}
	return ids
}

// planLayout computes the tmux window_layout string and focus pane id that
// applyLayoutLocked would apply for st's current strand table against live,
// without touching psmux. It maps every strand into render's projection via
// toRenderStrands (render, not the engine, drops not-live/pane-less/hidden
// strands from placement — GAP B, card 6) and calls render.Rules with
// keyed struct fields.
func (e *Engine) planLayout(st *MuxState, live []LivePane) (layout, focus string, err error) {
	strands := toRenderStrands(st.Strands, liveIDSet(live))
	return render.Rules(strands, render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}, render.Params{
		TopBandRows:        e.cfg.TopBandRows,
		CollapsedStripRows: e.cfg.CollapsedStripRows,
		MinFullRows:        e.cfg.MinFullRows,
	})
}

// applyLayoutLocked renders the current strand table into a tmux
// window_layout string and applies it via select-layout, then focuses the
// resolved focus pane via select-pane. It assumes the op lock is already
// held and that reconcile has already run against live (this function
// makes no reconcile decisions of its own). When live has fewer than two
// panes it skips both psmux calls entirely, since a single pane already
// fills the window and select-layout/select-pane would be a needless round
// trip.
func (e *Engine) applyLayoutLocked(st *MuxState, live []LivePane) error {
	layout, focus, err := e.planLayout(st, live)
	if err != nil {
		return fmt.Errorf("plan layout: %w", err)
	}

	if len(live) < 2 {
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
