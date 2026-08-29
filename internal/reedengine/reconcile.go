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
// deadPanesToKill and untrackedPanesToKill are carried apart, rather than merged into one slice,
// because reconcileLocked's reap log line must distinguish the two kill reasons without a caller
// having to cross-reference anything else.
type reconcilePlan struct {
	clearedGUIDs         []string
	deadPanesToKill      []string
	untrackedPanesToKill []string
	keptDeadPane         string
}

// planReconcile decides which pane bindings to clear, which dead panes to kill, and which
// untracked panes to reap.
// Pure logic; unit-testable without a running server.
// Keeps at least one pane alive (session-survival rule); spares header pane.
func planReconcile(strands []Strand, live []LivePane, headerPaneID string) reconcilePlan {
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

	// The header pane is exempt from the dead-pane kill too, not only from
	// the untracked reap below: nothing outside up/resume ever rebuilds the
	// header, so killing a pane_dead=1 header here would leave the session
	// headerless (breaking the always-on keepalive the header exists for)
	// with a stale HeaderPaneID until the next up/resume — and, before the
	// planLayout presence filter existed, that stale id was still emitted as
	// a layout cell, which a real tmux ACCEPTS (exit 0) and assigns
	// positionally, scrambling every strand's height (observed live,
	// tmux 3.6). A kept header corpse instead stays enumerable, keeps the
	// cell/pane count consistent, and is healed — killed and re-split — by
	// ensureHeaderPaneLocked on the next up/resume.
	killSet := make(map[string]bool, len(live))
	for _, p := range live {
		if p.Dead && p.ID != keptDeadPaneID && p.ID != headerPaneID {
			killSet[p.ID] = true
			plan.deadPanesToKill = append(plan.deadPanesToKill, p.ID)
		}
	}

	// Deterministic untracked-pane reaping (see the doc comment): kill every
	// live pane no strand owns, while EITHER some strand is bound to a
	// present pane OR the header itself is alive — killing an alive pane at
	// worst corpses it under remain-on-exit, so the surviving bound pane or
	// header always keeps the session alive. The header disjunct exists
	// because this reap fires from AddStrand/UpdateStrand once the
	// reap-before-allocate chokepoint lands, and neither of those paths ever
	// calls ensureHeaderPaneLocked — so a dead-but-present header must not
	// be allowed to authorize reaping the session's only alive pane; only an
	// ALIVE header may.
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

	// headerAlive is a third, separate local, never folded into
	// boundPaneIDs/anyBoundPresent/exemptPaneIDs: the header stays exempt
	// from being killed by mere presence (a header corpse is still never
	// killed), while only an alive header authorizes killing anything else.
	headerAlive := false
	if headerPaneID != "" {
		if p, present := liveByID[headerPaneID]; present && !p.Dead {
			headerAlive = true
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

	if anyBoundPresent || headerAlive {
		for _, p := range live {
			if !exemptPaneIDs[p.ID] && !killSet[p.ID] && p.ID != keptDeadPaneID {
				killSet[p.ID] = true
				plan.untrackedPanesToKill = append(plan.untrackedPanesToKill, p.ID)
			}
		}
	}

	for _, s := range strands {
		if s.PaneID == "" {
			continue
		}
		p, present := liveByID[s.PaneID]
		if !present || killSet[p.ID] {
			plan.clearedGUIDs = append(plan.clearedGUIDs, s.GUID)
		}
		// present and not being killed (including the kept dead pane):
		// binding stays, so render still places it.
	}

	plan.keptDeadPane = keptDeadPaneID
	return plan
}

// clearAllPaneBindings clears every strand's PaneID after server rebirth.
// Prevents stale bindings from colliding with reborn pane ids.
func clearAllPaneBindings(st *ReedState) {
	for i := range st.Strands {
		st.Strands[i].PaneID = ""
	}
}

// clearConflictingPaneBindings clears every strand PaneID that names a pane the strand cannot
// possibly own — the header pane, or a pane an earlier strand in the table already claims — and
// returns the GUIDs it cleared, in table order.
//
// A pane has exactly one owner. reed's own construction paths already guarantee that
// (planPaneTarget never adopts or splits the header, validateSplitCreatedNewPane refuses an id that
// already existed), so a table violating it is a CORRUPT table, not one reed produced: a stale
// reed.json restored over a newer session, a hand-edited file, a partially restored backup. Every
// such table reaches this package through LoadState, so the repair belongs at that one load
// chokepoint rather than at each of the several places a duplicate does damage.
//
// The damage is worth naming, because it is neither theoretical nor loud (R5 review finding R5-F3,
// both shapes reproduced live on tmux 3.6):
//   - A strand sharing the HEADER's pane id makes planLayout place that pane twice — once as
//     bandHeader's fixed top cell, once inside the stack body — and tmux answers a layout string
//     whose cell count exceeds the panes it can name by DESTROYING the panes it has no cell for.
//     Observed: a single `lyx reed up` reduced a two-pane session to one, reported ok:true, and then
//     reported the strand live:true against the header pane running `lyx reed header --blocking`.
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
	if st.HeaderPaneID != "" {
		claimed[st.HeaderPaneID] = true
	}

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
func (e *Engine) reconcileLocked(st *ReedState, live []LivePane) (killed []string, err error) {
	plan := planReconcile(st.Strands, live, st.HeaderPaneID)

	// Accumulate the ids actually destroyed, separately for each kill reason,
	// as the loops below progress -- never plan.deadPanesToKill /
	// plan.untrackedPanesToKill themselves, which name what was SCHEDULED,
	// not what a partial-kill error path actually reached. The two agree on
	// the success path and diverge on a mid-loop kill-pane failure, where a
	// log claiming to have destroyed a pane that is still alive is worse
	// than no log at all -- this is a destruction record, not a record of
	// intent.
	var deadKilled, untrackedKilled []string

	// This is Info, not Debug: per CONSTRAINTS.md's Live-Substrate Spawn
	// Observability lifecycle-vs-probe split, a real pane teardown is a
	// lifecycle event, not a probe. And it needs a trace at all because the
	// headerAlive disjunct above makes this reap fire on the zero-strand
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
