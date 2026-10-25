// driverules.go holds the shed driver's closed rule table as pure functions:
// the stop it acts on, parsed from the loop's envelope or built from a status, the disk facts the loop reads before deciding, and the one decision the table returns.
// No branch reads a row or recipe name; each reads the envelope's fields, its kind, its transient class and the facts, and the budget-exhausted row alone reads the prefix of the reason or error text.

package sheddriver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// driveRepairCap is how many re-steps of one interrupted stop the driver makes before the tackler is asked.
const driveRepairCap = 2

// Stop is what the loop's envelope says about one stop, or what a status says when the driver acts on a stop it did not step to.
// The loop-object fields are empty for a stop built from a status, and Raw holds the envelope bytes verbatim.
type Stop struct {
	OK           bool
	State        string
	Reason       string
	Kind         string
	Transient    string
	ParentNotice string
	Friction     string
	EnvelopePath string
	Error        string

	// Outcome is the stepped producer's outcome as the step envelope reports it; a stop built from a status or a resumed attempt carries none.
	Outcome string

	Stop            string
	CurrentProducer string
	HistoryLength   int
	LoopState       string
	InterruptPolicy string
	StatusMoved     bool
	TraceCopy       string
	StderrPath      string

	Raw        []byte
	FromStatus bool
	// Restep is set by the loop on a stop whose run an orch goto moved, so the loop re-steps instead of deciding.
	Restep bool
	// HandBack is true on a blocked stop that is a landing halt only the orch's resume lifts, as the loop's envelope marks it or the driver's told check answers for a status.
	HandBack bool
}

// parseDriveStop decodes the envelope shedverbs.RunUntilStop prints.
// An unknown key is tolerated, and bytes that do not decode are refused.
func parseDriveStop(envelope []byte) (Stop, error) {
	var decoded struct {
		OK           bool   `json:"ok"`
		Outcome      string `json:"outcome"`
		State        string `json:"state"`
		Reason       string `json:"reason"`
		Kind         string `json:"kind"`
		Transient    string `json:"transient"`
		ParentNotice string `json:"parent_notice"`
		Friction     string `json:"friction"`
		EnvelopePath string `json:"envelope_path"`
		Error        string `json:"error"`
		Loop         struct {
			Stop            string `json:"stop"`
			CurrentProducer string `json:"current_producer"`
			HistoryLength   int    `json:"history_length"`
			State           string `json:"state"`
			InterruptPolicy string `json:"interrupt_policy"`
			StatusMoved     bool   `json:"status_moved"`
			TraceCopy       string `json:"trace_copy"`
			StderrPath      string `json:"stderr_path"`
			HandBack        bool   `json:"hand_back"`
		} `json:"loop"`
	}
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return Stop{}, fmt.Errorf("sheddriver: drive: the loop's stop envelope does not decode: %w", err)
	}
	return Stop{
		OK:              decoded.OK,
		Outcome:         decoded.Outcome,
		State:           decoded.State,
		Reason:          decoded.Reason,
		Kind:            decoded.Kind,
		Transient:       decoded.Transient,
		ParentNotice:    decoded.ParentNotice,
		Friction:        decoded.Friction,
		EnvelopePath:    decoded.EnvelopePath,
		Error:           decoded.Error,
		Stop:            decoded.Loop.Stop,
		CurrentProducer: decoded.Loop.CurrentProducer,
		HistoryLength:   decoded.Loop.HistoryLength,
		LoopState:       decoded.Loop.State,
		InterruptPolicy: decoded.Loop.InterruptPolicy,
		StatusMoved:     decoded.Loop.StatusMoved,
		TraceCopy:       decoded.Loop.TraceCopy,
		StderrPath:      decoded.Loop.StderrPath,
		HandBack:        decoded.Loop.HandBack,
		Raw:             envelope,
	}, nil
}

