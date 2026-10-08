package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// moveStrandTo returns strands with the strand guid moved to index idx, the others keeping their
// relative order. An unknown guid or an out-of-range idx returns strands unchanged. It is pure so a
// Tier 1 test can pin the first, middle and last slot without a tmux session.
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

// ReplaceStrand atomically replaces the leaf strand guid with a new strand built from spec, and the
// new strand takes the replaced strand's slot in the strand table — and so its place in the stack —
// rather than the bottom.
// It runs under one op lock: it requires the session, removes guid (a strand with children is
// refused), kills and reaps its pane exactly as RemoveStrand does, adds the new strand, moves it to
// the removed strand's index, launches its pane, and reconciles and re-applies the layout once.
// The old pane is killed before the new one launches because launching reconciles away any untracked
// pane, which would otherwise kill it without the reap.
func (e *Engine) ReplaceStrand(guid string, spec AddSpec) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if _, err := e.validateNaming(spec); err != nil {
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

		reapPIDs := e.killStrandPanes(paneIDs)
		logger.Info("reed: replacing strand", "socket", e.Socket(), "session", e.SessionName(),
			"guid", guid, "removed", removed.Strands)

		// A failed add can leave its unlaunched strand appended to the table; cut it back off so the
		// reconcile tail below persists the removal alone.
		beforeAdd := len(st.Strands)
		strand, addErr := e.addStrandLocked(st, spec)
		if addErr != nil {
			st.Strands = st.Strands[:beforeAdd]
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
		}

		// The old pane is already killed: repair the layout and reap even when the add failed, so
		// the removal is persisted and the dying subtree is never left holding the worktree.
		_, applyErr := e.reconcileApplyPersistLocked(st)
		reapPaneChildren(reapPIDs, reapExitTimeout)
		// reconcileApplyPersistLocked ends in SaveState, so a nil applyErr means the new strand's record was persisted after all and its launch script stays.
		// When the post-add save failed and the tail failed too, no save carrying the new strand ever succeeded, so its script is deleted.
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
