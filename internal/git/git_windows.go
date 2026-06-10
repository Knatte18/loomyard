// git_windows.go — windowless git subprocess launching on Windows.
//
// hideProcWindow applies CREATE_NO_WINDOW to prevent console windows from
// flashing when git is launched as a child process.

package git

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideProcWindow makes a child process (git) run without a console window.
func hideProcWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
