// act.go declares the act rule: whether a verb may act on a run, wait for it or hand it over.
// It also declares the refusal that words the answer.

package runfsm

import (
	"fmt"
	"strings"
	"time"
)

// CodeTipGrace is how long a code tip the run's last transition on origin names may stay missing from origin.
// It is measured from the commit time of that transition; past it, a tip absent from the owner's repository is given up.
const CodeTipGrace = 30 * time.Minute

// Inputs are the machine-local facts and the derived state the act rule decides over.
type Inputs struct {
	// State is the run's derived state; Judge fills it from Derive.
	State State
	// OwnedElsewhere is true when another machine owns the run.
	OwnedElsewhere bool
	// Owner names the machine that owns the run, for the hand-over refusal.
	Owner string
	// AdoptSubject names the subject whose adopt takes the run over, for the hand-over refusal.
	AdoptSubject string
	// Held is true while an adopt hold stands over the hub.
	Held bool
	// HoldMachine names the machine that wrote the hold, for the held refusal.
	HoldMachine string
	// PauseCovers is true when this machine's pause record covers the run.
	PauseCovers bool
	// PushOwed is true while a push of the run is owed:
	// origin is unreachable, the records side was deferred or a teardown lock is held.
	PushOwed bool
	// PushOwedError is the owed push's last error, for the push-owed refusal.
	PushOwedError string
	// PushOwedWorktree is the worktree whose side is owed, for the push-owed refusal.
	PushOwedWorktree string
	// PushOwedWayForward is the owed push's own way forward, which the push-owed refusal names when it is set.
	PushOwedWayForward string
	// CodeTipMissing is true when the run's code tip is not on origin.
	CodeTipMissing bool
	// CodeTip is the code tip the run's last transition on origin names, for the code-tip refusal.
	CodeTip string
	// CodeTipMachine is the machine whose transition named CodeTip, for the code-tip refusal.
	CodeTipMachine string
	// PastParentMerge is true when origin's fetched parent branch holds the task's work.
	PastParentMerge bool
}

// Answer is the act rule's answer.
type Answer string

// The act rule's answers.
const (
	// AnswerAct lets the verb act.
	AnswerAct Answer = "act"
	// AnswerWait makes the verb refuse until the named condition clears.
	AnswerWait Answer = "wait"
	// AnswerHandOver makes the verb refuse because another machine owns the run.
	AnswerHandOver Answer = "hand_over"
)

// Reason is why the act rule answered wait or hand over.
type Reason string

// The reasons of a wait or hand-over answer.
const (
	ReasonPushOwed       Reason = "push_owed"
	ReasonOwnedElsewhere Reason = "owned_elsewhere"
	ReasonCodeTip        Reason = "code_tip"
	ReasonHeld           Reason = "held"
	ReasonPauseCovers    Reason = "pause_covers"
	ReasonPaused         Reason = "paused"
	ReasonCancelling     Reason = "cancelling"
	ReasonTerminal       Reason = "terminal"
)

// Verdict is the act rule's answer with what its refusal names.
type Verdict struct {
	Answer Answer
	Reason Reason
	State  State
	// Owner and AdoptSubject are copied from the inputs for the hand-over refusal.
	Owner        string
	AdoptSubject string
	// PushOwedError, PushOwedWorktree and PushOwedWayForward are copied from the inputs for the push-owed refusal.
	// CodeTip and CodeTipMachine are copied from the inputs for the code-tip refusal.
	PushOwedError      string
	PushOwedWorktree   string
	PushOwedWayForward string
	CodeTip            string
	CodeTipMachine     string
	// CancelMachine and CancelRemove are the record's cancel intent, for the cancelling refusal; Judge fills them.
	CancelMachine string
	CancelRemove  bool
	// PausedByIntent is true when a pause intent holds the paused run; Judge fills it.
	PausedByIntent bool
	// HoldMachine is copied from the inputs for the held refusal.
	HoldMachine string
}

