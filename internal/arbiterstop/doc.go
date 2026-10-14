// Package arbiterstop decides which halted stops the arbiter may settle.
//
// One function, Eligibility, holds the rule, and Check feeds it from the status file, the stop envelope and the disk.
// The arbiter verb, the Go driver's escalation fact and batten's eligible-stop hold all answer through it, so the three cannot disagree.
//
// It is not in the loom CLI package because batten must call it, and the batten CLI cannot import the loom CLI without linking tree-sitter into its test binary.
//
// An awaiting stop is eligible only with a pending review escalation of cause `hard-cap` or `circling`;
// a `budget` escalation, an unknown cause and an unreadable record all go to the run's parent.
// Every refusal ends with HandBackWayForward.
package arbiterstop
