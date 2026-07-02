// lock.go defines the Engine type — the domain kernel's public handle,
// holding a resolved Config, the worktree's hubgeometry.Layout, and the
// PsmuxCmd bound to this hub's socket — plus the single mux-operation lock
// every public engine op acquires exactly once at its outer boundary. Every
// other file in this package (reconcile.go, apply.go, spawn.go, strand.go,
// lifecycle.go) hangs its exported/*Locked methods off *Engine.

package muxengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
)

// muxLockFileName is the mux operation lock's file name inside a Layout's
// ephemeral .lyx directory. It is deliberately distinct from mux.json's own
// lock file (mux.json.lock, owned by internal/state): mux.lock guards the
// whole engine-op cycle (read -> mutate -> persist -> render -> apply),
// while mux.json.lock guards only the JSON file swap inside that cycle.
// Lock ordering is strictly outer(mux.lock) -> inner(mux.json.lock); an
// engine op only ever reaches mux.json.lock indirectly, through
// LoadState/SaveState, while it is already holding mux.lock.
const muxLockFileName = "mux.lock"

// Engine is the domain kernel's public handle: a resolved Config, the
// worktree's hubgeometry.Layout (the Hub Geometry Invariant's single owner
// of cwd/geometry), and the PsmuxCmd bound to this hub's socket. Every
// exported method returns a plain result struct and error — no cobra, no
// io.Writer, no exit codes (the engine-purity litmus muxcli, batch 6,
// depends on). The zero Engine is not valid; always build one via New.
type Engine struct {
	cfg    Config
	layout *hubgeometry.Layout
	psmux  PsmuxCmd
}

// New builds an Engine for the given resolved Config and Layout, deriving
// the PsmuxCmd from cfg.Psmux and this hub's socket name (server.go's
// socketName). Every psmux command an Engine method issues therefore
// targets the one named server this hub shares across its worktrees.
func New(cfg Config, layout *hubgeometry.Layout) *Engine {
	return &Engine{
		cfg:    cfg,
		layout: layout,
		psmux:  NewPsmuxCmd(cfg.Psmux, socketName(layout.Hub)),
	}
}

// Socket returns this engine's psmux -L socket name, so muxcli never needs
// the unexported socketName helper or the raw Layout to report it.
func (e *Engine) Socket() string {
	return socketName(e.layout.Hub)
}

// SessionName returns this engine's psmux session name (this worktree's
// directory slug), so muxcli never needs the raw Layout to report it.
func (e *Engine) SessionName() string {
	return SessionName(e.layout.WorktreeRoot)
}

// withOpLock acquires the mux operation lock at
// <worktree>/.lyx/mux.lock exactly once, runs fn while holding it, and
// releases it (via defer) before returning fn's error. This is the ONLY
// acquisition point for mux.lock in the whole package: every public op
// (AddStrand, UpdateStrand, RemoveStrand, Up, Resume, Down, Status) wraps
// its body in withOpLock and calls unexported *Locked helpers that assume
// the lock is already held. Public ops must never call each other, or
// call withOpLock a second time, while already holding the lock —
// gofrs/flock is non-reentrant across handles (even in-process, on
// Windows: a second Lock() call from a different *Flock/file handle blocks
// forever waiting on the first), so a nested acquisition would
// self-deadlock. CLI verbs (muxcli, batch 6) never call withOpLock or
// internal/lock directly; only Engine methods do.
func (e *Engine) withOpLock(fn func() error) error {
	dotLyx := e.layout.DotLyxDir()
	// gofrs/flock opens the lock file with O_CREATE but never creates
	// missing parent directories, so a brand-new worktree's first mux
	// operation (before .lyx exists at all) must create it here first,
	// matching internal/state's own MkdirAll-before-lock pattern.
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dotLyx, err)
	}

	lockPath := filepath.Join(dotLyx, muxLockFileName)
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire mux op lock: %w", err)
	}
	defer l.Release()
	return fn()
}
