// strand.go implements the three strand-mutation engine ops — AddStrand/UpdateStrand/RemoveStrand —
// plus the pure decision helpers each composes: parent existence/cycle validation, display-name
// resolution, the hidden<->visible transition rules, and the recursive-removal cascade.
// Each exported op acquires the op lock once, delegates to an unexported *Locked mutation helper,
// then runs the shared reconcile-apply-persist tail (spawn.go's reconcileApplyPersistLocked) —
// composing reconcile (card 17) and apply (card 18) exactly once per op, per the batch's
// single-layer-lock decision.

package reedengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// AddSpec carries the caller-supplied inputs AddStrand needs to build a new Strand.
type AddSpec struct {
	Role, Round, NameOverride string
	Parent                    string
	Cmd, ResumeCmd            string
	// SessionID is opaque caller metadata reed never reads — mirroring
	// Strand.SessionID in state.go, it is stamped verbatim into the
	// appended Strand and never interpreted or branched on by reed.
	SessionID string
	Display   render.Display
}

// Removed reports every strand RemoveStrand deleted: the target plus its whole cascaded descendant
// subtree.
type Removed struct {
	Strands []struct{ GUID, Name string }
}

// validateAnchor rejects an invalid Display anchor at the op boundary,
// before any pane is launched or state persisted.
func validateAnchor(anchor render.Anchor) error {
	switch anchor {
	case render.AnchorBelowParent, render.AnchorHidden:
		return nil
	case render.AnchorOwnWindow:
		return fmt.Errorf("anchor %q is deferred, not supported in v1", render.AnchorOwnWindow)
	default:
		return fmt.Errorf("invalid anchor %q; want below-parent|hidden", anchor)
	}
}

// strandIndex returns the index of the strand with the given guid, or -1.
func strandIndex(strands []Strand, guid string) int {
	for i, s := range strands {
		if s.GUID == guid {
			return i
		}
	}
	return -1
}

// strandByGUID returns the strand with the given guid and true, or a zero Strand and false.
func strandByGUID(strands []Strand, guid string) (Strand, bool) {
	if i := strandIndex(strands, guid); i != -1 {
		return strands[i], true
	}
	return Strand{}, false
}

// wouldFormCycle reports whether linking guid as a child of parent would
// create a cycle.
func wouldFormCycle(strands []Strand, guid, parent string) bool {
	byGUID := make(map[string]Strand, len(strands))
	for _, s := range strands {
		byGUID[s.GUID] = s
	}

	cur := parent
	for cur != "" {
		if cur == guid {
			return true
		}
		s, ok := byGUID[cur]
		if !ok {
			return false
		}
		cur = s.Parent
	}
	return false
}

// directChildren returns the GUIDs of strands whose Parent equals guid.
func directChildren(strands []Strand, guid string) []string {
	var out []string
	for _, s := range strands {
		if s.Parent == guid {
			out = append(out, s.GUID)
		}
	}
	return out
}

