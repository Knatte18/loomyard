//go:build windows

package vscode

import (
	"fmt"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Launch launches VS Code for the given worktree directory on Windows.
//
// It uses exec.Command to run "cmd /c code <worktreeDir>", which allows PATH resolution
// of code.cmd and applies the no-console-window flag pattern to prevent flashing.
func Launch(worktreeDir string) error {
	cmd := exec.Command("cmd", "/c", "code", worktreeDir)

	// Apply no-console-window flag pattern (from git_windows.go/spawn_windows.go)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch code: %w", err)
	}

	return nil
}
