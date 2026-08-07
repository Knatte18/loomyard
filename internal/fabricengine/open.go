// open.go implements Open, the location-based constructor every caller outside this package uses to
// obtain a *Fabric handle — the location-only entry point that lets the one-repo illusion hold at
// the public API boundary.

package fabricengine

import "github.com/Knatte18/loomyard/internal/lyxcwd"

// Open returns a handle on the fabric repo for this worktree.
// It is the only constructor any other package should call — see newPaired for the underlying stat
// validation Open relies on.
func Open(l *lyxcwd.Location) (*Fabric, error) {
	return newPaired(l.WorktreePath(), WeftWorktree(l))
}
