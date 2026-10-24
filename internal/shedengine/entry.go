// entry.go implements the entry gate: the evaluation of a row's EntryGate when the run enters the row, and the skip that routes a row whose gate does not hold.

package shedengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/state"
)

// enter evaluates def's entry gate when the run enters def, st being the status read at the start of the step.
// An ungated row, and a row st names as entered, are returned as they are without an evaluation.
// A holding gate persists the row as entered and returns it to call.
// A gate that does not hold appends an OutcomeSkipped entry carrying its reason, routes by def.OnDone as a Done does and spends no bounce budget; the next row is then entered in the same call, so consecutive skips chain.
// The returned ProducerDef is the row to call, and a non-nil StepResult means the call ended the step: the chain paused at a row, or a skip finished the run.
// An evaluation error, and a chain longer than the producer list, write the status failed and are returned.
func (s *Shed) enter(st Status, def ProducerDef) (ProducerDef, *StepResult, error) {
	var chain []string
	for {
		// The step's own pause checks cover the first row; a later row of the chain, gated or not, is checked here, before its gate is evaluated or the row is called.
		if len(chain) > 0 && st.PauseBefore != "" && st.PauseBefore == def.Name {
			reason := labelled(conditionReason("before", st.PauseBefore), st.PauseBeforeBy)
			if err := s.persistTransient(def.Name, StatePaused, "", st.History, st.PauseRequested, true, "", "", "", reason); err != nil {
				return def, nil, err
			}
			return def, &StepResult{Next: def.Name, State: StatePaused, Reason: reason, History: st.History}, nil
		}
		if def.EntryGate == nil || st.EnteredRow == def.Name {
			return def, nil, nil
		}
		if len(chain) >= len(s.Producers) {
			failErr := fmt.Errorf("shedengine: the entry gates skipped %s and reached %q again, so the skips form a chain longer than the producer list; way forward: fix the settings the entry gates read, or move the run with %s, then resume it", strings.Join(chain, ", "), def.Name, gotoCommand(s.RunID, "<producer>"))
			return def, nil, s.failEntry(def.Name, st.History, failErr)
		}

		holds, gateReason, err := def.EntryGate()
		if err != nil {
			failErr := fmt.Errorf("shedengine: the entry gate of %q could not be evaluated: %w; way forward: fix what the gate reads, then resume the run with %s", def.Name, err, runVerb("run", s.RunID))
			return def, nil, s.failEntry(def.Name, st.History, failErr)
		}
		if holds {
			if err := s.markEntered(def.Name); err != nil {
				return def, nil, err
			}
			return def, nil, nil
		}

		chain = append(chain, def.Name)
		nextHistory := appendOrFold(st.History, HistoryEntry{Producer: def.Name, Outcome: OutcomeSkipped, At: nowRFC3339(), Reason: gateReason})
		if def.OnDone == "" {
			if err := s.persistRouted(def.Name, StateDone, nextHistory, false, "", ""); err != nil {
				return def, nil, err
			}
			return def, &StepResult{Next: def.Name, State: StateDone, History: nextHistory}, nil
		}
		next, pauseReason, err := s.routedState(def)
		if err != nil {
			return def, nil, err
		}
		if err := s.persistRouted(def.OnDone, next, nextHistory, next == StatePaused, "", pauseReason); err != nil {
			return def, nil, err
		}
		if next == StatePaused {
			return def, &StepResult{Next: def.OnDone, State: next, Reason: pauseReason, History: nextHistory}, nil
		}

		// validate has already rejected an OnDone naming no producer in the list.
		def, _ = findProducer(s.Producers, def.OnDone)
		var found bool
		st, found, err = state.ReadJSONNoMkdir[Status](s.StatusPath, s.StatusLockPath)
		if err != nil {
			return def, nil, fmt.Errorf("shedengine: read status file %q: %w", s.StatusPath, err)
		}
		if !found {
			return def, nil, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
		}
	}
}

// failEntry writes the status failed at row with failErr's text and the history as it stands, and returns failErr, joined with the write's own error when that fails too.
// The row is left not entered, so a resume evaluates its gate again.
func (s *Shed) failEntry(row string, history []HistoryEntry, failErr error) error {
	if persistErr := s.persist(row, StateFailed, failErr.Error(), history, false, ""); persistErr != nil {
		return errors.Join(failErr, persistErr)
	}
	return failErr
}

// markEntered records row as the entered row of the status file, touching no other field.
func (s *Shed) markEntered(row string) error {
	return state.UpdateJSON(s.StatusPath, s.StatusLockPath, func(cur Status, found bool) (Status, error) {
		if !found {
			return Status{}, fmt.Errorf("shedengine: status file %q vanished mid-run; Shed refuses to create one", s.StatusPath)
		}
		cur.EnteredRow = row
		return cur, nil
	})
}
