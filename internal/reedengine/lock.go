// lock.go defines the Engine type, the domain kernel's public handle, and the reed operation lock.
// Every public engine op acquires that lock exactly once at its outer boundary.
// The other files in this package declare their methods on *Engine.

package reedengine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// reedLockFileName names the reed operation lock file, in the directory stateDir returns under Geometry.AnchorPath.
// It is distinct from reed.json's own lock file.
// reed.lock guards the whole engine-op cycle (read -> mutate -> persist -> render -> apply);
// reed.json.lock guards only the JSON file swap inside that cycle.
// Lock ordering is strictly outer (reed.lock) then inner (reed.json.lock):
// an engine op reaches reed.json.lock only through LoadState and SaveState, while it holds reed.lock.
const reedLockFileName = "reed.lock"

// Engine is the domain kernel's public handle:
// a resolved Config, the Geometry it was told, and the TmuxCmd bound to this hub's socket.
// The zero Engine is not valid: build one with New.
type Engine struct {
	cfg  Config
	geom Geometry
	tmux TmuxCmd
	// skipRevival is set under the op lock while a step runs after the earlier worktrees were revived.
	skipRevival bool
	// warnOut receives the operator-facing warning for a tmux config that failed to source.
	warnOut io.Writer
	// activityFailures guards the operator-activity read failure, which repeats on every watchdog tick.
	activityFailures logger.Episode
	// nameRepairFailures guards pane capture and rename failures of the name repair, one episode per strand and step.
	nameRepairFailures logger.EpisodeSet
	// calleeFailures guards tmux reads and writes in the watch and public-op callees, one per read; goroutines share it.
	calleeFailures logger.EpisodeSet
	// stateLoadNotices remembers the last binding clear a state load logged; a clear is never saved, so it repeats.
	stateLoadNotices stateLoadNotices
	// build is this binary's VCS identity, which the navigation bindings stamp.
	build buildvcs.Identity
	// scrub is the set of environment keys the tmux server spawn must not inherit.
	scrub EnvScrub
	// ensureServerGone is a test seam for the wait after kill-server; nil selects the production wait.
	ensureServerGone func(serverPID int) error
	// paneAge reports the elapsed time of a pane's first process; New sets it to paneProcessAge and a test sets a stub.
	paneAge func(pid int) (time.Duration, bool)
	// executablePath resolves the running lym binary's path; under go test that is the test binary, so tests stub it.
	executablePath func() (string, error)
}

// New builds an Engine for the given Config, Geometry and EnvScrub.
// The caller populates every Geometry field, since New validates none of them.
// New panics when scrub holds no name and no prefix.
// An engine registry always declares keys, so an empty set is a wiring defect.
func New(cfg Config, geom Geometry, scrub EnvScrub) *Engine {
	if len(scrub.Names) == 0 && len(scrub.Prefixes) == 0 {
		panic("reedengine: New needs a non-empty EnvScrub")
	}
	return &Engine{
		cfg:   cfg,
		geom:  geom,
		scrub: scrub,
		tmux:  NewTmuxCmd(cfg.Tmux, geom.SocketKey),

		warnOut:        os.Stderr,
		build:          buildvcs.Running(),
		paneAge:        paneProcessAge,
		executablePath: os.Executable,
	}
}

// Socket returns this engine's tmux -L socket name.
func (e *Engine) Socket() string {
	return e.geom.SocketKey
}

// SessionName returns this engine's tmux session name.
func (e *Engine) SessionName() string {
	return e.geom.SessionName
}

// TmuxPath returns the resolved tmux binary path this engine uses.
func (e *Engine) TmuxPath() string {
	return e.cfg.Tmux
}

// ShellPath returns the configured shell this engine's descendant-closure walk spawns on Windows.
// It validates and defaults nothing: an empty cfg.Shell is returned as the empty string.
// The shell is never validated on this path.
func (e *Engine) ShellPath() string {
	return e.cfg.Shell
}

