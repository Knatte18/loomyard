// dependency.go — when a depends_on dependency is met, and what a dependency on a project waits for.

package boardengine

import "slices"

// Dependencies judges depends_on dependencies over one set of board entries.
// A dependency is met once its entry is done, and a slug the entries do not hold was pruned as done.
// A task is done when its status is done, and so is a beam, whose landing into its parent sets that status.
// A plain project is done when its status is done, or when it has a landing member and every landing member is done.
// A landing member is a task or a project whose project key names the project;
// a note never lands, so it holds no project open.
type Dependencies struct {
	bySlug  map[string]Task
	members map[string][]string
}

// NewDependencies indexes entries for judging the dependencies among them.
func NewDependencies(entries []Task) Dependencies {
	dependencies := Dependencies{bySlug: make(map[string]Task, len(entries)), members: map[string][]string{}}
	for _, entry := range entries {
		dependencies.bySlug[entry.Slug] = entry
		if entry.Project != "" && (entry.Kind == KindTask || entry.Kind == KindProject) {
			dependencies.members[entry.Project] = append(dependencies.members[entry.Project], entry.Slug)
		}
	}
	return dependencies
}

// Pending returns the dependencies of task that are not met, in depends_on order, nil when every one is.
func (d Dependencies) Pending(task Task) []string {
	var pending []string
	for _, dep := range task.DependsOn {
		if !d.met(dep) {
			pending = append(pending, dep)
		}
	}
	return pending
}

// met reports whether a dependency on slug is met.
// A project that is its own landing member, directly or through other projects, is not met.
func (d Dependencies) met(slug string) bool {
	return d.metBelow(slug, map[string]bool{})
}

// metBelow reports whether a dependency on slug is met, given the projects already being judged on the path to it.
// A hand edit can store a membership loop, which reads as not met instead of recursing without end.
func (d Dependencies) metBelow(slug string, judging map[string]bool) bool {
	entry, found := d.bySlug[slug]
	switch {
	case !found || isDone(entry):
		return true
	case entry.Kind != KindProject || entry.Beam || judging[slug]:
		return false
	}
	judging[slug] = true
	defer delete(judging, slug)
	members := d.members[slug]
	return len(members) > 0 && !slices.ContainsFunc(members, func(member string) bool { return !d.metBelow(member, judging) })
}

// waitsOn returns the slugs entry waits on: a project's landing members, and every other entry's depends_on.
func (d Dependencies) waitsOn(entry Task) []string {
	if entry.Kind == KindProject {
		return d.members[entry.Slug]
	}
	return entry.DependsOn
}

// standsFor returns the entries a dependency on slug waits for.
// They are slug itself and, under a project, each landing member not yet done, at any depth.
func (d Dependencies) standsFor(slug string) []string {
	return d.standsForBelow(slug, map[string]bool{})
}

// standsForBelow returns the entries a dependency on slug waits for, given the projects already listed on the path to it.
// A project already on that path adds nothing, so a stored membership loop ends the walk.
func (d Dependencies) standsForBelow(slug string, listing map[string]bool) []string {
	if listing[slug] {
		return nil
	}
	listing[slug] = true
	defer delete(listing, slug)
	entries := []string{slug}
	for _, member := range d.members[slug] {
		if !d.met(member) {
			entries = append(entries, d.standsForBelow(member, listing)...)
		}
	}
	return entries
}

// overlap reports whether dependencies on a and b wait for a common entry.
func (d Dependencies) overlap(a, b string) bool {
	waitedForByB := d.standsFor(b)
	return slices.ContainsFunc(d.standsFor(a), func(slug string) bool { return slices.Contains(waitedForByB, slug) })
}
