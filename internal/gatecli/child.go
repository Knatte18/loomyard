// child.go runs a tier's or the build's shell as a child tied to this process.
// It ends what the shell leaves behind and names the signals that end the verb early.

package gatecli

import (
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// defaultPipeWaitDelay is how long a tier's or the build's output is still read after its shell exits.
const defaultPipeWaitDelay = 10 * time.Second

// terminatingSignalSet lists the signals that `lym gate test` catches.
// On each one it kills the child's tree and releases its slot before it exits.
// On Windows only the interrupt is delivered; the others are ignored.
var terminatingSignalSet = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// runTiedChild starts cmd tied to this process's life, waits for it and returns Wait's error.
// A cancelled context kills the child's whole tree, logging the kill.
// Once the child exits, its output is read for at most pipeWaitDelay.
// A process it left behind that holds the output pipe cannot hang the call;
// a zero exit cut short that way returns nil, and a failing exit keeps its own error.
// What is left of the child's process group is then killed, on Windows through the child's kill-on-close job.
// A non-nil started is called with the child's pid once it is running, before the wait.
func runTiedChild(cmd *exec.Cmd, pipeWaitDelay time.Duration, started func(pid int)) error {
	cmd.WaitDelay = pipeWaitDelay
	var tied atomic.Pointer[proc.Tied]
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		logger.Info("gate test: killing go test process group", "pid", pid)
		// A cancel racing the return of StartTied finds no handle yet and kills the child alone.
		handle := tied.Load()
		if handle == nil {
			return cmd.Process.Kill()
		}
		err := handle.Kill()
		if err != nil {
			logger.Warn("gate test: process group kill failed", "pid", pid, "cause", err)
		}
		return err
	}
	handle, err := proc.StartTied(cmd)
	if err != nil {
		return err
	}
	tied.Store(handle)
	if started != nil {
		started(cmd.Process.Pid)
	}
	waitErr := handle.Wait()
	killLeftovers(handle.Record())
	if errors.Is(waitErr, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		// Wait reports the delay only after a zero exit: a failing exit's own error wins over it.
		logger.Warn("gate test: a process the shell left behind held its output past the wait delay", "pid", cmd.Process.Pid, "delay", pipeWaitDelay)
		return nil
	}
	return waitErr
}

// killLeftovers kills what a tied child's shell left in its process group,
// so no leftover goes on holding the output pipe or running inside the slots.
// The kill reaches nothing on Windows, where closing the child's job has already ended its tree.
func killLeftovers(record proc.TreeRecord) {
	killed, err := proc.KillTree(record)
	switch {
	case err != nil:
		logger.Warn("gate test: could not kill what the shell left in its process group", "pid", record.PID, "cause", err)
	case killed:
		logger.Info("gate test: killed what the shell left in its process group", "pid", record.PID)
	}
}
