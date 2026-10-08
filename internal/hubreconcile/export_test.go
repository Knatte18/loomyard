// export_test.go exposes the hooked walk to the external test package, which cannot import internal/hubforge from inside this package.

package hubreconcile

// EnsureWithHooks runs Ensure's walk with test hooks: afterEnumerate after each worktree listing, and beforeCommit between a worktree's write and its commit.
// A nil hook is skipped.
func EnsureWithHooks(geom Geometry, opts Options, afterEnumerate func(), beforeCommit func(worktreePath string)) error {
	return ensure(geom, opts, walkHooks{afterEnumerate: afterEnumerate, beforeCommit: beforeCommit})
}
