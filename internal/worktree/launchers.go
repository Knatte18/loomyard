package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/mhgo/internal/paths"
)

// writeLaunchers writes per-worktree launchers for the given slug.
//
// On Windows: creates l.LauncherDir(slug) and writes ide.cmd with content:
//
//	@cd /d "%~dp0..\..\<slug>\<relpath-backslash>" && mhgo ide spawn <slug>
//
// (omit the trailing \<relpath> segment when RelPath is empty or ".")
//
// Also ensures l.LaunchersDir()/ide-menu.cmd exists: create it only if absent
// (never clobber) with static content:
//
//	@cd /d "%~dp0..\<hubname>\<relpath-backslash>" && mhgo ide menu
//
// On non-Windows: returns nil (no-op).
func writeLaunchers(l *paths.Layout, slug string) error {
	if runtime.GOOS != "windows" {
		return nil // No-op on non-Windows
	}

	// Create the launcher directory
	launcherDir := l.LauncherDir(slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		return fmt.Errorf("mkdir launcher dir %s: %w", launcherDir, err)
	}

	// Build the ide.cmd content
	var relPathPart string
	if l.RelPath != "" && l.RelPath != "." {
		// Convert forward slashes to backslashes
		relPathBackslash := strings.ReplaceAll(l.RelPath, "/", "\\")
		relPathPart = "\\" + relPathBackslash
	}

	ideCmdContent := fmt.Sprintf("@cd /d \"%%~dp0..\\..\\%s%s\" && mhgo ide spawn %s\r\n", slug, relPathPart, slug)

	// Write ide.cmd
	ideCmdPath := filepath.Join(launcherDir, "ide.cmd")
	if err := os.WriteFile(ideCmdPath, []byte(ideCmdContent), 0o644); err != nil {
		return fmt.Errorf("write ide.cmd: %w", err)
	}

	// Ensure ide-menu.cmd exists in the launchers root (never clobber)
	menuCmdPath := filepath.Join(l.LaunchersDir(), "ide-menu.cmd")
	if _, err := os.Stat(menuCmdPath); err == nil {
		// File exists, don't clobber it
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat ide-menu.cmd: %w", err)
	}

	// Create parent directory if needed
	if err := os.MkdirAll(l.LaunchersDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir launchers dir: %w", err)
	}

	// File does not exist, create it
	var relPathPartMenu string
	if l.RelPath != "" && l.RelPath != "." {
		relPathBackslash := strings.ReplaceAll(l.RelPath, "/", "\\")
		relPathPartMenu = "\\" + relPathBackslash
	}

	hubName := l.HubName()
	menuCmdContent := fmt.Sprintf("@cd /d \"%%~dp0..\\%s%s\" && mhgo ide menu\r\n", hubName, relPathPartMenu)

	if err := os.WriteFile(menuCmdPath, []byte(menuCmdContent), 0o644); err != nil {
		return fmt.Errorf("write ide-menu.cmd: %w", err)
	}

	return nil
}

// removeLaunchers removes the launcher directory for the given slug (idempotent).
//
// Uses os.RemoveAll to delete the entire l.LauncherDir(slug) directory.
// Leaves l.LaunchersDir()/ide-menu.cmd in place.
// Returns nil if the directory does not exist (os.RemoveAll returns nil for non-existent paths).
func removeLaunchers(l *paths.Layout, slug string) error {
	launcherDir := l.LauncherDir(slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}
	return nil
}
