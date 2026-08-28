// apply.go implements the render -> select-layout/select-pane engine op: planLayout is the pure
// half that maps the persisted strand table down to render.Rules and computes the layout string +
// focus target,
// and applyLayoutLocked composes that plan with the tmux apply I/O.
// Reconcile (reconcile.go) must run before this — kill dead -> re-enumerate live -> compute layout
// -> apply — so live reflects tmux's actual pane set at render time;
// this file makes no reconcile decisions itself.

package reedengine

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// liveIDSet builds the set of pane ids present in live — alive or
// dead-but-remain-on-exit — matching toRenderStrands' Live derivation:
// "present in the window", not "still running". Render uses this set because
// select-layout must enumerate every pane tmux still holds, dead ones
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

// paneIDsByTop returns live's pane ids sorted by vertical position
// (pane_top), top first — the window's actual top-to-bottom pane order,
// which is the order tmux applies layout cells against (see render.Rules'
// paneOrder contract). The sort is stable so panes reporting the same top
// (which tmux does not produce for a vertical stack, but a corrupt
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

// renderInputs is the single mapping from persisted state plus the live pane set down to the
// arguments the render package takes: the strand table, the height-policy params (including the
// header, blanked when its pane is no longer present), and the physical pane order. Both planLayout
// and fixedHeightPins are built on toRenderInputs and never compute this mapping themselves, so the
// two can never disagree about which header id — or which strand set — they are laying out.
type renderInputs struct {
	strands   []render.Strand
	params    render.Params
	paneOrder []string
}

// toRenderInputs performs the persisted-state-to-render mapping exactly once: it filters st.Strands
// to the present pane set, blanks st.HeaderPaneID when the header pane is not present, assembles the
// render.Params this engine's config implies, and orders live's pane ids top to bottom. It touches no
// tmux and queries nothing of its own — box and live are told to it by the caller, matching
// planLayout's own told-box contract.
func (e *Engine) toRenderInputs(st *ReedState, live []LivePane) renderInputs {
	presentIDs := liveIDSet(live)
	strands := toRenderStrands(st.Strands, presentIDs)
	headerPaneID := st.HeaderPaneID
	if !presentIDs[headerPaneID] {
		headerPaneID = ""
	}
	return renderInputs{
		strands: strands,
		params: render.Params{
			CollapsedStripRows: e.cfg.CollapsedStripRows,
			MinFullRows:        e.cfg.MinFullRows,
			Header:             render.Header{PaneID: headerPaneID, HeightRows: e.cfg.Header.HeightRows},
		},
		paneOrder: paneIDsByTop(live),
	}
}

// planLayout computes the tmux window_layout string and focus pane id for
// st's current strand table against live, within box, without touching tmux
// itself: box is always told to it by the caller, and it queries nothing of
// its own. The persisted-state-to-render mapping lives in toRenderInputs,
// which fixedHeightPins below shares, so the layout and the pin path can
// never be computed from a different header id than each other.
//
// The two callers pass two different box sources: applyLayoutLocked passes
// e.liveBoxLocked()'s live tmux window query (falling back to the configured
// box on failure), while AttachArgv (batch 2) passes the box it computes from
// the attaching client's own told terminal size and never calls
// liveBoxLocked — see the Shared Decision told-box-wins-live-query-is-the-fallback.
func (e *Engine) planLayout(st *ReedState, live []LivePane, box render.Box) (layout, focus string, err error) {
	in := e.toRenderInputs(st, live)
	return render.Rules(in.strands, box, in.params, in.paneOrder)
}

// fixedHeightPins reports the panes whose heights are absolute row budgets — the header band and
// every collapsed strip — for st's current strand table against live, within box. It calls
// toRenderInputs and queries nothing of its own: box is told to it by the caller exactly as
// planLayout is, and it must always be called with the same st, live and box triple the layout for
// that same call was planned from, so the pins it returns never disagree with what was actually laid
// out.
func (e *Engine) fixedHeightPins(st *ReedState, live []LivePane, box render.Box) []render.Pin {
	in := e.toRenderInputs(st, live)
	return render.FixedHeightPins(in.strands, box, in.params)
}

