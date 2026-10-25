// Package arbiterstop decides which halted stops the arbiter may settle.
//
// One function, Eligibility, holds the rule, and Check feeds it from the status file, the stop envelope and the disk.
// The arbiter verb, the Go driver's escalation fact and batten's eligible-stop hold all answer through it, so the three cannot disagree.
//
// The package also reads what batten's eligible-stop hold decides on: HoldReading returns the task worktree's arbiter facts as a battenshed.ChildArbiterStop.
//
// It is not in the loom CLI package because batten must call it, and the batten CLI cannot import the loom CLI without linking tree-sitter into its test binary.
//
// An awaiting stop is eligible only with a pending review escalation of cause `hard-cap` or `circling`;
// a `budget` or `decision` escalation, an unknown cause and an unreadable record all go to the run's parent.
// A blocked stop that is a landing halt, as the landing rows' hand-back predicate reads the status's producer and error, is refused too, since only the run's parent lifts it.
// So is a decision halt, a blocked stop whose error opens with the decision halt prefix.
// HandsBack is the one predicate of a blocked stop that goes to the run's parent: a landing halt or a decision halt.
// Every refusal ends with HandBackWayForward.
package arbiterstop
