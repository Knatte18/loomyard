//go:build windows

package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

// createJunction creates a Windows junction (directory symlink) from link to target.
//
// On Windows, junctions are created using cmd /c mklink /J after normalizing
// both paths to backslashes. The parent directory of the link is created if needed.
// If the link path already exists, returns an error (refuses to clobber).
// Returns an error if creation fails or if the command exits with a non-zero code.
func createJunction(link, target string) error {
	// Check if link already exists
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("junction already exists — remove it first: %s", link)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", link, err)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", link, err)
	}

	// Normalize paths to backslashes
	winLink := strings.ReplaceAll(link, "/", "\\")
	winTarget := strings.ReplaceAll(target, "/", "\\")

	// Create junction via cmd /c mklink /J
	cmd := exec.Command("cmd", "/c", "mklink", "/J", winLink, winTarget)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %s %s: %v (output: %s)", winLink, winTarget, err, string(output))
	}

	return nil
}
