// predicates.go implements Wired and HubPresent, the two cheap boolean predicates a
// standalone-capable CLI's pre-run consults before every command to decide between hub mode and
// standalone mode, and to gate the stencil seed.
// See doc.go for why the two are not interchangeable.

package preflight

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// Wired reports whether fabric is wired for this worktree: lyxcwd.Resolve(cwd) succeeds and
// fabricengine.Ready(l) reports true.
// This is the hub-mode trigger a standalone-capable CLI consults to choose hub mode over
// standalone mode.
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
// This is the stencil seed gate.
//
// HubPresent asks a hub-level question, not a per-worktree one, so a hub-level directory can exist
// while this particular worktree is not Wired — that resolved-but-not-wired case is exactly the
// one a standalone-capable CLI must still answer with standalone mode, which is why HubPresent is
// not merely a weaker Wired.
//
// It returns (nil, false) on any error rather than surfacing one, for the same never-block-a-command
// reason Wired documents, and it never spawns a process beyond the one lyxcwd.Resolve already
// performs.
func HubPresent(cwd string) (*lyxcwd.Location, bool) {
	l, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, false
	}
	if _, err := os.Stat(filepath.Join(fabricengine.BoardDir(l.HubPath), lyxdirs.LyxDirName)); err != nil {
		return nil, false
	}
	return l, true
}
