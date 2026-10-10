//go:build unix

// child_unix.go runs go test in its own process group, so `lyx gate test` can kill the whole tree without signalling the shell that called it.

package gatecli

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/Knatte18/loomyard/internal/logger"
)

// terminatingSignals are the signals `lyx gate test` catches to kill the child's group and release its slot before it exits.
var terminatingSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// armParentDeath makes the kernel kill a child when the thread that started it ends, where the platform can.
// child_linux.go sets it; elsewhere the child's group outlives an uncatchable kill of `lyx gate test`.
var armParentDeath = func(*syscall.SysProcAttr) {}

// runChild starts cmd in its own process group, waits for it and returns Wait's error.
// A cancelled context kills the whole group, and a failed group kill falls back to the child alone.
// The start and the wait run on one goroutine locked to its OS thread, because a parent-death signal fires when the thread that started the child exits.
func runChild(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	armParentDeath(cmd.SysProcAttr)
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		logger.Info("gate test: killing go test process group", "pid", pid)
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			logger.Warn("gate test: process group kill failed, killing go test alone", "pid", pid, "cause", err)
			return cmd.Process.Kill()
		}
		return nil
	}

	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := cmd.Start(); err != nil {
			done <- err
			return
		}
		done <- cmd.Wait()
	}()
	return <-done
}
