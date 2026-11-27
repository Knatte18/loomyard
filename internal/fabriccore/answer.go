// answer.go holds what a rule answers to a snapshot: a refusal, a phase request or an action list.

package fabriccore

// Refusal is a rule's refusal of a verb.
// Every refusal a rule returns names its way forward.
type Refusal struct {
	Reason     string
	WayForward string
}

// Error implements error with the reason, followed by the way forward when it is set.
func (r *Refusal) Error() string {
	if r.WayForward == "" {
		return r.Reason
	}
	return r.Reason + "; " + r.WayForward
}

// Answer is a rule's answer to a snapshot, holding exactly one of a refusal, a phase request or an action list.
// An answer with none of them means the verb is done; one with more than one is a wiring fault.
type Answer struct {
	// Refusal refuses the verb; any non-nil value refuses, whatever its reason holds.
	Refusal *Refusal
	// Phase is the phase the rule asks to be read.
	Phase Phase
	// Read names the fields to read in the requested phase.
	Read []Field
	// Actions is the action list to run.
	Actions []Action
	// Reread names the fields to read again once the action list ends.
	Reread []Field
}

// Rule is a verb's rule: it answers what the verb does next for a snapshot.
type Rule func(snapshot Snapshot) Answer
