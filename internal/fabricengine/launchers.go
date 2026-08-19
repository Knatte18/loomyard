// launchers.go writes and tears down the per-worktree launcher scripts and the container-root menu
// launcher.
// Launchers are cross-platform: a .cmd script on Windows, an executable .sh script everywhere else,
// both built from the pure content builder in launcher_content.go.
// The checkout launcher file (fabric-checkout<ext>) invokes "lyx fabric checkout", and the run
// launcher file (run<ext>) invokes "lyx loom run".

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

// writeLaunchers writes per-worktree launcher scripts (ide, fabric-checkout, and run)
// and ensures the menu launcher exists. The .cmd/.sh extension depends on GOOS;
// .sh files are written executable.
//
// Every write goes through an os.Root rooted at l.HubPath rather than a raw os.MkdirAll/os.WriteFile,
// and that is a containment property, not a style choice: _launchers is a structural directory
// directly under the hub, and a raw write to <hub>/_launchers/<AnchorRel>/<slug> followed a symlink
// planted at the leaf OR at the _launchers container out of the hub, writing executable script content
// to an attacker-chosen path while reporting success — the create-side twin of the delete-side M3 the
// removeLaunchers gate already closes. Rooting at the hub (the true containment boundary; _launchers
// is never legitimately a symlink) makes component resolution and each write one openat chain that
// refuses any component escaping the hub at write time. l.HubPath always exists here — writeLaunchers
// runs only after the hub and the worktree pair it serves already exist.
//
// rec is the calling verb's own recorder. Every entry here is conditional on an observation, never
// on plain success, because writeLaunchers doubles as the repair path (reconcile.go's
// restorePortalAndLaunchers calls it against an already-wired pair): an unconditional record would
// claim writes that did not happen on that path.
//   - The launcher directory: root.MkdirAll succeeds on an already-existing directory, so the path is
//     stat'd BEFORE the MkdirAll and KindDirCreated is recorded only when it was absent.
//   - The three scripts: written unconditionally with deterministic content, so a repair-path call
//     rewrites them byte-identically. Each is recorded with KindFileWritten only when the write
//     actually changes its bytes (absent counts as changed) — read-before-write is what decides,
//     not "did the write succeed".
//   - The menu launcher: never-clobber (writeLaunchers returns early when it already exists, and
//     removeLaunchers deliberately leaves it in place), so it is recorded only on the branch that
//     actually reaches its root.WriteFile.
func writeLaunchers(rec *Mutations, l *lyxcwd.Location, slug string) error {
	ext := launcherExt(runtime.GOOS)

	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer func() { _ = root.Close() }()

	// Create the mirrored launcher directory, recording only when this call is the one that minted
	// it — a repair-path call against an already-wired pair must not claim a directory creation that
	// happened on an earlier call. A hub-escaping symlink at any component surfaces as a root.Stat
	// error that is not os.IsNotExist, so dirWasAbsent stays false and the root.MkdirAll below fails
	// closed rather than following it.
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

	// Build and write the ide launcher from launcherSpawnRel
	spawnRel, err := launcherSpawnRel(l, slug)
	if err != nil {
		return err
	}
	ideContent, ideMode := launcherScript(runtime.GOOS, spawnRel, "ide spawn "+slug)
	idePath := filepath.Join(launcherDir, "ide"+ext)
	if err := writeLauncherScriptIfChanged(rec, root, l.HubPath, idePath, ideContent, ideMode); err != nil {
		return fmt.Errorf("write ide%s: %w", ext, err)
	}

	// Write the fabric-checkout launcher — a shortcut that runs coordinated
	// checkout for this worktree. It climbs to the worktree subpath the same
	// way the ide launcher does so the user can run it from the _launchers
	// directory.
	fabricCheckoutContent, fabricCheckoutMode := launcherScript(runtime.GOOS, spawnRel, "fabric checkout")
	fabricCheckoutPath := filepath.Join(launcherDir, "fabric-checkout"+ext)
	if err := writeLauncherScriptIfChanged(rec, root, l.HubPath, fabricCheckoutPath, fabricCheckoutContent, fabricCheckoutMode); err != nil {
		return fmt.Errorf("write fabric-checkout%s: %w", ext, err)
	}

	// Write the run launcher — the double-click entry point that hands this pair to loom's
	// session bootstrap. It reuses spawnRel like the ide launcher, and invokes the explicit
	// two-word verb rather than any root alias, so it keeps working regardless of what happens
	// to the alias.
	runContent, runMode := launcherScript(runtime.GOOS, spawnRel, "loom run")
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
		// File exists, don't clobber it
		return nil
	} else if !os.IsNotExist(err) {
		// A hub-escaping symlink at any component lands here too, failing closed rather than clobbering
		// whatever it points at.
		return fmt.Errorf("stat menu launcher: %w", err)
	}

	// File does not exist; create parent directory
	if err := root.MkdirAll(filepath.Dir(menuRelPath), 0o755); err != nil {
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
	if err := root.WriteFile(menuRelPath, menuContent, menuMode); err != nil {
		return fmt.Errorf("write menu launcher: %w", err)
	}
	rec.Append(KindFileWritten, menuPath, "")

	return nil
}

