// verifyhang.go classifies a failed verify run as a hang: a test binary stopped by go test's timeout, a goroutine dump, a signal or the CPU watchdog, whose rerun would only hang again.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

const (
	// watchdogHangMarker names a hang the gate test's CPU watchdog caused.
	watchdogHangMarker = "CPU watchdog SIGQUIT"
	// testTimeoutMarker is go test's report of a package that outlived its -timeout.
	testTimeoutMarker = "panic: test timed out after"
	// sigquitMarker is the header of the goroutine dump a Go program prints after SIGQUIT.
	sigquitMarker = "SIGQUIT: quit"
	// signalLinePrefix starts go test's report of a test binary ended by a signal.
	signalLinePrefix = "signal: "
	// firstSignalExit is the lowest shell exit code that reports death by a signal.
	firstSignalExit = 128
)

// verifyHang is a failed verify run read as a hang.
type verifyHang struct {
	// Matched is the marker, log line or `exit code <n>` that named the hang.
	Matched string
	// Stacks is set when the log holds goroutine stacks.
	Stacks bool
	// Window is the CPU watchdog's busy window, empty unless the watchdog quit a test binary.
	Window string
}

// classifyHang reads a failed result and its log and reports whether the run hung.
// The CPU watchdog line, go test's timeout panic and the SIGQUIT dump header are checked first, in that order, then a log line go test starts with `signal: `, then an exit code of 128 or above or a shell killed by a signal.
// A shell that could not start and an ordinary non-zero exit are no hang.
// The markers are matched as text anywhere in the log, so a test that prints one reads as a hang too.
func classifyHang(res verifytree.Result, log string) (verifyHang, bool) {
	if window, ok := gateslot.FindWatchdogQuit(log); ok {
		return verifyHang{Matched: watchdogHangMarker, Stacks: true, Window: window}, true
	}
	for _, marker := range []string{testTimeoutMarker, sigquitMarker} {
		if strings.Contains(log, marker) {
			return verifyHang{Matched: marker, Stacks: true}, true
		}
	}
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, signalLinePrefix) {
			return verifyHang{Matched: strings.TrimSpace(line)}, true
		}
	}
	if res.ExitCode >= firstSignalExit || (res.ExitCode == -1 && res.Detail == "") {
		return verifyHang{Matched: fmt.Sprintf("exit code %d", res.ExitCode)}, true
	}
	return verifyHang{}, false
}
