// arbitereligibility.go holds the way-forward clause that the arbiter's refusals end with.

package loomcli

import "github.com/Knatte18/loomyard/internal/arbiterstop"

// arbiterHandBackWayForward is the way-forward clause every arbiter refusal ends with:
// the hand-back to the run's parent.
const arbiterHandBackWayForward = arbiterstop.HandBackWayForward
