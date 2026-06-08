// store.go — the in-memory task store over tasks.json.
//
// Load/Save plus all CRUD and validation: dangling-dependency, isolated/deferred
// rules, and cycle detection, with batch and merge applied atomically. Save and
// Load take the fine-grained swap lock so a concurrent read never sees a
// half-written file.

package wiki

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// swapLockSuffix names the fine-grained lock that fences readers of a file
// against the instant a writer swaps it in via rename. It is deliberately
// separate from the coarse tasks.json.lock held across a whole write (which
// includes git network I/O) so reads wait microseconds, not seconds.
const swapLockSuffix = ".swaplock"

// BriefTask is the enriched read-only view returned by the list subcommand.
// Layer and HasProposal are computed at read time and are not stored in tasks.json.
type BriefTask struct {
	ID          int      `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	DependsOn   []string `json:"depends_on"`
	Isolated    bool     `json:"isolated"`
	Deferred    bool     `json:"deferred"`
	Brief       string   `json:"brief"`
	Status      *string  `json:"status,omitempty"`
	Layer       string   `json:"layer"`
	HasProposal bool     `json:"has_proposal"`
}

// Store holds the in-memory task list for one tasks.json file.
type Store struct {
	tasks    []Task
	filePath string
}

// NewStore creates an empty, unloaded Store. Call Load to populate from disk.
func NewStore(filePath string) *Store {
	return &Store{
		tasks:    []Task{},
		filePath: filePath,
	}
}

func (s *Store) Load() error {
	if s.filePath == "" {
		s.tasks = []Task{}
		return nil
	}

	// Hold a shared swap lock only for the read itself: it overlaps with other
	// readers but is fenced against a writer's rename, so we never open
	// tasks.json mid-swap (which on Windows would fail with a sharing violation
	// and otherwise silently look like an empty wiki).
	lock, err := AcquireReadLock(s.filePath + swapLockSuffix)
	if err != nil {
		return fmt.Errorf("acquire read lock: %w", err)
	}
	content, err := os.ReadFile(s.filePath)
	lock.Release()
	if err != nil {
		if os.IsNotExist(err) {
			s.tasks = []Task{}
			return nil
		}
		// A real read error must surface, not masquerade as an empty wiki.
		return fmt.Errorf("read %s: %w", s.filePath, err)
	}

	var tasks []Task
	err = json.Unmarshal(content, &tasks)
	if err != nil {
		// Silent fallback on parse error
		s.tasks = []Task{}
		return nil
	}

	// Normalize DependsOn: set to empty slice if nil
	for i := range tasks {
		if tasks[i].DependsOn == nil {
			tasks[i].DependsOn = []string{}
		}
	}

	s.tasks = tasks
	return nil
}

func (s *Store) Save(wikiPath, relPath string) error {
	content, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}

	// Hold the exclusive swap lock across the write so no reader has tasks.json
	// open during the rename. The body is just a temp-write + rename, so readers
	// are fenced out for microseconds, not for the surrounding git round-trip.
	lock, err := AcquireWriteLock(filepath.Join(wikiPath, relPath) + swapLockSuffix)
	if err != nil {
		return fmt.Errorf("acquire swap lock: %w", err)
	}
	defer lock.Release()

	if err := AtomicWrite(wikiPath, relPath, string(content)); err != nil {
		return fmt.Errorf("atomic write: %w", err)
	}

	return nil
}

func (s *Store) Tasks() []Task {
	result := make([]Task, len(s.tasks))
	copy(result, s.tasks)
	return result
}

func (s *Store) slugIndex() map[string]*Task {
	index := make(map[string]*Task)
	for i := range s.tasks {
		index[s.tasks[i].Slug] = &s.tasks[i]
	}
	return index
}

func (s *Store) nextID() int {
	if len(s.tasks) == 0 {
		return 0
	}
	maxID := s.tasks[0].ID
	for _, t := range s.tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1
}

// validateWrite checks incoming against snapshot for dangling deps, isolated/deferred
// constraints, and cycles. snapshot is the projected state after any pending removals —
// not necessarily equal to s.tasks.
func (s *Store) validateWrite(snapshot []Task, incoming Task) error {
	// Build index of snapshot tasks
	snapshotIndex := make(map[string]*Task)
	for i := range snapshot {
		snapshotIndex[snapshot[i].Slug] = &snapshot[i]
	}

	// (1) Dangling dependency check: every slug in incoming.DependsOn must exist in snapshot
	for _, dep := range incoming.DependsOn {
		if _, exists := snapshotIndex[dep]; !exists {
			return fmt.Errorf("dangling dependency: %q does not exist", dep)
		}
	}

	// (2) Target-not-schedulable check: a dep cannot be isolated or deferred
	for _, dep := range incoming.DependsOn {
		depTask := snapshotIndex[dep]
		if depTask.Isolated {
			return fmt.Errorf("cannot depend on isolated task %q", dep)
		}
		if depTask.Deferred {
			return fmt.Errorf("cannot depend on deferred task %q", dep)
		}
	}

	// (3) Cycle detection via DFS
	// Build adjacency list from snapshot, replacing incoming's own entry
	adjacency := make(map[string][]string)
	for _, t := range snapshot {
		if t.Slug == incoming.Slug {
			adjacency[t.Slug] = incoming.DependsOn
		} else {
			adjacency[t.Slug] = t.DependsOn
		}
	}
	// Ensure incoming.Slug exists in adjacency even if not in snapshot
	if _, exists := adjacency[incoming.Slug]; !exists {
		adjacency[incoming.Slug] = incoming.DependsOn
	}

	// DFS to detect cycle
	color := make(map[string]string) // "white", "gray", "black"
	for slug := range adjacency {
		color[slug] = "white"
	}

	var dfs func(slug string) error
	dfs = func(slug string) error {
		if color[slug] == "black" {
			return nil
		}
		if color[slug] == "gray" {
			return fmt.Errorf("cycle detected: %q -> %q", incoming.Slug, slug)
		}

		color[slug] = "gray"
		for _, dep := range adjacency[slug] {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		color[slug] = "black"
		return nil
	}

	if err := dfs(incoming.Slug); err != nil {
		return err
	}

	// (4) Reverse-isolate check: if incoming.Isolated, no task in snapshot may have incoming.Slug in its DependsOn
	if incoming.Isolated {
		for _, t := range snapshot {
			for _, dep := range t.DependsOn {
				if dep == incoming.Slug {
					return fmt.Errorf("cannot isolate task %q: %q depends on it", incoming.Slug, t.Slug)
				}
			}
		}
	}

	// (5) Reverse-defer check: if incoming.Deferred, no non-deferred task in snapshot may depend on incoming.Slug
	if incoming.Deferred {
		for _, t := range snapshot {
			if t.Deferred {
				continue
			}
			for _, dep := range t.DependsOn {
				if dep == incoming.Slug {
					return fmt.Errorf("cannot defer task %q: non-deferred task %q depends on it", incoming.Slug, t.Slug)
				}
			}
		}
	}

	return nil
}

// UpsertTask creates or updates the task identified by fields["slug"].
func (s *Store) UpsertTask(fields map[string]any) (Task, error) {
	index := s.slugIndex()
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}

	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return Task{}, fmt.Errorf("slug must be a non-empty string")
	}

	var incoming Task
	var err error

	if existing, exists := index[slugStr]; exists {
		incoming, err = ApplyPatch(*existing, fields)
	} else {
		incoming, err = NewTask(fields, s.nextID())
	}

	if err != nil {
		return Task{}, err
	}

	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return Task{}, err
	}

	// Update or append
	if _, exists := index[slugStr]; exists {
		for i := range s.tasks {
			if s.tasks[i].Slug == slugStr {
				s.tasks[i] = incoming
				break
			}
		}
	} else {
		s.tasks = append(s.tasks, incoming)
	}

	return incoming, nil
}

// GetTask looks up a task by integer ID or slug string. Returns (Task, true) if found.
func (s *Store) GetTask(idOrSlug any) (Task, bool) {
	switch v := idOrSlug.(type) {
	case int:
		for _, t := range s.tasks {
			if t.ID == v {
				return t, true
			}
		}
	case float64:
		id := int(v)
		for _, t := range s.tasks {
			if t.ID == id {
				return t, true
			}
		}
	case string:
		for _, t := range s.tasks {
			if t.Slug == v {
				return t, true
			}
		}
	}
	return Task{}, false
}

// RemoveTask deletes the task by ID or slug. Returns an error if not found.
func (s *Store) RemoveTask(idOrSlug any) error {
	var slugToRemove string

	switch v := idOrSlug.(type) {
	case int:
		for _, t := range s.tasks {
			if t.ID == v {
				slugToRemove = t.Slug
				break
			}
		}
	case float64:
		id := int(v)
		for _, t := range s.tasks {
			if t.ID == id {
				slugToRemove = t.Slug
				break
			}
		}
	case string:
		slugToRemove = v
		// Verify it exists
		found := false
		for _, t := range s.tasks {
			if t.Slug == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("task not found: %v", idOrSlug)
		}
	default:
		return fmt.Errorf("task not found: %v", idOrSlug)
	}

	if slugToRemove == "" {
		return fmt.Errorf("task not found: %v", idOrSlug)
	}

	for i, t := range s.tasks {
		if t.Slug == slugToRemove {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("task not found: %v", idOrSlug)
}

// SetPhase sets or clears the status field for the given task. Silent no-op if slug not found.
func (s *Store) SetPhase(idOrSlug any, phase *string) error {
	for i := range s.tasks {
		match := false
		switch v := idOrSlug.(type) {
		case int:
			match = s.tasks[i].ID == v
		case float64:
			match = s.tasks[i].ID == int(v)
		case string:
			match = s.tasks[i].Slug == v
		}

		if match {
			s.tasks[i].Status = phase
			return nil
		}
	}
	// SetPhase is idempotent: no error for missing task
	return nil
}

// SetDeps replaces the depends_on list for slug, running full validation. Returns error if slug not found.
func (s *Store) SetDeps(slug string, dependsOn []string) error {
	var task *Task
	for i := range s.tasks {
		if s.tasks[i].Slug == slug {
			task = &s.tasks[i]
			break
		}
	}

	if task == nil {
		return fmt.Errorf("task not found: %v", slug)
	}

	incoming := *task
	incoming.DependsOn = dependsOn

	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return err
	}

	*task = incoming
	return nil
}

// ListTasksBrief returns all tasks enriched with computed Layer and HasProposal fields.
func (s *Store) ListTasksBrief() []BriefTask {
	layerMap, err := ComputeLayers(s.tasks)
	if err != nil {
		// If layer computation fails, assign empty string
		layerMap = make(map[string]string)
		for _, t := range s.tasks {
			layerMap[t.Slug] = ""
		}
	}

	result := make([]BriefTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		brief := BriefTask{
			ID:          t.ID,
			Slug:        t.Slug,
			Title:       t.Title,
			DependsOn:   t.DependsOn,
			Isolated:    t.Isolated,
			Deferred:    t.Deferred,
			Brief:       t.Brief,
			Status:      t.Status,
			Layer:       layerMap[t.Slug],
			HasProposal: t.Body != "",
		}
		result = append(result, brief)
	}
	return result
}

// ListTasksFull returns a copy of the raw task list with no enrichment.
func (s *Store) ListTasksFull() []Task {
	result := make([]Task, len(s.tasks))
	copy(result, s.tasks)
	return result
}

// UpsertTasksBatch applies multiple upserts atomically — validates all first, then applies all or none.
func (s *Store) UpsertTasksBatch(tasks []map[string]any) error {
	// Project the full post-operation snapshot
	snapshot := s.tasks
	for _, fields := range tasks {
		slugVal, hasSlug := fields["slug"]
		if !hasSlug {
			return fmt.Errorf("slug key is missing in batch")
		}
		slugStr, ok := slugVal.(string)
		if !ok || slugStr == "" {
			return fmt.Errorf("slug must be a non-empty string in batch")
		}

		var incoming Task
		var err error

		// Find if this task exists in current snapshot
		foundIdx := -1
		for i, t := range snapshot {
			if t.Slug == slugStr {
				foundIdx = i
				break
			}
		}

		if foundIdx >= 0 {
			incoming, err = ApplyPatch(snapshot[foundIdx], fields)
			if err != nil {
				return err
			}
			snapshot[foundIdx] = incoming
		} else {
			// Generate next ID based on current snapshot, mirroring store.nextID()
			var nextID int
			if len(snapshot) == 0 {
				nextID = 0
			} else {
				maxID := snapshot[0].ID
				for _, t := range snapshot {
					if t.ID > maxID {
						maxID = t.ID
					}
				}
				nextID = maxID + 1
			}

			incoming, err = NewTask(fields, nextID)
			if err != nil {
				return err
			}
			snapshot = append(snapshot, incoming)
		}
	}

	// Validate all incoming tasks against projected snapshot
	for _, fields := range tasks {
		slugStr := fields["slug"].(string)
		var incoming Task
		for _, t := range snapshot {
			if t.Slug == slugStr {
				incoming = t
				break
			}
		}

		if err := s.validateWrite(snapshot, incoming); err != nil {
			return err
		}
	}

	// Execute all upserts
	for _, fields := range tasks {
		_, err := s.UpsertTask(fields)
		if err != nil {
			return err
		}
	}

	return nil
}

// MergeTasks removes slugs, upserts one task, and optionally sets a phase — all atomically.
// setPhase is [id_or_slug, phase_string_or_nil], or nil to skip the phase update.
func (s *Store) MergeTasks(removeSlugs []string, upsert map[string]any, setPhase *[2]any) (Task, error) {
	// Project snapshot: snapshot minus removeSlugs
	projected := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		shouldRemove := false
		for _, slug := range removeSlugs {
			if t.Slug == slug {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			projected = append(projected, t)
		}
	}

	// Validate the upserted task against projected snapshot
	slugVal, hasSlug := upsert["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing in merge upsert")
	}
	slugStr := slugVal.(string)

	var incoming Task
	var err error

	foundIdx := -1
	for i, t := range projected {
		if t.Slug == slugStr {
			foundIdx = i
			break
		}
	}

	if foundIdx >= 0 {
		incoming, err = ApplyPatch(projected[foundIdx], upsert)
	} else {
		var nextID int
		if len(projected) == 0 {
			nextID = 0
		} else {
			maxID := projected[0].ID
			for _, t := range projected {
				if t.ID > maxID {
					maxID = t.ID
				}
			}
			nextID = maxID + 1
		}
		incoming, err = NewTask(upsert, nextID)
	}

	if err != nil {
		return Task{}, err
	}

	if err := s.validateWrite(projected, incoming); err != nil {
		return Task{}, err
	}

	// Execute: remove all removeSlugs
	for _, slug := range removeSlugs {
		s.RemoveTask(slug)
	}

	// Execute: upsert
	upserted, err := s.UpsertTask(upsert)
	if err != nil {
		return Task{}, err
	}

	// Execute: set phase if provided
	if setPhase != nil && len(setPhase) >= 2 {
		idOrSlug := setPhase[0]
		var phase *string
		if setPhase[1] != nil {
			phaseStr := setPhase[1].(string)
			phase = &phaseStr
		}
		s.SetPhase(idOrSlug, phase)
	}

	return upserted, nil
}
