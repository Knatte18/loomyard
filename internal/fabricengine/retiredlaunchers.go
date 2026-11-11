// retiredlaunchers.go names the retired `_launchers` hub container and sweeps what a hub still holds of it.
// Fabric once wrote launcher scripts and a workspace file there; no verb writes the container now.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// launchersDirName is the directory name of the retired hub-level launchers container (<hub>/_launchers).
// It stays declared so that the name remains reserved and the sweep can find the container.
const launchersDirName = "_launchers"

// retiredLauncherFileNames are the script names fabric wrote under the launchers container, each as a .sh or a .cmd:
// a pair's ide, run and fabric-checkout scripts, and an anchor's ide-menu script.
var retiredLauncherFileNames = []string{
	"ide.sh", "ide.cmd",
	"run.sh", "run.cmd",
	"fabric-checkout.sh", "fabric-checkout.cmd",
	"ide-menu.sh", "ide-menu.cmd",
}

// launchersDir returns the path of the retired launchers container in the hub at hubPath.
func launchersDir(hubPath string) string {
	return filepath.Join(hubPath, launchersDirName)
}

// RetiredLaunchersSweep is what a sweep of the retired launchers container removed, or in a dry run would remove.
type RetiredLaunchersSweep struct {
	// Removed lists the absolute paths of the files and emptied directories, each directory after its contents.
	Removed []string `json:"removed"`
	// Kept lists one detail per entry the sweep left in place, naming it hub-relative and saying why.
	Kept []string `json:"kept"`
}

// SweepRetiredLaunchers removes the retired `<hub>/_launchers` container that fabric once wrote launchers into.
// With apply false it only lists what it would remove.
// It removes the launcher scripts fabric wrote and any `.code-workspace` file.
// Then it removes every directory that empties, the container last.
// Anything else, a link included, is kept and named in Kept, and the directories above it stay.
// A link is never followed, and every removal goes through the destructive gate under the container.
// A hub with no container is an empty sweep.
// A removal that fails stops the sweep and returns what it removed so far with the error.
func SweepRetiredLaunchers(rec *Mutations, hubPath string, apply bool) (RetiredLaunchersSweep, error) {
	sweep := RetiredLaunchersSweep{Removed: []string{}, Kept: []string{}}
	root, err := os.OpenRoot(hubPath)
	if err != nil {
		return sweep, fmt.Errorf("open hub root %s: %w", hubPath, err)
	}
	defer func() { _ = root.Close() }()

	info, err := root.Lstat(launchersDirName)
	if errors.Is(err, fs.ErrNotExist) {
		return sweep, nil
	}
	if err != nil {
		return sweep, fmt.Errorf("read %s: %w", launchersDir(hubPath), err)
	}
	if !info.IsDir() {
		sweep.Kept = append(sweep.Kept, retiredLauncherKept(hubPath, launchersDirName, "not a directory"))
		return sweep, nil
	}

	var candidates []string
	if _, err := planRetiredLaunchersSweep(root, hubPath, launchersDirName, &candidates, &sweep.Kept); err != nil {
		return sweep, err
	}
	for _, rel := range candidates {
		path := filepath.Join(hubPath, rel)
		if apply {
			if err := removeRetiredLauncherEntry(rec, hubPath, path); err != nil {
				return sweep, err
			}
		}
		sweep.Removed = append(sweep.Removed, path)
	}
	return sweep, nil
}

// planRetiredLaunchersSweep lists, under the hub-relative directory dirRel, what the sweep removes and what it keeps.
// It appends the removable entries to candidates, each directory after its contents, and a detail per kept entry to kept.
// It reports whether dirRel empties, which is when every entry under it is removable.
func planRetiredLaunchersSweep(root *os.Root, hubPath, dirRel string, candidates, kept *[]string) (bool, error) {
	names, err := readRetiredDir(root, dirRel)
	if err != nil {
		return false, fmt.Errorf("list %s: %w", filepath.Join(hubPath, dirRel), err)
	}
	empties := true
	for _, name := range names {
		rel := filepath.Join(dirRel, name)
		info, err := root.Lstat(rel)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", filepath.Join(hubPath, rel), err)
		}
		switch {
		case info.IsDir():
			childEmpties, err := planRetiredLaunchersSweep(root, hubPath, rel, candidates, kept)
			if err != nil {
				return false, err
			}
			empties = empties && childEmpties
		case info.Mode().IsRegular() && isRetiredLauncherFile(name):
			*candidates = append(*candidates, rel)
		default:
			*kept = append(*kept, retiredLauncherKept(hubPath, rel, "not a file fabric wrote there"))
			empties = false
		}
	}
	if empties {
		*candidates = append(*candidates, dirRel)
	}
	return empties, nil
}

// isRetiredLauncherFile reports whether name is a launcher script fabric wrote or a workspace file.
func isRetiredLauncherFile(name string) bool {
	return slices.Contains(retiredLauncherFileNames, name) || strings.HasSuffix(name, hubWorkspaceExt)
}

// retiredLauncherKept words a sweep detail naming the hub-relative rel and the reason it was kept.
func retiredLauncherKept(hubPath, rel, reason string) string {
	return fmt.Sprintf("kept %s: %s", hubRelativeTarget(hubPath, filepath.Join(hubPath, rel)), reason)
}

// removeRetiredLauncherEntry removes the one file or emptied directory at path through the destructive gate.
// The removal is non-recursive, so a directory that gained an entry since the listing is refused by the OS and stays.
// The container itself is removed from the hub; every entry below it is removed from the container.
func removeRetiredLauncherEntry(rec *Mutations, hubPath, path string) error {
	container := launchersDir(hubPath)
	if filepath.Clean(path) == container {
		container = hubPath
	}
	err := checkPathRequest(pathRequest{
		what:      "remove retired launcher entry",
		container: container,
		target:    path,
		slug:      nil,
		ownership: ownedUnderGeometryRoot(launchersDir(hubPath)),
		dirtiness: dirtinessNA("the retired launcher container holds only generated artifacts"),
		force:     false,
	})
	if err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	removed, _, err := removeContainedPath(container, path, false)
	if err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	if removed {
		rec.Append(KindPathRemoved, path, "single")
	}
	return nil
}
