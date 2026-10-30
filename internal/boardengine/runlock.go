// runlock.go — the run-status form and the lock that keeps a run-held board entry's scope fixed.

package boardengine

import (
	"errors"
	"fmt"
	"maps"
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
	_, ok := RunStatusProducer(status)
	return ok
}

// RunStatusProducer returns the producer half of a status IsRunStatus accepts, and false for any other status.
func RunStatusProducer(status *string) (string, bool) {
	if status == nil {
		return "", false
	}
	state, producer, found := strings.Cut(*status, runStatusSeparator)
	if !found || state == "" || producer == "" {
		return "", false
	}
	return producer, true
}

// holdFreeProducers are the producer rows at which a beam project's run holds nothing of its entry: batten's claim row and the pawl run's first two rows.
// A halt at any of them holds nothing either, since the hold is judged from the producer half of the status alone.
var holdFreeProducers = []string{"Worktree-Create", "Beam-Up", "Beam-Await"}

// RunHolds reports whether the run behind task's status holds the entry: whether the entry's members may not join, fork from or run inside it, and its scope fields take no edit.
// It is true for every entry whose status IsRunStatus accepts, except a beam project whose producer half is one of the hold-free rows.
// The pawl run stands at those rows from Worktree-Create through Beam-Await, and holds its beam from Parent-Align on.
// The readers that ask whether a run is in flight, rather than whether it holds its entry, use IsRunStatus.
// `battenrecipe`'s claim row and the pawl recipe's rows are pinned against the hold-free names by the recipe's own test, since this package imports no recipe package.
func RunHolds(task Task) bool {
	producer, ok := RunStatusProducer(task.Status)
	if !ok {
		return false
	}
	return !(task.Kind == KindProject && task.Beam && slices.Contains(holdFreeProducers, producer))
}

// ErrRunLocked matches, through errors.Is, a write refused because it would change the scope of an entry a run holds.
var ErrRunLocked = errors.New("board entry is held by a run")

// RunLockedError is the refusal of a write that changes a run-held entry's scope.
type RunLockedError struct {
	Slug   string
	Status string
}

// Error names the entry, its status and both ways forward: a note for the finding, and the batten cancel for an abandoned run.
func (e *RunLockedError) Error() string {
	return fmt.Sprintf("entry %q is held by a run (status %q) and takes no scope edits: "+
		"record the new finding as a note of its own with `lym board upsert` and \"kind\":\"note\"; "+
		"to unlock, run `lym batten status %s` from the hub's prime worktree, "+
		"and to abandon the run end it with `lym batten cancel %s` from the prime, "+
		"which ends its processes, removes its records and clears this status",
		e.Slug, e.Status, e.Slug, e.Slug)
}

// Is matches ErrRunLocked.
func (e *RunLockedError) Is(target error) bool {
	return target == ErrRunLocked
}

// BeamRunLockedError is the refusal of a write that removes a beam project whose run holds nothing of it, or changes its project, kind, recipe or beam.
type BeamRunLockedError struct {
	Slug   string
	Status string
}

// Error names the entry, its status and the batten cancel that ends the run first.
func (e *BeamRunLockedError) Error() string {
	return fmt.Sprintf("beam project %q has a run (status %q) and cannot be removed, or have its project, kind, recipe or beam changed, while it stands: "+
		"end the run first with `lym batten cancel %s` from the hub's prime worktree",
		e.Slug, e.Status, e.Slug)
}

// Is matches ErrRunLocked.
func (e *BeamRunLockedError) Is(target error) bool {
	return target == ErrRunLocked
}