// Act answers whether a verb may act on the run the inputs describe, in the rule's order.
// An owed push waits.
// Another machine's run is handed over.
// A missing code tip waits.
// A landing run past its parent-side merge acts unless a pause covers it.
// Then a hold, a covering pause, and the paused, cancelling and terminal states wait, and everything else acts.
func Act(in Inputs) Verdict {
	verdict := Verdict{
		Answer: AnswerWait, State: in.State, Owner: in.Owner, AdoptSubject: in.AdoptSubject,
		PushOwedError: in.PushOwedError, PushOwedWorktree: in.PushOwedWorktree, PushOwedWayForward: in.PushOwedWayForward,
		CodeTip: in.CodeTip, CodeTipMachine: in.CodeTipMachine, HoldMachine: in.HoldMachine,
	}
	switch {
	case in.PushOwed:
		verdict.Reason = ReasonPushOwed
	case in.OwnedElsewhere:
		verdict.Answer, verdict.Reason = AnswerHandOver, ReasonOwnedElsewhere
	case in.CodeTipMissing:
		verdict.Reason = ReasonCodeTip
	case in.State == Landing && in.PastParentMerge && !in.PauseCovers:
		verdict.Answer = AnswerAct
	case in.Held:
		verdict.Reason = ReasonHeld
	case in.PauseCovers:
		verdict.Reason = ReasonPauseCovers
	case in.State == Paused:
		verdict.Reason = ReasonPaused
	case in.State == Cancelling:
		verdict.Reason = ReasonCancelling
	case in.State == Landed || in.State == Cancelled:
		verdict.Reason = ReasonTerminal
	default:
		verdict.Answer = AnswerAct
	}
	return verdict
}

// Refusal returns nil for an act answer and otherwise one error naming the slug, the reason and its way forward.
// retry is the caller's own re-run, named where the way forward is to try again.
func (v Verdict) Refusal(slug, retry string) error {
	var reason, wayForward string
	switch v.Reason {
	case ReasonPushOwed:
		reason = "a push to origin is owed"
		if v.PushOwedError != "" {
			reason += fmt.Sprintf(" (last error: %s)", v.PushOwedError)
		}
		switch {
		case v.PushOwedWayForward != "":
			wayForward = fmt.Sprintf("%s; then run %s again", v.PushOwedWayForward, retry)
		case v.PushOwedWorktree != "":
			wayForward = fmt.Sprintf("run `lym fabric push` in %s once origin answers, then run %s again", v.PushOwedWorktree, retry)
		default:
			wayForward = fmt.Sprintf("run %s again once the owed push lands", retry)
		}
	case ReasonOwnedElsewhere:
		reason = fmt.Sprintf("machine %s owns the run", v.Owner)
		wayForward = fmt.Sprintf("run the same verb on %s, or run `lym adopt %s` on this machine first", v.Owner, v.AdoptSubject)
	case ReasonCodeTip:
		reason = "the code tip the run's last transition names is not on origin yet"
		if v.CodeTip != "" {
			reason = fmt.Sprintf("the code tip %s the run's last transition names is not on origin yet", v.CodeTip)
		}
		if v.CodeTipMachine != "" {
			reason += fmt.Sprintf("; machine %s owes it", v.CodeTipMachine)
			wayForward = fmt.Sprintf("run `lym fabric push` in the task worktree on machine %s, then run %s again once the code push lands",
				v.CodeTipMachine, retry)
		} else {
			wayForward = fmt.Sprintf("run %s again once the code push lands", retry)
		}
	case ReasonHeld:
		machine := v.HoldMachine
		if machine == "" {
			machine = "another machine"
		}
		reason = fmt.Sprintf("an adopt on machine %s holds the hub", machine)
		wayForward = fmt.Sprintf("wait for that adopt's hold to clear, then run %s again; if machine %s has stopped, ask the operator", retry, machine)
	case ReasonPauseCovers:
		reason = "a pause covers the run"
		wayForward = fmt.Sprintf("run `lym tender resume %s`, then run %s again", slug, retry)
	case ReasonPaused:
		reason = "the run is paused"
		if v.PausedByIntent {
			wayForward = fmt.Sprintf("run `lym tender resume %s`, then run %s again", slug, retry)
		} else {
			wayForward = fmt.Sprintf("run `lym batten run %s` from the prime to resume the run, or `lym loom resume` in the task worktree to resume the inner run", slug)
		}
	case ReasonCancelling:
		finish := fmt.Sprintf("`lym batten cancel %s`", slug)
		if v.CancelRemove {
			finish = fmt.Sprintf("`lym batten cancel %s --remove`", slug)
		}
		reason = fmt.Sprintf("a cancel started on machine %s has not finished", v.CancelMachine)
		wayForward = fmt.Sprintf("run %s on machine %s to finish it", finish, v.CancelMachine)
	case ReasonTerminal:
		return v.terminalRefusal(slug)
	default:
		return nil
	}
	return fmt.Errorf("run %q cannot act: %s; way forward: %s", slug, reason, wayForward)
}

