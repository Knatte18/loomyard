// discussionrecord.go copies a run's Discussion decision record onto its board entry: the one store call loomcli makes, and the Discussion approval seam that uses it.

package loomcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
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

// approveDiscussion is the body of Discussion-Bouncer's approve seam: it stores the working copy of the discussion decision record, which the approval commit then lands, on the board entry of the run.
// A read error, or a store error other than a done entry, is returned so the settle fails and a resume stores again.
// A done entry keeps the record it last stored and nothing a resume does can change that, so its refusal is logged at Warn and the settle goes on.
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
