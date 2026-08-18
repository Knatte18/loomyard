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
// that no later reed verb can address (see validateToldTmuxIdentity in server.go).
func (e *Engine) withOpLock(fn func() error) error {
	if err := validateToldTmuxIdentity(e.geom); err != nil {
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
	return fn()
}
