// root.go — the per-user temp root: where it lives, how it is created and the checks it must pass.

package lyxtemp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Root is a lyx temp root directory that run directories live under.
type Root struct {
	path string
	now  func() time.Time
	// remove, retryNow and sleep are the sweep's removal, retry-budget clock and retry pause, replaceable so a test retries without waiting.
	remove   func(string) error
	retryNow func() time.Time
	sleep    func(time.Duration)
}

// Path returns the root's absolute path.
func (r *Root) Path() string {
	return r.path
}

// ProcessRoot opens the temp root for the current user under the system temp directory, creating it when absent.
// It fails, creating nothing, when the root exists but is not a private directory of the user.
func ProcessRoot() (*Root, error) {
	uid := os.Getuid()
	return openRoot(resolveRootPath(os.TempDir(), uid), uid, time.Now)
}

// openRoot creates the root at path with mode 0700 when absent, then checks it, whatever created it.
// now is the clock the root's sweep reads.
func openRoot(path string, uid int, now func() time.Time) (*Root, error) {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("create lyx temp root %q: %w", path, err)
	}
	if err := checkRoot(path, uid); err != nil {
		return nil, err
	}
	return &Root{path: path, now: now, remove: os.RemoveAll, retryNow: time.Now, sleep: time.Sleep}, nil
}

// resolveRootPath returns the root path under tempDir.
// When tempDir is itself a run directory under a lyx root, it returns that root, so a child that inherited a run directory as its system temp directory works beside its parent.
func resolveRootPath(tempDir string, uid int) string {
	if strings.HasPrefix(filepath.Base(tempDir), runPrefix) {
		parent := filepath.Dir(tempDir)
		if filepath.Base(parent) == rootName(uid) {
			return parent
		}
	}
	return filepath.Join(tempDir, rootName(uid))
}

// checkRealDirectory fails unless path is a directory that is neither a link nor an irregular file.
func checkRealDirectory(path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lyx temp root %q: %w", path, err)
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !info.IsDir() {
		return nil, fmt.Errorf("lyx temp root %q is not a real directory", path)
	}
	return info, nil
}
