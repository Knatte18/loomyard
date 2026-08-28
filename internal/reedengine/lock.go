// lock.go defines the Engine type — the domain kernel's public handle, holding a resolved Config,
// the Geometry it was told, and the TmuxCmd bound to this hub's socket — plus the single
// reed-operation lock every public engine op acquires exactly once at its outer boundary.
// Every other file in this package (reconcile.go, apply.go, spawn.go, strand.go, lifecycle.go)
// hangs its exported/*Locked methods off *Engine.

package reedengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
)

// reedLockFileName is the reed operation lock's file name inside the ephemeral
// .lyx directory under the told Geometry.AnchorPath (see stateDir). It is
// deliberately distinct from reed.json's own
// lock file (reed.json.lock, owned by internal/state): reed.lock guards the
// whole engine-op cycle (read -> mutate -> persist -> render -> apply),
// while reed.json.lock guards only the JSON file swap inside that cycle.
// Lock ordering is strictly outer(reed.lock) -> inner(reed.json.lock); an
// engine op only ever reaches reed.json.lock indirectly, through
// LoadState/SaveState, while it is already holding reed.lock.
const reedLockFileName = "reed.lock"

// Engine is the domain kernel's public handle: a resolved Config, the Geometry it was told, and the
// TmuxCmd bound to this hub's socket.
// The zero Engine is not valid;
// build one via New.
type Engine struct {
	cfg  Config
	geom Geometry
	tmux TmuxCmd
}

// New builds an Engine for the given Config and Geometry.
// The caller owns populating every Geometry field; New validates none of them.
// hubgeom.ReedGeometry is the hub-mode answer.
func New(cfg Config, geom Geometry) *Engine {
	return &Engine{
		cfg:  cfg,
		geom: geom,
		tmux: NewTmuxCmd(cfg.Tmux, geom.SocketKey),
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

// withOpLock acquires the operation lock, runs fn while holding it,
// and releases it before returning. This is the only acquisition point
// for reed.lock in the package; it is non-reentrant.
//
// It is also the single chokepoint every public engine op passes, which is why the told-geometry
// pre-flight runs here rather than in New: New validates no Geometry field by documented contract
// (geometry.go), so this is the first point at which an unusable identity can be refused without
// changing that contract. Refusing before the lock — and therefore before any tmux round trip,
// any directory creation, and any state read — is what keeps a bad identity from creating substrate
// that no later reed verb can address, or state under a directory that is not the worktree (see
// validateToldTmuxIdentity and validateToldAnchorPath in server.go).
// The anchor check runs second on purpose: an unusable tmux identity is the more actionable
// diagnosis of the two, and the lock path below is what the anchor check protects, so it only has
// to precede that.
func (e *Engine) withOpLock(fn func() error) error {
	if err := validateToldTmuxIdentity(e.geom); err != nil {
		return err
	}
	if err := validateToldAnchorPath(e.geom); err != nil {
		return err
	}

	dotLyx := e.stateDir()
	// gofrs/flock opens the lock file with O_CREATE but never creates
	// missing parent directories, so a brand-new worktree's first reed
	// operation (before .lyx exists at all) must create it here first,
	// matching internal/state's own MkdirAll-before-lock pattern.
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dotLyx, err)
	}

	lockPath := filepath.Join(dotLyx, reedLockFileName)
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire reed op lock: %w", err)
	}
	defer l.Release()

	// The identity of the file this lock is actually held on, snapshotted while it is provably the
	// file at lockPath, so the post-op check below can tell whether it still is. A stat failure here
	// leaves no baseline to compare against, so the check is skipped rather than guessed at.
	heldLockFile, heldErr := os.Stat(lockPath)
	if heldErr != nil {
		logger.Debug("reed: could not identify the op lock file, skipping the post-op exclusion check",
			"socket", e.Socket(), "lock", lockPath, "err", heldErr)
	}

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
// A lock held by someone else is a DEFERRAL, not a failure: it is reported as (false, nil) with no
// error, and fn is never called. The watcher needs this rather than withOpLock because
// lock.AcquireWriteLock blocks with no timeout — this repo's own R5 measurement (opLockCompromisedError
// above) records a second `lyx reed status` blocking for 11027ms behind a held lock, a state the
// watcher's per-event retry contract cannot describe. Deferring is correct on the merits too, not just
// convenient: whatever holds the lock is another reed op, and every reed op ends by re-applying the
// layout itself, so the watcher's own re-apply would be redundant even if it could wait for its turn.
//
// It keeps both told-geometry pre-flight validations and the post-op lock-compromise check that
// withOpLock runs, because those are the reason withOpLock is a chokepoint rather than a bare lock
// acquisition — a non-blocking sibling that skipped them would let an unusable identity or a
// compromised lock pass silently through the one caller most likely to run unattended.
func (e *Engine) withTryOpLock(fn func() error) (acquired bool, err error) {
	if err := validateToldTmuxIdentity(e.geom); err != nil {
		return false, err
	}
	if err := validateToldAnchorPath(e.geom); err != nil {
		return false, err
	}

	dotLyx := e.stateDir()
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", dotLyx, err)
	}

	lockPath := filepath.Join(dotLyx, reedLockFileName)
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

// opLockCompromisedError reports an error when the file this op's lock was held on is no longer the
// file at lockPath — meaning the exclusion the lock was supposed to provide did not hold for the
// duration of the operation.
//
// A flock is held on an INODE, not on a name. Deleting .lyx while an op holds reed.lock — an
// ordinary `rm -rf`, or the `git clean -xdf` the Durable-vs-Ephemeral State Invariant makes a
// sanctioned operator action — unlinks that inode without disturbing the lock, and the next op's
// os.MkdirAll + O_CREATE then mints a FRESH inode whose lock is granted immediately. Two reed ops
// then drive the same tmux session and the same reed.json concurrently, last writer wins.
// Measured live (R5 review finding R5-F6): with reed.lock held, a second `lyx reed status` blocked
// for 11027ms as designed; with .lyx deleted mid-hold it was granted in 107ms.
//
// This DETECTS that, it does not prevent it, and the distinction is deliberate rather than a
// shortcut: once the inode is unlinked it is unreachable by name, so no name-based lock can make the
// second op wait for the first. What is achievable — and what an operator actually needs — is that
// an operation which ran without the exclusion it claimed says so, instead of returning ok:true over
// possibly-interleaved state. The same reasoning applies one layer down to reed.json.lock
// (internal/state), which is left alone: reed.lock is the outer lock, so a compromise there is
// already reported by the op that noticed it.
func opLockCompromisedError(lockPath string, heldLockFile os.FileInfo) error {
	current, err := os.Stat(lockPath)
	if err == nil && os.SameFile(heldLockFile, current) {
		return nil
	}
	return fmt.Errorf(
		"reed's operation lock %s was removed or replaced while this operation was running, so it did not exclude other reed processes — "+
			"something deleted the .lyx directory (an rm, or a `git clean -xdf` in the worktree) mid-operation. "+
			"Reed state and the tmux session may now disagree; run \"lyx reed status\" to inspect, or \"lyx reed down\" and \"lyx reed resume\" to rebuild",
		lockPath)
}
