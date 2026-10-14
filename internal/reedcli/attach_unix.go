//go:build !windows

// attach_unix.go hands the operator's terminal to tmux by replacing the lyx process with it.

package reedcli

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// handOver replaces this process with tmux running args, so no lyx process outlives the attach and an attach never keeps an old binary running.
// The exec skips the root's post-run, so the log line comes first; the process's run directory is left to the next start's sweep.
// The tmux name is resolved through PATH first, since exec itself does no lookup.
// A failed lookup or exec returns exit code 1 and the error.
// Under a test binary it runs tmux as a child instead, so an in-process test never replaces itself.
func handOver(tmux string, args []string) (exitCode int, err error) {
	if testing.Testing() {
		return runAttachChild(tmux, args)
	}
	resolved, err := exec.LookPath(tmux)
	if err != nil {
		return 1, err
	}
	logger.Info("reedcli: exec tmux attach", "tmux", resolved, "args", args)
	if err := syscall.Exec(resolved, append([]string{resolved}, args...), os.Environ()); err != nil {
		return 1, err
	}
	return 0, nil
}
