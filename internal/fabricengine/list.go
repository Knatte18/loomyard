// list.go exposes the worktree List operation as a Topology method over the package-local porcelain parser in worktreelist.go.

package fabricengine

// List returns a list of all git worktrees in the repository.
//
// The sourceDir is any worktree in the repository (usually the main checkout).
func (t *Topology) List(sourceDir string) ([]WorktreeEntry, error) {
	return List(sourceDir)
}
