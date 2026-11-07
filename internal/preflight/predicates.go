// predicates.go implements Wired, HubPresent and ResolveMode, which a CLI's pre-run consults before every command.

package preflight

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// Mode is the mode a cwd resolves to: hub mode or standalone mode.
// The zero Mode value is never a valid mode: it comes back only with a non-nil error,
// and a caller must never treat it as one.
type Mode int

const (
	// ModeHub is the verdict for a cwd whose resolved hub has a board-level lym directory.
	ModeHub Mode = iota + 1
	// ModeStandalone is the verdict for a cwd with no reachable hub geometry:
	// not a git repository, a plain git repo (root or subdirectory), or a hub damaged at the Bolt checkout's _lym.
	ModeStandalone
)

// Wired reports whether fabric is wired for this worktree:
// lymcwd.Resolve(cwd) succeeds, and fabricengine.Ready(l) reports true.
// It is the per-worktree fabric-readiness question a composing orchestrator asks.
// It does not choose between hub mode and standalone mode.
//
// Wired probes the paired sibling of the current worktree, not the hub.
// It is false at the Bolt checkout, in an unpaired sibling, and in a worktree whose pair was removed.
// These are healthy hub situations, so a false answer there does not mean the hub is broken.
//
// It returns (nil, false) on any error rather than surfacing one.
// A CLI pre-run calls it, and it must never block a command.
// It spawns no process beyond the one lymcwd.Resolve performs, since the pre-run runs before every command.
func Wired(cwd string) (*lymcwd.Location, bool) {
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	ready, err := fabricengine.Ready(l)
	if err != nil || !ready {
		return nil, false
	}
	return l, true
}

// HubPresent reports whether the hub this write targets exists:
// lymcwd.Resolve(cwd) succeeds, and a single os.Stat of the hub's board-level lym directory succeeds.
// It is both the stencil seed gate and the mode-selection trigger a standalone-capable CLI's pre-run consults:
// hub mode when a hub exists for this location, standalone mode when one does not.
//
// HubPresent asks a hub-level question, not a per-worktree one.
// A hub-level directory can exist while this worktree has no fabric wiring, and that case is still hub mode.
//
// It returns (nil, false) on any error rather than surfacing one, because a CLI pre-run must never block a command.
// It spawns no process beyond the one lymcwd.Resolve performs.
func HubPresent(cwd string) (*lymcwd.Location, bool) {
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	if !BoardLymPresent(l) {
		return nil, false
	}
	return l, true
}

// BoardLymPresent reports whether l's hub carries a board-level lym directory (the Bolt checkout's _lym).
// It is the single implementation of that check, so callers never drift into separate copies of it.
func BoardLymPresent(l *lymcwd.Location) bool {
	_, err := os.Stat(filepath.Join(fabricengine.BoltDir(l.HubPath), lymdirs.LymDirName))
	return err == nil
}

// ResolveMode is the single mode trigger every standalone-capable CLI's pre-run consults:
// it decides hub mode, standalone mode, or refuse for cwd.
//
// The discriminator is whether lym geometry exists at this location, not the error class alone:
// the outside-anchor error fires from an ordinary subdirectory of a plain git repo,
// and it also fires from a subdirectory of a wired hub worktree.
// This function tells the two apart by re-probing for hub geometry, not by inspecting the error.
//
// A non-nil error means refuse: it is surfaced verbatim and is never degraded to standalone.
// The zero Mode value appears only alongside that non-nil error.
//
// The extra worktree probe runs only on the outside-anchor path.
// The hub path and the not-a-repository path each pay for exactly the one lymcwd.Resolve call they already need.
//
// The case this function cannot separate from a plain repo is a hub damaged at the Bolt checkout's _lym:
// nothing distinguishes it there, so it degrades to standalone.
func ResolveMode(cwd string) (*lymcwd.Location, Mode, error) {
	l, err := lymcwd.Resolve(cwd)
	if err == nil {
		if BoardLymPresent(l) {
			return l, ModeHub, nil
		}
		return nil, ModeStandalone, nil
	}

	if errors.Is(err, lymcwd.ErrNotAGitRepo) {
		return nil, ModeStandalone, nil
	}

	if errors.Is(err, lymcwd.ErrCwdOutsideAnchor) {
		if wl, wErr := lymcwd.ResolveWorktree(cwd); wErr == nil && BoardLymPresent(wl) {
			// A wired hub worktree's subdirectory returns the original gated error, never the second probe's,
			// so the operator sees the message that names both the anchored directory and the marker file.
			return nil, 0, err
		}
		return nil, ModeStandalone, nil
	}

	return nil, 0, err
}
