// reconcile.go implements the reconcile-against-live-panes engine op: the pure planning function
// planReconcile decides which strand pane bindings to clear, which dead panes to kill, and which
// untracked panes to reap, and reconcileLocked composes that plan with the tmux kill I/O.
// Every public engine op runs reconcile first, under the op lock, so the persisted table never
// drifts from what tmux's list-panes actually reports.

package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// reconcilePlan is planReconcile's decision: which strand bindings to clear, which dead panes to
// kill, which untracked panes to reap, and which dead pane (if any) is kept alive so the session
// survives.
// ownWindowsToEnd names the own windows no strand holds.
// deadPanesToKill and untrackedPanesToKill are carried apart, rather than merged into one slice,
// because reconcileLocked's reap log line must distinguish the two kill reasons without a caller
// having to cross-reference anything else.
type reconcilePlan struct {
	clearedGUIDs         []string
	deadPanesToKill      []string
	untrackedPanesToKill []string
	keptDeadPane         string
	ownWindowsToEnd      []string
}

// planReconcile decides which pane bindings to clear, which dead panes to kill, which
// untracked panes to reap, and which own windows to end.
// live holds window 0's panes and windows the whole session's.
// The kill decisions and the survival rule read live alone.
// An own-window strand's binding is judged against windows, and a window carrying the own marker with no bound pane is ended.
// Pure logic; unit-testable without a running server.
// Keeps at least one pane alive (session-survival rule); spares Selvage.
func planReconcile(strands []Strand, live []LivePane, windows []windowPane, policy reapPolicy) reconcilePlan {
	var plan reconcilePlan

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
		if p.Dead && p.ID != keptDeadPaneID && !policy.exemptFromDeadKill(p.ID) {
			killSet[p.ID] = true
			plan.deadPanesToKill = append(plan.deadPanesToKill, p.ID)
		}
	}

	// Deterministic untracked-pane reaping (see the doc comment): kill every
	// live pane no strand owns, while EITHER some strand is bound to a
	// present pane OR Selvage's policy authorizes it.
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
	// reap below, and holds bound strand pane ids alone; anyBoundPresent
	// above stays computed from real strand bindings alone too (see this
	// function's doc comment). Selvage's own exemption is asked of the
	// policy separately, as its own disjunct, rather than being folded into
	// this map.
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
			if !listedByID[s.PaneID] {
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

// ownWindowsHoldingNoStrand lists, in listing order and once each, every window whose panes carry the own marker and none of whose pane ids is bound to a strand.
// A window without the marker never qualifies.
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

// clearConflictingPaneBindings clears every strand PaneID that names a pane the strand cannot
// possibly own — Selvage's pane, or a pane an earlier strand in the table already claims — and
// returns the GUIDs it cleared, in table order.
//
// A pane has exactly one owner. reed's own construction paths already guarantee that
// (planPaneTarget never yields Selvage as a split target while any non-Selvage pane exists, and
// its result is always validated as genuinely new by validateSplitCreatedNewPane), so a table
// violating it is a CORRUPT table, not one reed produced: a stale
// reed.json restored over a newer session, a hand-edited file, a partially restored backup. Every
// such table reaches this package through LoadState, so the repair belongs at that one load
// chokepoint rather than at each of the several places a duplicate does damage.
//
// The damage is worth naming, because it is neither theoretical nor loud (R5 review finding R5-F3,
// both shapes reproduced live on tmux 3.6):
//   - A strand sharing SELVAGE's pane id makes planLayout place that pane twice — once as
//     bandSelvage's fixed bottom cell, once inside the stack body — and tmux answers a layout string
//     whose cell count exceeds the panes it can name by DESTROYING the panes it has no cell for.
//     Observed: a single `lyx reed up` reduced a two-pane session to one, reported ok:true, and then
//     reported the strand live:true against the Selvage pane.
//   - Two strands sharing one pane id leave the second strand's REAL pane bound to nobody, so
//     planReconcile's deterministic untracked reap kills it. Observed: `up` reported ok:true and
//     strands:2 while destroying the second strand's pane and its running process, after which
//     status reported both strands live on the one surviving pane.
//
// First writer wins, so the repair is deterministic and order-stable rather than dependent on which
// conflict is noticed first. Clearing (rather than refusing the op) is what makes it self-healing:
// a cleared strand is simply not-live, which resume already knows how to rebuild, whereas a refusal
// would wedge the worktree on exactly the corruption this exists to survive.
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
// Kills panes per planReconcile's schedule; clears bindings for gone panes.
// It also ends the own windows no strand holds, moving a viewer on one to the relevant window first (the newest live agent, else the overview, never the shell); those windows are not in killed, since no window 0 pane went.
// A viewer on any other window is never moved.
func (e *Engine) reconcileLocked(st *ReedState, live []LivePane, windows []windowPane) (killed []string, err error) {
	plan := planReconcile(st.Strands, live, windows, newReapPolicy(st, live))

	// Accumulate the ids actually destroyed, separately for each kill reason,
	// as the loops below progress -- never plan.deadPanesToKill /
	// plan.untrackedPanesToKill themselves, which name what was SCHEDULED,
	// not what a partial-kill error path actually reached. The two agree on
	// the success path and diverge on a mid-loop kill-pane failure, where a
	// log claiming to have destroyed a pane that is still alive is worse
	// than no log at all -- this is a destruction record, not a record of
	// intent.
	var deadKilled, untrackedKilled []string

	// This is Info, not Debug: per
	// PATTERN-spawn-observability's lifecycle-vs-probe split, a real pane teardown is a
	// lifecycle event, not a probe. And it needs a trace at all because the
	// policy.authorizesReap() disjunct above makes this reap fire on the zero-strand
	// precondition (every AddStrand/UpdateStrand once the reap-before-
	// allocate chokepoint lands), taking it from near-dormant to routine --
	// and it destroys panes an operator may have created themselves.
	//
	// A defer (rather than duplicating the call before each return) is what
	// keeps the success and partial-kill-error paths from drifting apart
	// later: both reach this same one call, and a reconcile that killed
	// nothing on either path still logs nothing. Logging here is additive
	// only -- it must not swallow or alter the returned error.
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