// anyPlacedStrand reports whether at least one strand would be placed by
// render.Rules against the given present-pane set: a non-hidden strand whose
// PaneID names a pane still present in the window (matching
// partitionByAnchor's filter). applyLayoutLocked uses this to refuse to apply
// a layout that enumerates ZERO panes: tmux accepts an empty-cell layout
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
// panes it skips both tmux calls entirely, since a single pane already
// fills the window and select-layout/select-pane would be a needless round
// trip. It also skips them when no strand owns a present pane: the layout
// string would then enumerate zero panes, which tmux answers by destroying
// the session's entire pane set (see anyPlacedStrand) — with nothing of
// reed's to lay out, there is nothing worth destroying foreign panes over.
//
// The live box query (e.liveBoxLocked) runs only after both guards above have
// passed, not as an argument evaluated up front: liveBoxLocked is a real
// display-message round trip, and reconcileApplyPersistLocked (spawn.go) runs
// this function once per launch on Resume, so evaluating it eagerly would
// fire a wasted tmux call, repeated per strand, on exactly the degenerate
// paths this function skips. This also makes this call site agree with
// AttachArgv's ordering (batch 2), which evaluates the same two guards before
// it plans.
//
// select-layout with a layout string whose dimensions disagree with the live
// window exits 0 and silently rescales the layout proportionally, so every
// absolute row budget reed computes (Header.HeightRows, CollapsedStripRows,
// MinFullRows) was being scaled by live_height/cfg.Height on any window that
// is not exactly cfg.Height rows tall — this is why the box passed to
// planLayout below is always the live one, not the configured one.
//
// While detached, an over-budget layout string is accepted by select-layout
// and answered by GROWING the window to fit the cells, so a session with no
// client can end up taller than its configured boot height until the next
// client attaches and snaps it back — a consequence of the live-box query,
// not a bug.
//
// tmux redistributes every window-size delta evenly across the vertical cells and has no fixed-height
// pane concept, so no absolute row budget survives a resize on its own. This function therefore
// re-installs a window-resized hook re-pinning the fixed-height panes (the header band and every
// collapsed strip) after each successful apply, so a later client resize is corrected without a
// second reed operation.
//
// The guard-skip disposition is the opposite of the zero-pin one, and deliberately so: a path
// returning at either guard above issues NOTHING — not even the clear — so a previously installed
// array survives it, with no removal path. len(live) < 2 is harmless because resize-pane -y against a
// window's sole pane is a silent no-op — verified live on tmux 3.6, exit 0 with the pane's height
// unchanged — so the surviving header pin cannot contradict render.Rules' sole-cell branch even
// though it now names the only pane in the window. !anyPlacedStrand is the reachable, long-lived one:
// state.go documents an operator remedy that deletes reed.json while the session and its processes
// keep running untracked, after which anyPlacedStrand is false forever, and there the surviving array
// is a benefit — it keeps pinning the still-alive header and strips at the budgets reed last computed
// for them. Clearing ahead of the guards was considered and rejected: it would strip the pins from
// exactly that untracked-but-running session, and a clear with no rebuild behind it drifts on the
// very next resize, which is strictly worse than a slightly stale array.
func (e *Engine) applyLayoutLocked(st *ReedState, live []LivePane) error {
	if len(live) < 2 {
		return nil
	}
	if !anyPlacedStrand(st.Strands, liveIDSet(live)) {
		return nil
	}

	box := e.liveBoxLocked()
	layout, focus, err := e.planLayout(st, live, box)
	if err != nil {
		return fmt.Errorf("plan layout: %w", err)
	}

	session := e.SessionName()
	if err := e.tmux.run("select-layout", "-t", exactSessionWindowTarget(session), layout); err != nil {
		return fmt.Errorf("select-layout: %w", err)
	}
	e.installResizePinsLocked(e.fixedHeightPins(st, live, box))
	if focus == "" {
		return nil
	}
	if err := e.tmux.run("select-pane", "-t", focus); err != nil {
		return fmt.Errorf("select-pane: %w", err)
	}
	return nil
}
