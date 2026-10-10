// procgroup.go declares the error ConfigureGroupKill reports on a platform with no process group to signal.

package proc

import "errors"

// ErrNoProcessGroup is the group-kill error ConfigureGroupKill reports where the platform has no process group to signal, so only the started process itself is killed.
var ErrNoProcessGroup = errors.New("proc: no process group on this platform")
