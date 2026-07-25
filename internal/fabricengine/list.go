// list.go exposes the worktree List operation as a thin wrapper over the
// shared porcelain parser in internal/hubgeometry. Adapted from warpengine's
// list.go — identical delegation, package fabricengine.

package fabricengine

import (
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// WorktreeEntry is a type alias for hubgeometry.WorktreeEntry.
type WorktreeEntry = hubgeometry.WorktreeEntry

// List returns a list of all git worktrees in the repository.
//
// The sourceDir is any worktree in the repository (usually the main checkout).
// Delegates to hubgeometry.List for the actual implementation.
func (t *Topology) List(sourceDir string) ([]WorktreeEntry, error) {
	return hubgeometry.List(sourceDir)
}
