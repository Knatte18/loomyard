// gotoresume.go sets aside the interrupted shuttle runs a goto restarts, so the rows it moves onto start fresh sessions.

package loomcli

import "github.com/Knatte18/loomyard/internal/logger"

// gotoAdmitted returns the hook a written goto calls with its target.
// It first sets aside every interrupted shuttle run of the worktree:
// a goto restarts the work on purpose,
// and a start that resumed such a run's session would carry on the work the goto discards.
// It then resets a PR-Review review the move lands on.
// A receiver with no location returns nil.
func (c *loomCLI) gotoAdmitted() func(target string) error {
	reset := c.prReviewGotoAdmitted()
	if reset == nil {
		return nil
	}
	return func(target string) error {
		c.retireInterruptedRuns()
		return reset(target)
	}
}

// retireInterruptedRuns sets aside the worktree's interrupted shuttle runs and logs how many it set aside.
// A failure is logged and never fails the goto, since the move is already written;
// it can only let a row the goto restarts resume an interrupted session.
func (c *loomCLI) retireInterruptedRuns() {
	retire := c.retireRuns
	if retire == nil {
		retire = func() (int, error) {
			runner, err := c.stepStopRunner()
			if err != nil {
				return 0, err
			}
			return runner.RetireInterruptedRuns()
		}
	}
	retired, err := retire()
	if err != nil {
		logger.Warn("loom: a goto could not set aside the interrupted shuttle runs, so a row it restarts may resume one", "error", err)
	}
	if retired > 0 {
		logger.Info("loom: a goto set aside the interrupted shuttle runs, so the rows it restarts start fresh sessions", "runs", retired)
	}
}
