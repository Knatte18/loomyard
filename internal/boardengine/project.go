// project.go — the write rules that keep a project's membership and its dependents consistent.
//
// checkProjectRules judges the entries a write leaves against the entries as loaded, inside the write critical section.

package boardengine

import (
	"fmt"
	"slices"
	"strings"
)

// checkProjectRules refuses a write that leaves a project in a state its rules forbid.
// loaded is the entries before the write, under any rename the write made, and after the entries the write leaves.
// pairExists reports whether an entry's pair exists on this machine; nil reads every pair as absent.
// An entry is judged only for a field the write sets or changes, so a stored state that is now refused keeps taking unrelated writes.
func checkProjectRules(loaded, after []Task, pairExists func(slug string) bool) error {
	loadedBySlug := make(map[string]Task, len(loaded))
	for _, task := range loaded {
		loadedBySlug[task.Slug] = task
	}
	afterBySlug := make(map[string]Task, len(after))
	for _, task := range after {
		afterBySlug[task.Slug] = task
	}

	for _, task := range after {
		before, existed := loadedBySlug[task.Slug]
		if existed && slices.Equal(before.DependsOn, task.DependsOn) {
			continue
		}
		for _, dep := range task.DependsOn {
			if afterBySlug[dep].Kind != KindProject {
				continue
			}
			return dependsOnProjectError(task.Slug, dep, after)
		}
	}
	return nil
}

// dependsOnProjectError refuses entry's dependency on project, naming the project's task members as what to depend on and set-deps as the way to drop it.
func dependsOnProjectError(entry, project string, entries []Task) error {
	var members []string
	for _, task := range entries {
		if task.Project == project && task.Kind == KindTask {
			members = append(members, task.Slug)
		}
	}
	if len(members) == 0 {
		return fmt.Errorf("entry %q depends on project %q, and nothing depends on a project, which has no task member yet: drop the dependency with `lyx board set-deps`", entry, project)
	}
	return fmt.Errorf("entry %q depends on project %q, and nothing depends on a project: depend on its members (%s) instead, or drop the dependency with `lyx board set-deps`", entry, project, strings.Join(members, ", "))
}
