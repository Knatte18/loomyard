// state.go holds the durable run state Weaver keeps at _lym/weaver/state.json:
// the run identity, the shared run-plan state, and the attempt, fixer, batch-end and stop records.
// LoadState and SaveState are the only readers and writers of state.json.

package weaverengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/state"
)

// stateFileName is state.json's fixed filename inside Weaver's run directory.
const stateFileName = "state.json"

// State is the durable run state persisted at <runDir>/state.json.
type State struct {
	// RunGUID identifies this Weaver run, minted once at first init.
	RunGUID string `json:"runGuid"`
	// PlanRecordsCommit is the records-repository commit holding the plan that the first begin records.
	PlanRecordsCommit string `json:"planRecordsCommit,omitempty"`
	// State is the run-plan state: the plan fingerprint, the recorded partition and the batch records.
	// Its keys sit beside Weaver's own in state.json.
	runplan.State
	// Attempts records every batch attempt, in attempt order.
	Attempts []*AttemptState `json:"attempts,omitempty"`
	// BatchEnds maps each ended batch to its end commit:
	// the task branch's HEAD when the batch's attempt ended, done or failed.
	BatchEnds map[int]string `json:"batchEnds,omitempty"`
	// LastEnd is the end commit of the batch whose attempt ended last.
	// The repair advice for a corrupt state file compares it with the task branch's HEAD.
	LastEnd string `json:"lastEnd,omitempty"`
	// Fixers records every plan-level verify fixer, in spawn order.
	Fixers []*FixerState `json:"fixers,omitempty"`
	// Stop is the row's last batch, needs-parent or verify stop that has not been moved past.
	// A batch begin leaves it alone, which a re-entry's Reentered marking needs.
	// Only a batch recorded done and a passing verify re-run clear it.
	Stop *StopRecord `json:"stop,omitempty"`
}

// AttemptState is one batch attempt's record.
type AttemptState struct {
	// Batch is the number of the batch the attempt builds.
	Batch int `json:"batch"`
	// Number is the attempt number, one up per attempt of the batch: a same-rung retry and a re-entry attempt included.
	Number int `json:"number"`
	// Rung is the ladder rung the attempt runs on.
	Rung int `json:"rung"`
	// ReportPath is the attempt's report file, fixed before the spawn: the strand's identity.
	ReportPath string `json:"reportPath"`
	// StrandGUID and StrandName identify the attempt's strand.
	// They stay empty until the spawn returns or an attach reads them from shuttle's run record.
	StrandGUID string `json:"strandGuid,omitempty"`
	StrandName string `json:"strandName,omitempty"`
	// InFlight reports whether the attempt's strand may still be running.
	InFlight bool `json:"inFlight,omitempty"`
	// Outcome is empty while the attempt is in flight, then done, failed, lost, needs_parent or fault.
	Outcome string `json:"outcome,omitempty"`
	// Cause says why a failed, lost or fault attempt ended so.
	Cause string `json:"cause,omitempty"`
	// Uncommitted holds the tracked paths the attempt left uncommitted, which a reset counts as the run's own.
	Uncommitted []string `json:"uncommitted,omitempty"`
	// Delivered holds the decision headings already sent to the attempt's strand.
	Delivered []string `json:"delivered,omitempty"`
	// BeganAt is the attempt's start time in RFC 3339.
	BeganAt string `json:"beganAt,omitempty"`
	// Reentry is set on the first attempt a re-entry after a stop starts.
	// The batch's same-rung retry allowance starts counting again from it.
	Reentry bool `json:"reentry,omitempty"`
}

// FixerState is one plan-level verify fixer's record.
type FixerState struct {
	// Number is the fixer's number, one up per fixer of the run.
	Number int `json:"number"`
	// ReplyPath is the fixer's reply file, fix-<n>.md in the reports directory.
	ReplyPath string `json:"replyPath"`
	// Rung is the ladder rung the fixer runs on.
	Rung int `json:"rung"`
	// Record is the fixer strand's record, nil until gatefixer saves it.
	Record *gatefixer.Record `json:"record,omitempty"`
	// StrandName is the fixer strand's name.
	StrandName string `json:"strandName,omitempty"`
	// Outcome is empty while the fixer runs, then its verdict.
	Outcome string `json:"outcome,omitempty"`
	// Findings is the verify failure the fixer was given.
	Findings string `json:"findings,omitempty"`
	// Delivered holds the decision headings already sent to the fixer's strand.
	Delivered []string `json:"delivered,omitempty"`
	// Reentry is set on the first fixer a verify re-entry spawns.
	Reentry bool `json:"reentry,omitempty"`
}

// StopRecord is the stop a re-entry reads to know where to start again.
// A plan-conflict stop and a fault in Weaver's own config or state record none.
type StopRecord struct {
	// Kind is batch, needs_parent or verify.
	Kind string `json:"kind"`
	// Batch is the stopped batch, for a batch or needs_parent stop.
	Batch int `json:"batch,omitempty"`
	// Rung is the ladder rung the stop was on.
	Rung int `json:"rung,omitempty"`
	// Reentered is set by the save that records the first attempt or fixer the stop's re-entry starts.
	// A re-entry a crash interrupts therefore marks no second Reentry.
	Reentered bool `json:"reentered,omitempty"`
}

// StateRel returns the state file's anchor-relative path, `_lym/weaver/state.json`, always with forward slashes.
func StateRel() string {
	return path.Join(DirRel(), stateFileName)
}

// BegunBatches reports whether a non-nil st holds a batch record.
func BegunBatches(st *State) bool {
	return st != nil && len(st.Batches) > 0
}

// LoadState reads <runDir>/state.json, locked against <scratchDir>/state.json.lock.
// A missing file returns (nil, nil): no run has started yet, so it is not an error.
// An unreadable or malformed file is a wrapped error naming the path.
// A key the type does not know is kept, never refused.
func LoadState(runDir, scratchDir string) (*State, error) {
	statePath := filepath.Join(runDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	st, found, err := state.ReadJSON[State](statePath, lockPath)
	if err != nil {
		return nil, fmt.Errorf("weaver: load state %s: %w", statePath, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// SaveState writes st to <runDir>/state.json under an exclusive lock at <scratchDir>/state.json.lock.
// It creates the scratch directory first, then writes atomically, so a crash never leaves a half-written file.
func SaveState(runDir, scratchDir string, st *State) error {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return fmt.Errorf("weaver: create scratch dir %s: %w", scratchDir, err)
	}
	statePath := filepath.Join(runDir, stateFileName)
	lockPath := filepath.Join(scratchDir, stateFileName+".lock")

	if err := state.WriteJSON(statePath, lockPath, *st); err != nil {
		return fmt.Errorf("weaver: save state %s: %w", statePath, err)
	}
	return nil
}