// withOpLock acquires the operation lock, runs fn while holding it, and releases the lock before returning.
// It is the only acquisition point for reed.lock in the package, and it is non-reentrant.
//
// It is the single chokepoint every public engine op passes, so the told-geometry pre-flight runs here.
// The engine validates no Geometry field when it is built, so this is the first point to refuse an unusable identity.
// The pre-flight runs before the lock, and so before any tmux round trip, directory creation or state read.
// A bad identity therefore creates no substrate that no reed verb can address.
// Nor does it create state outside the worktree.
// The order is:
// told tmux identity, then told anchor shape, then told worktree-root shape and liveness, then MkdirAll, then the lock.
// The worktree-root liveness check runs last, because it is the only validator that touches the filesystem.
// The two cheap contract refusals therefore fire first on a geometry that is wrong in several ways at once.
// Under the lock and before fn, it re-pins the server's navigation bindings when an older build pinned them,
// so a deployed binary reaches a running server at its first op.
func (e *Engine) withOpLock(fn func() error) error {
	if err := validateToldTmuxIdentity(e.geom); err != nil {
		return err
	}
	if err := validateToldAnchorPath(e.geom); err != nil {
		return err
	}
	if err := validateToldWorktreeRootLive(e.geom); err != nil {
		return err
	}

	dotLym := e.stateDir()
	// gofrs/flock opens the lock file with O_CREATE but never creates missing parent directories,
	// so a brand-new worktree's first reed operation, before .lym exists, must create the directory here first.
	if err := os.MkdirAll(dotLym, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dotLym, err)
	}

	lockPath := filepath.Join(dotLym, reedLockFileName)
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire reed op lock: %w", err)
	}
	defer l.Release()

	// heldLockFile is the identity of the lock file, snapshotted while it is provably at lockPath.
	// The post-op check uses it to tell whether the lock file is still the file that was held.
	// A stat failure leaves no baseline, so the check is skipped.
	heldLockFile, heldErr := os.Stat(lockPath)
	if heldErr != nil {
		logger.Debug("reed: could not identify the op lock file, skipping the post-op exclusion check",
			"socket", e.Socket(), "lock", lockPath, "err", heldErr)
	}

	e.repinStaleBindingsLocked()
	opErr := fn()
	if heldErr != nil {
		return opErr
	}
	if lockErr := opLockCompromisedError(lockPath, heldLockFile); lockErr != nil {
		if opErr != nil {
			return fmt.Errorf("%w — and the operation itself failed: %w", lockErr, opErr)
		}
		return lockErr
	}
	return opErr
}

// withTryOpLock acquires the operation lock without blocking, runs fn while holding it if acquired,
// and releases it before returning.
//
// A lock held by someone else is a deferral, not a failure: it is reported as (false, nil) and fn is never called.
// The watcher needs non-blocking acquisition:
// lock.AcquireWriteLock blocks with no timeout, and a per-event retry cannot describe a long wait.
// Deferring is also correct: whatever holds the lock is another reed op, and every reed op re-applies the layout.
// The watcher's own re-apply would be redundant even if it could wait for its turn.
//
// It runs the three told-geometry pre-flight validations and the post-op lock-compromise check.
// Skipping them would let an unusable identity, a vanished worktree root, or a compromised lock pass unchecked.
// The resize watch loop is the caller most likely to run unattended.
func (e *Engine) withTryOpLock(fn func() error) (acquired bool, err error) {
	if err := validateToldTmuxIdentity(e.geom); err != nil {
		return false, err
	}
	if err := validateToldAnchorPath(e.geom); err != nil {
		return false, err
	}
	if err := validateToldWorktreeRootLive(e.geom); err != nil {
		return false, err
	}

	dotLym := e.stateDir()
	if err := os.MkdirAll(dotLym, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", dotLym, err)
	}

	lockPath := filepath.Join(dotLym, reedLockFileName)
	l, locked, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, fmt.Errorf("try acquire reed op lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer l.Release()

	heldLockFile, heldErr := os.Stat(lockPath)
	if heldErr != nil {
		logger.Debug("reed: could not identify the op lock file, skipping the post-op exclusion check",
			"socket", e.Socket(), "lock", lockPath, "err", heldErr)
	}

	opErr := fn()
	if heldErr != nil {
		return true, opErr
	}
	if lockErr := opLockCompromisedError(lockPath, heldLockFile); lockErr != nil {
		if opErr != nil {
			return true, fmt.Errorf("%w — and the operation itself failed: %w", lockErr, opErr)
		}
		return true, lockErr
	}
	return true, opErr
}

// opLockCompromisedError reports an error when the file this op's lock was held on is no longer the file at lockPath.
// That means the exclusion the lock provides did not hold for the operation.
//
// A flock is held on an inode, not on a name.
// Deleting .lym while an op holds reed.lock unlinks that inode without disturbing the lock.
// An ordinary `rm -rf` does this, and so does `git clean -xdf`, a sanctioned operator action.
// The next op's os.MkdirAll and O_CREATE then mint a fresh inode, and its lock is granted at once.
// Two reed ops then drive the same tmux session and the same reed.json concurrently, and the last writer wins.
//
// The check detects this and does not prevent it.
// Once the inode is unlinked it is unreachable by name.
// No name-based lock can therefore make the second op wait for the first.
// What is achievable is that such an operation says so, instead of returning ok:true over possibly interleaved state.
// The same reasoning applies to reed.json.lock in internal/state, which this check leaves alone:
// reed.lock is the outer lock, and the op that notices a compromise there already reports it.
func opLockCompromisedError(lockPath string, heldLockFile os.FileInfo) error {
	current, err := os.Stat(lockPath)
	if err == nil && os.SameFile(heldLockFile, current) {
		return nil
	}
	return fmt.Errorf(
		"reed's operation lock %s was removed or replaced while this operation was running, so it did not exclude other reed processes — "+
			"something deleted the .lym directory (an rm, or a `git clean -xdf` in the worktree) mid-operation. "+
			"Reed state and the tmux session may now disagree; run \"lym reed status\" to inspect, or \"lym reed down\" and \"lym reed resume\" to rebuild",
		lockPath)
}
