// launchers.go writes and tears down the per-worktree launcher scripts and the
// container-root ide-menu.cmd. Launchers are Windows-only; elsewhere it is a no-op.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/paths"
)

// writeLaunchers writes per-worktree launchers for the given slug.
//
// On Windows: creates l.LauncherDir(slug) and writes ide.cmd with content built
// from l.LauncherSpawnRel(slug), which climbs from _launchers/<RelPath>/<slug> to
// the target worktree's subpath:
//
//	@cd /d "%~dp0<climb-backslash>" && lyx ide spawn <slug>
//
// Also ensures l.MenuLauncherPath() exists: create it only if absent (never clobber)
// with static content built from l.MenuLauncherRel(), which climbs from
// _launchers/<RelPath> to the main worktree's subpath:
//
//	@cd /d "%~dp0<climb-backslash>" && lyx ide menu
//
// On non-Windows: returns nil (no-op).
func writeLaunchers(l *paths.Layout, slug string) error {
	if runtime.GOOS != "windows" {
		return nil // No-op on non-Windows
	}

	// Create the mirrored launcher directory
	launcherDir := l.LauncherDir(slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		return fmt.Errorf("mkdir launcher dir %s: %w", launcherDir, err)
	}

	// Build the ide.cmd content from LauncherSpawnRel
	spawnRelBackslash := strings.ReplaceAll(l.LauncherSpawnRel(slug), "/", "\\")
	ideCmdContent := fmt.Sprintf("@cd /d \"%%~dp0%s\" && lyx ide spawn %s\r\n", spawnRelBackslash, slug)

	// Write ide.cmd
	ideCmdPath := filepath.Join(launcherDir, "ide.cmd")
	if err := os.WriteFile(ideCmdPath, []byte(ideCmdContent), 0o644); err != nil {
		return fmt.Errorf("write ide.cmd: %w", err)
	}

	// Write warp-checkout.cmd — a shortcut that runs coordinated checkout for
	// this worktree. It climbs to the worktree subpath the same way ide.cmd does
	// so the user can double-click it from the _launchers directory.
	warpCheckoutContent := fmt.Sprintf("@cd /d \"%%~dp0%s\" && lyx warp checkout\r\n", spawnRelBackslash)
	warpCheckoutPath := filepath.Join(launcherDir, "warp-checkout.cmd")
	if err := os.WriteFile(warpCheckoutPath, []byte(warpCheckoutContent), 0o644); err != nil {
		return fmt.Errorf("write warp-checkout.cmd: %w", err)
	}

	// Ensure per-subpath menu launcher exists (never clobber)
	menuCmdPath := l.MenuLauncherPath()
	if _, err := os.Stat(menuCmdPath); err == nil {
		// File exists, don't clobber it
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat ide-menu.cmd: %w", err)
	}

	// File does not exist; create parent directory
	if err := os.MkdirAll(filepath.Dir(menuCmdPath), 0o755); err != nil {
		return fmt.Errorf("mkdir menu launcher dir: %w", err)
	}

	// Build menu content from MenuLauncherRel
	menuRelBackslash := strings.ReplaceAll(l.MenuLauncherRel(), "/", "\\")
	menuCmdContent := fmt.Sprintf("@cd /d \"%%~dp0%s\" && lyx ide menu\r\n", menuRelBackslash)

	if err := os.WriteFile(menuCmdPath, []byte(menuCmdContent), 0o644); err != nil {
		return fmt.Errorf("write ide-menu.cmd: %w", err)
	}

	return nil
}

// removeLaunchers removes the launcher directory for the given slug (idempotent).
//
// Uses os.RemoveAll to delete the entire l.LauncherDir(slug) directory, then
// prunes empty mirrored ancestors up to but not including l.LaunchersDir().
// Leaves l.MenuLauncherPath() (the per-subpath menu) in place; since it resides
// in the leaf _launchers/<RelPath>/ dir, the prune stops there in practice,
// removing only LauncherDir(slug) itself (intended asymmetry).
// Returns nil if the directory does not exist (os.RemoveAll returns nil for non-existent paths).
func removeLaunchers(l *paths.Layout, slug string) error {
	launcherDir := l.LauncherDir(slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}
	// Prune empty ancestors up to but not including LaunchersDir
	pruneEmptyAncestors(filepath.Dir(launcherDir), l.LaunchersDir())
	return nil
}
