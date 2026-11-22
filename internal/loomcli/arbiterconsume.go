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

// loomConsumeAttempt applies a resume verdict when the driver relaunches the loop, naming its attempt.
// It returns an error unless the attempt is a positive integer.
func (c *loomCLI) loomConsumeAttempt(_ context.Context, attempt string) error {
	number, err := strconv.Atoi(attempt)
	if err != nil || number < 1 {
		return fmt.Errorf("loom: arbiter attempt %q is not a positive integer; way forward: report the stop to the run's parent in the stop report, and the parent decides", attempt)
	}
	return shedarbiter.ConsumeAttempt(shedrun.ArbiterAttemptDir(c.location, c.runID, number), c.relaunchRequest(shedrun.ResolveRunID(c.location, c.runID), c.settledGuard()))
}

// relaunchRequest builds the request that relaunches the run runID over the receiver's status files.
// Its guard ends the run's fixers once guard admits the move,
// and its Admitted hook resets a PR-Review review that the move lands on.
func (c *loomCLI) relaunchRequest(runID string, guard shedengine.GotoGuard) shedengine.RelaunchRequest {
	return shedengine.RelaunchRequest{
		StatusPath:              c.shedPaths.StatusPath,
		LockPath:                c.shedPaths.LockPath,
		StatusLockPath:          c.shedPaths.StatusLockPath,
		Producers:               c.routing.Producers,
		RunID:                   runID,
		MissingStatusWayForward: c.shedPaths.MissingStatusWayForward,
		Guard:                   c.gotoGuardEndingFixers(guard),
		Admitted:                c.prReviewGotoAdmitted(),
	}
}
