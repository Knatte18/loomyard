// list.go exposes the worktree listing as the Topology method List.
// The listing is parsed by the porcelain parser in worktreelist.go.

package fabricengine

// List returns every git worktree in the repository.
//
// sourceDir is any worktree of the repository, usually the main checkout.
func (t *Topology) List(sourceDir string) ([]WorktreeEntry, error) {
	return List(sourceDir)
}
