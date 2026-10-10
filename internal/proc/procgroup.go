// procgroup.go makes a command's cancel or expiry kill its whole process group where the platform has one.

package proc

import (
	"errors"
	"os/exec"
)

// ErrNoProcessGroup is the group-kill error ConfigureGroupKill reports where the platform has no process group to signal, so only the started process itself is killed.
var ErrNoProcessGroup = errors.New("proc: no process group on this platform")

// ConfigureGroupKill makes a context cancel or expiry of cmd kill the command's whole process group,
// so a descendant of the command cannot outlive it.
// When the group kill fails it kills the command alone;
// where the platform has no process group it always kills the command alone and reports ErrNoProcessGroup.
// It calls report with the command's pid after each kill: a nil groupErr when the group was killed, otherwise the group kill's error.
// proc cannot log, so the caller's report callback is where a kill is logged.
func ConfigureGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	configurePlatformGroupKill(cmd, report)
}
