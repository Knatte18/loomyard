// spawn.go starts and stops the OS processes the hibernate module owns: the detached watch daemon and the machine's hibernate command.

package hibernatecli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// spawnDaemon starts `<this binary> hibernate watch` detached, from anchor, with its output appended to logPath, and returns its pid and start time.
// Under go test it starts nothing and returns an error, so no test re-executes the test binary and no request is written with a pid nobody runs.
func spawnDaemon(anchor, logPath string) (int, string, error) {
	if testing.Testing() {
		return 0, "", errors.New("hibernate: the daemon is not spawned under go test")
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, "", fmt.Errorf("hibernate: resolve this binary: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, "", fmt.Errorf("hibernate: create the log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, "", fmt.Errorf("hibernate: open the daemon log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "hibernate", "watch")
	cmd.Dir = anchor
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, "", fmt.Errorf("start %s: %w", exe, err)
	}
	pid := cmd.Process.Pid
	logger.Info("hibernate: spawned the watch daemon", "exe", exe, "pid", pid, "log", logPath)
	// The spawner exits before the daemon does, so the wait only reaps the child while the spawner lives.
	go func() {
		err := cmd.Wait()
		logger.Info("hibernate: the watch daemon exited", "pid", pid, "error", err)
	}()
	start, _ := proc.StartTime(pid)
	return pid, start, nil
}

// runHibernate runs `systemctl hibernate`, which returns once the job is queued.
// A non-zero exit returns an error carrying the exit and the last line of the output.
func runHibernate() error {
	cmd := exec.Command("systemctl", "hibernate")
	logger.Info("hibernate: running systemctl hibernate")
	output, err := cmd.CombinedOutput()
	logger.Info("hibernate: systemctl hibernate returned", "error", err)
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return fmt.Errorf("systemctl hibernate: %w: %s", err, lines[len(lines)-1])
}

// killDaemon kills the daemon with pid.
func killDaemon(pid int) error {
	logger.Info("hibernate: killing the watch daemon", "pid", pid)
	return proc.KillPID(pid)
}