// checkRunLocks judges each run-status entry of loaded by its net change to after.
// An entry RunHolds accepts is refused when it is absent from after, or when any field but Status differs; Unknown is never compared, since this binary cannot change a kept key.
// A beam project whose run holds nothing is refused only when it is absent from after, or its project, kind, recipe or beam differs, with a BeamRunLockedError.
// A dependency on a done entry that the same write removes does not count as a change, so prune passes.
// It returns the first refusal in loaded order.
func checkRunLocks(loaded, after []Task) error {
	afterBySlug := make(map[string]Task, len(after))
	for _, task := range after {
		afterBySlug[task.Slug] = task
	}
	removedDone := removedDoneSlugs(loaded, after)

	for _, before := range loaded {
		if !IsRunStatus(before.Status) {
			continue
		}
		now, kept := afterBySlug[before.Slug]
		if RunHolds(before) {
			if !kept || scopeChanged(before, now, removedDone) {
				return &RunLockedError{Slug: before.Slug, Status: *before.Status}
			}
			continue
		}
		if !kept || before.Project != now.Project || before.Kind != now.Kind || before.Recipe != now.Recipe || before.Beam != now.Beam {
			return &BeamRunLockedError{Slug: before.Slug, Status: *before.Status}
		}
	}
	return nil
}

// removedDoneSlugs is the set of done entries of loaded that after no longer holds.
func removedDoneSlugs(loaded, after []Task) map[string]bool {
	kept := make(map[string]bool, len(after))
	for _, task := range after {
		kept[task.Slug] = true
	}
	removedDone := make(map[string]bool)
	for _, task := range loaded {
		if !kept[task.Slug] && isDone(task) {
			removedDone[task.Slug] = true
		}
	}
	return removedDone
}

// scopeChanged reports whether any field but Status, Created and Revised differs between before and now.
// Slices compare element-wise with nil equal to empty;
// before's DependsOn and Order are first stripped of the slugs in removedDone.
// Project, Order and Beam are compared, so a held entry takes no membership or beam change.
// PullRequest is compared too, so a held entry takes no change to its pull-request override.
// RunSettings, Profile and OwnerMachine are never compared: a run settings, profile or owner change is not a scope change, so a run-held entry takes it.
// Unknown is never compared either, since this binary cannot change a kept key.
func scopeChanged(before, now Task, removedDone map[string]bool) bool {
	dependsOn := slices.DeleteFunc(slices.Clone(before.DependsOn), func(slug string) bool {
		return removedDone[slug]
	})
	order := slices.DeleteFunc(slices.Clone(before.Order), func(slug string) bool {
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
		before.Project != now.Project ||
		before.Beam != now.Beam ||
		before.PullRequest != now.PullRequest ||
		!slices.Equal(order, now.Order) ||
		!slices.Equal(before.Labels, now.Labels) ||
		!slices.Equal(before.Issues, now.Issues) ||
		!slices.Equal(dependsOn, now.DependsOn)
}

// cloneSettings returns a deep copy of a run settings map, nil for nil.
func cloneSettings(settings map[string]any) map[string]any {
	if settings == nil {
		return nil
	}
	cloned := make(map[string]any, len(settings))
	for key, value := range settings {
		cloned[key] = cloneSettingsValue(value)
	}
	return cloned
}

// cloneSettingsValue deep-copies the maps and lists a decoded JSON value holds.
func cloneSettingsValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSettings(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = cloneSettingsValue(element)
		}
		return cloned
	default:
		return value
	}
}

// cloneTasks copies tasks with their slice, map and status fields duplicated, so a later mutation of the store cannot alias the copy.
func cloneTasks(tasks []Task) []Task {
	cloned := make([]Task, len(tasks))
	for i, task := range tasks {
		task.Labels = slices.Clone(task.Labels)
		task.Issues = slices.Clone(task.Issues)
		task.DependsOn = slices.Clone(task.DependsOn)
		task.Order = slices.Clone(task.Order)
		task.RunSettings = cloneSettings(task.RunSettings)
		task.Unknown = maps.Clone(task.Unknown)
		if task.Status != nil {
			status := *task.Status
			task.Status = &status
		}
		cloned[i] = task
	}
	return cloned
}
