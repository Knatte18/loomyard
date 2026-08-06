// launch_linux.go launches VS Code on Linux by invoking the "code" binary directly from PATH;
// unlike Windows there is no cmd.exe PATH-resolution shim or console window to hide, so this is a thin wrapper around exec.Command.

package vscode

import (
	"fmt"
	"os/exec"
)

// Launch launches VS Code for the given worktree directory on Linux.
//
// It starts the "code" command detached (cmd.Start(), not Run()) so the caller does not block on the editor process.
// Wraps start failures with context.
func Launch(worktreeDir string) error {
	cmd := exec.Command("code", worktreeDir)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch code: %w", err)
	}

	return nil
}
