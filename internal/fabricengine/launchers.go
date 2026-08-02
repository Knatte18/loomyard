// launchers.go writes and tears down the per-worktree launcher scripts and the
// container-root menu launcher. Launchers are cross-platform: a .cmd script on
// Windows, an executable .sh script everywhere else, both built from the pure
// content builder in launcher_content.go. The checkout launcher file
// (fabric-checkout<ext>) invokes "lyx fabric checkout".

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// writeLaunchers writes per-worktree launcher scripts (ide and fabric-checkout)
// and ensures the menu launcher exists. The .cmd/.sh extension depends on GOOS;
// .sh files are written executable.
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

	// Write the fabric-checkout launcher — a shortcut that runs coordinated
	// checkout for this worktree. It climbs to the worktree subpath the same
	// way the ide launcher does so the user can run it from the _launchers
	// directory.
	fabricCheckoutContent, fabricCheckoutMode := launcherScript(runtime.GOOS, spawnRel, "fabric checkout")
	fabricCheckoutPath := filepath.Join(launcherDir, "fabric-checkout"+ext)
	if err := os.WriteFile(fabricCheckoutPath, fabricCheckoutContent, fabricCheckoutMode); err != nil {
		return fmt.Errorf("write fabric-checkout%s: %w", ext, err)
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

// removeLaunchers removes the launcher directory for the given slug, pruning
// empty ancestors. The menu launcher is left in place. Returns nil if the
// directory does not exist.
func removeLaunchers(l *hubgeometry.Layout, slug string) error {
	launcherDir := l.LauncherDir(slug)
	if err := os.RemoveAll(launcherDir); err != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}
	// Prune empty ancestors up to but not including LaunchersDir
	pruneEmptyAncestors(filepath.Dir(launcherDir), l.LaunchersDir())
	return nil
}
