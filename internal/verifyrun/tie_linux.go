// tie_linux.go gives a verify shell a parent-death SIGKILL on Linux.

package verifyrun

import (
	"os/exec"
	"syscall"
)

// tieToParent makes the kernel kill cmd's process when the calling process dies.
func tieToParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
