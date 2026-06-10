//go:build !windows

// git_other.go — non-Windows git subprocess handling.
//
// On non-Windows platforms, hideProcWindow is a no-op since there are no
// console window management issues.

package git

import (
	"os/exec"
)

// hideProcWindow is a no-op on non-Windows platforms.
func hideProcWindow(cmd *exec.Cmd) {}
