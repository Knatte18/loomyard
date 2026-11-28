// export_test.go exposes the hooked walk to the external test package.
// Code in this package cannot import internal/hubforge.

package hubreconcile

// SetSpawnBoardSyncForTest replaces the detached board sync that the hub-wide reconcile spawns.
// It returns a function that reinstates the previous sync.
// The swap is package-global, so a test calling it must not run in parallel.
func SetSpawnBoardSyncForTest(spawn func(boardPath string) error) (restore func()) {
	previous := spawnBoardSync
	spawnBoardSync = spawn
	return func() { spawnBoardSync = previous }
}

// EnsureWithHooks runs the reconcile walk with test hooks.
// afterEnumerate runs after each worktree listing, and beforeCommit runs between a worktree's write and its commit.
// A nil hook is skipped.
func EnsureWithHooks(geom Geometry, opts Options, afterEnumerate func(), beforeCommit func(worktreePath string)) error {
	return ensure(geom, opts, walkHooks{afterEnumerate: afterEnumerate, beforeCommit: beforeCommit})
}

// EnsureFailingAfterHubWideWrite runs the reconcile walk and fails its board write with failure.
// The failure comes once the hub-wide files and the board marker are written.
func EnsureFailingAfterHubWideWrite(geom Geometry, opts Options, failure error) error {
	return ensure(geom, opts, walkHooks{afterHubWideWrite: func() error { return failure }})
}
