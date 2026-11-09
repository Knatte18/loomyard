// projectrelease.go — the release of a project together with its draft member tasks, in one write.

package boardengine

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SetStatusReleasingProject sets or clears the status of the entry idOrSlug names under the write lock.
// A null status on a project whose status is empty also releases every direct member task in draft.
// The members are released in the same write, so one release lets the project's whole family start.
// Each member is validated as its own release would be.
// One failing member refuses the whole write.
// A null status on a project whose status is a run status or done is refused.
// The refusal names the release of each draft member as the way forward.
// It returns the slugs of the released members in store order, empty for any other write.
func (b *Board) SetStatusReleasingProject(idOrSlug any, status *string) ([]string, error) {
	var released []string
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		var err error
		released, err = store.setStatusReleasingProject(idOrSlug, status)
		return nil, err
	}, nil)
	return released, err
}

// setStatusReleasingProject applies the write SetStatusReleasingProject describes to the store, in memory.
// On an error the store may hold part of the release, so the caller must not save it.
func (s *Store) setStatusReleasingProject(selector any, status *string) ([]string, error) {
	current, found := s.GetTask(selector)
	if !found {
		return nil, fmt.Errorf("task not found: %v", selector)
	}
	if status != nil || current.Kind != KindProject {
		return nil, s.SetStatus(selector, status)
	}
	members := draftMembers(s.tasks, current.Slug)
	if IsRunStatus(current.Status) || isDone(current) {
		return nil, projectReleaseRefusal(current, members)
	}
	if !IsEmptyStatus(current.Status) {
		return nil, s.SetStatus(selector, nil)
	}
	if err := s.SetStatus(current.Slug, nil); err != nil {
		return nil, err
	}
	for _, slug := range members {
		if err := s.SetStatus(slug, nil); err != nil {
			return nil, fmt.Errorf("project %q is not released, nor any of its draft member tasks, since member %q cannot be released: %w", current.Slug, slug, err)
		}
	}
	return members, nil
}

// draftMembers returns, in store order, the slugs of the tasks in draft whose project is project.
// A nested project's members are not among them.
func draftMembers(tasks []Task, project string) []string {
	var members []string
	for _, t := range tasks {
		if t.Project == project && t.Kind == KindTask && isDraft(t) {
			members = append(members, t.Slug)
		}
	}
	return members
}

// projectReleaseRefusal is the refusal of a null status on a project whose status is a run status or done.
// It names the release of each draft member by its slug, or says there is none to release.
func projectReleaseRefusal(project Task, members []string) error {
	refusal := fmt.Sprintf("project %q has status %q, and only a project with no status is released with its draft member tasks", project.Slug, *project.Status)
	if len(members) == 0 {
		return fmt.Errorf("%s; it has no draft member task to release, so nothing was written", refusal)
	}
	commands := make([]string, len(members))
	for i, slug := range members {
		commands[i] = releaseCommand(slug)
	}
	return fmt.Errorf("%s: release each draft member on its own with %s", refusal, strings.Join(commands, "; "))
}

// releaseCommand returns the `lym board set-status` command line that releases the entry slug names.
// The JSON payload is single-quoted for a shell.
func releaseCommand(slug string) string {
	payload, _ := json.Marshal(map[string]any{"slug": slug, "status": nil})
	return "lym board set-status '" + string(payload) + "'"
}
