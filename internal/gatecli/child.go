// child.go runs a tier's or the build's shell as a child tied to this process.
// It ends what the shell leaves behind and names the signals that end the verb early.

package gatecli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// defaultPipeWaitDelay is how long a tier's or the build's output is still read after its shell exits.
const defaultPipeWaitDelay = 10 * time.Second

// childMarkerPrefix opens the name of the environment variable that marks every process of one tied child.
// The name carries this process's pid and the child's number, so a nested gate's marker never replaces it.
const childMarkerPrefix = "LYM_GATE_CHILD_"

// childCount numbers the tied children this process starts.
var childCount atomic.Int64

// newChildMarker returns a `NAME=1` environment entry no other tied child of any process carries.
func newChildMarker() string {
	return fmt.Sprintf("%s%d_%d=1", childMarkerPrefix, os.Getpid(), childCount.Add(1))
}

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
// So is every process that carries the child's marker, an environment entry cmd gets on top of its own.
// That reaches a process of the tree that left the group, such as a test host in a session of its own.
// A non-nil started is called with the child's pid once it is running, before the wait.
// A memoryLimit above 0 caps the memory of the child's tree at that many bytes through `proc.RunMemoryCappedVia`.
// capKilled reports that the kernel killed a process of the tree at the cap.
func runTiedChild(cmd *exec.Cmd, pipeWaitDelay time.Duration, memoryLimit int64, started func(pid int)) (capKilled bool, err error) {
	cmd.WaitDelay = pipeWaitDelay
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	marker := newChildMarker()
	cmd.Env = append(slices.Clone(env), marker)
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
	start := func(cmd *exec.Cmd) (func() error, error) {
		handle, err := proc.StartTied(cmd)
		if err != nil {
			return nil, err
		}
		tied.Store(handle)
		if started != nil {
			started(cmd.Process.Pid)
		}
		return handle.Wait, nil
	}
	var waitErr error
	if memoryLimit > 0 {
		capKilled, waitErr = proc.RunMemoryCappedVia(cmd, memoryLimit, start)
	} else if wait, startErr := start(cmd); startErr != nil {
		waitErr = startErr
	} else {
		waitErr = wait()
	}
	if handle := tied.Load(); handle != nil {
		killLeftovers(handle.Record())
	}
	killMarkedLeftovers(marker)
	if errors.Is(waitErr, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		// Wait reports the delay only after a zero exit: a failing exit's own error wins over it.
		logger.Warn("gate test: a process the shell left behind held its output past the wait delay", "pid", cmd.Process.Pid, "delay", pipeWaitDelay)
		return capKilled, nil
	}
	return capKilled, waitErr
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

// killMarkedLeftovers kills with SIGKILL every process whose environment holds marker, a tied child's whole entry.
// It reaches what left the child's process group, which no group kill reaches.
// Where no process's environment is readable, as on Windows, it kills nothing: the child's job has ended its tree there.
func killMarkedLeftovers(marker string) {
	pids, err := proc.PIDsWithEnv(marker)
	if errors.Is(err, errors.ErrUnsupported) {
		return
	}
	if err != nil {
		logger.Warn("gate test: could not list the processes the shell left outside its process group", "marker", marker, "cause", err)
		return
	}
	for _, pid := range pids {
		if err := proc.SignalProcess(pid, true); err != nil {
			logger.Debug("gate test: could not kill a process the shell left outside its process group", "pid", pid, "cause", err)
		}
	}
	if len(pids) > 0 {
		logger.Info("gate test: killed what the shell left outside its process group", "processes", len(pids))
	}
}
