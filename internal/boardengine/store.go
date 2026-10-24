// store.go — the in-memory entry store over a board directory's board.json.
//
// Load/Save plus all CRUD and validation: dangling-dependency, isolated, kind and label rules, and cycle detection, with batch and merge applied atomically.
// Load migrates the legacy tasks.json and notes.json in memory when board.json is absent and folds a pre-upgrade binary's done marks;
// Save writes board.json only.
// Save and Load take the fine-grained swap lock so a concurrent read never sees a half-written
// file.

package boardengine

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// swapLockSuffix is the fine-grained lock that fences readers during writer
// swap, separate from board.lock to minimize read latency.
const swapLockSuffix = ".swaplock"

// BriefTask is the enriched read-only view returned by list, with Layer and HasProposal computed at
// read time.
type BriefTask struct {
	ID          int       `json:"id"`
	Handle      string    `json:"handle,omitempty"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	DependsOn   []string  `json:"depends_on"`
	Kind        string    `json:"kind"`
	Labels      []string  `json:"labels"`
	Isolated    bool      `json:"isolated"`
	Brief       string    `json:"brief"`
	Priority    string    `json:"priority,omitempty"`
	Status      *string   `json:"status,omitempty"`
	Created     time.Time `json:"created,omitzero"`
	Revised     time.Time `json:"revised,omitzero"`
	Layer       string    `json:"layer"`
	HasProposal bool      `json:"has_proposal"`
	// OwnerMachine is the entry's effective owner: the recorded one, else the owner it reads as through its parent; empty for a note and where the owner cannot be resolved.
	OwnerMachine string `json:"owner_machine,omitempty"`
}

const (
	// boardFile names the one store file a board directory holds.
	boardFile = "board.json"

	// storeVersion is the only on-disk version Load accepts.
	storeVersion = 1
)

// storeFile is the on-disk shape of board.json.
// LegacyDone lists the slugs whose done mark came from a legacy file, so the fold never re-applies one.
// PrimeOwner names the machine that owns the hub's prime, empty when none is recorded.
// Unknown holds the top-level keys this binary does not know, written back after the known ones in key order.
type storeFile struct {
	Version    int                        `json:"version"`
	Entries    []Task                     `json:"entries"`
	LegacyDone []string                   `json:"legacy_done,omitempty"`
	PrimeOwner string                     `json:"prime_owner,omitempty"`
	Unknown    map[string]json.RawMessage `json:"-"`
}

// MarshalJSON encodes the known fields, then the Unknown members after them in key order.
func (f storeFile) MarshalJSON() ([]byte, error) {
	type plain storeFile
	object, err := json.Marshal(plain(f))
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, f.Unknown)
}

// loadedStoreFile is the decode shape of board.json: its entries may still carry the retired tier and type.
// Unknown holds the top-level keys this binary does not know, each with its raw JSON value.
type loadedStoreFile struct {
	Version    int                        `json:"version"`
	Entries    []storedEntry              `json:"entries"`
	LegacyDone []string                   `json:"legacy_done,omitempty"`
	PrimeOwner string                     `json:"prime_owner,omitempty"`
	Unknown    map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the known fields in one json pass, then walks the raw bytes once to keep every other key, top-level and per entry, in Unknown.
// The entries decode through decodedEntry, not storedEntry, so no json call is nested per entry and the load stays as cheap as one decode.
func (f *loadedStoreFile) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Version    int            `json:"version"`
		Entries    []decodedEntry `json:"entries"`
		LegacyDone []string       `json:"legacy_done,omitempty"`
		PrimeOwner string         `json:"prime_owner,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var rawEntries []byte
	unknown := map[string]json.RawMessage{}
	eachMember(data, func(key string, value []byte) {
		switch {
		case strings.EqualFold(key, "entries"):
			rawEntries = value
		case !loadedStoreFileKeys[strings.ToLower(key)]:
			unknown[key] = slices.Clone(value)
		}
	})
	entries := make([]storedEntry, 0, len(decoded.Entries))
	eachElement(rawEntries, func(raw []byte) {
		entries = append(entries, storedEntryFrom(decoded.Entries[len(entries)], raw))
	})
	*f = loadedStoreFile{Version: decoded.Version, Entries: entries, LegacyDone: decoded.LegacyDone, PrimeOwner: decoded.PrimeOwner}
	if len(unknown) > 0 {
		f.Unknown = unknown
	}
	return nil
}

