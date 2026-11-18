// hold.go implements the reed ops that end a strand's process while keeping its record.
// It also holds the op that relaunches such a strand under the same GUID.
// A held strand is one whose process was ended on purpose.
// Resume and reconcile leave it down until a caller ends the hold.

package reedengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// RelaunchSpec is the launch a held strand comes back up with.
// RelaunchStrand stores it on the strand's record in place of the commands and session id it held.
type RelaunchSpec struct {
	Cmd       string
	ResumeCmd string
	SessionID string
}

// EndStrandProcess marks guid held, then ends its pane and its pane's process tree, keeping the record.
// The record keeps its GUID and its commands, and its pane binding is cleared.
// The mark is saved before any pane is touched, so a failure after it returns the error with the strand still held.
// The caller then ends the hold with ReleaseStrandHold or RelaunchStrand.
// It requires a live session and refuses an unknown strand.
func (e *Engine) EndStrandProcess(guid string) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		idx := strandIndex(st.Strands, guid)
		if idx == -1 {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}

		st.Strands[idx].Held = true
		if err := SaveState(e.stateDir(), st); err != nil {
			return fmt.Errorf("persist hold: %w", err)
		}

		var paneIDs []string
		if paneID := st.Strands[idx].PaneID; paneID != "" {
			paneIDs = []string{paneID}
		}
		reapPIDs, leftWindows, _ := e.killStrandPanes(st, paneIDs, true)
		st.Strands[idx].PaneID = ""
		if warning := e.relayoutSharedWindowsLocked(leftWindows); warning != "" {
			logger.Warn("reed: ending a strand's process skipped the shared-window layout",
				"socket", e.Socket(), "session", e.SessionName(), "cause", warning)
		}

		// The reap follows the layout repair.
		// It runs even when the repair failed, since the panes are already killed and their subtrees are dying either way.
		_, applyErr := e.reconcileApplyPersistLocked(st)
		reapPaneChildren(reapPIDs, reapExitTimeout)
		return applyErr
	})
}

// ReleaseStrandHold clears guid's held mark under the op lock and saves the state, leaving its pane as it is.
// It touches only the strand table, so it needs no live session.
// An unknown guid wraps ErrUnknownStrand.
func (e *Engine) ReleaseStrandHold(guid string) error {
	return e.withOpLock(func() error {
		st, err := LoadState(e.stateDir())
		if err != nil {
			return err
		}
		if st == nil {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}
		idx := strandIndex(st.Strands, guid)
		if idx == -1 {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}
		st.Strands[idx].Held = false
		return SaveState(e.stateDir(), st)
	})
}

// RelaunchStrand brings the held strand guid back up under the same GUID.
// It replaces the strand's stored commands and session id with spec's and launches a pane running spec.Cmd.
// It then clears the held mark and saves.
// A strand that is not held, or not tracked, is refused, so a removal that ran in between wins.
// A failed launch leaves the strand held with its old commands.
// It requires a live session.
func (e *Engine) RelaunchStrand(guid string, spec RelaunchSpec) error {
	return e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		idx := strandIndex(st.Strands, guid)
		if idx == -1 {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}
		strand := &st.Strands[idx]
		if !strand.Held {
			return fmt.Errorf("strand %q is not held, so it is not relaunched", guid)
		}

		strand.Cmd = spec.Cmd
		strand.ResumeCmd = spec.ResumeCmd
		strand.SessionID = spec.SessionID
		if err := e.launchStrandLocked(st, strand, spec.Cmd); err != nil {
			return fmt.Errorf("launch strand: %w", err)
		}
		strand.Held = false
		// Persist after the launch and before the layout apply, so the new pane is tracked even if the apply fails.
		if err := SaveState(e.stateDir(), st); err != nil {
			return fmt.Errorf("persist strand: %w", err)
		}
		_, err = e.reconcileApplyPersistLocked(st)
		return err
	})
}
