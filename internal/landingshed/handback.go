// handback.go implements HandsBack and EscalateHalt, the predicates every driver reads to tell a landing halt that goes to the orch, or a loom or darn escalate halt that goes to the arbiter first, from a stop the recipe routes itself.

package landingshed

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// HandsBack reports whether a blocked stop at producer with the persisted reason is a landing halt for the orch.
// A stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix is one, as the fixer and its caps raise them.
// A stop at Merge-In, the row's name before its rename, is read the same way, since batten and the arbiter read a child's status file before any loom verb has renamed a run halted there.
// A stop at Finalize, the only landing row with a reroute edge, whose reason opens with the reroute-cap prefix is one too.
// Shed writes the reroute-cap prefix for every row, so the producer match keeps another row's reroute-cap halt out.
func HandsBack(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName:
		return strings.HasPrefix(reason, LandingStopPrefix)
	case finalizeName:
		return strings.HasPrefix(reason, LandingStopPrefix) || strings.HasPrefix(reason, shedengine.ReasonRerouteCapReached)
	}
	return false
}

// EscalateHalt reports whether a blocked stop at producer with the persisted reason is a loom or darn merge fixer's escalate halt.
// A stop at Parent-Align, Finalize or Merge-In, the row's name before its rename, whose reason opens with EscalateStopPrefix is one.
// Such a halt is no hand-back: the arbiter judges it first, and batten's binary re-step leaves it alone.
func EscalateHalt(producer, reason string) bool {
	switch producer {
	case parentAlignName, retiredMergeInName, finalizeName:
		return strings.HasPrefix(reason, EscalateStopPrefix)
	}
	return false
}
