// memorycap.go runs a command with the memory of its process tree capped, by whatever means the platform has.

package proc

import (
	"errors"
	"os/exec"
)

// StartFunc starts cmd and returns the wait that reaps it, standing in for cmd.Start and cmd.Wait.
type StartFunc func(cmd *exec.Cmd) (wait func() error, err error)

// ErrMemoryCap is wrapped by the error of a capped run whose cap could not be set or whose command never ran under it.
// An error that wraps it is no verdict on the command, and one that does not is the command's own or its start's.
var ErrMemoryCap = errors.New("the memory cap could not be set")

// RunMemoryCapped runs cmd as cmd.Run does, with a cap of limit bytes on the memory cmd and the processes it starts hold.
// It exists so a runaway command is ended alone, before it takes the memory of everything beside it.
// On Linux, cmd runs in a transient scope of the systemd user manager whose memory limit is limit, with no swap.
// The kernel kills a command that passes the limit, and killed reports that kill.
// After a SIGKILL of cmd or a kill at the cap, every process left in the scope is killed too,
// and the kill raises no desktop low-memory notice.
// cmd keeps its pid and process group there, so a cancel that kills the group still reaches it.
// On a Linux host whose user manager cannot cap a scope's memory, the data segment of each process is limited instead.
// On Windows, cmd runs in a job object whose memory limit is limit, and a cancel ends the whole job.
// cmd joins the job a moment after it starts, so a process it starts in that moment is outside the limit and the cancel.
// Under those two an allocation past the limit fails in the process that asks for it, and killed stays false.
// In a scope cmd.ExtraFiles is replaced, so cmd sets none.
// The error is cmd.Run's, or wraps ErrMemoryCap for a cap that could not be set, a scope that never ran cmd included.
func RunMemoryCapped(cmd *exec.Cmd, limit int64) (killed bool, err error) {
	killed, _, err = runMemoryCappedPlatform(cmd, limit, startDirect)
	return killed, err
}

// RunMemoryCappedVia runs cmd under the cap RunMemoryCapped sets, starting it through start and waiting through its wait.
// It lets a caller that ties its children to its own life, or watches them once started, keep doing so under the cap.
// treeKilled reports that the kernel killed a process of cmd's tree at the cap, cmd itself or one it started.
// It stays false where the cap fails an allocation instead of killing.
// The error is the wait's, or wraps ErrMemoryCap for a cap that could not be set, a scope that never ran cmd included.
func RunMemoryCappedVia(cmd *exec.Cmd, limit int64, start StartFunc) (treeKilled bool, err error) {
	_, treeKilled, err = runMemoryCappedPlatform(cmd, limit, start)
	return treeKilled, err
}

// startDirect starts cmd with cmd.Start and returns cmd.Wait.
func startDirect(cmd *exec.Cmd) (func() error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

// MemoryCapCoversTree reports whether a memory cap bounds a command's whole process tree as one.
// It is false where the cap limits each process alone.
// The processes of one command can then hold the limit several times over, so a caller runs one capped command at a time.
func MemoryCapCoversTree() bool {
	return memoryCapCoversTreePlatform()
}
