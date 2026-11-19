// rownotice.go holds a run's row-notice record: the last row visit lym told the run's parent of.
// It keeps a row called again within one visit from telling the parent twice.

package shedrun

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// RowNotice is one row visit the run's parent was told of.
// Visit counts the verdicts the run's history held when the run reached Row,
// so a later visit of the same row counts higher and a call within the same visit counts the same.
// A re-call after an escalation's answer or a resumed halt counts higher too.
// The call before it recorded a verdict.
type RowNotice struct {
	Row   string `json:"row"`
	Visit int    `json:"visit"`
}

// RowNoticeRecord returns the path to the ephemeral, never-tracked row-notice record of the run.
// The record is row-notice.json under ScratchDir(l, runID).
func RowNoticeRecord(l *lymcwd.Location, runID string) string {
	return filepath.Join(ScratchDir(l, runID), "row-notice.json")
}

// ReadRowNotice reads the row-notice record at path; an absent record reads as the zero RowNotice.
func ReadRowNotice(path string) (RowNotice, error) {
	notice, _, err := state.ReadJSON[RowNotice](path, path+".lock")
	if err != nil {
		return RowNotice{}, fmt.Errorf("shedrun: read the row-notice record: %w", err)
	}
	return notice, nil
}

// WriteRowNotice replaces the row-notice record at path with notice, atomically, creating its directory.
func WriteRowNotice(path string, notice RowNotice) error {
	if err := state.WriteJSON(path, path+".lock", notice); err != nil {
		return fmt.Errorf("shedrun: write the row-notice record: %w", err)
	}
	return nil
}
