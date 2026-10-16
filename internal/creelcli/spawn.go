// spawn.go starts the detached creel daemon a verb's client asks for when none answers, and opens the log file its stderr goes to.

package creelcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

// spawnDaemon starts `<this binary> creel daemon --hub-path <hub>` detached from the hub directory, with null stdin and stdout and its stderr appended to the log at logPath, and returns its pid.
// A log that cannot be opened leaves the daemon's stderr null.
// Under go test it starts nothing and returns an error, so no test re-executes the test binary.
func spawnDaemon(hubPath, logPath string) (int, error) {
	if testing.Testing() {
		return 0, errors.New("creel: the daemon is not spawned under go test")
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("creel: resolve this binary: %w", err)
	}
	cmd := exec.Command(exe, "creel", "daemon", "--hub-path", hubPath)
	cmd.Dir = hubPath
	if logFile, _ := openDaemonLog(logPath); logFile != nil {
		defer logFile.Close()
		cmd.Stderr = logFile
	}
	proc.Detach(cmd)
	// A detached start with no wait: the spawn is logged and there is no teardown to observe.
	logger.Info("creel: spawning the detached hub daemon", "exe", exe, "hub", hubPath, "log", logPath)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start %s: %w", exe, err)
	}
	return cmd.Process.Pid, nil
}
