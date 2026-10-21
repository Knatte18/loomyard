// export_test.go exposes the hooked walk to the external test package, which cannot import internal/hubforge from inside this package.

package hubreconcile

// SetSpawnBoardSyncForTest replaces the detached board sync the hub-wide reconcile spawns and returns a function that reinstates the previous one.
// The swap is package-global, so a test calling it must not run in parallel.
func SetSpawnBoardSyncForTest(spawn func(boardPath string) error) (restore func()) {
	previous := spawnBoardSync
	spawnBoardSync = spawn
	return func() { spawnBoardSync = previous }
}

// EnsureWithHooks runs Ensure's walk with test hooks: afterEnumerate after each worktree listing, and beforeCommit between a worktree's write and its commit.
// A nil hook is skipped.
func EnsureWithHooks(geom Geometry, opts Options, afterEnumerate func(), beforeCommit func(worktreePath string)) error {
	return ensure(geom, opts, walkHooks{afterEnumerate: afterEnumerate, beforeCommit: beforeCommit})
}