// loadedStoreFileKeys is the set of lower-cased JSON keys loadedStoreFile declares.
var loadedStoreFileKeys = jsonKeySet(reflect.TypeFor[loadedStoreFile]())

// Store holds the in-memory entry list for one board directory's board.json.
type Store struct {
	tasks      []Task
	legacyDone []string
	// primeOwner is the board.json prime_owner record, the machine that owns the hub's prime; empty when none is recorded.
	primeOwner string
	// unknown is the top-level keys of board.json this binary does not know, kept raw and written back by Save.
	unknown  map[string]json.RawMessage
	boardDir string
	// vocab is the label vocabulary validateWrite checks labels against.
	// Board sets it on every store it builds; only a store built directly by NewStore, in store-level tests, has none, and nil skips the label check alone.
	vocab *Vocabulary
	// parentOf names the slug of an entry's parent pair for the effective owner, "" for the prime; nil reads every parent as the prime.
	// Board sets it on every store it builds from Config.ParentOf.
	parentOf func(slug string) (string, error)
	// pairExists reports whether an entry's pair exists on this machine; nil reads every pair as absent.
	// Board sets it on every store it builds from Config.PairExists.
	pairExists func(slug string) bool
}

// NewStore creates an empty, unloaded Store over boardDir.
// Call Load to populate from disk.
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
// Entries in the old tier-and-type shape convert in memory and persist in the new shape on the next Save.
// An entry without a handle of its own letter gets one in memory, persisted on the next Save too.
func (s *Store) Load() error {
	s.tasks = []Task{}
	s.legacyDone = nil
	s.primeOwner = ""
	s.unknown = nil
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
	var unknown map[string]json.RawMessage
	var primeOwner string
	if fileExists(path) {
		// The raw bytes are read under the shared lock and decoded after it is released,
		// so the decode never holds off a writer waiting for the exclusive lock.
		raw, found, err := state.ReadJSON[json.RawMessage](path, path+swapLockSuffix)
		if err != nil {
			return fmt.Errorf("load store: %w", err)
		}
		if found {
			var file loadedStoreFile
			if err := json.Unmarshal(raw, &file); err != nil {
				return fmt.Errorf("load store: %w: unmarshal state: %w", state.ErrDecode, err)
			}
			if file.Version != storeVersion {
				return fmt.Errorf("load store: %s has version %d; this binary reads only version %d", boardFile, file.Version, storeVersion)
			}
			entries, legacyDone, unknown = migrateEntries(file.Entries, s.vocab), file.LegacyDone, file.Unknown
			primeOwner = file.PrimeOwner
		}
	} else if haveTasks || haveNotes {
		entries, legacyDone, err = migrateLegacy(tasksRecords, notesRecords, s.vocab)
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
		if entries[i].Issues == nil {
			entries[i].Issues = []int{}
		}
		if entries[i].Labels == nil {
			entries[i].Labels = []string{}
		}
	}
	assignHandles(entries, s.vocab)
	s.tasks = entries
	s.legacyDone = legacyDone
	s.primeOwner = primeOwner
	s.unknown = unknown
	return nil
}

// BoardFileRel returns the board file's name relative to the board directory, the board checkout's root.
func BoardFileRel() string {
	return boardFile
}

// ParseBoardTasks decodes board.json bytes read from a board commit, as Load decodes the stored file.
// It takes no lock and reads no legacy file; an old tier-and-type entry still yields its labels, and an unknown key is ignored.
// A version other than the one this binary reads is an error.
func ParseBoardTasks(data []byte) ([]Task, error) {
	var file loadedStoreFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("load store: %w: unmarshal state: %w", state.ErrDecode, err)
	}
	if file.Version != storeVersion {
		return nil, fmt.Errorf("load store: %s has version %d; this binary reads only version %d", boardFile, file.Version, storeVersion)
	}
	return migrateEntries(file.Entries, nil), nil
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
		PrimeOwner: s.primeOwner,
		Unknown:    s.unknown,
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

