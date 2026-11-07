//go:build windows

package vscode

import (
	"fmt"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// Launch opens target, a folder or a workspace file, in VS Code on Windows.
// Every file in files opens in an editor of that window.
//
// It runs "code" via cmd /c for PATH resolution and hides the console window.
func Launch(target string, files ...string) error {
	cmd := exec.Command("cmd", append([]string{"/c", "code", target}, files...)...)

	proc.HideWindow(cmd)

	logger.Info("vscode: spawning VS Code launch", "target", target, "files", files)
	if err := cmd.Start(); err != nil {
		logger.Warn("vscode: VS Code launch spawn failed", "target", target, "cause", err)
		return fmt.Errorf("launch code: %w", err)
	}
	proc.Reap(cmd)

	return nil
}
