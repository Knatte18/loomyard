// launch_linux.go launches VS Code on Linux by invoking the "code" binary directly from PATH;
// unlike Windows there is no cmd.exe PATH-resolution shim or console window to hide, so this is a
// thin wrapper around exec.Command.

package vscode

import (
	"fmt"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// Launch launches VS Code on Linux on target, a folder or a workspace file, opening each of files in an editor of the window.
//
// It starts the "code" command detached (cmd.Start(), not Run()) so the caller does not block on
// the editor process.
// Wraps start failures with context.
func Launch(target string, files ...string) error {
	cmd := exec.Command("code", append([]string{target}, files...)...)

	logger.Info("vscode: spawning VS Code launch", "target", target, "files", files)
	if err := cmd.Start(); err != nil {
		logger.Warn("vscode: VS Code launch spawn failed", "target", target, "cause", err)
		return fmt.Errorf("launch code: %w", err)
	}
	proc.Reap(cmd)

	return nil
}
