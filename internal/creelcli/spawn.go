// spawn.go starts the detached creel daemon a verb's client asks for when none answers, and opens the log file its stderr goes to.

package creelcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// daemonLogCap is the size above which a spawn truncates the daemon log before appending to it.
const daemonLogCap = 1 << 20

// openDaemonLog opens the daemon log at path for append, creating it and its directory.
// A log larger than daemonLogCap is truncated first, so the file stays bounded.
// A log that cannot be opened is logged at Warn and answered as a nil file with the error, so the spawn goes on with a null stderr.
func openDaemonLog(path string) (*os.File, error) {
	file, err := openAppendCapped(path)
	if err != nil {
		logger.Warn("creel: the daemon log cannot be opened, the daemon's stderr stays null", "log", path, "error", err)
		return nil, err
	}
	return file, nil
}

func openAppendCapped(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create the daemon log directory: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.Size() > daemonLogCap {
		if err := os.Truncate(path, 0); err != nil {
			return nil, fmt.Errorf("truncate the daemon log: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open the daemon log: %w", err)
	}
	return file, nil
}

// spawnDaemon starts `<this binary> creel daemon --hub-path <hub>` detached from the hub directory, with null stdin and stdout and its stderr appended to the log at logPath, and returns its pid with the signal of its exit.
// A background wait on the daemon reaps it and sends that signal when it exits.
// A log that cannot be opened leaves the daemon's stderr null.
// Under go test it starts nothing and returns an error, so no test re-executes the test binary.
func spawnDaemon(hubPath, logPath string) (creelengine.Spawn, error) {
	if testing.Testing() {
		return creelengine.Spawn{}, errors.New("creel: the daemon is not spawned under go test")
	}
	exe, err := os.Executable()
	if err != nil {
		return creelengine.Spawn{}, fmt.Errorf("creel: resolve this binary: %w", err)
	}
	cmd := exec.Command(exe, "creel", "daemon", "--hub-path", hubPath)
	cmd.Dir = hubPath
	if logFile, _ := openDaemonLog(logPath); logFile != nil {
		defer logFile.Close()
		cmd.Stderr = logFile
	}
	proc.Detach(cmd)
	logger.Info("creel: spawning the detached hub daemon", "exe", exe, "hub", hubPath, "log", logPath)
	if err := cmd.Start(); err != nil {
		return creelengine.Spawn{}, fmt.Errorf("start %s: %w", exe, err)
	}
	return creelengine.Spawn{PID: cmd.Process.Pid, Exited: reapDaemon(cmd)}, nil
}

// reapDaemon waits on the started command in the background, logs its exit with the pid and the exit status, and sends one SpawnExit.
// LostRace is true exactly when the exit status is 0: the daemon exits 0 when another daemon holds the lock or it stops cleanly, and 1 when its start fails.
// A wait error that is no exit code reads as not a lost race.
func reapDaemon(cmd *exec.Cmd) <-chan creelengine.SpawnExit {
	exited := make(chan creelengine.SpawnExit, 1)
	pid := cmd.Process.Pid
	go func() {
		waitErr := cmd.Wait()
		status := cmd.ProcessState.ExitCode()
		logger.Info("creel: the detached hub daemon exited", "pid", pid, "status", status, "error", waitErr)
		exited <- creelengine.SpawnExit{LostRace: waitErr == nil && status == 0}
	}()
	return exited
}
