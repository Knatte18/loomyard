// teardownlock.go — the per-slug teardown lock that orders a pair against its pushes and its removal.
// The lock file lives under the hub scratch directory, outside both worktrees of the pair.
// Neither the file nor a child's working directory or log therefore sits in a tree that a removal deletes.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// teardownLockDirName is the directory under the hub scratch directory that holds one teardown lock file per pair slug.
const teardownLockDirName = "teardown"

// ErrPairPushRunning is the sentinel a refusal wraps when another holder keeps the pair's teardown lock past the wait.
// Nothing has been mutated or pushed when it is returned.
// Re-running the refused command once the holder finishes retries it whole.
var ErrPairPushRunning = errors.New("a push of the pair is still running")

// detachedPushHeldHook, nil in production, runs inside DetachedPush while it holds the teardown lock shared.
var detachedPushHeldHook func()

// removeAfterTeardownHook, nil in production, runs inside Remove right after the worktree teardown, before it releases the teardown lock.
var removeAfterTeardownHook func()

// removeAfterLockHook, nil in production, runs inside a removal right after it takes the teardown lock; its error is returned as that step's failure, with both worktrees still in place.
var removeAfterLockHook func() error

// ErrPairBusy is the sentinel a local removal's error wraps when another writer holds a lock the removal would otherwise wait on.
// Nothing has been changed when it is returned, so the caller retries once the writer lets go.
var ErrPairBusy = errors.New("the pair is busy: another writer holds its lock")

// TeardownLockPath returns the lock file that orders slug's pair against its pushes:
// <HubScratchDir(hubPath)>/teardown/<slug>.lock.
// A push of the pair takes it shared; a mutation of the pair takes it exclusive.
func TeardownLockPath(hubPath, slug string) string {
	return filepath.Join(HubScratchDir(hubPath), teardownLockDirName, slug+".lock")
}

// ensureTeardownLockDir creates the directory of slug's teardown lock through an os.Root at the hub,
// so a symlink planted at any component that escapes the hub is refused.
func ensureTeardownLockDir(hubPath, slug string) error {
	root, err := os.OpenRoot(hubPath)
	if err != nil {
		return fmt.Errorf("open hub root %s: %w", hubPath, err)
	}
	defer root.Close()

	dirRel, err := hubRel(hubPath, filepath.Dir(TeardownLockPath(hubPath, slug)))
	if err != nil {
		return err
	}
	if err := root.MkdirAll(dirRel, 0o755); err != nil {
		return fmt.Errorf("create teardown lock dir %s: %w", dirRel, err)
	}
	return nil
}

// DetachedPush is the body of the detached push child:
// it pushes the sides req carries under the push lock, yielding to a removal of the pair.
// With req.TeardownSlug set it holds the pair's teardown lock shared for its whole run.
// It exits with a warning and a nil error when a removal holds that lock.
// A child also exits with a warning and a nil error when a worktree path it was given is no longer a directory.
// This holds with or without a teardown slug.
// The check runs before the child creates anything, so it leaves nothing at a removed path.
// A child whose pair's parent lock was not held at spawn (req.ParentLocked) exits when a pause covers it.
// It logs an Info line and returns a nil error before it takes the teardown lock or marks any flag.
// The commits stay local for the next push.
func DetachedPush(hubPath string, req PushSpawn) (PushResult, error) {
	if !req.ParentLocked && pauserecord.GateCovered() {
		logger.Info("fabricengine: a pause covers the push; exiting with the commits left local", "slug", req.TeardownSlug)
		return PushResult{}, nil
	}

	if req.TeardownSlug != "" {
		if err := ensureTeardownLockDir(hubPath, req.TeardownSlug); err != nil {
			return PushResult{}, err
		}
		held, acquired, err := lock.TryAcquireReadLock(TeardownLockPath(hubPath, req.TeardownSlug))
		if err != nil {
			return PushResult{}, fmt.Errorf("fabricengine: take the teardown lock: %w", err)
		}
		if !acquired {
			logger.Warn("fabricengine: the pair is being removed; its push is not run", "slug", req.TeardownSlug)
			return PushResult{}, nil
		}
		defer func() { _ = held.Release() }()

		if detachedPushHeldHook != nil {
			detachedPushHeldHook()
		}
	}

	for _, worktree := range []string{req.WarpPath, req.WeftPath} {
		if worktree == "" {
			continue
		}
		if info, err := os.Stat(worktree); err != nil || !info.IsDir() {
			logger.Warn("fabricengine: a worktree of the push is gone; its push is not run", "path", worktree)
			return PushResult{}, nil
		}
	}

	return CoalescePushRequest(req, SyncOptions{})
}

