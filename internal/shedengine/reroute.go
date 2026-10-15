// reroute.go implements the reroute edge: a Stuck verdict that takes a row's OnReroute target, at most rerouteCap times before a human resumes the row.

package shedengine

import (
	"errors"
	"fmt"
)

// rerouteCap is how many times a row reroutes since its latest Done or reroute-cap halt before the next reroute halts the run.
const rerouteCap = 3

// rerouteCount counts row's OutcomeReroute entries, newest first, back to row's latest Done entry or its latest RerouteCapped entry, whichever is later.
func rerouteCount(history []HistoryEntry, row string) int {
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		if entry.Producer != row {
			continue
		}
		if entry.Outcome == Done || entry.RerouteCapped {
			return count
		}
		if entry.Outcome == OutcomeReroute {
			count++
		}
	}
	return count
}

// rerouteCapWayForward returns the trailing clause of a reroute-cap halt, falling back to the run verb that resumes the row.
func (s *Shed) rerouteCapWayForward() string {
	if s.RerouteCapWayForward != "" {
		return s.RerouteCapWayForward
	}
	return fmt.Sprintf("%s resumes the row and grants it %d more reroutes", runVerb("run", s.RunID), rerouteCap)
}

// rerouteStuck handles a Stuck verdict of def that carries OutputPointer.Reroute, st being the status read at the start of the step.
// Below rerouteCap it moves the run to def.OnReroute with a reroute history entry and spends no bounce budget;
// at the cap it blocks the run at def with a RerouteCapped entry, and a row with no OnReroute is a wiring fault that fails the run.
func (s *Shed) rerouteStuck(st Status, def ProducerDef, output OutputPointer) (StepResult, error) {
	at := nowRFC3339()
	if def.OnReroute == "" {
		nextHistory := appendOrFold(st.History, HistoryEntry{Producer: def.Name, Outcome: Stuck, Output: output.Path, At: at, GateAttempts: output.GateAttempts, GateSpans: output.GateSpans, BudgetExempt: output.BudgetExempt})
		failErr := fmt.Errorf("shedengine: producer %q returned a reroute verdict but has no OnReroute", def.Name)
		if persistErr := s.persist(st.CurrentProducer, StateFailed, failErr.Error(), nextHistory, false, ""); persistErr != nil {
			return StepResult{}, errors.Join(failErr, persistErr)
		}
		return StepResult{}, failErr
	}

	count := rerouteCount(st.History, def.Name)
	if count < rerouteCap {
		nextHistory := appendOrFold(st.History, HistoryEntry{Producer: def.Name, Outcome: OutcomeReroute, Output: output.Path, At: at, GateAttempts: output.GateAttempts, GateSpans: output.GateSpans, RoutedTo: def.OnReroute})
		next, reason, err := s.routedState(def)
		if err != nil {
			return StepResult{}, err
		}
		if err := s.persistTransient(def.OnReroute, next, "", nextHistory, false, next == StatePaused, "", "", ""); err != nil {
			return StepResult{}, err
		}
		return StepResult{Producer: def.Name, Outcome: Stuck, Output: output.Path, Next: def.OnReroute, State: next, Reason: reason, History: nextHistory}, nil
	}

	nextHistory := appendOrFold(st.History, HistoryEntry{Producer: def.Name, Outcome: Stuck, Output: output.Path, At: at, GateAttempts: output.GateAttempts, GateSpans: output.GateSpans, BudgetExempt: true, RerouteCapped: true})
	reason := fmt.Sprintf("reroute cap reached: %s rerouted to %s %d times since its last done; %s; way forward: %s", def.Name, def.OnReroute, count, stuckReason(output.Reason), s.rerouteCapWayForward())
	if err := s.persist(st.CurrentProducer, StateBlocked, reason, nextHistory, false, ""); err != nil {
		return StepResult{}, err
	}
	return StepResult{Producer: def.Name, Outcome: Stuck, Output: output.Path, Next: st.CurrentProducer, State: StateBlocked, Reason: reason, History: nextHistory}, nil
}
