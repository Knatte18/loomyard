// predicates.go implements Wired and HubPresent, the two cheap boolean predicates a
// standalone-capable CLI's pre-run consults before every command to decide between hub mode and
// standalone mode, and to gate the stencil seed.
// See doc.go for why the two are not interchangeable.

package preflight

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// Mode discriminates the three-way verdict ResolveMode returns: hub mode, standalone mode, or
// refuse.
// The zero Mode value is never a valid mode — it appears only alongside a non-nil error, the
// refuse case, and a caller must never treat it as a fourth mode.
type Mode int

const (
	// ModeHub is the verdict for a cwd whose resolved hub has a board-level lyx directory.
	ModeHub Mode = iota + 1
	// ModeStandalone is the verdict for a cwd with no reachable hub geometry: not a git
	// repository, a plain git repo (root or subdirectory), or a hub damaged precisely at
	// <hub>/_board/_lyx.
	ModeStandalone
)

// Wired reports whether fabric is wired for this worktree: lyxcwd.Resolve(cwd) succeeds and
// fabricengine.Ready(l) reports true.
// This is the per-worktree fabric-readiness question a composing orchestrator asks; it is
// explicitly not the mode trigger a standalone-capable CLI's pre-run consults to choose hub mode
// over standalone mode — see HubPresent for that.
//
// Wired probes the paired sibling of the current worktree, not the hub, so it is false at
// <hub>/_board, false in an unpaired sibling, and false in a worktree whose pair was removed —
// all three of which are real, healthy hub situations that HubPresent still answers true for.
//
// It returns (nil, false) on any error rather than surfacing one, because it is consumed by a CLI
// pre-run that must never block a command; it never spawns a process beyond the one
// lyxcwd.Resolve already performs — no fabricengine.Clean, no fabricengine.Healthy, no
// fabricengine.PrimeName, since both predicates in this file run before every single lyx command.
func Wired(cwd string) (*lyxcwd.Location, bool) {
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	ready, err := fabricengine.Ready(l)
	if err != nil || !ready {
		return nil, false
	}
	return l, true
}

// HubPresent reports whether the hub this write targets actually exists: lyxcwd.Resolve(cwd)
// succeeds and a single os.Stat of the hub's board-level lyx directory succeeds.
// This is both the stencil seed gate and the mode-selection trigger a standalone-capable CLI's
// pre-run consults: hub mode when a hub exists for this location, standalone mode when one does
// not.
//
// HubPresent asks a hub-level question, not a per-worktree one, so a hub-level directory can exist
// while this particular worktree is not Wired — that resolved-but-not-wired case still answers hub
// mode under HubPresent, which is why HubPresent is not merely a weaker Wired: see doc.go for why
// this is the correct trigger and Wired is not.
//
// It returns (nil, false) on any error rather than surfacing one, for the same never-block-a-command
// reason Wired documents, and it never spawns a process beyond the one lyxcwd.Resolve already
// performs.
func HubPresent(cwd string) (*lyxcwd.Location, bool) {
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	if !boardLyxPresent(l) {
		return nil, false
	}
	return l, true
}

// boardLyxPresent reports whether l's hub carries a board-level lyx directory
// (<hub>/_board/_lyx). It is the single implementation of that stat, shared by HubPresent and
// ResolveMode, so the two never drift into two copies of the same check.
func boardLyxPresent(l *lyxcwd.Location) bool {
	_, err := os.Stat(filepath.Join(fabricengine.BoardDir(l.HubPath), lyxdirs.LyxDirName))
	return err == nil
}

// ResolveMode is the single mode trigger every standalone-capable CLI's pre-run consults: it
// decides hub mode, standalone mode, or refuse for cwd.
//
// The discriminator is whether lyx geometry exists at this location, never the error class alone
// — lyxcwd.Resolve's ErrCwdOutsideAnchor fires both from an ordinary subdirectory of a plain git
// repo and from a subdirectory of a wired hub worktree, and this function tells the two apart by
// re-probing for hub geometry rather than by inspecting the error.
//
// A non-nil error means refuse: it is surfaced verbatim and is never degraded to standalone. The
// zero Mode value appears only alongside that non-nil error.
//
// The extra git rev-parse this function pays for is charged only on the ErrCwdOutsideAnchor path
// — the hub path and the not-a-repository path each pay for exactly the one lyxcwd.Resolve call
// they already needed.
//
// The one residual class this function cannot separate from a plain repo is a hub damaged
// precisely at <hub>/_board/_lyx: at that point nothing distinguishes it from a plain repo, so it
// degrades to standalone.
func ResolveMode(cwd string) (*lyxcwd.Location, Mode, error) {
	l, err := lyxcwd.Resolve(cwd)
	if err == nil {
		if boardLyxPresent(l) {
			return l, ModeHub, nil
		}
		return nil, ModeStandalone, nil
	}

	if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		return nil, ModeStandalone, nil
	}

	if errors.Is(err, lyxcwd.ErrCwdOutsideAnchor) {
		if wl, wErr := lyxcwd.ResolveWorktree(cwd); wErr == nil && boardLyxPresent(wl) {
			// A wired hub worktree's subdirectory: return the original gated error,
			// never the second probe's, so the operator sees the message naming both
			// the anchored directory and the marker file.
			return nil, 0, err
		}
		return nil, ModeStandalone, nil
	}

	return nil, 0, err
}
