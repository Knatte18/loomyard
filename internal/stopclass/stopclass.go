// stopclass.go declares the stop classes, autonomy levels, verdicts, the settlement table and Settle, which reads it.

package stopclass

import "fmt"

// Class names one row of the settlement table: a kind of stop a run can reach.
// Its value is the stamp a halt records as its class.
type Class string

// The stop classes, one per row of the settlement table.
const (
	// ClassBoardClaim is a board claim push refused at Worktree-Create, a start failing on a held board lock among them.
	ClassBoardClaim Class = "board-claim"
	// ClassPause is a pause that covers the run.
	ClassPause Class = "pause"
	// ClassAgentFailure is an agent launch that failed, a session that timed out, a failed fixer fork or a lost strand.
	ClassAgentFailure Class = "agent-failure"
	// ClassStaleBuild is a run driven by a build older than the newest one.
	ClassStaleBuild Class = "stale-build"
	// ClassReviewEscalation is a review escalation of cause hard-cap, circling, decision or stalemate.
	ClassReviewEscalation Class = "review-escalation"
	// ClassBudgetEscalation is a review escalation of cause budget.
	ClassBudgetEscalation Class = "budget-escalation"
	// ClassBounceBudget is a row's bounce budget exhausted.
	ClassBounceBudget Class = "bounce-budget"
	// ClassGateAttempts is a Stuck after a row's gate attempts ran out.
	ClassGateAttempts Class = "gate-attempts"
	// ClassRowDefect is a Webster failure, a run-exit audit finding, or a defect or hub config fault at a row.
	ClassRowDefect Class = "row-defect"
	// ClassLanding is a landing stop or a deploy failure at Land.
	ClassLanding Class = "landing"
	// ClassDriverNotParked is a verb refused because the run's driver has not parked; the verb waits instead.
	ClassDriverNotParked Class = "driver-not-parked"
	// ClassDeadDriver is a dead driver or an agent API error, reported by the dobby's notices rather than at a row.
	ClassDeadDriver Class = "dead-driver"
	// ClassRunQuestion is an agent question, an entry overtaken, a scope change or a cancel, which the orch classifies.
	ClassRunQuestion Class = "run-question"
	// ClassOwnerHalt is a halt for the run's owner machine or a takeover.
	ClassOwnerHalt Class = "owner-halt"
	// ClassOperatorStep is a step only the operator can take, or a halt with no settling action open.
	// The orch classifies it.
	ClassOperatorStep Class = "operator-step"
	// ClassUnnamed is a stop no other class names, which the stop tackler takes first.
	ClassUnnamed Class = "unnamed"
)

// Level is a beam.autonomy level: how far the orch settles a run's halts without the operator.
// Its values are the strings the config holds, so a level read from config converts with Level(value).
type Level string

// The autonomy levels.
const (
	// LevelOff relays to the operator every halt the orch does not settle.
	LevelOff Level = "false"
	// LevelOn has the orch settle every halt but the severe ones.
	LevelOn Level = "true"
	// LevelUnattended has the orch settle every halt and never ask the operator; only a severe halt reaches them.
	LevelUnattended Level = "unattended"
)

// Verdict names who settles a stop.
// Its value is the stamp a halt records as its settler.
type Verdict string

// The verdicts.
const (
	// VerdictLym has lym settle the stop itself.
	VerdictLym Verdict = "lym"
	// VerdictOrch hands the stop to the orch.
	VerdictOrch Verdict = "orch"
	// VerdictOperator makes the stop severe: it reaches the operator.
	VerdictOperator Verdict = "operator"
)

// BoundUnlimited is the bound of a class lym settles every time: a wait that ends on its own.
const BoundUnlimited = -1

// severeHaltAtRow is the halt count at a row from which a third-halt operator case holds.
const severeHaltAtRow = 3

// Decision is the settlement table's answer for one stop.
type Decision struct {
	// Verdict names who settles the stop.
	Verdict Verdict
	// Bound is how many settles of the class at the row the table grants lym: zero, a count or BoundUnlimited.
	Bound int
}

// operatorCase is when a class reaches the operator under the levels "true" and "unattended".
type operatorCase int

const (
	// operatorNever is a class whose operator column holds no counted case.
	operatorNever operatorCase = iota
	// operatorThirdHalt is a class that reaches the operator at the third halt at its row.
	operatorThirdHalt
	// operatorAlways is a class that always reaches the operator.
	operatorAlways
)

// settlement is one row of the settlement table.
type settlement struct {
	lymBound int
	operator operatorCase
}

// table is the settlement table, one row per class.
var table = map[Class]settlement{
	ClassBoardClaim:       {lymBound: 1, operator: operatorThirdHalt},
	ClassPause:            {lymBound: BoundUnlimited, operator: operatorNever},
	ClassAgentFailure:     {lymBound: 1, operator: operatorThirdHalt},
	ClassStaleBuild:       {lymBound: 0, operator: operatorThirdHalt},
	ClassReviewEscalation: {lymBound: 0, operator: operatorThirdHalt},
	ClassBudgetEscalation: {lymBound: 0, operator: operatorThirdHalt},
	ClassBounceBudget:     {lymBound: 1, operator: operatorThirdHalt},
	ClassGateAttempts:     {lymBound: 1, operator: operatorThirdHalt},
	ClassRowDefect:        {lymBound: 0, operator: operatorThirdHalt},
	ClassLanding:          {lymBound: 0, operator: operatorThirdHalt},
	ClassDriverNotParked:  {lymBound: BoundUnlimited, operator: operatorNever},
	ClassDeadDriver:       {lymBound: 0, operator: operatorNever},
	ClassRunQuestion:      {lymBound: 0, operator: operatorNever},
	ClassOwnerHalt:        {lymBound: 0, operator: operatorAlways},
	ClassOperatorStep:     {lymBound: 0, operator: operatorAlways},
	ClassUnnamed:          {lymBound: 0, operator: operatorThirdHalt},
}

// Settle decides who settles a stop of class at level, from the row's halt count and lym's settles of the class there.
// It answers the operator when the level is "true" or "unattended" and the class's operator case holds.
// Otherwise it answers lym while lymSettles is within the class's bound, and the orch after that.
// It panics on a class or level outside its constants, which only a caller's own bug produces.
func Settle(class Class, level Level, haltsAtRow, lymSettles int) Decision {
	row, ok := table[class]
	if !ok {
		panic(fmt.Sprintf("stopclass: unknown class %q", class))
	}
	decision := Decision{Verdict: VerdictOrch, Bound: row.lymBound}
	switch level {
	case LevelOff:
	case LevelOn, LevelUnattended:
		if row.operator == operatorAlways || (row.operator == operatorThirdHalt && haltsAtRow >= severeHaltAtRow) {
			decision.Verdict = VerdictOperator
			return decision
		}
	default:
		panic(fmt.Sprintf("stopclass: unknown level %q", level))
	}
	if row.lymBound == BoundUnlimited || lymSettles < row.lymBound {
		decision.Verdict = VerdictLym
	}
	return decision
}