// descendantSubtree returns guid and every descendant beneath it (breadth-first).
func descendantSubtree(strands []Strand, guid string) []string {
	childrenOf := make(map[string][]string, len(strands))
	for _, s := range strands {
		if s.Parent != "" {
			childrenOf[s.Parent] = append(childrenOf[s.Parent], s.GUID)
		}
	}

	out := []string{guid}
	queue := []string{guid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range childrenOf[cur] {
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}

// resolveStrandName computes AddStrand's display name: NameOverride wins,
// else Role fills the template, else the short guid.
func resolveStrandName(template string, spec AddSpec, guid, worktreeRoot string) string {
	if spec.NameOverride != "" {
		return spec.NameOverride
	}
	if spec.Role == "" {
		return guid[:8]
	}
	parts := map[string]string{
		"<ROLE>":       spec.Role,
		"<ROUND>":      spec.Round,
		"<WORKTREE>":   filepath.Base(worktreeRoot),
		"<SHORT_GUID>": guid[:8],
	}
	return FormatStrandName(template, parts)
}

// needsLaunchOnAdd reports whether AddStrand must realize display into a
// live pane: every anchor except hidden.
func needsLaunchOnAdd(display render.Display) bool {
	return display.Anchor != render.AnchorHidden
}

// needsLaunchOnSurface reports whether an UpdateStrand call is a
// hidden->visible surface that must realize a pane.
func needsLaunchOnSurface(wasHidden bool, display render.Display) bool {
	return wasHidden && display.Anchor != render.AnchorHidden
}

// addStrandLocked builds and registers a new Strand from spec, validating
// the parent chain and realizing non-hidden strands into a live pane.
func (e *Engine) addStrandLocked(st *ReedState, spec AddSpec) (Strand, error) {
	if err := validateAnchor(spec.Display.Anchor); err != nil {
		return Strand{}, err
	}

	guid, err := newGUID()
	if err != nil {
		return Strand{}, fmt.Errorf("generate guid: %w", err)
	}

	if spec.Parent != "" {
		if _, ok := strandByGUID(st.Strands, spec.Parent); !ok {
			return Strand{}, fmt.Errorf("unknown parent %q", spec.Parent)
		}
		if wouldFormCycle(st.Strands, guid, spec.Parent) {
			return Strand{}, fmt.Errorf("parent %q would form a cycle", spec.Parent)
		}
	}

	st.Strands = append(st.Strands, Strand{
		GUID:      guid,
		Name:      resolveStrandName(e.cfg.StrandName, spec, guid, e.geom.WorktreeRoot),
		Worktree:  e.geom.WorktreeRoot,
		Parent:    spec.Parent,
		Cmd:       spec.Cmd,
		ResumeCmd: spec.ResumeCmd,
		SessionID: spec.SessionID,
		Display:   spec.Display,
	})
	strand := &st.Strands[len(st.Strands)-1]

	if needsLaunchOnAdd(spec.Display) {
		if err := e.launchStrandLocked(st, strand, strand.Cmd); err != nil {
			return Strand{}, fmt.Errorf("launch strand: %w", err)
		}
	}

	return *strand, nil
}

// updateStrandLocked mutates guid's Display, rejecting visible->hidden
// transitions and realizing hidden->visible ones into a pane.
func (e *Engine) updateStrandLocked(st *ReedState, guid string, display render.Display) (Strand, error) {
	if err := validateAnchor(display.Anchor); err != nil {
		return Strand{}, err
	}

	idx := strandIndex(st.Strands, guid)
	if idx == -1 {
		return Strand{}, fmt.Errorf("unknown strand %q", guid)
	}
	strand := &st.Strands[idx]

	wasHidden := strand.Display.Anchor == render.AnchorHidden
	if !wasHidden && display.Anchor == render.AnchorHidden {
		return Strand{}, fmt.Errorf("cannot hide a live strand in v1")
	}

	strand.Display = display

	if needsLaunchOnSurface(wasHidden, display) {
		if err := e.launchStrandLocked(st, strand, strand.Cmd); err != nil {
			return Strand{}, fmt.Errorf("launch strand: %w", err)
		}
	}

	return *strand, nil
}

// removeStrandLocked removes guid, rejecting non-leaf strands without
// recursive, and cascading descendants. It returns pane ids of every
// removed strand that held a live binding.
func (e *Engine) removeStrandLocked(st *ReedState, guid string, recursive bool) (Removed, []string, error) {
	if _, ok := strandByGUID(st.Strands, guid); !ok {
		return Removed{}, nil, fmt.Errorf("unknown strand %q", guid)
	}
	if len(directChildren(st.Strands, guid)) > 0 && !recursive {
		return Removed{}, nil, fmt.Errorf("strand has children, use --recursive")
	}

	toRemove := descendantSubtree(st.Strands, guid)
	removeSet := make(map[string]bool, len(toRemove))
	for _, g := range toRemove {
		removeSet[g] = true
	}

	var removed Removed
	var paneIDs []string
	remaining := make([]Strand, 0, len(st.Strands))
	for _, s := range st.Strands {
		if removeSet[s.GUID] {
			removed.Strands = append(removed.Strands, struct{ GUID, Name string }{s.GUID, s.Name})
			if s.PaneID != "" {
				paneIDs = append(paneIDs, s.PaneID)
			}
			continue
		}
		remaining = append(remaining, s)
	}
	st.Strands = remaining

	return removed, paneIDs, nil
}

// removalEmptiedSession classifies whether a remove drained the session of
// every strand that should still own a live pane, so a confirmed-gone
// session is an expected terminal state rather than a failure. It returns
// true iff sessionGone is true AND no strand in remaining is non-hidden —
// mirroring anyPlacedStrand's "expected to own a live pane" filter
// (apply.go: Anchor != render.AnchorHidden) so the two share one notion of
// that concept rather than a second, driftable classification. An empty
// remaining slice therefore returns true when sessionGone, since nothing is
// left that should still own a pane.
func removalEmptiedSession(remaining []Strand, sessionGone bool) bool {
	if !sessionGone {
		return false
	}
	for _, s := range remaining {
		if s.Display.Anchor != render.AnchorHidden {
			return false
		}
	}
	return true
}

// AddStrand registers a new strand from spec and, unless added anchor:hidden, realizes it into a
// live pane and runs its cmd, then reconciles and re-applies the layout.
// The engine, not the caller, stamps Worktree and generates GUID, since it owns both this
// worktree's geometry and guid generation (the guid-dependent <SHORT_GUID> name token cannot be
// computed before the guid exists).
// Pre-flights the session's existence (mirroring Status) so running add before up fails with the
// same friendly no-session error (see requireSessionLocked/noSessionMessage) instead of a raw tmux
// error surfacing later from inside launchStrandLocked.
func (e *Engine) AddStrand(spec AddSpec) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		strand, err := e.addStrandLocked(st, spec)
		if err != nil {
			return err
		}

		// Persist immediately after the launch succeeds, before the layout
		// apply. If apply then fails, the strand is already tracked (with its
		// new PaneID), so the next reconcile repairs the layout — the launched
		// pane never becomes an untracked orphan the next select-layout would
		// silently reap.
		if err := SaveState(e.stateDir(), st); err != nil {
			return fmt.Errorf("persist strand: %w", err)
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, strand.GUID)
		return nil
	})
	return result, err
}

