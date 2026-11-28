// reconcile.go implements the reconcile-against-live-panes engine op.
// planReconcile is its pure planning step, and reconcileLocked applies the plan through tmux kill I/O.
// Every public engine op runs reconcile first, under the op lock,
// so the persisted table never drifts from what tmux list-panes reports.

package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// reconcilePlan is the decision of one reconcile pass.
// It names the strand bindings to clear, the dead panes to kill, and the untracked panes to reap.
// It also names the dead pane kept alive so the session survives, and the own windows to end.
// The dead and untracked kills stay in separate lists so the reap log can report each kill reason.
type reconcilePlan struct {
	clearedGUIDs         []string
	deadPanesToKill      []string
	untrackedPanesToKill []string
	keptDeadPane         string
	ownWindowsToEnd      []string
}

// planReconcile decides what a reconcile pass does to the table and the panes.
// It names the bindings to clear, the dead panes to kill, the untracked panes to reap, and the own windows to end.
// live holds window 0's panes, and windows holds the panes of every window in the session.
// The kill decisions and the survival rule read live alone.
// An own-window strand's binding is judged against windows,
// and an own window that no strand holds is ended.
// elsewhere holds the pane ids found in other sessions of the server.
// An own-window strand bound to one of them keeps its binding.
// No pane in another session is ever killed, since the kill sets read live alone.
// Pure logic; unit-testable without a running server.
// Keeps at least one pane alive (session-survival rule); spares Selvage.
func planReconcile(strands []Strand, live []LivePane, windows []windowPane, policy reapPolicy, elsewhere map[string]string) reconcilePlan {
	var plan reconcilePlan

	liveByID := make(map[string]LivePane, len(live))
	for _, p := range live {
		liveByID[p.ID] = p
	}

	// If any pane is still alive, killing every dead pane leaves the session
	// with at least that live pane,
	// so every dead pane is killable.
	// If every pane in the window is dead, one must be spared, since killing the last pane ends the session;
	// keptDeadPaneID names the first dead pane to keep.
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
		if p.Dead && p.ID != keptDeadPaneID && !policy.exemptFromDeadKill(p.ID) {
			killSet[p.ID] = true
			plan.deadPanesToKill = append(plan.deadPanesToKill, p.ID)
		}
	}

	// The untracked-pane reap kills every live pane no strand owns.
	// It runs when some strand is bound to a present pane, or when the policy authorizes it.
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

	// exemptPaneIDs holds the bound strand pane ids alone, and it gates only which untracked panes escape the reap.
	// anyBoundPresent is likewise computed from the strand bindings alone.
	// Selvage's exemption is asked of the policy separately, as its own disjunct, rather than folded into this map.
	exemptPaneIDs := boundPaneIDs

	if anyBoundPresent || policy.authorizesReap() {
		for _, p := range live {
			if !exemptPaneIDs[p.ID] && !policy.exemptFromUntrackedReap(p.ID) && !killSet[p.ID] && p.ID != keptDeadPaneID {
				killSet[p.ID] = true
				plan.untrackedPanesToKill = append(plan.untrackedPanesToKill, p.ID)
			}
		}
	}

	listedByID := make(map[string]bool, len(windows))
	for _, w := range windows {
		listedByID[w.PaneID] = true
	}
	for _, s := range strands {
		if s.PaneID == "" {
			continue
		}
		if isOwnWindow(s.Display) {
			// A present own-window pane keeps its binding, alive or dead, and is never killed here.
			if _, inOtherSession := elsewhere[s.PaneID]; !listedByID[s.PaneID] && !inOtherSession {
				plan.clearedGUIDs = append(plan.clearedGUIDs, s.GUID)
			}
			continue
		}
		p, present := liveByID[s.PaneID]
		if !present || killSet[p.ID] {
			plan.clearedGUIDs = append(plan.clearedGUIDs, s.GUID)
		}
		// present and not being killed (including the kept dead pane):
		// binding stays, so render still places it.
	}

	plan.ownWindowsToEnd = ownWindowsHoldingNoStrand(strands, windows)
	plan.keptDeadPane = keptDeadPaneID
	return plan
}

// ownWindowsHoldingNoStrand lists each own window that holds no strand-bound pane, in listing order and once each.
// A window without the own marker never qualifies.
func ownWindowsHoldingNoStrand(strands []Strand, windows []windowPane) []string {
	bound := make(map[string]bool, len(strands))
	for _, s := range strands {
		if s.PaneID != "" {
			bound[s.PaneID] = true
		}
	}
	held := make(map[string]bool)
	for _, w := range windows {
		if bound[w.PaneID] {
			held[w.WindowID] = true
		}
	}
	var ending []string
	seen := make(map[string]bool)
	for _, w := range windows {
		if w.Own && !held[w.WindowID] && !seen[w.WindowID] {
			seen[w.WindowID] = true
			ending = append(ending, w.WindowID)
		}
	}
	return ending
}

// clearAllPaneBindings clears every strand's PaneID after server rebirth.
// Prevents stale bindings from colliding with reborn pane ids.
func clearAllPaneBindings(st *ReedState) {
	for i := range st.Strands {
		st.Strands[i].PaneID = ""
	}
}

