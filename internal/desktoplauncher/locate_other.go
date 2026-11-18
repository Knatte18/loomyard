//go:build !windows

// locate_other.go reads the Windows desktop directory from inside WSL, through powershell.exe and wslpath.

package desktoplauncher

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// queryTimeout bounds one query process, so a Windows side that does not answer cannot hold up a dobby start.
const queryTimeout = 30 * time.Second

// windowsDesktopDir returns the Windows desktop directory of the current user as a WSL path.
// The desktop's known folder follows a redirected desktop, such as one OneDrive holds.
func windowsDesktopDir() (string, error) {
	windowsPath, err := query("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Environment]::GetFolderPath('Desktop')")
	if err != nil {
		return "", err
	}
	return query("wslpath", "-u", windowsPath)
}

// query runs program with args, waits for it, and returns its trimmed standard output; empty output is an error.
func query(program string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	logger.Info("desktoplauncher: spawning a desktop query", "program", program, "args", args)
	output, err := exec.CommandContext(ctx, program, args...).Output()
	logger.Debug("desktoplauncher: the desktop query ended", "program", program, "error", err)
	if err != nil {
		return "", fmt.Errorf("run %s: %w", program, err)
	}
	answer := strings.TrimSpace(string(output))
	if answer == "" {
		return "", fmt.Errorf("%s printed no path", program)
	}
	return answer, nil
}