// UpdateStrand mutates guid's display settings, then reconciles and re-applies the layout.
// It rejects a visible->hidden transition ("cannot hide a live strand in v1");
// a hidden->visible transition surfaces the strand (creates its pane, runs its cmd).
// Pre-flights the session's existence (like AddStrand/RemoveStrand) so surfacing a hidden strand
// before "up" fails with the friendly no-session error (see requireSessionLocked/noSessionMessage)
// instead of a raw tmux error from inside launchStrandLocked.
// UpdateStrand is engine-API-only in v1 — there is no CLI verb for it.
func (e *Engine) UpdateStrand(guid string, display render.Display) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		if _, err := e.updateStrandLocked(st, guid, display); err != nil {
			return err
		}

		// Persist immediately after a possible surface launch, before the
		// layout apply, for the same orphan-avoidance reason as AddStrand.
		if err := SaveState(e.stateDir(), st); err != nil {
			return fmt.Errorf("persist strand: %w", err)
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, guid)
		return nil
	})
	return result, err
}

// safeReapRoot reports whether p's #{pane_pid} may be used as a descendant-closure root by a
// pane-destroying op.
// Only a still-running pane qualifies: tmux keeps reporting a dead pane's recorded #{pane_pid} long
// after that process exited (remain-on-exit is on for every reed session, so a corpse can sit in the
// window for hours), and once the OS recycles that pid the closure walk would expand it into an
// UNRELATED process's whole subtree — which the caller then waits on and finally SIGKILLs.
// It is the single declaration of that rule so the two reap-root snapshots below cannot drift apart
// again; they did, and Down was the one missing the filter (R2 review finding R2-F2).
func safeReapRoot(p LivePane) bool {
	return !p.Dead && p.PID > 0
}

