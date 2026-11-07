// Package arbiterstop decides which halted stops the arbiter may settle.
//
// Eligibility holds the rule, and Check feeds it from the status file, the stop envelope and the disk.
// Every verb and hold that judges a stop answers through them, so the answers cannot disagree.
//
// The package also reads what batten's eligible-stop hold decides on:
// HoldReading returns the task worktree's arbiter facts as a battenshed.ChildArbiterStop.
//
// It is not in the loom CLI package because batten must call it,
// and the batten CLI cannot import the loom CLI without linking tree-sitter into its test binary.
//
// An awaiting stop is eligible only with a pending review escalation of cause `hard-cap`, `circling` or `decision`.
// Each of those causes is a judgment call the arbiter settles before the run's parent hears of it.
// A `budget` escalation, an unknown cause and an unreadable record all go to the run's parent.
// A blocked stop that is a landing halt is refused too, since only the run's parent lifts it;
// the landing rows' hand-back predicate reads the status's producer and error.
// A decision halt, a blocked stop whose error opens with the decision halt prefix, is refused as well.
// A fixer's escalate halt opens with neither prefix.
// So it is no landing halt, and it is eligible as a blocked stop.
// HandsBack is the one predicate of a blocked stop that goes to the run's parent: a landing halt or a decision halt.
// Every refusal ends with HandBackWayForward.
//
// The rule is per recipe.
// Readers.Batten, set by NewBattenReaders, selects batten's rule:
// a stop the tackler escalated is eligible, error stops of any kind and blocked stops included.
// A hand-back to the run's parent is the exception.
// The hand-backs are a spent bounce budget, an unmet precondition and a blocked Land stop that honours a pause,
// and each is refused with a refusal ending in BattenHandBackWayForward.
// Batten has no landing halt, so no batten stop is refused as one.
// Every other reader set answers by the loom rule above.
package arbiterstop
