// launchers.go writes and tears down the per-worktree launcher scripts and the
// container-root menu launcher. Launchers are cross-platform: a .cmd script on
// Windows, an executable .sh script everywhere else, both built from the pure
// content builder in launcher_content.go.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// writeLaunchers writes per-worktree launchers for the given slug.
//
// Creates l.LauncherDir(slug) and writes ide<ext> with content built from
// l.LauncherSpawnRel(slug), which climbs from _launchers/<RelPath>/<slug> to
// the target worktree's subpath, and warp-checkout<ext> with the same climb
// but invoking "lyx warp checkout". The extension is ".cmd" on Windows and
// ".sh" elsewhere (see launcherExt); the .sh files are written executable.
//
// Also ensures l.MenuLauncherPath() exists: create it only if absent (never
// clobber) with content built from l.MenuLauncherRel(), which climbs from
// _launchers/<RelPath> to the main worktree's subpath and invokes
// "lyx ide menu". MenuLauncherPath is itself GOOS-aware (hubgeometry), so its
// extension already matches launcherExt(runtime.GOOS).
func writeLaunchers(l *hubgeometry.Layout, slug string) error {
	ext := launcherExt(runtime.GOOS)

	// Create the mirrored launcher directory
	launcherDir := l.LauncherDir(slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		return fmt.Errorf("mkdir launcher dir %s: %w", launcherDir, err)
	}

	// Build and write the ide launcher from LauncherSpawnRel
	spawnRel := l.LauncherSpawnRel(slug)
	ideContent, ideMode := launcherScript(runtime.GOOS, spawnRel, "ide spawn "+slug)
	idePath := filepath.Join(launcherDir, "ide"+ext)
	if err := os.WriteFile(idePath, ideContent, ideMode); err != nil {
		return fmt.Errorf("write ide%s: %w", ext, err)
	}

	// Write the warp-checkout launcher — a shortcut that runs coordinated
	// checkout for this worktree. It climbs to the worktree subpath the same
	// way the ide launcher does so the user can run it from the _launchers
	// directory.
	warpCheckoutContent, warpCheckoutMode := launcherScript(runtime.GOOS, spawnRel, "warp checkout")
	warpCheckoutPath := filepath.Join(launcherDir, "warp-checkout"+ext)
	if err := os.WriteFile(warpCheckoutPath, warpCheckoutContent, warpCheckoutMode); err != nil {
		return fmt.Errorf("write warp-checkout%s: %w", ext, err)
	}

	// Ensure per-subpath menu launcher exists (never clobber)
	menuPath := l.MenuLauncherPath()
	if _, err := os.Stat(menuPath); err == nil {
		// File exists, don't clobber it
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat menu launcher: %w", err)
	}

	// File does not exist; create parent directory
	if err := os.MkdirAll(filepath.Dir(menuPath), 0o755); err != nil {
		return fmt.Errorf("mkdir menu launcher dir: %w", err)
	}

	// Build menu content from MenuLauncherRel
	menuContent, menuMode := launcherScript(runtime.GOOS, l.MenuLauncherRel(), "ide menu")
	if err := os.WriteFile(menuPath, menuContent, menuMode); err != nil {
		return fmt.Errorf("write menu launcher: %w", err)
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
func removeLaunchers(l *hubgeometry.Layout, slug string) error {
	launcherDir := l.LauncherDir(slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}
	// Prune empty ancestors up to but not including LaunchersDir
	pruneEmptyAncestors(filepath.Dir(launcherDir), l.LaunchersDir())
	return nil
}
