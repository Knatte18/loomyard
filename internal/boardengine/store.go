// store.go — the in-memory entry store over a board directory's board.json.
//
// Load/Save plus all CRUD and validation: dangling-dependency, isolated and tier rules, and cycle
// detection, with batch and merge applied atomically.
// Load migrates the legacy tasks.json and notes.json in memory when board.json is absent and folds a
// pre-upgrade binary's done marks; Save writes board.json only.
// Save and Load take the fine-grained swap lock so a concurrent read never sees a half-written
// file.

package boardengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/state"
)

// swapLockSuffix is the fine-grained lock that fences readers during writer
// swap, separate from board.lock to minimize read latency.
const swapLockSuffix = ".swaplock"

// BriefTask is the enriched read-only view returned by list, with Layer and HasProposal computed at
// read time.
type BriefTask struct {
	ID          int      `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	DependsOn   []string `json:"depends_on"`
	Tier        int      `json:"tier"`
	Type        string   `json:"type"`
	Isolated    bool     `json:"isolated"`
	Brief       string   `json:"brief"`
	Status      *string  `json:"status,omitempty"`
	Layer       string   `json:"layer"`
	HasProposal bool     `json:"has_proposal"`
}

const (
	// boardFile names the one store file a board directory holds.
	boardFile = "board.json"

	// storeVersion is the only on-disk version Load accepts.
	storeVersion = 1
)

// storeFile is the on-disk shape of board.json.
// LegacyDone lists the slugs whose done mark came from a legacy file, so the fold never re-applies one.
type storeFile struct {
	Version    int      `json:"version"`
	Entries    []Task   `json:"entries"`
	LegacyDone []string `json:"legacy_done,omitempty"`
}

// Store holds the in-memory entry list for one board directory's board.json.
type Store struct {
	tasks      []Task
	legacyDone []string
	boardDir   string
}

// NewStore creates an empty, unloaded Store over boardDir. Call Load to populate from disk.
// An empty boardDir gives a purely in-memory store that Load leaves empty.
func NewStore(boardDir string) *Store {
	return &Store{
		tasks:    []Task{},
		boardDir: boardDir,
	}
}

// Load populates the store from boardDir and never writes.
// board.json wins when it exists; otherwise the legacy files that exist are migrated in memory.
// Whenever a legacy file exists, its done marks are folded into the loaded entries.
func (s *Store) Load() error {
	s.tasks = []Task{}
	s.legacyDone = nil
	if s.boardDir == "" {
		return nil
	}

	tasksRecords, haveTasks, err := readLegacyFile(filepath.Join(s.boardDir, legacyTasksFile))
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}
	notesRecords, haveNotes, err := readLegacyFile(filepath.Join(s.boardDir, legacyNotesFile))
	if err != nil {
		return fmt.Errorf("load store: %w", err)
	}

	path := filepath.Join(s.boardDir, boardFile)
	var entries []Task
	var legacyDone []string
	if fileExists(path) {
		file, found, err := state.ReadJSON[storeFile](path, path+swapLockSuffix)
		if err != nil {
			return fmt.Errorf("load store: %w", err)
		}
		if found {
			if file.Version != storeVersion {
				return fmt.Errorf("load store: %s has version %d; this binary reads only version %d", boardFile, file.Version, storeVersion)
			}
			entries, legacyDone = file.Entries, file.LegacyDone
		}
	} else if haveTasks || haveNotes {
		entries, legacyDone, err = migrateLegacy(tasksRecords, notesRecords)
		if err != nil {
			return fmt.Errorf("load store: %w", err)
		}
	}

	if haveTasks || haveNotes {
		entries, legacyDone = foldLegacyDone(entries, legacyDone, append(slices.Clone(tasksRecords), notesRecords...))
	}

	if entries == nil {
		entries = []Task{}
	}
	for i := range entries {
		if entries[i].DependsOn == nil {
			entries[i].DependsOn = []string{}
		}
	}
	s.tasks = entries
	s.legacyDone = legacyDone
	return nil
}

// readLegacyFile reads one legacy file when it exists, under the swap lock the pre-upgrade binary wrote it with.
// Existence is checked first so a read never creates a lock file for an absent file.
func readLegacyFile(path string) ([]legacyRecord, bool, error) {
	if !fileExists(path) {
		return nil, false, nil
	}
	records, found, err := state.ReadJSON[[]legacyRecord](path, path+swapLockSuffix)
	if err != nil {
		return nil, false, err
	}
	return records, found, nil
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save writes board.json and nothing else.
func (s *Store) Save() error {
	if s.boardDir == "" {
		return fmt.Errorf("save store: no board directory")
	}
	path := filepath.Join(s.boardDir, boardFile)
	return state.WriteJSON(path, path+swapLockSuffix, storeFile{
		Version:    storeVersion,
		Entries:    s.tasks,
		LegacyDone: s.legacyDone,
	})
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
	return nextIDIn(s.tasks)
}

// nextIDIn returns the ID for a task appended to tasks, used so projection-based
// operations assign the same IDs as sequential upserts would.
func nextIDIn(tasks []Task) int {
	if len(tasks) == 0 {
		return 0
	}
	maxID := tasks[0].ID
	for _, t := range tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1
}

// validateWrite checks incoming against snapshot for dangling deps, isolated and tier
// constraints, and cycles. snapshot is the projected state after any pending removals.
func (s *Store) validateWrite(snapshot []Task, incoming Task) error {
	snapshotIndex := make(map[string]*Task)
	for i := range snapshot {
		snapshotIndex[snapshot[i].Slug] = &snapshot[i]
	}

	for _, dep := range incoming.DependsOn {
		if _, exists := snapshotIndex[dep]; !exists {
			return fmt.Errorf("dangling dependency: %q does not exist", dep)
		}
	}

	for _, dep := range incoming.DependsOn {
		depTask := snapshotIndex[dep]
		if depTask.Isolated {
			return fmt.Errorf("cannot depend on isolated task %q", dep)
		}
	}

	adjacency := make(map[string][]string)
	for _, t := range snapshot {
		if t.Slug == incoming.Slug {
			adjacency[t.Slug] = incoming.DependsOn
		} else {
			adjacency[t.Slug] = t.DependsOn
		}
	}
	if _, exists := adjacency[incoming.Slug]; !exists {
		adjacency[incoming.Slug] = incoming.DependsOn
	}

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

	if incoming.Isolated {
		for _, t := range snapshot {
			for _, dep := range t.DependsOn {
				if dep == incoming.Slug {
					return fmt.Errorf("cannot isolate task %q: %q depends on it", incoming.Slug, t.Slug)
				}
			}
		}
	}

	return validateTier(snapshot, incoming)
}

func isDone(t Task) bool {
	return t.Status != nil && *t.Status == "done"
}

// validateTier enforces the tier dependency rule over snapshot with incoming applied:
// a dependent's tier must be greater than or equal to its dependency's tier, compared only while
// the dependency is not done.
// The dependent's own status never exempts the edge.
func validateTier(snapshot []Task, incoming Task) error {
	index := make(map[string]Task, len(snapshot))
	for _, t := range snapshot {
		index[t.Slug] = t
	}

	for _, dep := range incoming.DependsOn {
		depTask, ok := index[dep]
		if !ok || isDone(depTask) {
			continue
		}
		if depTask.Tier > incoming.Tier {
			return fmt.Errorf("tier rule: %q (tier %d) depends on %q (tier %d), which has a higher tier; promote %q to tier %d or lower, or demote %q to tier %d or higher",
				incoming.Slug, incoming.Tier, dep, depTask.Tier, dep, incoming.Tier, incoming.Slug, depTask.Tier)
		}
	}

	if isDone(incoming) {
		return nil
	}
	for _, t := range snapshot {
		if t.Slug == incoming.Slug {
			continue
		}
		for _, dep := range t.DependsOn {
			if dep == incoming.Slug && t.Tier < incoming.Tier {
				return fmt.Errorf("tier rule: %q (tier %d) depends on %q (tier %d), which has a higher tier; promote %q to tier %d or lower, or demote %q to tier %d or higher",
					t.Slug, t.Tier, incoming.Slug, incoming.Tier, incoming.Slug, t.Tier, t.Slug, incoming.Tier)
			}
		}
	}

	return nil
}

// MergeStatusUpdate carries the resolved set_status step for a MergeTasks call.
// Selector is a Go string when the task is identified by slug,
// or float64 when identified by numeric id (JSON numbers decode as float64).
// Status is nil to clear the status field.
type MergeStatusUpdate struct {
	Selector any
	Status   *string
}

// upsertAllowedKeys is the authoritative set of field names accepted by all upsert paths,
// enforced at the store boundary. "id" is auto-assigned; "phase" and "group" are excluded.
var upsertAllowedKeys = map[string]bool{
	"slug":       true,
	"title":      true,
	"depends_on": true,
	"isolated":   true,
	"brief":      true,
	"body":       true,
	"status":     true,
	"tier":       true,
	"type":       true,
	"recipe":     true,
	"short_name": true,
}

// validateUpsertFields reports an error for any field not in upsertAllowedKeys, with a hint for "phase" typos.
func validateUpsertFields(fields map[string]any) error {
	for k := range fields {
		if !upsertAllowedKeys[k] {
			if k == "phase" {
				return fmt.Errorf("unknown field: %q (did you mean \"status\"?)", k)
			}
			if k == "deferred" {
				return fmt.Errorf("unknown field: %q (deferred is retired; use \"tier\": 3 for someday work)", k)
			}
			return fmt.Errorf("unknown field: %q", k)
		}
	}
	return nil
}

// UpsertTask creates or updates the task identified by fields["slug"].
// Rejects unknown fields via validateUpsertFields before any other processing.
func (s *Store) UpsertTask(fields map[string]any) (Task, error) {
	if err := validateUpsertFields(fields); err != nil {
		return Task{}, err
	}

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

// SetStatus sets or clears the status field for the task identified by idOrSlug.
// Returns fmt.Errorf("task not found: %v", idOrSlug) when no task matches id or slug.
func (s *Store) SetStatus(idOrSlug any, status *string) error {
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
			// Reopening can strand a lower-tier dependent behind a now-live dependency;
			// setting done never introduces a violation, so it stays unchecked.
			if status == nil || *status != "done" {
				incoming := s.tasks[i]
				incoming.Status = status
				if err := validateTier(s.tasks, incoming); err != nil {
					return err
				}
			}
			s.tasks[i].Status = status
			return nil
		}
	}
	// No task matched — error so callers get clear feedback instead of a silent no-op.
	return fmt.Errorf("task not found: %v", idOrSlug)
}

// SetDeps replaces the depends_on list for slug, running full validation.
// Returns error if slug not found.
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

	// README order, so list, find and --text agree with the rendered board; store order when the layers fail.
	ordered := s.tasks
	if ro, err := RenderOrder(s.tasks); err == nil {
		ordered = make([]Task, len(ro))
		for i, twl := range ro {
			ordered[i] = twl.Task
		}
	}

	result := make([]BriefTask, 0, len(s.tasks))
	for _, t := range ordered {
		brief := BriefTask{
			ID:          t.ID,
			Slug:        t.Slug,
			Title:       t.Title,
			DependsOn:   t.DependsOn,
			Tier:        t.Tier,
			Type:        t.Type,
			Isolated:    t.Isolated,
			Brief:       t.Brief,
			Status:      t.Status,
			Layer:       layerMap[t.Slug],
			HasProposal: t.Body != "",
		}
		result = append(result, brief)
	}
	return result
}

// Promote moves the entry identified by idOrSlug to a lower tier number.
// A nil target means one tier lower; the target must be at least MinTier and strictly below the
// entry's current tier, and skipping tiers is allowed.
// The promoted entry passes validateWrite, so a dependency left at a higher tier refuses it.
func (s *Store) Promote(idOrSlug any, target *int) (Task, error) {
	current, ok := s.GetTask(idOrSlug)
	if !ok {
		return Task{}, fmt.Errorf("task not found: %v", idOrSlug)
	}

	to := current.Tier - 1
	if target != nil {
		to = *target
	}
	if to < MinTier || to >= current.Tier {
		return Task{}, fmt.Errorf("cannot promote %q from tier %d to tier %d: the target must be between %d and %d; demotion goes through upsert with a tier",
			current.Slug, current.Tier, to, MinTier, current.Tier-1)
	}

	incoming := current
	incoming.Tier = to
	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return Task{}, err
	}

	for i := range s.tasks {
		if s.tasks[i].Slug == current.Slug {
			s.tasks[i] = incoming
			break
		}
	}
	return incoming, nil
}

// Prune removes every done entry, strips the removed slugs from the survivors' depends_on, and
// returns the removed slugs in store order.
// An abandoned entry survives.
func (s *Store) Prune() []string {
	removed := []string{}
	gone := make(map[string]bool)
	kept := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if isDone(t) {
			removed = append(removed, t.Slug)
			gone[t.Slug] = true
			continue
		}
		kept = append(kept, t)
	}

	for i := range kept {
		deps := make([]string, 0, len(kept[i].DependsOn))
		for _, dep := range kept[i].DependsOn {
			if !gone[dep] {
				deps = append(deps, dep)
			}
		}
		kept[i].DependsOn = deps
	}

	s.tasks = kept
	return removed
}

// Find returns the entries whose slug, title, brief or body contains text, case-insensitively,
// done entries included, in ListTasksBrief's shape and order.
func (s *Store) Find(text string) []BriefTask {
	needle := strings.ToLower(text)
	matches := make(map[string]bool)
	for _, t := range s.tasks {
		for _, field := range []string{t.Slug, t.Title, t.Brief, t.Body} {
			if strings.Contains(strings.ToLower(field), needle) {
				matches[t.Slug] = true
				break
			}
		}
	}

	result := []BriefTask{}
	for _, b := range s.ListTasksBrief() {
		if matches[b.Slug] {
			result = append(result, b)
		}
	}
	return result
}

// ListTasksFull returns a copy of the raw task list with no enrichment.
func (s *Store) ListTasksFull() []Task {
	result := make([]Task, len(s.tasks))
	copy(result, s.tasks)
	return result
}

// UpsertTasksBatch applies multiple upserts atomically — validates all first, then applies all or
// none.
// Each task's field map is validated by the upsert allowlist before projection.
func (s *Store) UpsertTasksBatch(tasks []map[string]any) error {
	for _, fields := range tasks {
		if err := validateUpsertFields(fields); err != nil {
			return err
		}
	}

	snapshot := make([]Task, len(s.tasks))
	copy(snapshot, s.tasks)
	projectedSlugs := make([]string, 0, len(tasks))
	for _, fields := range tasks {
		slugVal, hasSlug := fields["slug"]
		if !hasSlug {
			return fmt.Errorf("slug key is missing in batch")
		}
		slugStr, ok := slugVal.(string)
		if !ok || slugStr == "" {
			return fmt.Errorf("slug must be a non-empty string in batch")
		}
		projectedSlugs = append(projectedSlugs, slugStr)

		foundIdx := -1
		for i, t := range snapshot {
			if t.Slug == slugStr {
				foundIdx = i
				break
			}
		}
		if foundIdx >= 0 {
			incoming, err := ApplyPatch(snapshot[foundIdx], fields)
			if err != nil {
				return err
			}
			snapshot[foundIdx] = incoming
		} else {
			incoming, err := NewTask(fields, nextIDIn(snapshot))
			if err != nil {
				return err
			}
			snapshot = append(snapshot, incoming)
		}
	}

	for _, slugStr := range projectedSlugs {
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

	s.tasks = snapshot
	return nil
}

// MergeTasks removes slugs, upserts one task, and optionally sets a status — all atomically.
// setStatus is the resolved status-update step,
// or nil to skip it.
// When setStatus targets a missing task, SetStatus returns an error and boardCriticalSection discards the in-memory mutation without saving, leaving the on-disk state unchanged.
func (s *Store) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
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

	if err := validateUpsertFields(upsert); err != nil {
		return Task{}, err
	}

	slugVal, hasSlug := upsert["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing in merge upsert")
	}
	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return Task{}, fmt.Errorf("slug must be a non-empty string")
	}

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
		incoming, err = NewTask(upsert, nextIDIn(projected))
	}

	if err != nil {
		return Task{}, err
	}

	if err := s.validateWrite(projected, incoming); err != nil {
		return Task{}, err
	}

	for _, slug := range removeSlugs {
		s.RemoveTask(slug)
	}

	upserted, err := s.UpsertTask(upsert)
	if err != nil {
		return Task{}, err
	}

	if setStatus != nil {
		if err := s.SetStatus(setStatus.Selector, setStatus.Status); err != nil {
			return Task{}, err
		}
	}

	return upserted, nil
}
