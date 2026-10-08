// hubreconcile.go declares the hub config walk's geometry, options, entry point and error types.

package hubreconcile

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// DefaultLockWait bounds how long Ensure waits for the hub lock when Options.LockWait is zero.
// It is well above a walk with no contended pair lock.
const DefaultLockWait = 2 * time.Minute

// Geometry is the told geometry of one hub: the paths the walk works on.
type Geometry struct {
	// BoardDir is the hub's board dir, where the hub-wide config lives at _lyx/config/ and the stamp and lock live at .lyx/config/.
	BoardDir string
	// WorktreePath is the caller's code worktree root, from which the walk lists the hub's code worktrees.
	WorktreePath string
}

// StampPath returns the build stamp file, in the never-tracked mirror of the hub-wide config dir.
func (g Geometry) StampPath() string {
	return filepath.Join(configengine.ScratchDir(g.BoardDir), "build-stamp.json")
}

// LockPath returns the hub lock file, beside the build stamp.
func (g Geometry) LockPath() string {
	return filepath.Join(configengine.ScratchDir(g.BoardDir), "reconcile.lock")
}

// Options tunes one Ensure call.
type Options struct {
	// LockWait bounds the wait for the hub lock; zero means DefaultLockWait.
	LockWait time.Duration
	// Pair is a code worktree root.
	// When set, Ensure reconciles that one pair unconditionally, whatever the build stamp says, and neither reads nor writes the stamp.
	Pair string
}

// Ensure reconciles the hub's config once per binary build, under the hub lock, and commits what it wrote.
// A build stamp naming the running build returns nil after one file read, with no lock and no git.
// Options.Pair selects the unconditional single-pair call instead of the stamp-gated walk over the whole hub.
// A lock not acquired within the wait returns *LockTimeoutError; a reconcile or commit failure returns *WorktreeError.
func Ensure(geom Geometry, opts Options) error {
	return ensure(geom, opts, walkHooks{})
}

// LockTimeoutError reports that another lyx command held the hub lock for the whole wait.
type LockTimeoutError struct {
	// Path is the hub lock file.
	Path string
	// Wait is how long the caller waited for it.
	Wait time.Duration
}

// Error names the lock and ends with the way forward.
func (e *LockTimeoutError) Error() string {
	return fmt.Sprintf("hubreconcile: another lyx command holds the hub's reconcile lock %s (waited %s); re-run this command once it finishes", e.Path, e.Wait)
}

// WorktreeError reports a reconcile or commit failure in one worktree.
type WorktreeError struct {
	// Worktree is the root of the worktree, or the board dir, that failed.
	Worktree string
	// File is the config file that failed; empty when the failure names no file.
	File string
	// Module is the config module of File; empty when File is.
	Module string
	// Err is the underlying failure.
	Err error
}

// Error names the worktree, the file and the cause, and ends with the way forward.
func (e *WorktreeError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("hubreconcile: reconcile config in %s: %v; re-run this command", e.Worktree, e.Err)
	}
	return fmt.Sprintf("hubreconcile: reconcile config in %s: %s: %v; fix %s (or run \"lyx config %s\" on it), then re-run this command", e.Worktree, e.File, e.Err, e.File, e.Module)
}

// Unwrap returns the underlying failure.
func (e *WorktreeError) Unwrap() error {
	return e.Err
}
