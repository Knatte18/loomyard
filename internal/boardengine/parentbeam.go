// parentbeam.go resolves the beam an entry belongs to, read from the project links of the board's entries.

package boardengine

import "os"

// ParentBeam returns the slug of the beam the entry slug belongs to, the empty slug meaning the prime.
// It walks the entry's project links upward and returns the nearest open project whose beam flag is set.
// Done projects and projects that are not beams are skipped.
// It returns the empty slug for a slug tasks does not hold, an entry with no project, or a chain with no open beam.
// A chain that loops back on itself also yields the empty slug, which a write cannot store but a hand edit can.
func ParentBeam(tasks []Task, slug string) string {
	bySlug := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		bySlug[task.Slug] = task
	}
	current, held := bySlug[slug]
	if !held {
		return ""
	}
	visited := map[string]bool{slug: true}
	nearest := ""
	for current.Project != "" {
		project, found := bySlug[current.Project]
		if !found {
			break
		}
		if visited[project.Slug] {
			return ""
		}
		visited[project.Slug] = true
		if nearest == "" && project.Kind == KindProject && project.Beam && !isDone(project) {
			nearest = project.Slug
		}
		current = project
	}
	return nearest
}

// ReadTasks returns the entries the local board.json under boardDir holds, for callers that hold no open board.
// It reads under the swap lock, folds a legacy file no further than a board read does, and writes nothing.
// An absent board file is an empty list.
func ReadTasks(boardDir string) ([]Task, error) {
	store := NewStore(boardDir)
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store.ListTasksFull(), nil
}

// ParentBeamOf returns the beam the entry slug belongs to, "" meaning the prime, persisting nothing.
// When a parent function is configured, it gives the answer;
// otherwise the answer comes from the entries the board holds.
func (b *Board) ParentBeamOf(slug string) (string, error) {
	if b.parentOf != nil {
		return b.parentOf(slug)
	}
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return "", nil
	}
	store, err := b.loadStore()
	if err != nil {
		return "", err
	}
	return ParentBeam(store.tasks, slug), nil
}

// BeamUnder returns the beam an entry whose project link is project belongs to, "" meaning the prime, persisting nothing.
// It is project itself when project is an open beam project, and otherwise the beam that project belongs to.
// An empty project, and one the board does not hold, read as the prime.
// It reads project as the board stores it,
// so it answers for a new entry or one whose project link a write is about to change.
func (b *Board) BeamUnder(project string) (string, error) {
	if project == "" {
		return "", nil
	}
	entry, found, err := b.GetTask(project)
	if err != nil || !found {
		return "", err
	}
	if entry.Kind == KindProject && entry.Beam && !isDone(entry) {
		return project, nil
	}
	return b.ParentBeamOf(project)
}
