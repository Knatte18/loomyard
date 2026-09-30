// spawn.go declares the production watcher spawn: the detached `lyx orch watch` child `start` launches so the handoff cycle outlives the command that began it.

package orchcli

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// spawnWatcherProcess starts `<lyx> orch watch` detached, with its output appended to the watch log.
// The child runs from the prime's anchor so its own cwd resolution finds the prime.
// It is a no-op under `go test`, since re-executing os.Executable() there would run the test binary.
func (c *orchCLI) spawnWatcherProcess() error {
	if testing.Testing() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("orch: resolve executable: %w", err)
	}
	logFile, err := os.OpenFile(c.paths.WatchLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("orch: open watch log: %w", err)
	}
	cmd := exec.Command(exe, "orch", "watch")
	cmd.Dir = c.location.AnchorPath()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("orch: start watcher: %w", err)
	}
	logger.Info("orch: spawned detached watcher", "pid", cmd.Process.Pid, "log", c.paths.WatchLogPath)
	// The child holds its own duplicated descriptor from Start.
	_ = logFile.Close()
	// Reap the child in the background so a watcher that exits while this process lives is not left a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}
