// memorycap_windows.go caps a command's memory with the memory limit of a kill-on-close job object.

package proc

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/windows"
)

// runMemoryCappedPlatform runs cmd in a job object whose memory limit is limit bytes, starting it through start.
// The job fails an allocation past the limit in the process that asks for it, so killed and treeKilled are always false.
// A cancel of cmd ends every process of the job before it ends cmd, and the job ends with the call.
// cmd joins the job once it has started, so a process it starts in between is outside the limit and the cancel.
func runMemoryCappedPlatform(cmd *exec.Cmd, limit int64, start StartFunc) (killed, treeKilled bool, err error) {
	job, err := createKillOnCloseJob(nil, uintptr(limit))
	if err != nil {
		return false, false, fmt.Errorf("%w: create the job object: %w", ErrMemoryCap, err)
	}
	defer windows.CloseHandle(job)
	if cancel := cmd.Cancel; cancel != nil {
		cmd.Cancel = func() error {
			_ = windows.TerminateJobObject(job, 1)
			return cancel()
		}
	}
	wait, err := start(cmd)
	if err != nil {
		return false, false, err
	}
	if err := assignProcessToJob(job, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = wait()
		return false, false, fmt.Errorf("%w: assign the command to the job object: %w", ErrMemoryCap, err)
	}
	return false, false, wait()
}

// memoryCapCoversTreePlatform is always true: a job object holds every process its command starts.
func memoryCapCoversTreePlatform() bool {
	return true
}
