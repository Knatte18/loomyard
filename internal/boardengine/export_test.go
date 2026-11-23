// export_test.go exposes the run-lock judgment to the external tests, for the changes no board write can reach.
// It also holds the promote entry points that take no recipe, which the tests use to make a task.

package boardengine

// CheckRunLocks judges the net change from loaded to after as a board write does.
var CheckRunLocks = checkRunLocks

// Promote makes the entry identified by idOrSlug a task under the write lock, with no recipe and no depends_on.
// An entry that is already a task is returned unchanged without a write.
func (b *Board) Promote(idOrSlug any) (Task, error) {
	result, err := b.PromoteTo(idOrSlug, "", nil)
	return result.Task, err
}

// Promote makes the entry identified by idOrSlug a task and clears its priority, which only a note carries.
// A task is returned unchanged with changed false, so the caller knows nothing was written.
// A project is refused, since only a task is claimable bar a beam's landing.
// The promoted entry must pass the store's validation, so an entry without a type label is refused.
func (s *Store) Promote(idOrSlug any) (task Task, changed bool, err error) {
	return s.promoteTo(idOrSlug, "", nil, nil)
}

// PromoteWithDeps promotes as Promote does, and a non-nil dependsOn replaces the entry's depends_on in the same write.
func (s *Store) PromoteWithDeps(idOrSlug any, dependsOn []string) (task Task, changed bool, err error) {
	return s.promoteTo(idOrSlug, "", dependsOn, nil)
}