// lockPairTeardown takes slug's teardown lock exclusive.
// It waits at most t.teardownWait for a push of the pair or another exclusive holder to let go of it.
// With try set it makes one attempt instead and returns an error wrapping ErrPairBusy when the lock is held.
// It returns the release function, or an error wrapping ErrPairPushRunning when the wait runs out;
// that error names rerun, the re-run of the caller's command that retries it.
// A lock file this call creates is recorded in rec, whether or not the lock was won, since the file outlives the call.
func (t *Topology) lockPairTeardown(rec *Mutations, l *lymcwd.Location, slug string, try bool, rerun string) (release func(), err error) {
	lockPath := TeardownLockPath(l.HubPath, slug)
	existed := pathPresent(lockPath)
	if err := ensureTeardownLockDir(l.HubPath, slug); err != nil {
		return nil, err
	}
	var held *lock.FileLock
	var acquired bool
	if try {
		held, acquired, err = lock.TryAcquireWriteLock(lockPath)
	} else {
		held, acquired, err = lock.AcquireWriteLockWithin(lockPath, t.teardownWait)
	}
	if !existed && pathPresent(lockPath) {
		rec.Append(KindFileWritten, lockPath, "")
	}
	if err != nil {
		return nil, fmt.Errorf("take the teardown lock of the pair %q: %w", slug, err)
	}
	if !acquired {
		return nil, teardownLockRefusal(slug, try, rerun)
	}
	return func() { _ = held.Release() }, nil
}

// teardownLockRefusal is the refusal for a teardown lock of slug that another holder kept.
// With try set it wraps ErrPairBusy after one attempt; otherwise it wraps ErrPairPushRunning and names rerun.
func teardownLockRefusal(slug string, try bool, rerun string) error {
	if try {
		return fmt.Errorf("the teardown lock of the pair %q is held by a push or another removal: %w", slug, ErrPairBusy)
	}
	return fmt.Errorf("a push or removal of the pair %q is still running or waiting under a pause; %s once it finishes or the pause is resumed, or end the stuck push or teardown child, whose pid `lym tender status` lists under the pair's owner: %w", slug, rerun, ErrPairPushRunning)
}

// respawnPairPush spawns the push of the pair's worktrees that survive a failed Remove,
// so a child that exited on the held teardown lock loses nothing.
// l is the location Remove ran from, the prime, so the pair's own location is built from it.
// A failed spawn is logged: the pair's next push carries its commits.
func respawnPairPush(l *lymcwd.Location, slug string) {
	taskWorktree := WorktreePath(l, slug)
	recordsWorktree := RecordsWorktreePath(l, slug)

	switch {
	case pathPresent(taskWorktree):
		pairLocation, err := lymcwd.ResolveWorktree(taskWorktree)
		if err != nil {
			logger.Warn("fabricengine: could not respawn the pair's push after a failed remove", "slug", slug, "error", err)
			return
		}
		if _, err := SpawnStatusPush(pairLocation, ""); err != nil {
			logger.Warn("fabricengine: respawning the pair's push after a failed remove failed", "slug", slug, "error", err)
		}
	case pathPresent(recordsWorktree):
		req := PushSpawn{WeftPath: recordsWorktree, PauseSlug: slug, TeardownSlug: slug, WorkDir: BoltDir(l.HubPath)}
		if err := spawnDetachedPushFn(req); err != nil {
			logger.Warn("fabricengine: respawning the pair's records push after a failed remove failed", "slug", slug, "error", err)
		}
	}
}