// alivePanePIDs returns the safe reap roots (see safeReapRoot) among the panes named by paneIDs.
// RemoveStrand uses this targeted form to snapshot only the doomed strands' panes.
func alivePanePIDs(paneIDs []string, live []LivePane) []int {
	wanted := make(map[string]bool, len(paneIDs))
	for _, id := range paneIDs {
		wanted[id] = true
	}
	var pids []int
	for _, p := range live {
		if wanted[p.ID] && safeReapRoot(p) {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// paneIDsInSession returns the subset of paneIDs that live actually contains — i.e. the ones that
// are panes of the session live was enumerated from.
//
// It exists because a persisted pane id is NOT self-validating as a tmux target. The -L socket is
// per hub and shared by every worktree in it, and tmux pane ids are server-global, so a pane id a
// stale or copied reed.json carries is routinely a valid, addressable id belonging to a SIBLING
// worktree's live session. Reproduced live (R5 review finding R5-F4): with svc-beta's reed.json
// copied into svc-alpha, a `lyx reed remove` run in svc-alpha killed svc-beta's strand pane and its
// running process and reported ok:true, while svc-beta was left showing only that its strand had
// died.
// Membership, not aliveness, is the filter: a dead-but-present corpse is still this session's pane
// and killing it is exactly what remove should do.
func paneIDsInSession(paneIDs []string, live []LivePane) []string {
	present := liveIDSet(live)
	inSession := make([]string, 0, len(paneIDs))
	for _, id := range paneIDs {
		if present[id] {
			inSession = append(inSession, id)
		}
	}
	return inSession
}

// sessionReapRoots returns the safe reap roots (see safeReapRoot) among EVERY pane in live.
// Down uses this whole-session form, since it tears the session down entirely rather than a named
// subset of its panes;
// it shares safeReapRoot with alivePanePIDs so the two forms can never disagree about which pids are
// safe to expand and kill.
func sessionReapRoots(live []LivePane) []int {
	var pids []int
	for _, p := range live {
		if safeReapRoot(p) {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// RemoveStrand removes guid and, when it has descendants, cascades the removal through its whole
// subtree (recursive must be true for a non-leaf, or the call errors instead of silently deleting
// descendants), then reconciles and re-applies the layout.
// Returns every strand actually removed.
// Pre-flights the session's existence (mirroring Status) so running remove before up fails with the
// same friendly no-session error (see requireSessionLocked/noSessionMessage) instead of a raw tmux
// error surfacing later from inside reconcileApplyPersistLocked's listPanes.
// Like Down, it waits for the destroyed panes' process subtrees to exit before returning: tmux
// terminates a pane's children asynchronously, and on Windows the process actually holding the
// worktree directory is a deep descendant of #{pane_pid} — a remove that returned without the reap
// could leave a removed strand's grandchild alive and the worktree dir busy (the same "no stray
// state" gap Down's reap closed).
func (e *Engine) RemoveStrand(guid string, recursive bool) (Removed, error) {
	var result Removed
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		removed, paneIDs, err := e.removeStrandLocked(st, guid, recursive)
		if err != nil {
			return err
		}

		// Snapshot the doomed panes' process subtrees BEFORE kill-pane, while
		// the panes still exist to be listed and their pids are guaranteed
		// un-reused (the processes are still running).
		// The same enumeration narrows the kill list to panes that really belong to THIS
		// session, so a stale or copied reed.json can never make this remove destroy a sibling
		// worktree's pane on the shared per-hub server (paneIDsInSession, R5 review finding
		// R5-F4). It is free: this call site already had to list panes for the reap snapshot.
		// A failed enumeration kills nothing rather than falling back to the unchecked list —
		// list-panes exits non-zero precisely when the session is gone, in which case the panes
		// are gone with it and there is nothing this remove still needs to destroy.
		var reapPIDs []int
		var killPaneIDs []string
		if len(paneIDs) > 0 {
			live, err := e.tmux.listPanes(e.SessionName())
			if err != nil {
				logger.Warn("reed: could not enumerate panes before removing a strand, killing none",
					"socket", e.Socket(), "session", e.SessionName(), "err", err)
			} else {
				killPaneIDs = paneIDsInSession(paneIDs, live)
				reapPIDs = e.descendantClosurePIDs(alivePanePIDs(killPaneIDs, live))
			}
		}

		// Kill the removed strands' panes explicitly rather than relying on
		// select-layout to reap panes missing from the layout string: psmux
		// reaps the extra panes as a side effect, and tmux does NOT reject a
		// mismatched layout either — it accepts a cell/pane count mismatch
		// with exit 0 and assigns cells positionally (observed live,
		// tmux 3.6), so neither backend reaps deterministically enough to
		// lean on. Best-effort: a pane may already be dead or gone. What
		// killing a session's LAST pane does next is BINARY-DEPENDENT, not
		// universal: on psmux, remain-on-exit corpses it as pane_dead=1
		// (exit 0), keeping the session alive; on tmux, killing a session's
		// true last pane DESTROYS the session (and, if it was the server's
		// only session, the server exits) — the reconcile tail below then
		// fails its listPanes call against the now-gone session. RemoveStrand
		// below handles both outcomes by re-probing hasSession and swallowing
		// that failure as an expected success only when the session is
		// confirmed gone (the tmux case); on psmux the reconcile tail simply
		// re-enumerates and re-applies — a strand's pane is now always a
		// fresh split, so a corpse is never reused.
		for _, id := range killPaneIDs {
			_ = e.tmux.run("kill-pane", "-t", id)
		}

		// Reap after the layout repair, so the surviving panes re-tile
		// immediately and only the return is gated on the async pane teardown
		// finishing — but ALWAYS reap, even when the repair failed: the panes
		// were already killed above, so their subtrees are dying
		// asynchronously either way, and skipping the reap on a transient
		// apply error would leave a removed strand's grandchild holding the
		// worktree directory (the same never-skip-the-reap rule down
		// follows). Uses the same saturation-tolerant deadline as down
		// (reapExitTimeout): the reap confirms each pid is gone rather than
		// trusting a fixed timer.
		_, applyErr := e.reconcileApplyPersistLocked(st)
		reapPaneChildren(reapPIDs, reapExitTimeout)
		if applyErr != nil {
			// applyErr alone cannot tell "the removal legitimately emptied
			// the session" (an expected terminal state — see the tmux/psmux
			// last-pane split above) apart from a genuine failure, so
			// re-probe the session directly rather than string-match
			// applyErr. hasSession maps a "no server running" exit (1) to
			// (false, nil), the same classification requireSessionLocked
			// already relies on.
			up, herr := e.tmux.hasSession(e.SessionName())
			sessionGone := herr == nil && !up
			if removalEmptiedSession(st.Strands, sessionGone) {
				// removeStrandLocked already pruned st.Strands in memory, but
				// reconcileApplyPersistLocked's own SaveState never ran (it
				// failed before reaching it) — persist the pruned state here
				// so a later "lyx reed resume" does not resurrect the strand
				// this call just removed.
				if err := SaveState(e.stateDir(), st); err != nil {
					return fmt.Errorf("save state after emptying session: %w", err)
				}
				result = removed
				return nil
			}
			return applyErr
		}

		result = removed
		return nil
	})
	return result, err
}
