// launchers.go writes and tears down the per-worktree launcher scripts and the container-root menu launcher.
// Launchers are cross-platform.
// On Windows a launcher is a .cmd script; elsewhere it is an executable .sh script.
// Both are built from the pure content builder in launcher_content.go.
// The checkout launcher file (fabric-checkout<ext>) invokes "lym fabric checkout",
// and the run launcher file (run<ext>) invokes "lym loom start".

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// launchersDirName is the directory name for the hub-level launchers container (i.e. <hub>/_launchers).
// It is declared here rather than in internal/lymcwd.
// The launcher surface is fabric's own illusion-maintenance code, not a resolution primitive lymcwd exposes.
const launchersDirName = "_launchers"

// launchersDir returns the path to the _launchers directory in the hub.
func launchersDir(l *lymcwd.Location) string {
	return filepath.Join(l.HubPath, launchersDirName)
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
// It is mirrored into the repo subpath structure, including AnchorRel segments.
// It is exported so the external fabricengine_test package can call it.
func LauncherDir(l *lymcwd.Location, slug string) string {
	return filepath.Join(l.HubPath, launchersDirName, l.AnchorRel, slug)
}

// menuLauncherPath returns the path to the per-subpath menu launcher script.
// It is mirrored into the repo subpath structure.
// The extension follows GOOS: ".cmd" on Windows, ".sh" elsewhere.
func menuLauncherPath(l *lymcwd.Location) string {
	return filepath.Join(l.HubPath, launchersDirName, l.AnchorRel, menuLauncherName())
}

// menuLauncherName returns the OS-appropriate filename for the menu launcher script.
func menuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// launcherSpawnRel returns the relative path from a launcher directory to the target worktree's subpath for spawning.
// The error is returned rather than collapsed into an empty string.
// Both paths are absolute and hub-rooted, so filepath.Rel cannot fail for them,
// but an empty result would be written into a launcher script that then spawns in the wrong directory.
// A wrong directory is worse than a failed write.
func launcherSpawnRel(l *lymcwd.Location, slug string) (string, error) {
	launcherDir := LauncherDir(l, slug)
	target := filepath.Join(filepath.Join(l.HubPath, slug), l.AnchorRel)
	rel, err := filepath.Rel(launcherDir, target)
	if err != nil {
		return "", fmt.Errorf("relate launcher dir %s to worktree subpath %s: %w", launcherDir, target, err)
	}
	return rel, nil
}

// menuLauncherRel returns the relative path from the menu launcher directory to the prime worktree's subpath.
// primeName is the main worktree's base name, which the caller sources from PrimeName.
func menuLauncherRel(l *lymcwd.Location, primeName string) (string, error) {
	menuDir := filepath.Dir(menuLauncherPath(l))
	target := filepath.Join(l.HubPath, primeName, l.AnchorRel)
	rel, err := filepath.Rel(menuDir, target)
	if err != nil {
		return "", fmt.Errorf("relate menu launcher dir %s to prime subpath %s: %w", menuDir, target, err)
	}
	return rel, nil
}

// writeLaunchers writes the per-worktree launcher scripts (ide, fabric-checkout, and run).
// It also ensures the menu launcher exists.
// The extension follows GOOS: .cmd on Windows, .sh elsewhere.
// Files with the .sh extension are written executable.
//
// Every write goes through an os.Root rooted at l.HubPath, not a raw os.MkdirAll or os.WriteFile.
// This is a containment property: _launchers sits directly under the hub and is never legitimately a symlink,
// so a symlink planted at the leaf or at _launchers must not redirect an executable script write outside the hub.
// Rooting at the hub makes each write one openat chain.
// That chain refuses any component that escapes the hub.
// l.HubPath always exists here, because writeLaunchers runs only after the hub and the worktree pair it serves exist.
//
// rec is the calling verb's own recorder.
// An entry is recorded only when an observation shows an effect, never on plain success.
// writeLaunchers doubles as the repair path: restoreLaunchers in reconcile.go calls it against an already-wired pair,
// and an unconditional record would claim writes that did not happen on that path.
//   - The launcher directory is recorded as created only when it was absent before the call.
//     root.MkdirAll succeeds on an existing directory, so the path is stat'd before the MkdirAll.
//   - The three scripts are written unconditionally with deterministic content, so a repair-path call
//     rewrites them byte-identically.
//     Each is recorded as KindFileWritten only when the write changes its bytes; an absent file counts as changed.
//     The read before the write decides the record, not whether the write succeeded.
//   - The menu launcher is never clobbered: writeLaunchers returns early when it exists,
//     and removeLaunchers leaves it in place.
//     It is recorded only on the branch that reaches its root.WriteFile.
func writeLaunchers(rec *Mutations, l *lymcwd.Location, slug string) error {
	ext := launcherExt(runtime.GOOS)

	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer func() { _ = root.Close() }()

	// Create the mirrored launcher directory. It is recorded only when this call creates it:
	// a repair-path call against an already-wired pair must not claim a creation that an earlier call made.
	// A symlink escaping the hub at any component surfaces as a root.Stat error other than os.IsNotExist.
	// dirWasAbsent then stays false, so root.MkdirAll fails closed rather than following it.
	launcherDir := LauncherDir(l, slug)
	launcherDirRel, err := hubRel(l.HubPath, launcherDir)
	if err != nil {
		return err
	}
	_, statErr := root.Stat(launcherDirRel)
	dirWasAbsent := os.IsNotExist(statErr)
	if err := root.MkdirAll(launcherDirRel, 0o755); err != nil {
		return fmt.Errorf("mkdir launcher dir %s: %w", launcherDir, err)
	}
	if dirWasAbsent {
		rec.Append(KindDirCreated, launcherDir, "")
	}

	// Build and write the ide launcher from spawnRel, the path from the launcher directory to the worktree subpath.
	spawnRel, err := launcherSpawnRel(l, slug)
	if err != nil {
		return err
	}
	ideContent, ideMode := launcherScript(runtime.GOOS, spawnRel, "ide spawn "+slug)
	idePath := filepath.Join(launcherDir, "ide"+ext)
	if err := writeLauncherScriptIfChanged(rec, root, l.HubPath, idePath, ideContent, ideMode); err != nil {
		return fmt.Errorf("write ide%s: %w", ext, err)
	}

	// Write the fabric-checkout launcher, a shortcut that runs coordinated checkout for this worktree.
	// It climbs to the worktree subpath the same way the ide launcher does,
	// so the user can run it from the _launchers directory.
	fabricCheckoutContent, fabricCheckoutMode := launcherScript(runtime.GOOS, spawnRel, "fabric checkout")
	fabricCheckoutPath := filepath.Join(launcherDir, "fabric-checkout"+ext)
	if err := writeLauncherScriptIfChanged(rec, root, l.HubPath, fabricCheckoutPath, fabricCheckoutContent, fabricCheckoutMode); err != nil {
		return fmt.Errorf("write fabric-checkout%s: %w", ext, err)
	}

	// Write the run launcher, the double-click entry point that hands this pair to loom's session bootstrap.
	// It reuses spawnRel like the ide launcher and invokes the explicit two-word verb rather than a root alias,
	// so it keeps working whatever happens to the alias.
	runContent, runMode := launcherScript(runtime.GOOS, spawnRel, "loom start")
	runPath := filepath.Join(launcherDir, "run"+ext)
	if err := writeLauncherScriptIfChanged(rec, root, l.HubPath, runPath, runContent, runMode); err != nil {
		return fmt.Errorf("write run%s: %w", ext, err)
	}

	// Ensure per-subpath menu launcher exists (never clobber)
	menuPath := menuLauncherPath(l)
	menuRelPath, err := hubRel(l.HubPath, menuPath)
	if err != nil {
		return err
	}
	if _, err := root.Stat(menuRelPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		// A symlink escaping the hub at any component also lands here.
		// The write then fails closed rather than clobbering whatever it points at.
		return fmt.Errorf("stat menu launcher: %w", err)
	}

	if err := root.MkdirAll(filepath.Dir(menuRelPath), 0o755); err != nil {
		return fmt.Errorf("mkdir menu launcher dir: %w", err)
	}

	// Build the menu content from menuLauncherRel, with the prime worktree's name from PrimeName.
	// A launcher pointing at an unresolved prime is worse than a failed wire,
	// so the error is returned rather than replaced by an empty prime name.
	primeName, err := PrimeName(l)
	if err != nil {
		return fmt.Errorf("resolve prime worktree name: %w", err)
	}
	menuRel, err := menuLauncherRel(l, primeName)
	if err != nil {
		return err
	}
	menuContent, menuMode := launcherScript(runtime.GOOS, menuRel, "ide menu")
	if err := root.WriteFile(menuRelPath, menuContent, menuMode); err != nil {
		return fmt.Errorf("write menu launcher: %w", err)
	}
	rec.Append(KindFileWritten, menuPath, "")

	return nil
}

// hubRel returns path expressed relative to hubPath, the form every os.Root call under the hub takes.
// It returns an error rather than degrading.
// A path that cannot be related to hubPath is a bug in the geometry derivation, not a case to write blindly.
func hubRel(hubPath, path string) (string, error) {
	rel, err := filepath.Rel(hubPath, path)
	if err != nil {
		return "", fmt.Errorf("relate %s to hub %s: %w", path, hubPath, err)
	}
	return rel, nil
}

// writeLauncherScriptIfChanged writes content to path with the given mode, through root rooted at hubPath.
// It records KindFileWritten against rec only when the bytes change; an absent file counts as changed.
// The repair-path call in writeLaunchers rewrites an already-wired pair byte-identically.
// This lets it report no change, since exit code alone cannot tell such a rewrite from a fresh authoring.
// The read and the write both go through root, so a symlink at any component that escapes the hub is refused by both.
func writeLauncherScriptIfChanged(rec *Mutations, root *os.Root, hubPath, path string, content []byte, mode os.FileMode) error {
	rel, err := hubRel(hubPath, path)
	if err != nil {
		return err
	}
	existing, readErr := root.ReadFile(rel)
	changed := readErr != nil || string(existing) != string(content)
	if err := root.WriteFile(rel, content, mode); err != nil {
		return err
	}
	if changed {
		rec.Append(KindFileWritten, path, "")
	}
	return nil
}

// removeLaunchers removes the launcher scripts fabric wrote for the given slug.
// It then removes the directory itself and prunes empty ancestors.
// The menu launcher is left in place.
// It returns nil if the directory does not exist.
//
// It deletes the named scripts rather than the whole tree, so anything else under the directory survives.
// An os.RemoveAll there would destroy whatever the operator had put beside the launchers.
// The directory removal below is non-recursive, so it declines to delete a non-empty directory.
// A leftover file is reported by that removal, never silently swept.
// rec is the calling verb's own recorder.
// The script removals route through removePath, which records for itself.
// The directory removal routes through removeContainedPath directly, non-recursively.
// The OS still refuses a non-empty directory.
// That removal records KindPathRemoved with detail "single" at its own success site.
// It follows the same record-only-on-observed-effect rule the gate executors follow.
func removeLaunchers(rec *Mutations, l *lymcwd.Location, slug string) error {
	launcherDir := LauncherDir(l, slug)
	if err := refuseUncontainedPath(launchersDir(l), launcherDir, "launcher dir"); err != nil {
		return err
	}

	ext := launcherExt(runtime.GOOS)
	// Every script writeLaunchers writes must be listed here.
	// The directory removal that follows is non-recursive.
	// A missing script leaves the directory non-empty, so the removal fails outright rather than orphaning a stray file.
	for _, name := range []string{"ide" + ext, "fabric-checkout" + ext, "run" + ext} {
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
		if err := removePath(rec, req); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove launcher script %s: %w", target, err)
		}
	}

	// The directory itself runs the gate's checks (containment, ownership, dirtiness) via checkPathRequest.
	// It is removed through removeContainedPath with recursive=false, not through removePath.
	// removePath's directory branch is RemoveAll.
	// That would silently destroy anything the operator had put beside the launchers, so this removal is non-recursive.
	// The non-recursive branch is os.Root.Remove.
	// The OS refuses it on a non-empty directory, so foreign content in the directory is preserved.
	// TestRemoveLaunchers_PreservesForeignContent pins that.
	// Rooting the unlink at the launchers container makes it one openat chain.
	// Component resolution then cannot leave the container.
	dirReq := pathRequest{
		what:      "remove launcher dir",
		container: launchersDir(l),
		target:    launcherDir,
		slug:      nil,
		ownership: ownedUnderGeometryRoot(launchersDir(l)),
		dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
		force:     false,
	}
	if err := checkPathRequest(dirReq); err != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
	}
	removed, _, removeErr := removeContainedPath(launchersDir(l), launcherDir, false)
	if removeErr != nil {
		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, removeErr)
	}
	if removed {
		rec.Append(KindPathRemoved, launcherDir, "single")
	}

	// Prune empty ancestors up to but not including launchersDir
	pruneEmptyAncestors(filepath.Dir(launcherDir), launchersDir(l))
	return nil
}
