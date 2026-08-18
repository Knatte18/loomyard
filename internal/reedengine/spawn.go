// spawn.go implements the shared pane-launch helper every strand-realizing path composes: AddStrand
// launching a freshly added strand, UpdateStrand surfacing a hidden->visible strand, and Resume
// replaying a not-live strand all call launchStrandLocked to actually create (or adopt) a tmux pane
// and run the strand's command in it (GAP A) — without this shared helper, add would register a
// record and re-render but never create a pane or run anything.
// This file also carries the two other small cross-file bootstrap helpers strand.go and
// lifecycle.go both need: loadOrInitStateLocked (fresh-worktree state bootstrap) and
// reconcileApplyPersistLocked (the reconcile-then-apply-then-persist tail every public op ends
// with).

package reedengine

import (
	"fmt"
	"strings"
)

// planPaneTarget decides how the next strand realization obtains its pane:
// adopt an existing alive pane when no strand holds a binding, or split
// the tallest alive non-header pane otherwise. Exactly one of adoptID or
// splitTargetID is non-empty on success.
func planPaneTarget(strands []Strand, live []LivePane, headerPaneID string) (adoptID, splitTargetID string, err error) {
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
			if p.ID == headerPaneID {
				continue
			}
			if !p.Dead {
				return p.ID, "", nil
			}
		}
	}

	splitTargetID = ""
	tallestAlive := -1
	for _, p := range live {
		if p.ID == headerPaneID || p.Dead {
			continue
		}
		if p.Height > tallestAlive {
			tallestAlive = p.Height
			splitTargetID = p.ID
		}
	}
	if splitTargetID == "" {
		// No alive non-header pane: fall back to any present non-header
		// pane (a dead corpse), mirroring the pre-header "every pane dead"
		// fallback.
		for _, p := range live {
			if p.ID != headerPaneID {
				splitTargetID = p.ID
				break
			}
		}
	}
	if splitTargetID == "" {
		// No non-header pane exists at all: every strand has been removed
		// and only the header remains. Split the header itself so this add
		// still has a pane to split.
		splitTargetID = live[0].ID
	}
	return "", splitTargetID, nil
}

// validateSplitCreatedNewPane returns an error unless paneID is genuinely
// new — absent from preSplitLive. psmux's split-window on a too-small pane
// fails silently and prints an existing pane's id, which would bind two
// owners to one pane.
func validateSplitCreatedNewPane(paneID string, preSplitLive []LivePane, target string) error {
	if paneID == "" || liveIDSet(preSplitLive)[paneID] {
		return fmt.Errorf("split-window created no new pane (got %q; target %s likely too small to split)", paneID, target)
	}
	return nil
}

// sendKeysLiteralArg returns the argument tmux send-keys -l must receive
// so text types verbatim. tmux parses dash-leading arguments as flags,
// silently dropping them, so a dash-leading cmd must be prefixed with space.
func sendKeysLiteralArg(text string) string {
	if strings.HasPrefix(text, "-") {
		return " " + text
	}
	return text
}

// launchStrandLocked realizes s into a live tmux pane and runs launchCmd in it,
// adopting the initial pane or splitting the tallest alive one. It validates
// the split created a new pane and sets only s.PaneID; Live is derived from
// pane binding downstream.
func (e *Engine) launchStrandLocked(st *ReedState, s *Strand, launchCmd string) error {
	session := e.SessionName()

	live, err := e.tmux.listPanes(session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	adoptID, splitTargetID, err := planPaneTarget(st.Strands, live, st.HeaderPaneID)
	if err != nil {
		return err
	}

	paneID := adoptID
	if paneID == "" {
		// -c pins the new pane's cwd to the told anchor, exactly as
		// new-session and the header split (lifecycle.go) already do. Without
		// it tmux resolves the cwd from the invoking CLIENT — verified live
		// (tmux 3.6): a split issued from outside tmux lands in the calling
		// process's cwd, neither the target pane's cwd nor the session's. That
		// happens to be the anchor whenever lyx runs under lyxcwd.Resolve's
		// exact-equality cwd gate, and stops being the anchor the moment a
		// caller injects a cwd through the RunCLIIn seam instead — at which
		// point every strand command would run against the wrong tree while
		// reed reported success.
		out, err := e.tmux.output("split-window", "-t", splitTargetID, "-c", e.geom.AnchorPath, "-P", "-F", "#{pane_id}")
		if err != nil {
			return fmt.Errorf("split window: %w", err)
		}
		paneID = strings.TrimSpace(out)
		if err := validateSplitCreatedNewPane(paneID, live, splitTargetID); err != nil {
			return err
		}
	}

	s.PaneID = paneID
	// Send the command as a literal string (-l) so tmux never reinterprets
	// any part of the opaque launchCmd as a key name (e.g. "Enter", "C-c") or
	// splits it on an embedded ';' — the caller (shuttle) builds arbitrary
	// PowerShell command chains. A separate Enter then submits it.
	if err := e.tmux.run("send-keys", "-t", paneID, "-l", sendKeysLiteralArg(launchCmd)); err != nil {
		return fmt.Errorf("send launch command: %w", err)
	}
	if err := e.tmux.run("send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("submit launch command: %w", err)
	}
	return nil
}

// loadOrInitStateLocked loads or initializes a ReedState stamped with
// the engine's server/socket/session identity for a fresh worktree.
func (e *Engine) loadOrInitStateLocked() (*ReedState, error) {
	st, err := LoadState(e.stateDir())
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if st == nil {
		st = &ReedState{
			Socket:  e.Socket(),
			Session: e.SessionName(),
		}
	}
	return st, nil
}

// reconcileApplyPersistLocked fetches current panes, reconciles dead
// pane bindings, reapplies the layout, and persists the state. It is the
// shared tail every public op composes after mutation.
func (e *Engine) reconcileApplyPersistLocked(st *ReedState) ([]LivePane, error) {
	session := e.SessionName()
	live, err := e.tmux.listPanes(session)
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
		live, err = e.tmux.listPanes(session)
		if err != nil {
			return nil, fmt.Errorf("list panes after reconcile: %w", err)
		}
	}

	if err := e.applyLayoutLocked(st, live); err != nil {
		return nil, fmt.Errorf("apply layout: %w", err)
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return live, nil
}