// hubRel returns path expressed relative to hubPath, the form every os.Root call under the hub takes.
// It errors rather than degrading, because a launcher/portal path that cannot be related to the hub
// is a bug in the geometry derivation, not a case to write blindly.
func hubRel(hubPath, path string) (string, error) {
	rel, err := filepath.Rel(hubPath, path)
	if err != nil {
		return "", fmt.Errorf("relate %s to hub %s: %w", path, hubPath, err)
	}
	return rel, nil
}

// writeLauncherScriptIfChanged writes content to path (expressed through root, rooted at hubPath) with
// the given mode, recording KindFileWritten against rec only when the bytes actually change — an absent
// file counts as changed. This is what lets writeLaunchers' repair-path call (an already-wired pair,
// rewritten byte-identically) report no change at all, since the deterministic content it writes is
// otherwise indistinguishable from a fresh authoring by exit code alone.
// The read and the write both go through root, so a symlink planted at any component that would escape
// the hub is refused at both rather than read-from or written-to.
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

// removeLaunchers removes the launcher scripts fabric wrote for the given slug and then the
// directory itself, pruning empty ancestors.
// The menu launcher is left in place. Returns nil if the directory does not exist.
//
// It deletes the named scripts rather than the whole tree so anything else under the directory
// survives: an os.RemoveAll there destroyed whatever the operator had put beside the launchers,
// while the portal half of the same teardown (fslink.Remove) correctly declines to delete a
// non-empty real directory. A leftover file is reported by the directory removal below, never
// silently swept.
// rec is the calling verb's own recorder: the script removals route through removePath, which
// records for itself, while the directory removal below routes through removeContainedPath directly
// (non-recursive, so a non-empty directory is still refused by the OS) and therefore records
// KindPathRemoved with detail "single" itself, at its own success site, using the same
// record-only-on-observed-effect rule the gate executors follow.
func removeLaunchers(rec *Mutations, l *lyxcwd.Location, slug string) error {
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
		if err := removePath(rec, req); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove launcher script %s: %w", target, err)
		}
	}

	// The directory itself runs the gate's checks (containment, ownership, dirtiness) via
	// checkPathRequest, and then removes through removeContainedPath with recursive=false rather than
	// through removePath: removePath's directory branch is RemoveAll, which would silently destroy
	// anything the operator had put beside the launchers — exactly the defect
	// TestRemoveLaunchers_PreservesForeignContent exists to police, and the portal half of the same
	// teardown already declines to delete a non-empty real directory for the same reason.
	// removeContainedPath's non-recursive branch is os.Root.Remove, which the OS refuses on a non-empty
	// directory exactly as a bare os.Remove does, so that preservation property is unchanged — what the
	// routing adds is binding containment to the ACT. This used to be a raw, unrooted single-entry
	// removal of the nominal path, which made it the one arbitrary-path removal left in the package still resolving
	// containment at one instant and unlinking at a later one: a symlink planted at an intermediate
	// segment (an AnchorRel component under _launchers), dangling when checkPathRequest ran and flipped
	// live-and-escaping before the unlink, carried the removal outside the hub while the record named the
	// hub-relative path the removed inode never was — the same window R3 closed for removePath/removeLink
	// and the same false-success shape as R2's M3. Rooting the unlink at the launchers container makes
	// component resolution and the unlink one openat chain the kernel refuses to walk out of.
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
