// handback.go implements HandsBack, the one predicate every driver reads to tell a landing halt that goes to the orch from a stop the recipe routes itself.

package landingshed

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// HandsBack reports whether a blocked stop at producer with the persisted reason is a landing halt for the orch.
// A stop at Merge-In or Finalize whose reason opens with LandingStopPrefix is one, as the fixer and its caps raise them.
// A stop at Finalize, the only landing row with a reroute edge, whose reason opens with the reroute-cap prefix is one too.
// Shed writes the reroute-cap prefix for every row, so the producer match keeps another row's reroute-cap halt out.
func HandsBack(producer, reason string) bool {
	switch producer {
	case mergeInName:
		return strings.HasPrefix(reason, LandingStopPrefix)
	case finalizeName:
		return strings.HasPrefix(reason, LandingStopPrefix) || strings.HasPrefix(reason, shedengine.ReasonRerouteCapReached)
	}
	return false
}
