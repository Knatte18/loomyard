// discussionrecord.go moves a run's Discussion decision record between the run and its board entry.
// It stores the record on the entry when the discussion is approved,
// and it seeds the stored record of a discussed entry into the run.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// storeDecisionRecord opens the hub board and stores record as the decision record of the entry slug.
// It is the only place loom opens the board to store a record.
// A done entry's refusal matches boardengine.ErrRecordEntryDone under errors.Is.
func storeDecisionRecord(location *lymcwd.Location, slug string, record []byte) error {
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		return err
	}
	return board.StoreDecisionRecord(slug, record)
}

// approveDiscussion stores the working copy of the discussion decision record on the board entry of the run.
// The approval commit then lands the working copy.
// A read error, or a store error other than a done entry, is returned so the settle fails and a resume stores again.
// A done entry keeps the record it last stored, and no resume can change that,
// so its refusal is logged at Warn and the settle goes on.
func approveDiscussion(location *lymcwd.Location, runID string) error {
	record, err := os.ReadFile(loomengine.DiscussionDecisionRecord(location))
	if err != nil {
		return fmt.Errorf("read the decision record to store on the board entry: %w; way forward: restore the record at %s, then resume the run", err, loomengine.DiscussionDecisionRecordRel())
	}
	slug := shedrun.ResolveRunID(location, runID)
	err = storeDecisionRecord(location, slug, record)
	if errors.Is(err, boardengine.ErrRecordEntryDone) {
		logger.Warn("loom: the board entry has landed, so its decision record was not stored", "slug", slug, "error", err)
		return nil
	}
	return err
}

// seedDiscussedRecord seeds a run from its board entry when the entry is flagged discussed and holds its stored record.
// It writes the stored record as the run's decision record, and it writes the support log when none exists.
// Then it commits the discussion directory and reports true,
// so the run passes over Discussion and Plan-Write,
// and the reviews read the record as they read one the run approved.
// An absent entry, or one not flagged discussed, writes nothing and reports false.
// A failed read, write or commit is returned, so the gate fails the run and a resume runs the gate again.
func seedDiscussedRecord(location *lymcwd.Location, runID string, record runRecord) (bool, error) {
	board, err := boardengine.OpenHub(location.HubPath)
	if err != nil {
		return false, err
	}
	stored, found, err := board.DecisionRecord(shedrun.ResolveRunID(location, runID))
	if err != nil || !found {
		return false, err
	}
	if err := os.MkdirAll(record.dir, 0o755); err != nil {
		return false, fmt.Errorf("create the discussion directory for the stored decision record: %w", err)
	}
	if err := fsx.AtomicWriteBytes(record.path, stored); err != nil {
		return false, fmt.Errorf("write the stored decision record: %w", err)
	}
	if _, err := os.Stat(record.supportLog); errors.Is(err, fs.ErrNotExist) {
		if err := fsx.AtomicWriteBytes(record.supportLog, loomshed.StoredRecordSupportLog()); err != nil {
			return false, fmt.Errorf("write the support log beside the stored decision record: %w", err)
		}
	} else if err != nil {
		return false, fmt.Errorf("stat the support log: %w", err)
	}
	if err := commitDiscussion(location); err != nil {
		return false, fmt.Errorf("commit the stored decision record: %w", err)
	}
	return true, nil
}
