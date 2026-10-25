// arbiterconsume.go wires loom's ConsumeAttempt hook to the shed-layer arbiter's consumption of a resume verdict.

package loomcli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// loomConsumeAttempt is loom's ConsumeAttempt hook: it parses attempt as a positive attempt number of the run and consumes that attempt's directory.
func (c *loomCLI) loomConsumeAttempt(_ context.Context, attempt string) error {
	number, err := strconv.Atoi(attempt)
	if err != nil || number < 1 {
		return fmt.Errorf("loom: arbiter attempt %q is not a positive integer; way forward: report the stop to the run's parent in the stop report, and the parent decides", attempt)
	}
	return shedarbiter.ConsumeAttempt(shedrun.ArbiterAttemptDir(c.location, c.runID, number), shedengine.RelaunchRequest{
		StatusPath:              c.shedPaths.StatusPath,
		LockPath:                c.shedPaths.LockPath,
		StatusLockPath:          c.shedPaths.StatusLockPath,
		Producers:               c.routing.Producers,
		RunID:                   shedrun.ResolveRunID(c.location, c.runID),
		MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
		Guard:                   c.settledGuard(),
	})
}
