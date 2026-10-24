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
// An entry is judged only for a field the write sets or changes, an entry new to the board counting as set,
// so a stored state that is now refused keeps taking unrelated writes.
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
		if !existed || !slices.Equal(before.DependsOn, task.DependsOn) {
			for _, dep := range task.DependsOn {
				if afterBySlug[dep].Kind == KindProject {
					return dependsOnProjectError(task.Slug, dep, after)
				}
			}
		}
		if (!existed || before.Project != task.Project) && task.Project != "" {
			if err := checkProjectField(task, afterBySlug); err != nil {
				return err
			}
		}
		if existed && before.Project != task.Project && pairExists != nil && pairExists(task.Slug) {
			return fmt.Errorf("entry %q has a pair, and a change of its project moves its pair out from under its project's beam: end the pair first with `lyx batten cancel %s --remove`, then change project", task.Slug, task.Slug)
		}
		if (!existed || !slices.Equal(before.Order, task.Order)) && len(task.Order) > 0 {
			if err := checkOrderField(task, afterBySlug); err != nil {
				return err
			}
		}
	}
	return nil
}

// membershipError is the refusal of an entry's project or order field at slug, naming the way forward of correcting that slug or dropping it from the field.
func membershipError(entry, field, slug, reason string) error {
	return fmt.Errorf("entry %q has %s %q, which %s: correct the slug, or drop it from %s", entry, field, slug, reason, field)
}

// checkProjectField refuses task's project when it names an absent entry, a non-project, the entry itself, a done project, a run-held project or a chain of projects with a cycle.
func checkProjectField(task Task, afterBySlug map[string]Task) error {
	project, found := afterBySlug[task.Project]
	switch {
	case task.Project == task.Slug:
		return membershipError(task.Slug, "project", task.Project, "is the entry itself")
	case !found:
		return membershipError(task.Slug, "project", task.Project, "names no entry")
	case project.Kind != KindProject:
		return membershipError(task.Slug, "project", task.Project, "is not a project")
	case isDone(project):
		return membershipError(task.Slug, "project", task.Project, "is done")
	case IsRunStatus(project.Status):
		return membershipError(task.Slug, "project", task.Project, "is held by a run")
	}
	visited := map[string]bool{task.Slug: true}
	for current := task.Project; current != ""; current = afterBySlug[current].Project {
		if visited[current] {
			return membershipError(task.Slug, "project", task.Project, "is in a chain of projects that loops back on itself")
		}
		visited[current] = true
	}
	return nil
}

// checkOrderField refuses a project's order when it names an entry whose project is not this project, or names one entry twice.
func checkOrderField(project Task, afterBySlug map[string]Task) error {
	seen := make(map[string]bool, len(project.Order))
	for _, slug := range project.Order {
		if seen[slug] {
			return membershipError(project.Slug, "order", slug, "is named twice")
		}
		seen[slug] = true
		if afterBySlug[slug].Project != project.Slug {
			return membershipError(project.Slug, "order", slug, "is no entry of this project")
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
