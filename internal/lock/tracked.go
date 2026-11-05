// tracked.go places the lock files of tracked files under a checkout's never-tracked lock directory and refuses a lock path inside a tracked lym or Bolt directory.

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// TrackedLockPath returns the lock file named name of the tracked checkout tree at root: name joined onto root's never-tracked lock directory.
// root is resolved through symlinks, so every path that reaches one checkout names the same lock file;
// a root that does not resolve is used as given.
// It creates nothing.
func TrackedLockPath(root, name string) string {
	return filepath.Join(locksDir(resolve(root)), name)
}

// TrackedFileLockPath returns the lock file guarding the file at path, with suffix appended to the lock's name.
// For a file under a tracked lym directory, the lock mirrors the file's place under the nearest such directory above it, inside the never-tracked lock directory of the checkout that physically holds that directory.
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

// checkoutHolding returns the directory that physically holds the tracked lym directory at lymDir, following a link to where the directory lives.
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

// refuseTrackedTree returns an error when lockPath sits inside a tracked lym directory or a tracked directory of the Bolt checkout, where the lock file would show as a change a commit can stage.
// The checkout's lock directory is the exemption, a never-tracked directory below the Bolt checkout's root is not tracked,
// and a directory named like the Bolt checkout that holds no checkout, such as its entry in a git dir, is no Bolt checkout.
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

// refusedTrackedTree returns the refusal of a lock at lockPath inside the tracked directory named tracked, with the way forward.
func refusedTrackedTree(lockPath, tracked string) error {
	return fmt.Errorf("lock: %s sits in a tracked %s directory; way forward: build the lock path with lock.TrackedFileLockPath or lock.TrackedLockPath, which put it under the checkout's %s", lockPath, tracked, filepath.Join(lymdirs.DotLymDirName, lymdirs.LocksDirName))
}
