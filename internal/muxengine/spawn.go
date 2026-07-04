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

// planPaneTarget decides how the next strand realization obtains its pane:
// adopt an existing pane (adoptID != "") or split off a target pane
// (splitTargetID != ""). Exactly one of the two is non-empty on success.
//
// Adoption is the fresh-session case: no strand currently holds a pane
// binding AND an alive (present, not pane_dead) pane exists — the first
// alive pane in live, i.e. the new-session initial shell pane. A dead pane
// is never adopted: psmux's display-message happily names a pane_dead=1
// corpse as the active pane, and send-keys into a corpse exits 0 while
// running nothing, so blind adoption silently swallows the strand's command.
//
// Every other realization splits. The split target is the tallest alive
// pane (the stretch/active pane under mux's height policy, so always the
// most splittable): psmux's split-window on a too-small pane fails
// SILENTLY — exit 0, no new pane, and it prints an existing pane's id — so
// the target must never be left to whatever pane happens to be active.
// When no pane is alive (only the sole kept pane_dead corpse remains), the
// tallest present pane is the target — splitting a dead pane works, and the
// caller's reconcile tail reaps the corpse once the new pane is alive.
// A session with no panes at all cannot host a strand and is an error.
func planPaneTarget(strands []Strand, live []LivePane) (adoptID, splitTargetID string, err error) {
	if len(live) == 0 {
		return "", "", fmt.Errorf("session has no panes to adopt or split")
	}

	anyBound := false
	for _, s := range strands {
		if s.PaneID != "" {
			anyBound = true
			break
		}
	}
	if !anyBound {
		for _, p := range live {
			if !p.Dead {
				return p.ID, "", nil
			}
		}
	}

	splitTargetID = ""
	tallestAlive := -1
	for _, p := range live {
		if !p.Dead && p.Height > tallestAlive {
			tallestAlive = p.Height
			splitTargetID = p.ID
		}
	}
	if splitTargetID == "" {
		// Every pane is dead: split off the (kept) corpse.
		splitTargetID = live[0].ID
	}
	return "", splitTargetID, nil
}

// launchStrandLocked realizes s into a live psmux pane and runs launchCmd
// in it: it adopts the session's initial new-session pane when no other
// strand currently holds a pane binding and that pane is alive, or splits
// the tallest alive pane otherwise (planPaneTarget decides which), captures
// the resulting pane id into s.PaneID, and sends launchCmd via send-keys.
// A split whose reported pane id is not genuinely new (psmux's silent
// too-small-to-split failure prints an existing pane's id with exit 0) is a
// hard error — recording it would bind two strands to one pane, and the
// next select-layout string would then carry a duplicate pane number, which
// psmux answers by destroying the session's panes wholesale. It assumes the
// op lock is already held. This is the single realization path
// add/surface/resume all share (GAP A); it always makes a real psmux round
// trip, so no hermetic test calls it directly with a non-hidden/surfacing
// strand.
//
// s.PaneID is the only field this sets — there is no persisted "live" flag
// to set alongside it (Strand carries none): once PaneID is populated and
// the next list-panes enumerates it, toRenderStrands derives Live true from
// that binding downstream, the same way reconcile's binding-clear alone
// (without a stored flag) derives Live false.
func (e *Engine) launchStrandLocked(st *MuxState, s *Strand, launchCmd string) error {
	session := e.SessionName()

	live, err := e.psmux.listPanes(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	adoptID, splitTargetID, err := planPaneTarget(st.Strands, live)
	if err != nil {
		return err
	}

	paneID := adoptID
	if paneID == "" {
		out, err := e.psmux.output("split-window", "-t", splitTargetID, "-P", "-F", "#{pane_id}")
		if err != nil {
			return fmt.Errorf("split window: %w", err)
		}
		paneID = strings.TrimSpace(out)
		if paneID == "" || liveIDSet(live)[paneID] {
			return fmt.Errorf("split-window created no new pane (got %q; target %s likely too small to split)", paneID, splitTargetID)
		}
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
