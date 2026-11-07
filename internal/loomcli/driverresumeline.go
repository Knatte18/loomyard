// driverresumeline.go composes the one line typed into a parked loom driver session to resume it.
// It lives beside driverprompt.go, which composes the launch prompt the session parks under.

package loomcli

import "fmt"

// driverResumeLine composes the single line that resumes a parked loom driver.
// It names the run by run-id.
// It tells the driver to take a fresh baseline and to reset its repair and re-step budgets.
// It also tells it to write its next stop report to reportPath.
//
// The line carries no newline, so shuttleengine.Runner.Send accepts it.
// It is prompt text alone.
// It names no provider flag or command line, so the provider choice stays out of the prompt.
//
// A resume line that arrives while a step is in flight starts no second step.
// The driver stencil owns that rule; the line only carries the report path and the reset.
func driverResumeLine(runID string, reportPath string) string {
	return fmt.Sprintf(
		"Resume the loom driver run %q: take a fresh baseline, reset your repair and re-step budgets, and write your next stop report to %q.",
		runID, reportPath,
	)
}
