// child.go runs go test as a child tied to this process and names the signals that end the verb early.

package gatecli

import (
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// terminatingSignalSet are the signals `lyx gate test` catches to kill the child's tree and release its slot before it exits.
// Windows delivers only the interrupt and ignores the rest.
var terminatingSignalSet = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// runTiedChild starts cmd tied to this process's life, waits for it and returns Wait's error.
// A cancelled context kills the child's whole tree, logging the kill.
func runTiedChild(cmd *exec.Cmd) error {
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
	started, err := proc.StartTied(cmd)
	if err != nil {
		return err
	}
	tied.Store(started)
	return started.Wait()
}
