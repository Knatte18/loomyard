// spawnserver.go starts the board server as a detached process.

package boardcli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// spawnServer starts `lym board serve` in dir, detached, with its stderr appended to the file at logPath.
// The server is reaped in the background while this process lives.
func spawnServer(dir, logPath string) error {
	exe, err := selfexec.LymBinary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create the server log's directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open the server log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "board", "serve")
	cmd.Dir = dir
	cmd.Stderr = logFile
	proc.Detach(cmd)
	logger.Info("boardcli: spawning the detached board server", "exe", exe, "dir", dir, "log", logPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", exe, err)
	}
	proc.Reap(cmd)
	return nil
}
