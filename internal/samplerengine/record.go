// record.go — the Sampler-Session row's record, its locked reads and writes, and the entry stamp a done flag is judged against.

package samplerengine

import (
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// Record is the state the Sampler-Session row keeps in one JSON file in the run's scratch directory.
type Record struct {
	// StrandGUID, StrandName, SessionID and Engine name the interactive session the row last recorded, so a later pass resumes it.
	StrandGUID string `json:"strand_guid,omitempty"`
	StrandName string `json:"strand_name,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Engine     string `json:"engine,omitempty"`
	// DoneStamp is the entry stamp the operator's done flag carries, 0 meaning no flag.
	DoneStamp int `json:"done_stamp,omitempty"`
}

// ReadRecord reads the record at path under the lock at lockPath.
// It reports false when the file does not exist, and accepts a key Record does not declare.
func ReadRecord(path, lockPath string) (Record, bool, error) {
	return state.ReadJSON[Record](path, lockPath)
}

// UpdateRecord applies mutate to the record at path under one exclusive lock and writes the result.
// It starts from the zero Record when the file is absent, and creates the directory the file and its lock live in.
// A key of the file that Record does not declare survives the rewrite.
func UpdateRecord(path, lockPath string, mutate func(*Record)) error {
	return state.UpdateJSON(path, lockPath, func(cur Record, _ bool) (Record, error) {
		mutate(&cur)
		return cur, nil
	})
}

// EntryStamp counts the times history shows the run entering row from another row.
// An entry with outcome Done or skipped whose producer is in predecessors enters row, once per call it folds, and leaves the run at row.
// A goto onto row enters it only when the run did not already stand there; a goto onto another row leaves the run elsewhere.
// A Done entry of row, or a Stuck entry of row routed to another row, leaves the run elsewhere; any other entry of row is a halt and keeps it at row.
// An entry of any other producer leaves the run elsewhere.
// A halt, a failure that appended no entry and a resume therefore leave the stamp unchanged, while every return by a goto from another row raises it.
func EntryStamp(history []shedengine.HistoryEntry, row string, predecessors []string) int {
	stamp := 0
	atRow := false
	for _, entry := range history {
		switch {
		case entry.Producer == row && entry.Outcome == shedengine.OutcomeGoto:
			if !atRow {
				stamp++
			}
			atRow = true
		case entry.Producer == row:
			atRow = entry.Outcome != shedengine.Done && !(entry.Outcome == shedengine.Stuck && entry.RoutedTo != "" && entry.RoutedTo != row)
		case isPredecessorEntry(entry, predecessors):
			stamp += 1 + entry.Repeats
			atRow = true
		default:
			atRow = false
		}
	}
	return stamp
}

// isPredecessorEntry reports whether entry is a Done or skipped call of one of predecessors.
func isPredecessorEntry(entry shedengine.HistoryEntry, predecessors []string) bool {
	if entry.Outcome != shedengine.Done && entry.Outcome != shedengine.OutcomeSkipped {
		return false
	}
	for _, predecessor := range predecessors {
		if entry.Producer == predecessor {
			return true
		}
	}
	return false
}
