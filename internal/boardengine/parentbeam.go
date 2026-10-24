// parentbeam.go — the board's half of the one parent resolver: the beam an entry belongs to, read from the project links of the entries.

package boardengine

import "os"

// ParentBeam returns the slug of the beam the entry slug belongs to, the empty slug meaning the prime.
// It walks the entry's project links upward and returns the nearest open project whose beam flag is set.
// Done projects and projects that are not beams are skipped.
// It returns the empty slug for a slug tasks does not hold, an entry with no project, a chain with no open beam,
// and a chain that loops back on itself, which a write cannot store but a hand edit can.
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

// ReadTasks returns the entries the local board.json under boardDir holds, for a reader outside a Board.
// It reads under the swap lock and writes nothing; the load folds no more of a legacy file than Board's own reads do.
// An absent board file is an empty list.
func ReadTasks(boardDir string) ([]Task, error) {
	store := NewStore(boardDir)
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store.ListTasksFull(), nil
}

// ParentBeamOf returns the beam the entry slug belongs to, "" meaning the prime, persisting nothing.
// It answers what Config.ParentOf answers when that seam is set, and ParentBeam over the board's own entries otherwise.
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
