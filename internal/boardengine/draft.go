// draft.go — the draft status every new task takes, and the rule that gives it at the write critical section.

package boardengine

// StatusDraft is the status of a task that is not released: every new task takes it, and a draft entry holds no run lock.
const StatusDraft = "draft"

// IsEmptyStatus reports whether status is nil or the empty string, the one meaning of an empty status in the start condition, the migration and the release paths.
func IsEmptyStatus(status *string) bool {
	return status == nil || *status == ""
}

// isDraft reports whether t carries the draft status, so it is not released.
func isDraft(t Task) bool {
	return t.Status != nil && *t.Status == StatusDraft
}

// migrateDraftStatus marks every open task of the store whose status is empty draft, once per board: only while the store's draft_status key is absent.
// It sets the key and reports whether it marked any task; the write it rides on saves both together.
func migrateDraftStatus(store *Store) bool {
	if store.draftStatus {
		return false
	}
	store.draftStatus = true
	marked := false
	for i := range store.tasks {
		task := &store.tasks[i]
		if task.Kind == KindTask && !isDone(*task) && IsEmptyStatus(task.Status) {
			draft := StatusDraft
			task.Status = &draft
			marked = true
		}
	}
	return marked
}

// applyDraftRule sets StatusDraft on each entry of kind task in after whose status is empty.
// An entry that was a task in loaded keeps an empty status when the write left it untouched, the same empty value as loaded, or set it to nil, which is the release.
// So a new task, a note or project made a task, and a task whose status the write emptied from a non-empty one all read draft.
func applyDraftRule(loaded, after []Task) {
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	for i := range after {
		if after[i].Kind != KindTask || !IsEmptyStatus(after[i].Status) {
			continue
		}
		if before, wasTask := loadedBySlug[after[i].Slug]; wasTask && before.Kind == KindTask {
			untouched := (before.Status == nil && after[i].Status == nil) ||
				(before.Status != nil && after[i].Status != nil && *before.Status == *after[i].Status)
			if untouched || after[i].Status == nil {
				continue
			}
		}
		draft := StatusDraft
		after[i].Status = &draft
	}
}
