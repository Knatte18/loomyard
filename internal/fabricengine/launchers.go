// launchers.go writes and tears down the per-worktree launcher scripts and the container-root menu
// launcher.
// Launchers are cross-platform: a .cmd script on Windows, an executable .sh script everywhere else,
// both built from the pure content builder in launcher_content.go.
// The checkout launcher file (fabric-checkout<ext>) invokes "lyx fabric checkout".

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// launchersDirName is the directory name for the hub-level launchers
// container (i.e. <hub>/_launchers). Declared here rather than in
// internal/lyxcwd: the launcher surface is fabric's own illusion-maintenance
// plumbing, not a resolution primitive lyxcwd needs to expose.
const launchersDirName = "_launchers"

// launchersDir returns the path to the _launchers directory in the hub.
func launchersDir(l *lyxcwd.Location) string {
	return filepath.Join(l.HubPath, launchersDirName)
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
// It is mirrored into the repo subpath structure, including AnchorRel segments.
// Exported because its live test caller (reconcile_stale_registration_test.go) sits in the external
// package fabricengine_test, where an unexported identifier does not compile.
func LauncherDir(l *lyxcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, launchersDirName, l.AnchorRel, slug)
}

// menuLauncherPath returns the path to the per-subpath menu launcher script.
// It is mirrored into the repo subpath structure. The extension is
// GOOS-selected: ".cmd" on Windows, ".sh" elsewhere.
func menuLauncherPath(l *lyxcwd.Location) string {
	return filepath.Join(l.HubPath, launchersDirName, l.AnchorRel, menuLauncherName())
}

// menuLauncherName returns the OS-appropriate filename for the menu launcher script.
func menuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// launcherSpawnRel returns the relative path from a launcher directory to
// the target worktree's subpath for spawning.
// The error is propagated rather than collapsed into an empty string: both paths are absolute and
// hub-rooted so filepath.Rel cannot fail today, but an empty result would be written into a launcher
// script that then spawns in the wrong directory, which is worse than a failed write.
func launcherSpawnRel(l *lyxcwd.Location, slug string) (string, error) {
	launcherDir := LauncherDir(l, slug)
	target := filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel)
	rel, err := filepath.Rel(launcherDir, target)
	if err != nil {
		return "", fmt.Errorf("relate launcher dir %s to worktree subpath %s: %w", launcherDir, target, err)
	}
	return rel, nil
}

// menuLauncherRel returns the relative path from the menu launcher directory
// to the primeName worktree's subpath for menu spawning. primeName is the
// main worktree's base name, sourced by the caller via the package-local
// PrimeName(l).
func menuLauncherRel(l *lyxcwd.Location, primeName string) (string, error) {
	menuDir := filepath.Dir(menuLauncherPath(l))
	target := filepath.Join(l.HubPath, primeName, l.AnchorRel)
	rel, err := filepath.Rel(menuDir, target)
	if err != nil {
		return "", fmt.Errorf("relate menu launcher dir %s to prime subpath %s: %w", menuDir, target, err)
	}
	return rel, nil
}

// writeLaunchers writes per-worktree launcher scripts (ide and fabric-checkout)
// and ensures the menu launcher exists. The .cmd/.sh extension depends on GOOS;
// .sh files are written executable.
func writeLaunchers(l *lyxcwd.Location, slug string) error {
	ext := launcherExt(runtime.GOOS)

	// Create the mirrored launcher directory
	launcherDir := LauncherDir(l, slug)
	if err := os.MkdirAll(launcherDir, 0o755); err != nil {
		return fmt.Errorf("mkdir launcher dir %s: %w", launcherDir, err)
	}

	// Build and write the ide launcher from launcherSpawnRel
	spawnRel, err := launcherSpawnRel(l, slug)
	if err != nil {
		return err
	}
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
	menuPath := menuLauncherPath(l)
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

	// Build menu content from menuLauncherRel, sourcing the prime worktree's
	// name via the package-local PrimeName rather than a Layout field — a
	// launcher pointing at an unresolved prime is worse than a failed wire, so
	// propagate the error rather than degrading to an empty prime name.
	primeName, err := PrimeName(l)
	if err != nil {
		return fmt.Errorf("resolve prime worktree name: %w", err)
	}
	menuRel, err := menuLauncherRel(l, primeName)
	if err != nil {
		return err
	}
	menuContent, menuMode := launcherScript(runtime.GOOS, menuRel, "ide menu")
	if err := os.WriteFile(menuPath, menuContent, menuMode); err != nil {
		return fmt.Errorf("write menu launcher: %w", err)
	}

	return nil
}

// removeLaunchers removes the launcher scripts fabric wrote for the given slug and then the
// directory itself, pruning empty ancestors.
// The menu launcher is left in place. Returns nil if the directory does not exist.
//
// It deletes the named scripts rather than the whole tree so anything else under the directory
// survives: an os.RemoveAll there destroyed whatever the operator had put beside the launchers,
// while the portal half of the same teardown (fslink.Remove) correctly declines to delete a
// non-empty real directory. A leftover file is reported by the directory removal below, never
// silently swept.
func removeLaunchers(l *lyxcwd.Location, slug string) error {
	launcherDir := LauncherDir(l, slug)
	if err := refuseUncontainedPath(launchersDir(l), launcherDir, "launcher dir"); err != nil {
		return err
	}

	ext := launcherExt(runtime.GOOS)
	for _, name := range []string{"ide" + ext, "fabric-checkout" + ext} {
		target := filepath.Join(launcherDir, name)
		req := pathRequest{
			what:      "remove launcher script",
			container: launchersDir(l),
			target:    target,
			slug:      nil,
			ownership: ownedUnderGeometryRoot(launchersDir(l)),
			dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
			force:     false,
		}
		if err := removePath(req); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove launcher script %s: %w", target, err)
		}
	}

	dirReq := pathRequest{
		what:      "remove launcher dir",
		container: launchersDir(l),
		target:    launcherDir,
		slug:      nil,
		ownership: ownedUnderGeometryRoot(launchersDir(l)),
		dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
		force:     false,
	}
	if err := removePath(dirReq); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}

	// Prune empty ancestors up to but not including launchersDir
	pruneEmptyAncestors(filepath.Dir(launcherDir), launchersDir(l))
	return nil
}