// stopFromStatus builds the stop the driver acts on from a status alone.
// It has FromStatus set and no kind, and a paused status carries its pause_reason as the stop's reason; a failed status reads as an error stop.
// handsBack is the told check of a landing halt, answered for a blocked status's current producer and error; nil marks nothing.
func stopFromStatus(st shedengine.Status, handsBack func(producer, reason string) bool) Stop {
	reason := ""
	if st.State == shedengine.StatePaused {
		reason = st.PauseReason
	}
	return Stop{
		HandBack:        handsBack != nil && st.State == shedengine.StateBlocked && handsBack(st.CurrentProducer, st.Error),
		Reason:          reason,
		OK:              st.State != shedengine.StateFailed,
		State:           string(st.State),
		Transient:       st.Transient,
		ParentNotice:    st.ParentNotice,
		Error:           st.Error,
		CurrentProducer: st.CurrentProducer,
		HistoryLength:   len(st.History),
		LoopState:       string(st.State),
		FromStatus:      true,
	}
}

// Key is the stop key the stop left: the loop's current producer, history length and state, falling back to the top-level state.
func (s Stop) Key() StopKey {
	state := s.LoopState
	if state == "" {
		state = s.State
	}
	return StopKey{CurrentProducer: s.CurrentProducer, HistoryLength: s.HistoryLength, State: state}
}

// driveFacts carries the disk reads the loop makes before deciding.
type driveFacts struct {
	// EscalationPending is true when the arbiter would admit the awaiting stop: its eligibility, not mere pendency of an escalation.
	EscalationPending bool
	// Watched is true when a live batten watches the run.
	Watched bool
	// BuildChanged is true when the on-disk build differs from the last step's.
	BuildChanged bool
	// HandedOver is true when a hand-over settle record exists for this stop key and build.
	HandedOver bool
	// RepairCount is the number of re-steps already made at this stop key.
	RepairCount int
	// ResumeReturned is true when an applied tackler resume exists for this stop key.
	ResumeReturned bool
}

// driveAction is what the loop does for a decision.
type driveAction string

const (
	driveActionDone          driveAction = "done"
	driveActionBusyExit      driveAction = "busy-exit"
	driveActionHandBack      driveAction = "hand-back"
	driveActionRelayHandBack driveAction = "relay-hand-back"
	driveActionArbiter       driveAction = "arbiter"
	driveActionHandOver      driveAction = "hand-over"
	driveActionTackler       driveAction = "tackler"
	driveActionRestep        driveAction = "restep"
)

// driveDecision is the table's answer for one stop.
// Rule is the rule text the stop report quotes.
// FrictionNote asks for a friction note on a paused stop that carries no reason, from the envelope or the status.
type driveDecision struct {
	Action       driveAction
	Rule         string
	FrictionNote bool
}

// binaryEligible reports whether a newer on-disk build could change what happens at the stop:
// a blocked stop, a producer, bootstrap, unseeded or interrupted error, or a transient error.
func binaryEligible(stop Stop) bool {
	if stop.Key().State == string(shedengine.StateBlocked) {
		return true
	}
	if stop.OK {
		return false
	}
	switch stop.Kind {
	case shedverbs.KindProducer, shedverbs.KindBootstrap, shedverbs.KindUnseeded, shedverbs.KindInterrupted:
		return true
	}
	return stop.Transient != ""
}

// repairCount counts the restep settle records of the stop key.
func repairCount(records []SettleRecord, key StopKey) int {
	count := 0
	for _, record := range records {
		if record.Action == shedrun.SettleRestep && record.StopKey == key {
			count++
		}
	}
	return count
}

