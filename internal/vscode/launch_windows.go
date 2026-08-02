//go:build windows

package vscode

import (
	"fmt"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/proc"
)

// Launch launches VS Code for the given worktree directory on Windows.
//
// It runs "code" via cmd /c for PATH resolution and hides the console window.
func Launch(worktreeDir string) error {
	cmd := exec.Command("cmd", "/c", "code", worktreeDir)

	proc.HideWindow(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch code: %w", err)
	}

	return nil
}
