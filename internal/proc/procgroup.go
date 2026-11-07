// procgroup.go makes a command's cancel or expiry kill its whole process group where the platform has one.

package proc

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

// ErrNoProcessGroup is the group-kill error reported where the platform has no process group to signal.
// Only the started process itself is then killed.
var ErrNoProcessGroup = errors.New("proc: no process group on this platform")

// ConfigureGroupKill makes a context cancel or expiry of cmd kill the command's whole process group.
// This keeps a descendant of the command from outliving it.
// When the group kill fails it kills the command alone;
// where the platform has no process group it always kills the command alone and reports ErrNoProcessGroup.
// It calls report with the command's pid after each kill:
// a nil groupErr when the group was killed, otherwise the group kill's error.
// proc cannot log, so the caller's report callback is where a kill is logged.
func ConfigureGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	configurePlatformGroupKill(cmd, report)
}

// ConfigureGroupTerminate makes a context cancel or expiry of cmd send SIGTERM to the command's whole process group.
// It sends SIGKILL to the same group after killAfter, so a descendant can shut down but cannot outlive the grace.
// Where the platform has no process group the cancel kills the command alone and reports ErrNoProcessGroup.
// It calls report with the command's pid, the signal sent and the signal's error after each send.
// A SIGTERM that fails because the group is gone is returned from the cancel.
// Any other failed SIGTERM kills the command alone.
// Neither case starts a SIGKILL timer.
// proc cannot log, so the caller's report callback is where a signal is logged.
//
// The returned awaitGone is called once cmd.Wait or cmd.Run has returned.
// It returns at once when the cancel never sent a signal;
// otherwise it returns once the group is empty, or once the SIGKILL has been sent;
// when the group is empty, it also stops the SIGKILL timer.
func ConfigureGroupTerminate(cmd *exec.Cmd, killAfter time.Duration, report func(pid int, sig os.Signal, groupErr error)) (awaitGone func()) {
	return configurePlatformGroupTerminate(cmd, killAfter, report)
}
