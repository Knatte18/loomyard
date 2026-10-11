// driverresume.go implements the resume branch both `lyx loom start` and `lyx loom resume` take: typing the one resume line into a parked loom driver's pane instead of spawning a fresh driver.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// driverResumeSendAttempts and driverResumeSendInterval bound the wait for a parked driver's pane to be input-ready when the engine has no idle reading: about a minute in all.
// Against Claude the send's own send_ready_timeout_s window bounds that wait instead.
// The attempt count is the cap, not elapsed time, so a fake wait in a test cannot loop forever.
const (
	driverResumeSendAttempts = 20
	driverResumeSendInterval = 3 * time.Second
)

// resumeReportPath returns the stop-report path the resume line for the run's current park asks the driver to write.
// The path is picked once per park and kept in the resume-report file beside the park marker, keyed to the marker's content, the stop report the driver parked on;
// every rerun against that park returns the same path, so the line it types is identical.
// A park with different marker content picks a fresh path.
func (c *loomCLI) resumeReportPath(runID string) (string, error) {
	markerContent, err := os.ReadFile(shedrun.ParkMarker(c.location, runID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	park := strings.TrimSpace(string(markerContent))
	resumeReport := shedrun.ResumeReport(c.location, runID)
	stored, err := os.ReadFile(resumeReport)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if storedPark, storedPath, ok := strings.Cut(string(stored), "\n"); ok && storedPark == park {
		if storedPath = strings.TrimSpace(storedPath); storedPath != "" {
			return storedPath, nil
		}
	}
	reportPath := driverReportPath(c.location, runID, time.Now, newDriverReportRand())
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(resumeReport, []byte(park+"\n"+reportPath+"\n"), 0o644); err != nil {
		return "", err
	}
	return reportPath, nil
}

// driverBusyMessage is the refusal message for a resume line that a busy driver pane would not take.
// err carries shuttle's reason, which quotes the draft; the message adds the way forward, with retry the calling verb.
func driverBusyMessage(err error, retry string) string {
	return err.Error() + `; clear or send the draft in the driver's pane, then run "` + retry + `" again, or run "lyx batten run <slug>" from the prime`
}

// resumeParkedDriver resumes the live, parked driver strand guid by typing driverResumeLine into its pane, then removes the park marker and the resume-report file.
//
// Only a not-ready pane (shuttleengine.ErrPaneNotReady), which an engine without the idle reading refuses at once, is waited on and retried.
// Against Claude a busy or not-ready pane waits inside the send and fails with shuttleengine.ErrSessionBusy, which the loop does not retry;
// that error is returned wrapped with its kind intact and without the "may have resumed" advice, since nothing was typed, and the caller refuses with shedrun.StartDriverBusyKind.
// Any other Send error ends the loop at once: Send already replays its own keystrokes internally,
// and a "never appeared" verification failure can follow a delivery the pane hid, so re-sending could type the line into the driver twice.
// A failure leaves the marker in place,
// and its message names retry, the calling verb, as the way to try again.
// It returns the path of the stop report the driver is asked to write.
func (c *loomCLI) resumeParkedDriver(guid, retry string) (string, error) {
	runID := shedrun.ResolveRunID(c.location, c.runID)
	reportPath, err := c.resumeReportPath(runID)
	if err != nil {
		return "", err
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
		if errors.Is(sendErr, shuttleengine.ErrSessionBusy) {
			return "", fmt.Errorf("loom: the driver's pane did not take the resume line: %w", sendErr)
		}
		return "", fmt.Errorf("loom: could not deliver the resume line to the driver after %d attempt(s): %w; the driver may have resumed on its own (a re-step it starts itself removes the park marker), so check the run's status with \"lyx loom status\", and run \"%s\" again if it is still halted", attempts, sendErr, retry)
	}

	if err := os.Remove(shedrun.ParkMarker(c.location, runID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Remove(shedrun.ResumeReport(c.location, runID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	logger.Info("loom: resumed the parked driver", "guid", guid, "report", reportPath)
	return reportPath, nil
}
