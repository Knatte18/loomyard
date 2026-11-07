package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// moveStrandTo returns strands with the strand guid moved to index idx, the others keeping their relative order.
// An unknown guid or an out-of-range idx returns strands unchanged.
// It is pure, so tests can pin the first, middle and last slot without a tmux session.
func moveStrandTo(strands []Strand, guid string, idx int) []Strand {
	from := strandIndex(strands, guid)
	if from == -1 || idx < 0 || idx >= len(strands) || from == idx {
		return strands
	}
	moved := strands[from]
	rest := make([]Strand, 0, len(strands))
	rest = append(rest, strands[:from]...)
	rest = append(rest, strands[from+1:]...)

	out := make([]Strand, 0, len(strands))
	out = append(out, rest[:idx]...)
	out = append(out, moved)
	out = append(out, rest[idx:]...)
	return out
}

// ReplaceStrand replaces the leaf strand guid with a new strand built from spec, under one op lock.
// The new strand takes the replaced strand's slot in the strand table, so it keeps its place in the stack.
// It requires the session, and it refuses a strand with children.
// The old pane is killed and reaped before the new one launches.
// Launching reconciles away any untracked pane, which would kill the old one without reaping it.
// Once the new pane launches, the layout is reconciled and reapplied once.
// An overview replacement that ends the window the operator is viewing selects the new overview window.
// It does so only once the launch succeeds.
// Any other replacement that ends that window moves the operator's view to the strands window.
// A replacement whose old strand shared a window that stays open ends no window and switches nothing.
func (e *Engine) ReplaceStrand(guid string, spec AddSpec) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if _, err := e.validateNaming(spec); err != nil {
			return err
		}
		// Refuse an invalid display before the old strand is removed and its pane killed.
		// A check that runs later, once the old strand is gone, could not keep it.
		if err := validateDisplay(spec.Display); err != nil {
			return err
		}
		if err := validateWindowConfig(e.cfg); err != nil {
			return err
		}
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		slot := strandIndex(st.Strands, guid)
		removed, paneIDs, err := e.removeStrandLocked(st, guid, false)
		if err != nil {
			return err
		}

		replacingWithOverview := isOverview(spec.Display)
		reapPIDs, leftWindows, endedCurrent := e.killStrandPanes(st, paneIDs, !replacingWithOverview)
		if warning := e.relayoutSharedWindowsLocked(leftWindows); warning != "" {
			logger.Warn("reed: replace skipped the shared-window layout", "socket", e.Socket(), "session", e.SessionName(), "cause", warning)
		}
		logger.Info("reed: replacing strand", "socket", e.Socket(), "session", e.SessionName(),
			"guid", guid, "removed", removed.Strands)

		// A failed add can leave its unlaunched strand appended to the table; cut it back off so the
		// reconcile tail below persists the removal alone.
		beforeAdd := len(st.Strands)
		strand, addErr := e.addStrandLocked(st, spec)
		if addErr != nil {
			st.Strands = st.Strands[:beforeAdd]
			pruneWindowGroups(st)
		}
		postAddSaveFailed := false
		if addErr == nil {
			st.Strands = moveStrandTo(st.Strands, strand.GUID, slot)
			if err := SaveState(e.stateDir(), st); err != nil {
				addErr = fmt.Errorf("persist strand: %w", err)
				postAddSaveFailed = true
			}
		}
		if addErr == nil {
			logger.Info("reed: replacement strand launched", "socket", e.Socket(), "session", e.SessionName(),
				"guid", strand.GUID, "name", strand.Name)
			if endedCurrent && replacingWithOverview {
				e.selectStrandWindowLocked(strand.PaneID)
			}
		}

		// The old pane is already killed: repair the layout and reap even when the add failed, so
		// the removal is persisted and the dying subtree is never left holding the worktree.
		_, applyErr := e.reconcileApplyPersistLocked(st)
		reapPaneChildren(reapPIDs, reapExitTimeout)
		// reconcileApplyPersistLocked ends in SaveState.
		// A nil applyErr therefore means the new strand's record was persisted after all, so its launch script stays.
		// When the post-add save failed and the tail failed too, no save carrying the new strand ever succeeded,
		// so its script is deleted.
		if postAddSaveFailed && applyErr != nil {
			removeLaunchScripts(shell.ForGOOS(), e.stateDir(), []string{strand.GUID})
		}
		if addErr != nil {
			return addErr
		}
		if applyErr != nil {
			return applyErr
		}

		result, _ = strandByGUID(st.Strands, strand.GUID)
		return nil
	})
	return e.withColor(result), err
}
