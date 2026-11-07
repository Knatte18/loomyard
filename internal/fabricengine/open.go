// open.go implements Open, the constructor other packages use to get a *Fabric handle from a location.

package fabricengine

import "github.com/Knatte18/loomyard/internal/lymcwd"

// Open returns a handle on the fabric repo for l's worktree.
// Other packages should obtain their handle through it.
func Open(l *lymcwd.Location) (*Fabric, error) {
	return newPaired(l.WorktreePath(), RecordsWorktree(l))
}
