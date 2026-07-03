// spawn.go implements the shared pane-launch helper every strand-realizing
// path composes: AddStrand launching a freshly added strand, UpdateStrand
// surfacing a hidden->visible strand, and Resume replaying a not-live
// strand all call launchStrandLocked to actually create (or adopt) a psmux
// pane and run the strand's command in it (GAP A) — without this shared
// helper, add would register a record and re-render but never create a
// pane or run anything. This file also carries the two other small
// cross-file bootstrap helpers strand.go and lifecycle.go both need:
// loadOrInitStateLocked (fresh-worktree state bootstrap) and
// reconcileApplyPersistLocked (the reconcile-then-apply-then-persist tail
// every public op ends with).

package muxengine

import (
	"fmt"
	"strings"
)

// planLaunch reports whether the next strand realized into a pane should
// adopt the session's initial new-session pane rather than split a fresh
// one. psmux's new-session always creates one initial shell pane; the
// first strand into a fresh session adopts it (no other strand in st
// currently holds a live pane binding), and every subsequent strand splits
// a new pane. This is pure so the decision is unit-testable without psmux.
func planLaunch(st *MuxState) (adopt bool) {
	for _, s := range st.Strands {
		if s.PaneID != "" {
			return false
		}
	}
	return true
}

// launchStrandLocked realizes s into a live psmux pane and runs launchCmd
// in it: it adopts the session's initial new-session pane when no other
// strand currently holds a live pane (planLaunch decides which), or splits
// a fresh pane otherwise, captures the resulting pane id into s.PaneID, and
// sends launchCmd via send-keys. It assumes the op lock is already held.
// This is the single realization path add/surface/resume all share (GAP
// A); it always makes a real psmux round trip, so no hermetic test calls it
// directly with a non-hidden/surfacing strand.
//
// s.PaneID is the only field this sets — there is no persisted "live" flag
// to set alongside it (Strand carries none): once PaneID is populated and
// the next list-panes enumerates it, toRenderStrands derives Live true from
// that binding downstream, the same way reconcile's binding-clear alone
// (without a stored flag) derives Live false.
func (e *Engine) launchStrandLocked(st *MuxState, s *Strand, launchCmd string) error {
	session := e.SessionName()

	var paneID string
	if planLaunch(st) {
		id, err := e.psmux.activePaneID(session)
		if err != nil {
			return fmt.Errorf("adopt new-session pane: %w", err)
		}
		paneID = id
	} else {
		out, err := e.psmux.output("split-window", "-t", session, "-P", "-F", "#{pane_id}")
		if err != nil {
			return fmt.Errorf("split window: %w", err)
		}
		paneID = strings.TrimSpace(out)
	}

	s.PaneID = paneID
	// Send the command as a literal string (-l) so psmux never reinterprets
	// any part of the opaque launchCmd as a key name (e.g. "Enter", "C-c") or
	// splits it on an embedded ';' — the caller (shuttle) builds arbitrary
	// PowerShell command chains. A separate Enter then submits it.
	if err := e.psmux.run("send-keys", "-t", paneID, "-l", launchCmd); err != nil {
		return fmt.Errorf("send launch command: %w", err)
	}
	if err := e.psmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("submit launch command: %w", err)
	}
	return nil
}

// loadOrInitStateLocked loads the persisted MuxState, or initializes a
// fresh one stamped with this engine's server/socket/session identity when
// no mux.json exists yet (a brand-new worktree's first mutation). Assumes
// the op lock is already held.
func (e *Engine) loadOrInitStateLocked() (*MuxState, error) {
	st, err := LoadState(e.layout.DotLyxDir())
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if st == nil {
		st = &MuxState{
			Server:  e.Socket(),
			Socket:  e.Socket(),
			Session: e.SessionName(),
		}
	}
	return st, nil
}

// reconcileApplyPersistLocked runs the shared tail every public engine op
// composes after its own mutation: fetch the current live pane set,
// reconcile (clears dead-pane bindings, keeps records, kills
// dead-except-sole), re-apply the layout against what remains, then
// persist. It assumes the op lock is already held, and always makes at
// least one live psmux round trip (list-panes, plus a second one when
// reconcile actually killed something, plus select-layout/select-pane
// inside applyLayoutLocked once there are 2+ panes) — so it is the one seam
// every public op's hermetic unit test must not cross. Tests instead
// exercise the pure planning helpers (planReconcile, planLayout) and the
// mutation-only *Locked helpers (addStrandLocked, updateStrandLocked,
// removeStrandLocked) directly. Returns the final live pane set so a
// caller that needs it for reporting (Status) does not have to re-query.
func (e *Engine) reconcileApplyPersistLocked(st *MuxState) ([]LivePane, error) {
	session := e.SessionName()
	live, err := e.psmux.listPanes(session)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}

	killed, err := e.reconcileLocked(st, live)
	if err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	if len(killed) > 0 {
		// Order matters: kill dead -> re-enumerate live -> compute layout
		// -> apply. The kill-pane calls above mutate the pane set the next
		// select-layout must enumerate, so enumeration must follow them.
		live, err = e.psmux.listPanes(session)
		if err != nil {
			return nil, fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	if err := e.applyLayoutLocked(st, live); err != nil {
		return nil, fmt.Errorf("apply layout: %w", err)
	}
	if err := SaveState(e.layout.DotLyxDir(), st); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return live, nil
}
