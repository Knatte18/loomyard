// handback.go holds the predicates every driver reads to sort a blocked stop: HandsBack and EscalateHalt.
// HandsBack marks a landing halt that goes to the orch.
// EscalateHalt marks a tapestry, darn or sampler merge fixer's escalate halt that goes to the arbiter first.
// Other stops are routed by the recipe itself.

package landingshed

import "strings"

// HandsBack reports whether a blocked stop at producer with the persisted reason is a landing halt for the orch.
// A stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix is one.
// The fixer and its caps raise such stops.
// A stop at Merge-In, the row's name before its rename, is read the same way:
// batten and the arbiter read a child's status file before any loom verb has renamed a run halted there.
func HandsBack(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName, finalizeName:
		return strings.HasPrefix(reason, LandingStopPrefix)
	}
	return false
}

// EscalateHalt reports whether a blocked stop at producer with the persisted reason is an escalate halt.
// The tapestry, darn or sampler merge fixer raises such a halt.
// A stop at Parent-Align, Finalize or Merge-In whose reason opens with EscalateStopPrefix is one.
// Merge-In is the row's name before its rename.
// An escalate halt is no hand-back: the arbiter judges it first, and batten's binary re-step leaves it alone.
func EscalateHalt(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName, finalizeName:
		return strings.HasPrefix(reason, EscalateStopPrefix)
	}
	return false
}
