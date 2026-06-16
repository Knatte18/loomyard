// list.go exposes the worktree List operation as a thin wrapper over the shared
// porcelain parser in internal/paths.

package worktree

import (
	"github.com/Knatte18/loomyard/internal/paths"
)

// WorktreeEntry is a type alias for paths.WorktreeEntry.
type WorktreeEntry = paths.WorktreeEntry

// List returns a list of all git worktrees in the repository.
//
// The sourceDir is any worktree in the repository (usually the main checkout).
// Delegates to paths.List for the actual implementation.
func (w *Worktree) List(sourceDir string) ([]WorktreeEntry, error) {
	return paths.List(sourceDir)
}
