// handback.go implements HandsBack and EscalateHalt, the predicates every driver reads to tell a landing halt that goes to the orch, or a tapestry, darn or sampler escalate halt that goes to the arbiter first, from a stop the recipe routes itself.

package landingshed

import "strings"

// HandsBack reports whether a blocked stop at producer with the persisted reason is a landing halt for the orch.
// A stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix is one, as the fixer and its caps raise them.
// A stop at Merge-In, the row's name before its rename, is read the same way, since batten and the arbiter read a child's status file before any loom verb has renamed a run halted there.
func HandsBack(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName, finalizeName:
		return strings.HasPrefix(reason, LandingStopPrefix)
	}
	return false
}

// EscalateHalt reports whether a blocked stop at producer with the persisted reason is a tapestry, darn or sampler merge fixer's escalate halt.
// A stop at Parent-Align, Finalize or Merge-In, the row's name before its rename, whose reason opens with EscalateStopPrefix is one.
// Such a halt is no hand-back: the arbiter judges it first, and batten's binary re-step leaves it alone.
func EscalateHalt(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName, finalizeName:
		return strings.HasPrefix(reason, EscalateStopPrefix)
	}
	return false
}
