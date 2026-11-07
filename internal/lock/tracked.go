// tracked.go places the lock files of tracked files under a checkout's never-tracked lock directory.
// It refuses a lock path inside a tracked lym or Bolt directory.

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// TrackedLockPath returns the lock file named name in the never-tracked lock directory of the checkout tree at root.
// root is resolved through symlinks, so every path that reaches one checkout names the same lock file;
// a root that does not resolve is used as given.
// It creates nothing.
func TrackedLockPath(root, name string) string {
	return filepath.Join(locksDir(resolve(root)), name)
}

// TrackedFileLockPath returns the lock file guarding the file at path, with suffix appended to the lock's name.
// For a file under a tracked lym directory, the lock mirrors the file's place under the nearest such directory above it.
// The lock sits in the never-tracked lock directory of the checkout that physically holds that directory.
// A lock reached through a linked lym directory is therefore the lock reached through the directory itself.
// A file under no tracked lym directory gets its lock beside it, path with suffix appended.
// It creates nothing.
func TrackedFileLockPath(path, suffix string) string {
	clean := filepath.Clean(path)
	rel := filepath.Base(clean)
	dir := filepath.Dir(clean)
	for {
		if filepath.Base(dir) == lymdirs.LymDirName {
			return filepath.Join(locksDir(checkoutHolding(dir)), lymdirs.LymDirName, rel) + suffix
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return path + suffix
		}
		rel = filepath.Join(filepath.Base(dir), rel)
		dir = parent
	}
}

// checkoutHolding returns the directory that physically holds the tracked lym directory at lymDir.
// It follows a link at lymDir to where the directory lives.
func checkoutHolding(lymDir string) string {
	if resolved, err := filepath.EvalSymlinks(lymDir); err == nil {
		return filepath.Dir(resolved)
	}
	return resolve(filepath.Dir(lymDir))
}

// resolve returns dir with its symlinks followed, or dir unchanged when it does not resolve.
func resolve(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

// locksDir returns the never-tracked lock directory of the checkout tree at root.
func locksDir(root string) string {
	return filepath.Join(root, lymdirs.DotLymDirName, lymdirs.LocksDirName)
}

// refuseTrackedTree returns an error when lockPath sits in a tracked lym directory or in a tracked Bolt checkout.
// Such a lock would show as a change that a commit can stage.
// The checkout's lock directory is exempt, and so is any never-tracked directory below the Bolt checkout's root.
// A directory named like the Bolt checkout but holding no checkout, such as its entry in a git dir, is not one.
func refuseTrackedTree(lockPath string) error {
	segments := strings.Split(filepath.ToSlash(filepath.Clean(lockPath)), "/")
	inBolt := false
	for i, segment := range segments {
		switch segment {
		case lymdirs.DotLymDirName:
			if i+1 < len(segments) && segments[i+1] == lymdirs.LocksDirName {
				return nil
			}
			inBolt = false
		case lymdirs.LymDirName:
			return refusedTrackedTree(lockPath, lymdirs.LymDirName)
		case lymdirs.BoltDirName:
			candidate := filepath.FromSlash(strings.Join(segments[:i+1], "/"))
			_, err := os.Lstat(filepath.Join(candidate, ".git"))
			inBolt = err == nil
		}
	}
	if inBolt {
		return refusedTrackedTree(lockPath, lymdirs.BoltDirName)
	}
	return nil
}

// refusedTrackedTree returns an error for a lock at lockPath inside the tracked directory named tracked.
// The error says to build the lock path under the checkout's lock directory.
func refusedTrackedTree(lockPath, tracked string) error {
	return fmt.Errorf("lock: %s sits in a tracked %s directory; way forward: build the lock path with lock.TrackedFileLockPath or lock.TrackedLockPath, which put it under the checkout's %s", lockPath, tracked, filepath.Join(lymdirs.DotLymDirName, lymdirs.LocksDirName))
}