// terminalRefusal words the refusal of a landed or cancelled run.
func (v Verdict) terminalRefusal(slug string) error {
	if v.State == Cancelled {
		return fmt.Errorf("run %q cannot act: the run was cancelled; way forward: run `lym batten cancel %s --remove`, "+
			"which removes its record and pair, before a fresh run", slug, slug)
	}
	steps := []string{
		"`lym batten cleanup --apply` from the prime, which removes its run directory",
		fmt.Sprintf("`lym fabric remove %s` if its pair still stands", slug),
		fmt.Sprintf("for a task `lym board set-status '{\"slug\":\"%s\",\"status\":null}'`, which releases the entry", slug),
		"deleting the leftover task branch, locally and on the remote",
	}
	return fmt.Errorf("run %q cannot act: the slug has landed; way forward: to run it again, run %s; "+
		"or else give the new task another slug, the one way forward for a landed beam, whose done status the board refuses to release",
		slug, strings.Join(steps, ", then "))
}

// Aside names the states a verb sets aside before the act rule decides, because the verb itself resolves them.
type Aside struct {
	// Pause drops the pause intent and a covering pause.
	Pause bool
	// Cancel drops the cancel intent, finished or not.
	Cancel bool
	// UnfinishedCancel drops only an unfinished cancel intent.
	// A finished one stays, so a cancelled run still reads cancelled.
	UnfinishedCancel bool
	// Hold drops the adopt hold.
	Hold bool
	// ShedPause reads a shed state of paused as not paused.
	ShedPause bool
	// PushOwed drops the owed push: Inputs.PushOwed, its error, its worktree and its way forward.
	PushOwed bool
}

// Judge derives the run's state from r and decides over it, with what aside names cleared first.
// It returns Derive's reading and Act's verdict.
// The verdict carries the cancel intent and the pause intent for the refusals.
func Judge(r Records, in Inputs, aside Aside) (Reading, Verdict) {
	if aside.Cancel || (aside.UnfinishedCancel && r.Batten.Cancel != nil && r.Batten.Cancel.Finish == nil) {
		r.Batten.Cancel = nil
	}
	if aside.Pause {
		r.Batten.PauseIntent = nil
		in.PauseCovers = false
	}
	if aside.ShedPause {
		r.Batten.State = clearShedPause(r.Batten.State)
		r.Child.State = clearShedPause(r.Child.State)
	}
	if aside.Hold {
		in.Held = false
	}
	if aside.PushOwed {
		in.PushOwed = false
		in.PushOwedError = ""
		in.PushOwedWorktree = ""
		in.PushOwedWayForward = ""
	}
	reading := Derive(r)
	in.State = reading.State
	verdict := Act(in)
	if r.BattenFound && r.Batten.Cancel != nil {
		verdict.CancelMachine = r.Batten.Cancel.Machine
		verdict.CancelRemove = r.Batten.Cancel.Remove
	}
	verdict.PausedByIntent = reading.State == Paused && r.BattenFound && r.Batten.PauseIntent != nil
	return reading, verdict
}
