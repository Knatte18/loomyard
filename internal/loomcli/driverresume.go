// driverresume.go implements the resume branch of `lyx loom start`: typing the one resume line into
// a parked ly-drive driver's pane instead of spawning a fresh driver.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverResumeSendAttempts and driverResumeSendInterval bound the wait for a parked driver's pane to
// be input-ready: about a minute in all. The attempt count is the cap, not elapsed time, so a fake
// wait in a test cannot loop forever.
const (
	driverResumeSendAttempts = 20
	driverResumeSendInterval = 3 * time.Second
)

// resumeParkedDriver resumes the live, parked driver strand guid by typing driverResumeLine into its
// pane, then removes the park marker.
//
// Only a not-ready pane (shuttleengine.ErrPaneNotReady) is waited on and retried. Any other Send
// error ends the loop at once: Send already replays its own keystrokes internally, and a "never
// appeared" verification failure can follow a delivery the pane hid, so re-sending could type the
// line into the driver twice. A failure leaves the marker in place, so the retry is a second
// `lyx loom start`.
func (c *loomCLI) resumeParkedDriver(guid string) error {
	runID := shedrun.ResolveRunID(c.location, c.runID)
	reportPath := driverReportPath(c.location, runID, time.Now, newDriverReportRand())
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return err
	}
	line := driverResumeLine(runID, reportPath)

	var sendErr error
	attempts := 0
	for attempts < driverResumeSendAttempts {
		if attempts > 0 && c.driverResumeWait != nil {
			c.driverResumeWait()
		}
		attempts++
		sendErr = c.driverSender.SendDriver(guid, line)
		logger.Info("loom: resume line send attempt", "guid", guid, "attempt", attempts, "err", sendErr)
		if sendErr == nil || !errors.Is(sendErr, shuttleengine.ErrPaneNotReady) {
			break
		}
	}
	if sendErr != nil {
		logger.Warn("loom: could not resume the parked driver", "guid", guid, "attempts", attempts, "err", sendErr)
		return fmt.Errorf("loom: could not resume the parked driver after %d attempt(s): %w; run \"lyx loom start\" again once the driver's pane is ready", attempts, sendErr)
	}

	if err := os.Remove(shedrun.ParkMarker(c.location, runID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	logger.Info("loom: resumed the parked driver", "guid", guid, "report", reportPath)
	return nil
}
