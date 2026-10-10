// runlock.go — the run-status form and the lock that keeps a run-held board entry's scope fixed.

package boardengine

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// runStatusSeparator joins the shed state and the producer in a run status.
const runStatusSeparator = " · "

// RunStatus composes the status of a board entry held by a run: the shed state and the producer it stands at.
func RunStatus(state, producer string) string {
	return state + runStatusSeparator + producer
}

// IsRunStatus reports whether status has the RunStatus form: non-nil, and two non-empty halves around the separator.
// Any other status, including nil, empty, `done` and a hand-set word, is not a run status.
func IsRunStatus(status *string) bool {
	if status == nil {
		return false
	}
	state, producer, found := strings.Cut(*status, runStatusSeparator)
	return found && state != "" && producer != ""
}

// ErrRunLocked matches, through errors.Is, a write refused because it would change the scope of an entry a run holds.
var ErrRunLocked = errors.New("board entry is held by a run")

// RunLockedError is the refusal of a write that changes a run-held entry's scope.
type RunLockedError struct {
	Slug   string
	Status string
}

// Error names the entry, its status and both ways forward.
func (e *RunLockedError) Error() string {
	return fmt.Sprintf("entry %q is held by a run (status %q) and takes no scope edits: "+
		"record the new finding as a note of its own with `lyx board upsert` and \"kind\":\"note\"; "+
		"to unlock, run `lyx batten status %s` from the hub's prime worktree, and clear the status with `lyx board set-status` "+
		"only when no run holds the entry, which is when that check refuses because the slug has no batten seed, reports state `done`, "+
		"or reports a halted state (`paused`, `blocked`, `failed`, `awaiting`) whose run you have decided to abandon; "+
		"a `running` state means a run holds it",
		e.Slug, e.Status, e.Slug)
}

// Is matches ErrRunLocked.
func (e *RunLockedError) Is(target error) bool {
	return target == ErrRunLocked
}

// checkRunLocks judges each run-held entry of loaded by its net change to after.
// It refuses when the entry is absent from after, or when any field but Status differs.
// A dependency on a done entry that the same write removes does not count as a change, so prune passes.
// It returns the first refusal in loaded order.
func checkRunLocks(loaded, after []Task) error {
	afterBySlug := make(map[string]Task, len(after))
	for _, task := range after {
		afterBySlug[task.Slug] = task
	}

	removedDone := make(map[string]bool)
	for _, task := range loaded {
		if _, kept := afterBySlug[task.Slug]; !kept && isDone(task) {
			removedDone[task.Slug] = true
		}
	}

	for _, before := range loaded {
		if !IsRunStatus(before.Status) {
			continue
		}
		now, kept := afterBySlug[before.Slug]
		if !kept || scopeChanged(before, now, removedDone) {
			return &RunLockedError{Slug: before.Slug, Status: *before.Status}
		}
	}
	return nil
}

// scopeChanged reports whether any field but Status differs between before and now.
// Slices compare element-wise with nil equal to empty;
// before's DependsOn is first stripped of the slugs in removedDone.
func scopeChanged(before, now Task, removedDone map[string]bool) bool {
	dependsOn := slices.DeleteFunc(slices.Clone(before.DependsOn), func(slug string) bool {
		return removedDone[slug]
	})
	return before.Title != now.Title ||
		before.Kind != now.Kind ||
		before.Recipe != now.Recipe ||
		before.Priority != now.Priority ||
		before.Isolated != now.Isolated ||
		before.Brief != now.Brief ||
		before.Body != now.Body ||
		before.ShortName != now.ShortName ||
		!slices.Equal(before.Labels, now.Labels) ||
		!slices.Equal(before.Issues, now.Issues) ||
		!slices.Equal(dependsOn, now.DependsOn)
}

// cloneTasks copies tasks with their slice fields and status duplicated, so a later mutation of the store cannot alias the copy.
func cloneTasks(tasks []Task) []Task {
	cloned := make([]Task, len(tasks))
	for i, task := range tasks {
		task.Labels = slices.Clone(task.Labels)
		task.Issues = slices.Clone(task.Issues)
		task.DependsOn = slices.Clone(task.DependsOn)
		if task.Status != nil {
			status := *task.Status
			task.Status = &status
		}
		cloned[i] = task
	}
	return cloned
}
