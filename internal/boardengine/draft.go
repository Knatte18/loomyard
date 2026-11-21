// draft.go — the draft status, and the rule that gives it to new tasks inside the write critical section.

package boardengine

import "slices"

// StatusDraft is the status of a task that is not released.
// Every new task takes it, bar a task a write releases.
// A draft entry holds no run lock.
const StatusDraft = "draft"

// IsEmptyStatus reports whether status is nil or the empty string, the two forms of an empty status.
func IsEmptyStatus(status *string) bool {
	return status == nil || *status == ""
}

// isDraft reports whether t carries the draft status.
func isDraft(t Task) bool {
	return t.Status != nil && *t.Status == StatusDraft
}

// applyDraftRule sets StatusDraft on each entry of kind task in after whose status is empty.
// An entry that was a task in loaded keeps an empty status if the write leaves it unchanged.
// So a new task, a note or project made a task, and a task the write emptied from a non-empty status all read draft.
// A nil status is the release, which the rule never drafts.
// released lists the slugs of the tasks the write leaves with an empty status, which the rule skips:
// a released task is startable in the write that makes it.
func applyDraftRule(loaded, after []Task, released []string) {
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	for i := range after {
		if after[i].Kind != KindTask || !IsEmptyStatus(after[i].Status) || slices.Contains(released, after[i].Slug) {
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
