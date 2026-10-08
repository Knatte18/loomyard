// directory.go owns the read-only name directory behind `lyx reed list`: one row per strand in this worktree's state, with the pane's current title beside the name it should carry.

package reedengine

import "fmt"

// DirectoryRow is one strand's entry in the name directory.
type DirectoryRow struct {
	Name     string
	GUID     string
	Worktree string
	PaneID   string
	// Title is the bound pane's current title; empty when the strand is not live.
	Title string
	Live  bool
	// Drift is true when the strand is live and its pane title differs from its name.
	Drift bool
	// Retiring is the strand's own Retiring flag: it is about to remove itself.
	Retiring bool
}

// Directory lists this worktree's strands under the op lock without requiring a session.
// With no session up every row is dormant with an empty title; otherwise panes are listed once and each row is filled from them.
// It is read-only: nothing is repaired and nothing is persisted.
func (e *Engine) Directory() ([]DirectoryRow, error) {
	var rows []DirectoryRow
	err := e.withOpLock(func() error {
		up, err := e.tmux.hasSession(e.SessionName())
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}
		if !up {
			st, err := LoadState(e.stateDir())
			if err != nil {
				return err
			}
			if st != nil {
				rows = directoryRows(st.Strands, nil)
			}
			return nil
		}
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}
		live, err := e.listStrandPanes(st)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}
		rows = directoryRows(st.Strands, live)
		return nil
	})
	return rows, err
}

// directoryRows maps strands onto the live pane set; a nil live set makes every row dormant.
// Drift is whatever planTitleRepairs would repair, so the two never disagree.
func directoryRows(strands []Strand, live []LivePane) []DirectoryRow {
	titles := make(map[string]string, len(live))
	for _, p := range live {
		titles[p.ID] = p.Title
	}
	alive := aliveIDSet(live)
	drifted := make(map[string]bool)
	for _, r := range planTitleRepairs(strands, live) {
		drifted[r.GUID] = true
	}
	rows := make([]DirectoryRow, 0, len(strands))
	for _, s := range strands {
		row := DirectoryRow{Name: s.Name, GUID: s.GUID, Worktree: s.Worktree, PaneID: s.PaneID, Retiring: s.Retiring}
		if s.PaneID != "" && alive[s.PaneID] {
			row.Live = true
			row.Title = titles[s.PaneID]
			row.Drift = drifted[s.GUID]
		}
		rows = append(rows, row)
	}
	return rows
}
