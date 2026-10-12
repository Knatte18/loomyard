// watchdogline.go renders and finds the line `lyx gate test`'s CPU watchdog prints after it quits a Go test binary, and names the way forward a halt gives after one.

package gateslot

import (
	"fmt"
	"strings"
	"time"
)

const (
	watchdogLinePrefix = "lyx gate test: CPU watchdog: "
	watchdogLineBusy   = " kept a CPU core busy for "
	watchdogLineSuffix = "; sent SIGQUIT, so its goroutine stacks follow in the output"
)

// BusyQuitWayForward is the way forward a halt names after the CPU watchdog quit a test binary.
const BusyQuitWayForward = "raise busy_quit_sec in the hub's gate.yaml through \"lyx config gate\" from the prime"

// WatchdogLine renders the line the watchdog prints after sending the test binary at pid one SIGQUIT, window in time.Duration's own form.
func WatchdogLine(binary string, pid int, window time.Duration) string {
	return fmt.Sprintf("%s%s (pid %d)%s%s%s", watchdogLinePrefix, binary, pid, watchdogLineBusy, window, watchdogLineSuffix)
}

// FindWatchdogQuit returns the window text of the first watchdog line anywhere in log, and false when the log holds none.
func FindWatchdogQuit(log string) (window string, ok bool) {
	for _, line := range strings.Split(log, "\n") {
		rest, found := strings.CutPrefix(line, watchdogLinePrefix)
		if !found {
			continue
		}
		_, afterBusy, found := strings.Cut(rest, watchdogLineBusy)
		if !found {
			continue
		}
		if window, found := strings.CutSuffix(afterBusy, watchdogLineSuffix); found {
			return window, true
		}
	}
	return "", false
}
