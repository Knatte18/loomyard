// driverresumeline.go composes the one line `lyx loom start` types into a parked ly-drive session to
// resume it. It lives beside driverprompt.go, which composes the launch prompt the session parks under.

package loomcli

import "fmt"

// driverResumeLine composes the single line that resumes a parked ly-drive driver. It names the run
// by run-id and tells the driver to take a fresh baseline, reset its repair and re-step budgets, and
// write its next stop report to reportPath.
//
// The line carries no newline, so shuttleengine.Runner.Send accepts it, and it is prompt text alone:
// it names no provider flag or command line, per the Shuttle Provider-Seam Invariant.
//
// A resume line that arrives while a step is in flight starts no second step. The skill owns that
// rule; the line only carries the report path and the reset.
func driverResumeLine(runID string, reportPath string) string {
	return fmt.Sprintf(
		"Resume the ly-drive run %q: take a fresh baseline, reset your repair and re-step budgets, and write your next stop report to %q.",
		runID, reportPath,
	)
}