// clearConflictingPaneBindings clears every strand PaneID that names a pane the strand cannot own.
// It returns the GUIDs it cleared, in table order.
// A strand cannot own Selvage's pane, nor a pane an earlier strand in the table already claims.
//
// A pane has exactly one owner.
// reed's own construction paths keep that true, since every strand pane is created new in a window of its own.
// A table that breaks the rule is corrupt, not one reed produced:
// a stale reed.json restored over a newer session, a hand-edited file, or a partially restored backup.
// Every table reaches this package through LoadState.
// The repair therefore runs at that one load point, rather than at each place a duplicate would do damage.
//
// Two shapes of damage follow from a shared pane id:
//   - A strand sharing Selvage's pane id makes one pane answer to two owners, so the strand reports
//     live:true against the Selvage pane.
//   - Two strands sharing one pane id leave the second strand's pane bound to nobody, so the
//     untracked-pane reap kills it, along with the process running in it.
//
// First writer wins, so the repair is deterministic and independent of the order conflicts are noticed.
// Clearing rather than refusing the op makes the repair self-healing:
// a cleared strand is not live, which resume already knows how to rebuild.
// A refusal would wedge the worktree on the corruption this repair exists to survive.
func clearConflictingPaneBindings(st *ReedState) []string {
	claimed := make(map[string]bool, len(st.Strands)+1)
	seedSelvageClaim(st, claimed)

	var clearedGUIDs []string
	for i := range st.Strands {
		paneID := st.Strands[i].PaneID
		if paneID == "" {
			continue
		}
		if claimed[paneID] {
			st.Strands[i].PaneID = ""
			clearedGUIDs = append(clearedGUIDs, st.Strands[i].GUID)
			continue
		}
		claimed[paneID] = true
	}
	return clearedGUIDs
}

// reconcileLocked reconciles the persisted table against live panes.
// It kills the panes the plan schedules and clears bindings for gone panes.
// It also ends the own windows no strand holds.
// A viewer on one of those windows is first moved to the relevant window.
// That window is the newest live agent, else the lowest-index overview, and never the shell.
// Those windows are not in killed, since no window 0 pane went.
// A viewer on any other window is never moved.
func (e *Engine) reconcileLocked(st *ReedState, live []LivePane, windows []windowPane) (killed []string, err error) {
	plan := planReconcile(st.Strands, live, windows, newReapPolicy(st, live), e.strandsElsewhereLocked(st.Strands, windows))

	// deadKilled and untrackedKilled collect the ids actually destroyed, one list per kill reason.
	// The plan's kill lists name what was scheduled, not what a partial kill-pane failure reached,
	// so the log reads these lists instead.
	// The two agree on the success path and diverge only on a mid-loop kill-pane failure.
	// A log that reports a pane destroyed while it is still alive is worse than no log,
	// so this is a record of destruction, not of intent.
	var deadKilled, untrackedKilled []string

	// This is Info, not Debug: a real pane teardown is a lifecycle event, not a probe.
	// It needs a trace at all.
	// The `policy.authorizesReap()` disjunct lets this reap fire even when no strand is bound to a present pane.
	// Such a reap destroys panes an operator may have created themselves.
	//
	// The log is a defer rather than a call before each return,
	// so the success and partial-kill error paths cannot drift apart: both reach this one call.
	// A reconcile that killed nothing logs nothing.
	// Logging here is additive only; it must not swallow or alter the returned error.
	defer func() {
		if len(deadKilled) == 0 && len(untrackedKilled) == 0 {
			return
		}
		logger.Info("reed: reconcile reaped panes",
			"socket", e.Socket(), "session", e.SessionName(),
			"dead_panes_killed", deadKilled, "untracked_panes_killed", untrackedKilled)
	}()

	for _, id := range plan.deadPanesToKill {
		if err := e.tmux.run("kill-pane", "-t", id); err != nil {
			return killed, fmt.Errorf("kill pane %s: %w", id, err)
		}
		killed = append(killed, id)
		deadKilled = append(deadKilled, id)
	}
	for _, id := range plan.untrackedPanesToKill {
		if err := e.tmux.run("kill-pane", "-t", id); err != nil {
			return killed, fmt.Errorf("kill pane %s: %w", id, err)
		}
		killed = append(killed, id)
		untrackedKilled = append(untrackedKilled, id)
	}

	if len(plan.ownWindowsToEnd) > 0 {
		e.selectRelevantWindowIfCurrentLocked(st, plan.ownWindowsToEnd)
		for _, id := range plan.ownWindowsToEnd {
			if err := e.tmux.run("kill-window", "-t", id); err != nil {
				return killed, fmt.Errorf("kill window %s: %w", id, err)
			}
		}
		logger.Info("reed: reconcile ended own windows no strand holds",
			"socket", e.Socket(), "session", e.SessionName(), "windows", plan.ownWindowsToEnd)
	}

	clearSet := make(map[string]bool, len(plan.clearedGUIDs))
	for _, g := range plan.clearedGUIDs {
		clearSet[g] = true
	}
	for i := range st.Strands {
		if clearSet[st.Strands[i].GUID] {
			st.Strands[i].PaneID = ""
		}
	}

	return killed, nil
}