// validateWrite checks incoming against snapshot for dangling deps, isolated constraints, cycles, the kind rules and, when the store has a vocabulary, the labels.
// snapshot is the projected state after any pending removals.
func (s *Store) validateWrite(snapshot []Task, incoming Task) error {
	snapshotIndex := make(map[string]*Task)
	for i := range snapshot {
		snapshotIndex[snapshot[i].Slug] = &snapshot[i]
	}

	seenIssues := make(map[int]bool, len(incoming.Issues))
	for _, n := range incoming.Issues {
		if n <= 0 {
			return fmt.Errorf("entry %q lists issue %d, and an issue number is a positive integer: correct or drop it from issues", incoming.Slug, n)
		}
		if seenIssues[n] {
			return fmt.Errorf("entry %q lists issue %d more than once: list each issue number once", incoming.Slug, n)
		}
		seenIssues[n] = true
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

	if incoming.Kind == KindNote && len(incoming.DependsOn) > 0 {
		return fmt.Errorf("note %q has depends_on, and only a task can depend on another entry: clear it with \"lyx board set-deps\", or make the entry a task with \"lyx board promote\"", incoming.Slug)
	}
	for _, dep := range incoming.DependsOn {
		if snapshotIndex[dep].Kind == KindNote {
			return fmt.Errorf("%q depends on note %q, and a note cannot be depended on: promote %q with \"lyx board promote\", or drop the dependency with \"lyx board set-deps\"", incoming.Slug, dep, dep)
		}
	}
	if incoming.Kind == KindNote {
		var dependents []string
		for _, t := range snapshot {
			if t.Slug != incoming.Slug && slices.Contains(t.DependsOn, incoming.Slug) {
				dependents = append(dependents, t.Slug)
			}
		}
		if len(dependents) > 0 {
			return fmt.Errorf("note %q cannot be a note: %s depend on it; clear their depends_on with \"lyx board set-deps\", or keep the entry a task", incoming.Slug, strings.Join(dependents, ", "))
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

	if s.vocab != nil {
		return validateLabels(incoming, *s.vocab)
	}
	return nil
}

func isDone(t Task) bool {
	return t.Status != nil && *t.Status == "done"
}

// MergeStatusUpdate carries the resolved set_status step for a MergeTasks call.
// Selector is a Go string when the task is identified by slug, float64 when identified by numeric id (JSON numbers decode as float64), or a Handle when identified by handle.
// Status is nil to clear the status field.
type MergeStatusUpdate struct {
	Selector any
	Status   *string
}

// UpsertPayloadKeys returns the keys a task payload carries on every upsert path: the required keys and the optional ones.
// "id" and "handle" are assigned by the board and never accepted; "phase" and "group" are retired.
// "owner_machine" is accepted only as the entry's effective owner, a no-op, and "beam" only as the entry's stored flag.
func UpsertPayloadKeys() (required, optional []string) {
	return []string{"slug"}, []string{"title", "brief", "body", "depends_on", "isolated", "status", "kind", "labels", "recipe", "priority", "short_name", "issues", "created", "revised", "run_settings", "profile", "owner_machine", "project", "order", "beam"}
}

// upsertAllowedKeys is the set of field names every upsert path accepts, enforced at the store boundary.
var upsertAllowedKeys = func() map[string]bool {
	required, optional := UpsertPayloadKeys()
	allowed := map[string]bool{}
	for _, key := range append(required, optional...) {
		allowed[key] = true
	}
	return allowed
}()

// validateUpsertFields reports an error for any field not in upsertAllowedKeys, with a hint for "phase" typos.
func validateUpsertFields(fields map[string]any) error {
	for k := range fields {
		if !upsertAllowedKeys[k] {
			if k == "phase" {
				return fmt.Errorf("unknown field: %q (did you mean \"status\"?)", k)
			}
			if k == "deferred" {
				return fmt.Errorf("unknown field: %q (deferred is retired; use \"kind\": \"note\" for someday work)", k)
			}
			if k == "tier" {
				return fmt.Errorf("unknown field: %q (tier is retired; use \"kind\")", k)
			}
			if k == "type" {
				return fmt.Errorf("unknown field: %q (type is retired; use \"labels\")", k)
			}
			if k == "handle" {
				return fmt.Errorf("unknown field: %q (the board assigns an entry's handle; drop it from the payload)", k)
			}
			return fmt.Errorf("unknown field: %q", k)
		}
	}
	return nil
}

// UpsertTask creates or updates the task identified by fields["slug"].
// Rejects unknown fields via validateUpsertFields before any other processing.
// An owner_machine key equal to the entry's effective owner is dropped from fields, and any other value is refused.
func (s *Store) UpsertTask(fields map[string]any) (Task, error) {
	if err := validateUpsertFields(fields); err != nil {
		return Task{}, err
	}
	if err := checkOwnerPayload(s, fields); err != nil {
		return Task{}, err
	}
	if err := checkBeamPayload(s, fields); err != nil {
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

// selects reports whether selector names t: an int or float64 matches its id, a string its slug and a Handle its handle, a letter matching case-insensitively.
func selects(t Task, selector any) bool {
	switch v := selector.(type) {
	case int:
		return t.ID == v
	case float64:
		return t.ID == int(v)
	case string:
		return t.Slug == v
	case Handle:
		letter, number, ok := ParseHandle(string(v))
		storedLetter, storedNumber, stored := ParseHandle(t.Handle)
		return ok && stored && letter == storedLetter && number == storedNumber
	}
	return false
}

// index returns the position of the entry selector names, a slug string, a numeric id or a Handle, and false when no entry matches.
func (s *Store) index(selector any) (int, bool) {
	for i := range s.tasks {
		if selects(s.tasks[i], selector) {
			return i, true
		}
	}
	return -1, false
}

// GetTask looks up the entry selector names, a slug string, a numeric id or a Handle. Returns (Task, true) if found.
func (s *Store) GetTask(selector any) (Task, bool) {
	i, found := s.index(selector)
	if !found {
		return Task{}, false
	}
	return s.tasks[i], true
}

// RemoveTask deletes the entry selector names, a slug string, a numeric id or a Handle. Returns an error if not found.
func (s *Store) RemoveTask(selector any) error {
	i, found := s.index(selector)
	if !found {
		return fmt.Errorf("task not found: %v", selector)
	}
	s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
	return nil
}

// SetStatus sets or clears the status field of the entry selector names, a slug string, a numeric id or a Handle.
// Returns fmt.Errorf("task not found: %v", selector) when no entry matches.
func (s *Store) SetStatus(selector any, status *string) error {
	i, found := s.index(selector)
	if !found {
		// An error, so callers get clear feedback instead of a silent no-op.
		return fmt.Errorf("task not found: %v", selector)
	}
	incoming := s.tasks[i]
	incoming.Status = status
	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return err
	}
	s.tasks[i].Status = status
	return nil
}

// SetDeps replaces the depends_on list of the entry selector names, a slug string, a numeric id or a Handle, running full validation.
// Returns fmt.Errorf("task not found: %v", selector) when no entry matches.
func (s *Store) SetDeps(selector any, dependsOn []string) error {
	i, found := s.index(selector)
	if !found {
		return fmt.Errorf("task not found: %v", selector)
	}

	incoming := s.tasks[i]
	incoming.DependsOn = dependsOn

	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return err
	}

	s.tasks[i] = incoming
	return nil
}

// hasAllLabels reports whether t carries every label in want; an empty want matches everything.
func hasAllLabels(t Task, want []string) bool {
	for _, w := range want {
		if !slices.Contains(t.Labels, w) {
			return false
		}
	}
	return true
}

// anyEntryCarries reports whether any entry, of any kind or status, carries label.
func (s *Store) anyEntryCarries(label string) bool {
	for _, t := range s.tasks {
		if slices.Contains(t.Labels, label) {
			return true
		}
	}
	return false
}

// ListTasksBrief returns the tasks carrying every label in labels (all tasks when empty), enriched with computed Layer and HasProposal fields.
func (s *Store) ListTasksBrief(labels []string) []BriefTask {
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

	owners := ownersOf(s.primeOwner, s.tasks)
	parentOf := parentOfOrPrime(s.parentOf)
	result := make([]BriefTask, 0, len(s.tasks))
	for _, t := range ordered {
		if !hasAllLabels(t, labels) {
			continue
		}
		owner := t.OwnerMachine
		if owner == "" && (t.Kind == KindTask || t.Kind == KindProject) {
			// A resolver error leaves the cell empty: a list never fails on a parent it cannot read.
			owner, _ = owners.Effective(t.Slug, parentOf)
		}
		brief := BriefTask{
			ID:           t.ID,
			Handle:       t.Handle,
			Slug:         t.Slug,
			Title:        t.Title,
			DependsOn:    t.DependsOn,
			Kind:         t.Kind,
			Labels:       t.Labels,
			Isolated:     t.Isolated,
			Brief:        t.Brief,
			Priority:     t.Priority,
			Status:       t.Status,
			Created:      t.Created,
			Revised:      t.Revised,
			Layer:        layerMap[t.Slug],
			HasProposal:  t.Body != "",
			OwnerMachine: owner,
		}
		result = append(result, brief)
	}
	return result
}

// Promote makes the entry identified by idOrSlug a task and clears its priority, which only a note carries.
// A task is returned unchanged with changed false, so the caller knows nothing was written.
// A project is refused, since only a task is claimable bar a beam's landing.
// The promoted entry passes validateWrite, so a missing type label refuses it.
func (s *Store) Promote(idOrSlug any) (task Task, changed bool, err error) {
	current, ok := s.GetTask(idOrSlug)
	if !ok {
		return Task{}, false, fmt.Errorf("task not found: %v", idOrSlug)
	}
	if current.Kind == KindProject {
		return Task{}, false, fmt.Errorf("entry %q is a project, and a project is never made a task: make it a beam with `lyx beam create %s`", current.Slug, current.Slug)
	}
	if current.Kind == KindTask {
		return current, false, nil
	}

	incoming := current
	incoming.Kind = KindTask
	incoming.Priority = ""
	if err := s.validateWrite(s.tasks, incoming); err != nil {
		return Task{}, false, err
	}

	for i := range s.tasks {
		if s.tasks[i].Slug == current.Slug {
			s.tasks[i] = incoming
			break
		}
	}
	return incoming, true, nil
}

// Prune removes every done entry, strips the removed slugs from the survivors' depends_on and order, and returns the removed slugs in store order.
// An abandoned entry survives, and so does a done project while an entry the same prune does not remove names it in project.
func (s *Store) Prune() []string {
	gone := make(map[string]bool)
	for _, t := range s.tasks {
		if isDone(t) {
			gone[t.Slug] = true
		}
	}
	// A done project stays while a surviving entry names it, which can in turn keep the done project that entry is a member of.
	for changed := true; changed; {
		changed = false
		for _, t := range s.tasks {
			if !gone[t.Slug] && t.Project != "" && gone[t.Project] {
				delete(gone, t.Project)
				changed = true
			}
		}
	}

	removed := []string{}
	kept := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if gone[t.Slug] {
			removed = append(removed, t.Slug)
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
		if len(kept[i].Order) > 0 {
			kept[i].Order = slices.DeleteFunc(slices.Clone(kept[i].Order), func(slug string) bool { return gone[slug] })
		}
	}

	s.tasks = kept
	return removed
}

// Find returns the entries whose slug, title, brief or body contains text, case-insensitively, done entries included, in ListTasksBrief's shape and order.
// Only entries carrying every label in labels are kept.
func (s *Store) Find(text string, labels []string) []BriefTask {
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
	for _, b := range s.ListTasksBrief(labels) {
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
		if err := checkOwnerPayload(s, fields); err != nil {
			return err
		}
		if err := checkBeamPayload(s, fields); err != nil {
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

// carriedIssues returns own followed by the issues of each removeSlugs entry, in removeSlugs order.
// A number already present is not added again,
// and a slug that matches nothing adds nothing.
func (s *Store) carriedIssues(own []int, removeSlugs []string) []int {
	carried := slices.Clone(own)
	for _, slug := range removeSlugs {
		for _, t := range s.tasks {
			if t.Slug != slug {
				continue
			}
			for _, n := range t.Issues {
				if !slices.Contains(carried, n) {
					carried = append(carried, n)
				}
			}
		}
	}
	return carried
}

// oldestCreated returns the earliest known creation time among own and the removeSlugs entries, zero when none is known.
func (s *Store) oldestCreated(own time.Time, removeSlugs []string) time.Time {
	oldest := own
	for _, t := range s.tasks {
		if !slices.Contains(removeSlugs, t.Slug) || t.Created.IsZero() {
			continue
		}
		if oldest.IsZero() || t.Created.Before(oldest) {
			oldest = t.Created
		}
	}
	return oldest
}

// firstRemovedProject returns the project of the first entry in removeSlugs the store holds, "" when that entry names none or the store holds none.
func (s *Store) firstRemovedProject(removeSlugs []string) string {
	for _, slug := range removeSlugs {
		if removed, held := s.GetTask(slug); held {
			return removed.Project
		}
	}
	return ""
}

// checkRemovedProjectsUnnamed refuses a merge that removes a project an entry that survives it still names in project, naming those entries as the ones to move first.
// projected is the entries without the removed ones, and incoming is the upserted entry, which survives under its own slug.
func (s *Store) checkRemovedProjectsUnnamed(projected []Task, incoming Task, removeSlugs []string) error {
	for _, slug := range removeSlugs {
		removed, held := s.GetTask(slug)
		if !held || removed.Kind != KindProject || slug == incoming.Slug {
			continue
		}
		var members []string
		for _, t := range projected {
			if t.Slug != incoming.Slug && t.Project == slug {
				members = append(members, t.Slug)
			}
		}
		if incoming.Project == slug {
			members = append(members, incoming.Slug)
		}
		if len(members) > 0 {
			return fmt.Errorf("cannot remove project %q: %s still name it in project: move them to another project, or drop their project, with `lyx board upsert` first", slug, strings.Join(members, ", "))
		}
	}
	return nil
}

// mergeOrders rewrites every order that names a removed slug to name into once, in the place of the first removed slug it names.
// The upserted slug is never a removed one, so an order naming it keeps it.
func (s *Store) mergeOrders(removeSlugs []string, into string) {
	removed := slices.DeleteFunc(slices.Clone(removeSlugs), func(slug string) bool { return slug == into })
	for i := range s.tasks {
		if !slices.ContainsFunc(s.tasks[i].Order, func(slug string) bool { return slices.Contains(removed, slug) }) {
			continue
		}
		placed := slices.Contains(s.tasks[i].Order, into)
		var order []string
		for _, slug := range s.tasks[i].Order {
			switch {
			case !slices.Contains(removed, slug):
				order = append(order, slug)
			case !placed:
				order = append(order, into)
				placed = true
			}
		}
		s.tasks[i].Order = order
	}
}

// MergeTasks removes slugs, upserts one task, and optionally sets a status — all atomically.
// The upserted entry's issues are its own followed by those of each removed entry,
// so a merge never loses a recorded issue,
// and its creation time is the oldest known among them.
// An upserted entry with no project of its own takes the first removed entry's, every order naming a removed slug names the upserted slug once in its place,
// and a merge that removes a project another surviving entry names in project is refused.
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
	if err := checkOwnerPayload(s, upsert); err != nil {
		return Task{}, err
	}
	if err := checkBeamPayload(s, upsert); err != nil {
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

	incoming.Issues = s.carriedIssues(incoming.Issues, removeSlugs)
	incoming.Created = s.oldestCreated(incoming.Created, removeSlugs)
	upsert = maps.Clone(upsert)
	upsert["issues"] = incoming.Issues
	if !incoming.Created.IsZero() {
		upsert["created"] = incoming.Created
	}
	if incoming.Project == "" {
		if carried := s.firstRemovedProject(removeSlugs); carried != "" {
			incoming.Project = carried
			upsert["project"] = carried
		}
	}
	if err := s.checkRemovedProjectsUnnamed(projected, incoming, removeSlugs); err != nil {
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
	s.mergeOrders(removeSlugs, upserted.Slug)

	if setStatus != nil {
		if err := s.SetStatus(setStatus.Selector, setStatus.Status); err != nil {
			return Task{}, err
		}
	}

	return upserted, nil
}
