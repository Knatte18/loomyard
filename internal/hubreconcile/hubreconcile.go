// hubreconcile.go declares the hub config walk's geometry, options, entry point and error types.

package hubreconcile

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
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
// A build stamp naming the running build returns nil after one file read, one stat per per-worktree module's repo default and one for the board's base-sparse marker, with no lock and no git;
// an absent repo default or an absent marker makes the stamp stale.
// Options.Pair selects the unconditional single-pair call instead of the stamp-gated walk over the whole hub.
// A lock not acquired within the wait returns *LockTimeoutError; a reconcile or commit failure returns *WorktreeError.
func Ensure(geom Geometry, opts Options) error {
	return ensure(geom, opts, walkHooks{})
}

// reconcileWayForward is the way forward a failed reconcile's warning names.
const reconcileWayForward = "fix the cause the error names, then run a start verb to retry the reconcile: `lyx loom resume` in the task worktree, or `lyx batten run` or `lyx orch start` from the prime"

// EnsureOrWarn is Ensure for a caller that reads a hub's config and goes on when the reconcile fails.
// A failure logs one warning naming the error and the way forward and returns nothing, leaving the stamp stale so the next start verb retries the walk.
func EnsureOrWarn(geom Geometry, opts Options) {
	if err := Ensure(geom, opts); err != nil {
		logger.Warn("hubreconcile: hub config reconcile failed, going on with the config as it is", "error", err, "way_forward", reconcileWayForward)
	}
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
	// Module is the config module of File; empty when File is, or when File belongs to no config module, such as the board marker.
	Module string
	// Board marks a failure at the board dir, whose files are edited at the default layer from the prime worktree.
	Board bool
	// Err is the underlying failure.
	Err error
}

// Error names the worktree, the file and the cause, and ends with the way forward.
// A failing board file names the default-layer command, and a file that belongs to no config module names only the file and the re-run.
func (e *WorktreeError) Error() string {
	switch {
	case e.File == "":
		return fmt.Sprintf("hubreconcile: reconcile config in %s: %v; re-run this command", e.Worktree, e.Err)
	case e.Module == "":
		return fmt.Sprintf("hubreconcile: reconcile config in %s: %s: %v; fix %s, then re-run this command", e.Worktree, e.File, e.Err, e.File)
	case e.Board:
		return fmt.Sprintf("hubreconcile: reconcile config in %s: %s: %v; fix %s (or run \"lyx config %s --layer default\" from the hub's prime worktree), then re-run this command", e.Worktree, e.File, e.Err, e.File, e.Module)
	}
	return fmt.Sprintf("hubreconcile: reconcile config in %s: %s: %v; fix %s (or run \"lyx config %s\" on it), then re-run this command", e.Worktree, e.File, e.Err, e.File, e.Module)
}

// Unwrap returns the underlying failure.
func (e *WorktreeError) Unwrap() error {
	return e.Err
}
