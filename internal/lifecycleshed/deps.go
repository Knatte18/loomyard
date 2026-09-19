// deps.go declares the three seam types this package's producers are constructed with: PrimeLock,
// shared by WorktreeCreate and WorktreeTeardown, and InnerRunDeps and TeardownDeps, each specific
// to one producer.

package lifecycleshed

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// PrimeLock carries the told absolute path to a hub-scoped advisory lock plus the injected
// acquire closure both WorktreeCreate and WorktreeTeardown hold it behind, so the two producers
// that mutate the hub's worktree registry concurrently with each other never race.
//
// Acquire mirrors lock.TryAcquireWriteLock's own contract: a (nil, false, nil) return means the
// lock is already held by someone else -- contention, not an error -- while a non-nil err means
// acquisition itself failed. The lock file this package acquires carries no holder record, so a
// contention stuck reason can name Path and nothing else: there is no way to say who is holding
// it.
type PrimeLock struct {
	// Path is the told absolute lock-file path, named in a contention stuck reason.
	Path string
	// Acquire attempts the lock without blocking. A (nil, false, nil) return is contention: the
	// lock is held elsewhere right now. A non-nil release, when returned, must be called exactly
	// once by the caller to release the lock.
	Acquire func() (release func() error, ok bool, err error)
}

// InnerRunDeps carries every told value and injected closure NewInnerRun needs: spawning the
// inner shed run, resolving and reading its persisted status, and the two seams a test replaces to
// keep the poll loop out of real time.
type InnerRunDeps struct {
	// Spawn starts the inner shed run and blocks until it exits. InnerRun waits for its child
	// rather than detaching, per the Live-Substrate Spawn Observability invariant.
	Spawn func(ctx context.Context) error
	// ResolveStatus resolves the absolute status-file path and its companion lock path for the
	// task worktree. It is evaluated on Call, never at wiring time: the task worktree this status
	// file lives in does not exist until WorktreeCreate has already run, so resolving it any
	// earlier would resolve a path that is not there yet.
	ResolveStatus func() (statusPath, statusLockPath string, err error)
	// ReadStatus reads and decodes the persisted status file under statusLockPath's protection,
	// reporting found == false when no status file exists yet.
	ReadStatus func(statusPath, statusLockPath string) (shedengine.Status, bool, error)
	// Now returns the current time. A nil Now resolves to time.Now in NewInnerRun, so production
	// code never sets this field; a test holds the clock still by setting it.
	Now func() time.Time
	// Sleep pauses for d. A nil Sleep resolves to time.Sleep in NewInnerRun; a test replaces it
	// with a no-op so the attempt-cap test proves the bound is attempt-counted, not
	// wall-clock-timed, without spending any real time.
	Sleep func(d time.Duration)
}

// TeardownDeps carries the two closures NewWorktreeTeardown calls in sequence: Shutdown strictly
// before Remove. The two are separate fields, not one closure, because the producer must never
// call Remove at all when Shutdown fails, must say which of the two failed in its stuck reason,
// and must surface the abandoned-session value Shutdown returns on an otherwise-Done row -- none
// of which a single combined closure could expose.
type TeardownDeps struct {
	// Shutdown ends the loom session's own driving process, if one is still attached, reporting
	// the name of any session it had to abandon rather than cleanly end.
	Shutdown func(ctx context.Context) (abandonedSession string, err error)
	// Remove deletes the task worktree pair. Called only after Shutdown has succeeded.
	Remove func(ctx context.Context) error
}