// decideDrive returns the first row of the rule table that matches the stop, and a hand-back when none does.
func decideDrive(stop Stop, facts driveFacts) driveDecision {
	state := stop.Key().State
	switch {
	case stop.OK && state == string(shedengine.StateDone):
		return driveDecision{Action: driveActionDone, Rule: "the run is done"}
	case stop.Kind == shedverbs.KindBusy:
		return driveDecision{Action: driveActionBusyExit, Rule: "another holder has the run: exit and leave it to them"}
	case stop.Kind == shedverbs.KindOwnership || (!stop.OK && stop.Kind == ""):
		return driveDecision{Action: driveActionHandBack, Rule: "an ownership error, or an error with no kind: hand back"}
	case state == string(shedengine.StatePaused):
		return driveDecision{Action: driveActionHandBack, Rule: "the run is paused: hold and hand back", FrictionNote: stop.Reason == ""}
	case stop.HandBack && state == string(shedengine.StateBlocked):
		return driveDecision{Action: driveActionHandBack, Rule: "a landing halt: only the orch's resume lifts it"}
	case budgetExhausted(stop):
		return driveDecision{Action: driveActionHandBack, Rule: "a bounce-budget-exhausted halt: only the parent's resume lifts it"}
	case preconditionUnmet(stop):
		return driveDecision{Action: driveActionHandBack, Rule: "a precondition-unmet halt: hand back, no tackler settles it"}
	case state == string(shedengine.StateAwaiting) && facts.EscalationPending:
		return driveDecision{Action: driveActionArbiter, Rule: "an awaiting review escalation: the arbiter decides"}
	case state == string(shedengine.StateAwaiting) && stop.ParentNotice != "":
		return driveDecision{Action: driveActionRelayHandBack, Rule: "an awaiting stop with a parent notice: relay it and hand back"}
	case state == string(shedengine.StateAwaiting):
		return driveDecision{Action: driveActionHandBack, Rule: "an awaiting stop: planned hand-back"}
	case binaryEligible(stop) && facts.BuildChanged && !facts.HandedOver:
		return driveDecision{Action: driveActionHandOver, Rule: "the on-disk build changed since the last step: hand over to a fresh driver"}
	case facts.ResumeReturned:
		return driveDecision{Action: driveActionArbiter, Rule: "a tackler resume returned to the same stop: the arbiter decides"}
	case !stop.OK && stop.Transient != "":
		return driveDecision{Action: driveActionTackler, Rule: "a transient error that outlived its re-step: the tackler decides"}
	case stop.Kind == shedverbs.KindInterrupted && stop.InterruptPolicy == shedverbs.InterruptPolicyReinvoke && facts.RepairCount < driveRepairCap:
		return driveDecision{Action: driveActionRestep, Rule: "an interrupted step under the reinvoke policy: re-step"}
	case stop.Kind == shedverbs.KindInterrupted && (stop.InterruptPolicy == shedverbs.InterruptPolicyHandback || stop.InterruptPolicy == ""):
		return driveDecision{Action: driveActionHandBack, Rule: "an interrupted step under the handback policy: hand back"}
	case isTacklerKind(stop.Kind) && !stop.OK, stop.Kind == shedverbs.KindInterrupted && stop.InterruptPolicy == shedverbs.InterruptPolicyReinvoke, state == string(shedengine.StateBlocked):
		return driveDecision{Action: driveActionTackler, Rule: "no rule settles the stop: the tackler decides"}
	}
	return driveDecision{Action: driveActionHandBack, Rule: "no rule names the stop: hand back"}
}

// budgetExhausted reports whether stop is a blocked stop whose reason, or whose error for a stop built from a status, starts with shedengine.ReasonBounceBudgetExhausted.
// Only the parent's resume lifts such a halt, so the table hands it back and never resumes it.
func budgetExhausted(stop Stop) bool {
	if stop.Key().State != string(shedengine.StateBlocked) {
		return false
	}
	return strings.HasPrefix(stop.Reason, shedengine.ReasonBounceBudgetExhausted) || strings.HasPrefix(stop.Error, shedengine.ReasonBounceBudgetExhausted)
}

// preconditionUnmet reports whether stop is a blocked stop whose reason, or whose error for a stop built from a status, starts with shedengine.ReasonPreconditionUnmet.
// No tackler can settle such a halt, so the table hands it back and never resumes it.
func preconditionUnmet(stop Stop) bool {
	if stop.Key().State != string(shedengine.StateBlocked) {
		return false
	}
	return strings.HasPrefix(stop.Reason, shedengine.ReasonPreconditionUnmet) || strings.HasPrefix(stop.Error, shedengine.ReasonPreconditionUnmet)
}

// isTacklerKind reports whether kind is an error kind the tackler judges when no earlier rule settled it.
func isTacklerKind(kind string) bool {
	switch kind {
	case shedverbs.KindProducer, shedverbs.KindBootstrap, shedverbs.KindUnseeded:
		return true
	}
	return false
}
