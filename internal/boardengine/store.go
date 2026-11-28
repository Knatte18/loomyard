// store.go — the in-memory entry store over a board directory's board.json.
//
// Load reads board.json, and Save writes board.json only.
// The store applies the CRUD and validation rules:
// dangling-dependency, isolated, kind, label and offered-recipe rules, and cycle detection.
// Batch and merge apply atomically.
// Save and Load take the fine-grained swap lock so a concurrent read never sees a half-written file.

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

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// swapLockSuffix names the fine-grained lock that fences readers during a writer swap.
// It is separate from the board write lock to keep read latency low.
const swapLockSuffix = ".swaplock"

// swapLockPath returns the swap lock guarding the board file at path.
// The lock lives in the board checkout's never-tracked lock directory.
func swapLockPath(path string) string {
	return lock.TrackedLockPath(filepath.Dir(path), filepath.Base(path)+swapLockSuffix)
}

// BriefTask is an entry's enriched read-only view, with Layer, Pending and HasProposal computed at read time.
type BriefTask struct {
	ID        int      `json:"id"`
	Handle    string   `json:"handle,omitempty"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	DependsOn []string `json:"depends_on"`
	// Pending lists the dependencies that are not yet met, in depends_on order; empty when the entry waits on nothing.
	Pending     []string  `json:"pending,omitempty"`
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
	// OwnerMachine is the effective owner, recorded or read through the parent; empty for a note or when unresolved.
	OwnerMachine string `json:"owner_machine,omitempty"`
	// ParentBeam is the slug of the beam the entry belongs to, RootSubject for the prime;
	// empty when the beam cannot be resolved.
	ParentBeam string `json:"parent_beam,omitempty"`
}

// IsDone reports whether the entry is done, the state that sorts it after every open entry.
func (t BriefTask) IsDone() bool {
	return t.Layer == doneLayer
}

const (
	// boardFile names the one store file a board directory holds.
	boardFile = "board.json"

	// storeVersion is the only on-disk version Load accepts.
	storeVersion = 1
)

// storeFile is the on-disk shape of board.json.
// PrimeOwner names the machine that owns the hub's prime, empty when none is recorded.
// Machines is the machine registry and Holds the adopt holds, each omitted while empty.
// Unknown holds the top-level keys this binary does not know, written back after the known ones in key order.
type storeFile struct {
	Version    int                        `json:"version"`
	Entries    []Task                     `json:"entries"`
	PrimeOwner string                     `json:"prime_owner,omitempty"`
	Machines   Registry                   `json:"machines,omitempty"`
	Holds      []Hold                     `json:"holds,omitempty"`
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

// loadedStoreFile is the decode shape of board.json.
// Unknown holds the top-level keys this binary does not know, each with its raw JSON value.
type loadedStoreFile struct {
	Version    int                        `json:"version"`
	Entries    []Task                     `json:"entries"`
	PrimeOwner string                     `json:"prime_owner,omitempty"`
	Machines   Registry                   `json:"machines,omitempty"`
	Holds      []Hold                     `json:"holds,omitempty"`
	Unknown    map[string]json.RawMessage `json:"-"`
}

// taskFields is Task without its methods, so decoding it fills the known fields alone and never runs Task.UnmarshalJSON.
type taskFields Task

// UnmarshalJSON decodes the known fields in one json pass.
// It then walks the raw bytes once to keep every other key, top-level and per entry, in Unknown.
// The entries decode through taskFields, not Task.
// No json call is nested per entry, so the load stays as cheap as one decode.
func (f *loadedStoreFile) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Version    int          `json:"version"`
		Entries    []taskFields `json:"entries"`
		PrimeOwner string       `json:"prime_owner,omitempty"`
		Machines   Registry     `json:"machines,omitempty"`
		Holds      []Hold       `json:"holds,omitempty"`
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
	entries := make([]Task, 0, len(decoded.Entries))
	eachElement(rawEntries, func(raw []byte) {
		task := Task(decoded.Entries[len(entries)])
		task.Unknown = unknownMembers(raw, taskKeys)
		entries = append(entries, task)
	})
	*f = loadedStoreFile{Version: decoded.Version, Entries: entries, PrimeOwner: decoded.PrimeOwner, Machines: decoded.Machines, Holds: decoded.Holds}
	if len(unknown) > 0 {
		f.Unknown = unknown
	}
	return nil
}

// loadedStoreFileKeys is the set of lower-cased JSON keys loadedStoreFile declares.
var loadedStoreFileKeys = jsonKeySet(reflect.TypeFor[loadedStoreFile]())

// Store holds the in-memory entry list for one board directory's board.json.
type Store struct {
	tasks []Task
	// primeOwner is the board.json prime_owner record, the machine that owns the hub's prime; empty when none is recorded.
	primeOwner string
	// machines is the board.json machine registry, kept and written back unchanged; nil when none is recorded.
	machines Registry
	// holds is the board.json adopt holds, kept and written back unchanged.
	holds []Hold
	// unknown is the top-level keys of board.json this binary does not know, kept raw and written back by Save.
	unknown  map[string]json.RawMessage
	boardDir string
	// vocab is the label vocabulary that writes check labels against.
	// The board sets it on every store it builds; a bare NewStore store has none, and nil skips the label check.
	vocab *Vocabulary
	// parentOf gives the slug of an entry's parent pair, "" for the prime; nil reads every parent as the prime.
	// The board sets it on every store it builds.
	parentOf func(slug string) (string, error)
	// pairExists reports whether an entry's pair exists on this machine; nil reads every pair as absent.
	// The board sets it on every store it builds.
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
// An absent board.json leaves the store empty.
// An entry without a handle of its own letter gets one in memory, persisted on the next Save.
// A task written with a retired pull_request or notify_on_land key has the setting it carries folded.
// The fold goes into its beam run settings.
// A pull_request of off is dropped with nothing folded, and a project keeps both keys as kept keys.
// The fold is in memory, persisted on the next Save.
func (s *Store) Load() error {
	s.tasks = []Task{}
	s.primeOwner = ""
	s.machines = nil
	s.holds = nil
	s.unknown = nil
	if s.boardDir == "" {
		return nil
	}

	path := filepath.Join(s.boardDir, boardFile)
	var entries []Task
	var unknown map[string]json.RawMessage
	var primeOwner string
	var machines Registry
	var holds []Hold
	if fileExists(path) {
		// The raw bytes are read under the shared lock and decoded after it is released,
		// so the decode never holds off a writer waiting for the exclusive lock.
		raw, found, err := state.ReadJSON[json.RawMessage](path, swapLockPath(path))
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
			entries, unknown = file.Entries, file.Unknown
			primeOwner = file.PrimeOwner
			machines, holds = file.Machines, file.Holds
		}
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
		foldLegacyBeamFields(&entries[i])
	}
	assignHandles(entries, s.vocab)
	s.tasks = entries
	s.primeOwner = primeOwner
	s.machines = machines
	s.holds = holds
	s.unknown = unknown
	return nil
}

// The retired board.json entry keys that foldLegacyBeamFields and BeamSettingsByEntry read.
const (
	legacyPullRequestKey  = "pull_request"
	legacyNotifyOnLandKey = "notify_on_land"
)

// legacyBeamSettings returns the beam settings that the retired entry keys in unknown carry, by beam key.
// A non-empty pull_request becomes beam.pull_request, and a true notify_on_land becomes beam.land_notice on.
// A pull_request of off carries nothing:
// the old resolver still opened a pull request for it under a required or merge parent,
// so a fold drops it rather than land such a task with none.
func legacyBeamSettings(unknown map[string]json.RawMessage) map[string]string {
	settings := map[string]string{}
	if pullRequest := legacyPullRequest(unknown); pullRequest != "" && pullRequest != beamsetting.PullRequestOff {
		settings[beamsetting.PullRequestKey] = pullRequest
	}
	var notify bool
	if json.Unmarshal(unknown[legacyNotifyOnLandKey], &notify) == nil && notify {
		settings[beamsetting.LandNoticeKey] = beamsetting.LandNoticeOn
	}
	return settings
}

// legacyPullRequest returns the retired pull_request entry key in unknown, empty when it is absent or not text.
func legacyPullRequest(unknown map[string]json.RawMessage) string {
	var pullRequest string
	if json.Unmarshal(unknown[legacyPullRequestKey], &pullRequest) != nil {
		return ""
	}
	return pullRequest
}

// foldLegacyBeamFields moves the retired pull_request and notify_on_land keys of t onto its beam run settings.
// A board.json written by an older binary holds them as entry keys, which now decode into Unknown,
// and the per-task beam settings read only run_settings.
// The settings legacyBeamSettings returns are set under beam, and a value t's run settings already set there wins.
// Both keys are dropped from Unknown, which becomes nil when nothing else is left.
// A project carries no run settings, so its keys stay in Unknown, which a save writes back;
// BeamSettingsByEntry still reads them, so the hub reconcile can carry them into the project's worktree after any save.
func foldLegacyBeamFields(t *Task) {
	if t.Kind == KindProject {
		return
	}
	_, hasPullRequest := t.Unknown[legacyPullRequestKey]
	_, hasNotify := t.Unknown[legacyNotifyOnLandKey]
	if !hasPullRequest && !hasNotify {
		return
	}
	settings := legacyBeamSettings(t.Unknown)
	delete(t.Unknown, legacyPullRequestKey)
	delete(t.Unknown, legacyNotifyOnLandKey)
	if len(t.Unknown) == 0 {
		t.Unknown = nil
	}
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		setBeamSettingIfAbsent(t, key, settings[key])
	}
}

// setBeamSettingIfAbsent sets key under the beam module of t's run settings unless it is already set there.
// A beam entry that is not a mapping is left untouched.
func setBeamSettingIfAbsent(t *Task, key, value string) {
	beam, isMapping := t.RunSettings[beamsetting.Module].(map[string]any)
	if t.RunSettings[beamsetting.Module] != nil && !isMapping {
		return
	}
	if _, set := beam[key]; set {
		return
	}
	if beam == nil {
		beam = map[string]any{}
		if t.RunSettings == nil {
			t.RunSettings = map[string]any{}
		}
		t.RunSettings[beamsetting.Module] = beam
	}
	beam[key] = value
}

// BoardFileRel returns the board file's name relative to the board directory, the board checkout's root.
func BoardFileRel() string {
	return boardFile
}

// ParseBoardTasks decodes board.json bytes read from a board commit.
// It takes no lock, and an unknown key is ignored.
// Each task's retired pull_request and notify_on_land keys are folded as a load folds them.
// A version other than the one this binary reads is an error.
func ParseBoardTasks(data []byte) ([]Task, error) {
	entries, err := decodeBoardEntries(data)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		foldLegacyBeamFields(&entries[i])
	}
	return entries, nil
}

// decodeBoardEntries decodes the entries of board.json bytes with their unknown keys kept and nothing folded.
// A version other than the one this binary reads is an error.
func decodeBoardEntries(data []byte) ([]Task, error) {
	var file loadedStoreFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("load store: %w: unmarshal state: %w", state.ErrDecode, err)
	}
	if file.Version != storeVersion {
		return nil, fmt.Errorf("load store: %s has version %d; this binary reads only version %d", boardFile, file.Version, storeVersion)
	}
	return file.Entries, nil
}

// EntryBeamSettings is the beam settings one board entry holds.
type EntryBeamSettings struct {
	// Set holds the entry's pull_request and land_notice by beam key.
	Set map[string]string
	// RetiredOff reports that the entry's retired pull_request is off, which carries no setting.
	// It is false when the entry's run settings set a pull_request.
	RetiredOff bool
}

// BeamSettingsByEntry returns the beam settings that each board entry of boardDir's board.json holds, by entry slug.
// An entry's settings are the pull_request and land_notice of its beam run settings,
// and its retired pull_request and notify_on_land keys while they remain; a run setting wins.
// A save drops a task's retired keys after folding them into its run settings, a pull_request of off apart,
// and a project keeps its retired keys.
// An entry that holds no setting is left out, and an absent board.json gives none.
// It reads the file as it is on disk, with nothing folded.
func BeamSettingsByEntry(boardDir string) (map[string]EntryBeamSettings, error) {
	path := filepath.Join(boardDir, boardFile)
	if !fileExists(path) {
		return nil, nil
	}
	raw, found, err := state.ReadJSON[json.RawMessage](path, swapLockPath(path))
	if err != nil {
		return nil, fmt.Errorf("read legacy beam settings: %w", err)
	}
	if !found {
		return nil, nil
	}
	entries, err := decodeBoardEntries(raw)
	if err != nil {
		return nil, err
	}
	settings := map[string]EntryBeamSettings{}
	for _, entry := range entries {
		held := EntryBeamSettings{Set: legacyBeamSettings(entry.Unknown)}
		if beam, isMapping := entry.RunSettings[beamsetting.Module].(map[string]any); isMapping {
			for _, key := range []string{beamsetting.PullRequestKey, beamsetting.LandNoticeKey} {
				if value, isText := beam[key].(string); isText && value != "" {
					held.Set[key] = value
				}
			}
		}
		_, setPullRequest := held.Set[beamsetting.PullRequestKey]
		held.RetiredOff = !setPullRequest && legacyPullRequest(entry.Unknown) == beamsetting.PullRequestOff
		if len(held.Set) > 0 || held.RetiredOff {
			settings[entry.Slug] = held
		}
	}
	return settings, nil
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
	return state.WriteJSON(path, swapLockPath(path), storeFile{
		Version:    storeVersion,
		Entries:    s.tasks,
		PrimeOwner: s.primeOwner,
		Machines:   s.machines,
		Holds:      s.holds,
		Unknown:    s.unknown,
	})
}

// Tasks returns a copy of the entry slice, so the caller can reorder it without touching the store.
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

// nextIDIn returns the ID for a task appended to tasks.
// Projection-based operations use it so they assign the same IDs as sequential upserts would.
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

// validateWrite checks incoming against snapshot for dangling dependencies and isolated constraints,
// and it checks cycles, the kind rules and, when the store has a vocabulary, the labels.
// snapshot is the projected state after any pending removals.
func (s *Store) validateWrite(snapshot []Task, incoming Task) error {
	snapshotIndex := make(map[string]*Task)
	for i := range snapshot {
		snapshotIndex[snapshot[i].Slug] = &snapshot[i]
	}

	if err := checkOfferedRecipe(incoming, snapshotIndex[incoming.Slug]); err != nil {
		return err
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

	projected := slices.Clone(snapshot)
	if index := slices.IndexFunc(projected, func(t Task) bool { return t.Slug == incoming.Slug }); index >= 0 {
		projected[index] = incoming
	} else {
		projected = append(projected, incoming)
	}
	// A project waits on its landing members, so a dependency on a project that one of them leads back to is a cycle.
	dependencies := NewDependencies(projected)
	adjacency := make(map[string][]string, len(projected))
	membership := make(map[string][]string, len(projected))
	for _, t := range projected {
		adjacency[t.Slug] = dependencies.waitsOn(t)
		if t.Kind == KindProject {
			membership[t.Slug] = dependencies.members[t.Slug]
		}
	}

	// A cycle made of project membership alone is left to checkProjectRules.
	// Its refusal names the project field to correct.
	if hit, cyclic := findCycle(adjacency, incoming.Slug); cyclic {
		if _, membershipOnly := findCycle(membership, incoming.Slug); !membershipOnly {
			return fmt.Errorf("cycle detected: %q -> %q", incoming.Slug, hit)
		}
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
		return validateLabels(incoming, snapshotIndex[incoming.Slug], *s.vocab)
	}
	return nil
}

// checkOfferedRecipe refuses a write that gives an entry the arras recipe, which is no longer offered.
// The refusal names motif, the design-first route, as the way forward.
// prior is the entry of the same slug as it stood before the write, nil for a new entry:
// an entry that already holds the recipe keeps taking writes, so a run started under it goes on to its end.
func checkOfferedRecipe(incoming Task, prior *Task) error {
	if incoming.Recipe != shedrun.RecipeArras || (prior != nil && prior.Recipe == shedrun.RecipeArras) {
		return nil
	}
	return fmt.Errorf("entry %q names recipe %q, which is not offered: a design-first entry runs %q, whose Discussion stores the decision record and leaves a Sketch its raddle run splits; "+
		"way forward: write the entry again with \"recipe\":%q", incoming.Slug, shedrun.RecipeArras, shedrun.RecipeMotif, shedrun.RecipeMotif)
}

// findCycle walks adjacency depth-first from start and reports the first slug it meets again on its own path.
func findCycle(adjacency map[string][]string, start string) (string, bool) {
	const (
		onPath   = 1
		finished = 2
	)
	state := make(map[string]int, len(adjacency))
	var walk func(slug string) (string, bool)
	walk = func(slug string) (string, bool) {
		switch state[slug] {
		case finished:
			return "", false
		case onPath:
			return slug, true
		}
		state[slug] = onPath
		for _, next := range adjacency[slug] {
			if hit, cyclic := walk(next); cyclic {
				return hit, true
			}
		}
		state[slug] = finished
		return "", false
	}
	return walk(start)
}

// StatusDone is the status of an entry whose work landed, or that was marked done by hand.
const StatusDone = "done"

// IsDoneStatus reports whether status is StatusDone.
func IsDoneStatus(status *string) bool {
	return status != nil && *status == StatusDone
}

func isDone(t Task) bool {
	return IsDoneStatus(t.Status)
}

// MergeStatusUpdate carries the resolved set_status step for a MergeTasks call.
// Selector identifies the task: a Go string for a slug, a float64 for a numeric id, or a Handle for a handle.
// JSON numbers decode as float64, so a numeric id arrives in that type.
// Status is nil to clear the status field.
type MergeStatusUpdate struct {
	Selector any
	Status   *string
}

// UpsertPayloadKeys returns the keys a task payload carries on every upsert path:
// the required keys and the optional ones.
// "id" and "handle" are assigned by the board and never accepted; "phase" and "group" are retired.
// "owner_machine" is accepted only as the entry's effective owner, a no-op, and "beam" only as the entry's stored flag.
// "front" and "on_land" are accepted only as the entry's stored values, as "beam" is.
// "discussed" is accepted only as the entry's stored value too.
// "beam_color" is output only: a beam carries it in what is read back, the owner setter records it,
// and an upsert naming it is refused as an unknown field.
// "record_body_sha256" is output only in the same way: a write that stores the entry's decision record stamps it.
func UpsertPayloadKeys() (required, optional []string) {
	return []string{"slug"}, []string{"title", "brief", "body", "depends_on", "isolated", "status", "kind", "labels", "recipe", "priority", "short_name", "issues", "created", "revised", "run_settings", "profile", "owner_machine", "project", "order", "beam", "front", onLandKey, "discussed"}
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

// validateUpsertFields reports an error for any field not in upsertAllowedKeys.
// For a few retired or board-assigned field names, the error adds a hint.
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
			if k == legacyPullRequestKey {
				return fmt.Errorf("unknown field: %q (%s moved to run_settings: set \"run_settings\": {%q: {%q: \"<%s>\"}})",
					k, k, beamsetting.Module, beamsetting.PullRequestKey, strings.Join(beamsetting.PullRequestValues(), "|"))
			}
			if k == legacyNotifyOnLandKey {
				return fmt.Errorf("unknown field: %q (%s moved to run_settings: set \"run_settings\": {%q: {%q: %q}})",
					k, k, beamsetting.Module, beamsetting.LandNoticeKey, beamsetting.LandNoticeOn)
			}
			return fmt.Errorf("unknown field: %q", k)
		}
	}
	return nil
}

// UpsertTask creates or updates the task identified by fields["slug"].
// Unknown fields are rejected before any other processing.
// An owner_machine key equal to the entry's effective owner is dropped as a no-op, and any other value is refused.
// The caller's fields map is never changed, so callers may share one payload across concurrent writes.
func (s *Store) UpsertTask(fields map[string]any) (Task, error) {
	fields = maps.Clone(fields)
	if err := validateUpsertFields(fields); err != nil {
		return Task{}, err
	}
	if err := checkOwnerPayload(s, fields); err != nil {
		return Task{}, err
	}
	if err := checkBeamPayload(s, fields); err != nil {
		return Task{}, err
	}
	if err := checkFrontPayload(s, fields); err != nil {
		return Task{}, err
	}
	if err := checkDiscussedPayload(s, fields); err != nil {
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

	// Replace an existing entry in place, which keeps its position in the store; otherwise append.
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

// selects reports whether selector identifies t.
// An int or float64 matches the id, a string matches the slug, and a Handle matches the handle.
// A Handle's letter matches case-insensitively.
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

// index returns the position of the entry that selector identifies, and false when no entry matches.
// The selector is a slug string, a numeric id or a Handle.
func (s *Store) index(selector any) (int, bool) {
	for i := range s.tasks {
		if selects(s.tasks[i], selector) {
			return i, true
		}
	}
	return -1, false
}

// GetTask returns the entry that selector identifies: a slug string, a numeric id or a Handle.
// The boolean reports whether an entry matched.
func (s *Store) GetTask(selector any) (Task, bool) {
	i, found := s.index(selector)
	if !found {
		return Task{}, false
	}
	return s.tasks[i], true
}

// RemoveTask deletes the entry that selector identifies: a slug string, a numeric id or a Handle.
// It strips the entry's slug from the depends_on of every remaining entry.
// It returns an error when no entry matches.
func (s *Store) RemoveTask(selector any) error {
	return s.removeEntry(selector, true)
}

// removeEntry deletes the entry that selector identifies.
// When stripDependencies is set, it also strips that entry's slug from the depends_on of every remaining entry.
// A caller that puts an entry of the same slug back in the same write passes false, so its dependents keep waiting.
func (s *Store) removeEntry(selector any, stripDependencies bool) error {
	i, found := s.index(selector)
	if !found {
		return fmt.Errorf("task not found: %v", selector)
	}
	slug := s.tasks[i].Slug
	s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
	if !stripDependencies {
		return nil
	}
	for j := range s.tasks {
		if slices.Contains(s.tasks[j].DependsOn, slug) {
			s.tasks[j].DependsOn = slices.DeleteFunc(slices.Clone(s.tasks[j].DependsOn), func(dep string) bool { return dep == slug })
		}
	}
	return nil
}

// SetStatus sets or clears the status field of the entry that selector identifies.
// The selector is a slug string, a numeric id or a Handle.
// It returns an error when no entry matches.
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

// SetDeps replaces the depends_on list of the entry that selector identifies.
// The selector is a slug string, a numeric id or a Handle.
// It runs full validation before the write, and it returns an error when no entry matches.
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

// ListTasksBrief returns the tasks carrying every label in labels, or all tasks when labels is empty.
// Each brief is enriched with the computed Layer and HasProposal fields.
// A task and a project carry their effective owner, which for a task comes from its beam or the root;
// a note carries none.
func (s *Store) ListTasksBrief(labels []string) []BriefTask {
	layerMap, err := ComputeLayers(s.tasks)
	if err != nil {
		// If layer computation fails, assign empty string
		layerMap = make(map[string]string)
		for _, t := range s.tasks {
			layerMap[t.Slug] = ""
		}
	}

	// Use README order so list, find and --text agree with the rendered board;
	// fall back to store order when RenderOrder fails.
	ordered := s.tasks
	if ro, err := RenderOrder(s.tasks); err == nil {
		ordered = make([]Task, len(ro))
		for i, twl := range ro {
			ordered[i] = twl.Task
		}
	}

	owners := ownersOf(s.primeOwner, s.tasks)
	parentOf := parentOfOrPrime(s.parentOf)
	dependencies := NewDependencies(s.tasks)
	result := make([]BriefTask, 0, len(s.tasks))
	for _, t := range ordered {
		if !hasAllLabels(t, labels) {
			continue
		}
		owner := ""
		if t.Kind == KindTask || t.Kind == KindProject {
			// A resolver error leaves the cell empty: a list never fails on a parent it cannot read.
			owner, _ = owners.Effective(t.Slug, parentOf)
		}
		// The listing prints the prime as RootSubject, since an empty value reads as no beam.
		// A resolver error leaves the cell empty, as for the owner.
		parentBeam := ParentBeam(s.tasks, t.Slug)
		var parentErr error
		if s.parentOf != nil {
			parentBeam, parentErr = s.parentOf(t.Slug)
		}
		switch {
		case parentErr != nil:
			parentBeam = ""
		case parentBeam == "":
			parentBeam = RootSubject
		}
		brief := BriefTask{
			ID:           t.ID,
			Handle:       t.Handle,
			Slug:         t.Slug,
			Title:        t.Title,
			DependsOn:    t.DependsOn,
			Pending:      dependencies.Pending(t),
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
			ParentBeam:   parentBeam,
		}
		result = append(result, brief)
	}
	return result
}

// promoteTo makes the entry identified by idOrSlug a task and clears its priority.
// When recipe is not empty, it is set in the same write.
// The task keeps the note's depends_on, and a non-nil dependsOn replaces it in the same write,
// so a write that also releases the task never lets a run start ahead of them.
// An entry that is already a task takes no write, so a non-nil dependsOn that differs from its depends_on is refused.
// A discussed note takes recipe raddle when recipe is empty and its record is stored, or it carries the sketch label.
// Raddle is the one recipe such a note may take.
// With recipe raddle, a note whose record is stored keeps its record and flag,
// and it gets the sketch label back when a note write dropped it.
// A nil recordStored reports no record stored, so only the label then gives recipe raddle.
func (s *Store) promoteTo(idOrSlug any, recipe string, dependsOn []string, recordStored func(slug string) bool) (task Task, changed bool, err error) {
	current, ok := s.GetTask(idOrSlug)
	if !ok {
		return Task{}, false, fmt.Errorf("task not found: %v", idOrSlug)
	}
	if current.Kind == KindProject {
		return Task{}, false, fmt.Errorf("entry %q is a project, and a project is never made a task: make it a beam with `lym beam create %s`", current.Slug, current.Slug)
	}
	if current.Kind == KindTask {
		if dependsOn != nil && !slices.Equal(current.DependsOn, dependsOn) {
			return Task{}, false, fmt.Errorf("entry %q is a task already, so promote writes nothing and sets no depends_on: change it with `lym board set-deps`", current.Slug)
		}
		return current, false, nil
	}

	incoming := current
	incoming.Kind = KindTask
	incoming.Priority = ""
	if dependsOn != nil {
		incoming.DependsOn = dependsOn
	}
	recordHeld := current.Discussed && recordStored != nil && recordStored(current.Slug)
	if recipe == "" && current.Discussed && (recordHeld || slices.Contains(current.Labels, SketchType)) {
		recipe = shedrun.RecipeRaddle
	}
	if recipe != "" {
		incoming.Recipe = recipe
	}
	if recipe == shedrun.RecipeRaddle && recordHeld && !slices.Contains(incoming.Labels, SketchType) {
		incoming.Labels = append([]string{SketchType}, incoming.Labels...)
	}
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

// Prune removes every done entry except the survivors described here, and returns the removed slugs in store order.
// It strips the removed slugs from the depends_on and order of the surviving entries.
// An abandoned entry survives, and so does a done project that a surviving entry names as its project.
func (s *Store) Prune() []string {
	gone := make(map[string]bool)
	for _, t := range s.tasks {
		if isDone(t) {
			gone[t.Slug] = true
		}
	}
	// A done project stays while a surviving entry names it,
	// and a kept project can in turn keep the done project it is a member of.
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

// Find returns the BriefTask entries whose slug, title, brief or body contains text, case-insensitively.
// Done entries are included; only entries carrying every label in labels are kept, in store order.
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

// UpsertTasksBatch applies multiple upserts atomically: it validates all of them first, then applies all or none.
// Each task's field map is checked against the upsert field allowlist before projection.
// The caller's field maps are never changed, so callers may share one payload across concurrent writes.
func (s *Store) UpsertTasksBatch(tasks []map[string]any) error {
	payloads := make([]map[string]any, len(tasks))
	for i, fields := range tasks {
		payloads[i] = maps.Clone(fields)
	}
	tasks = payloads
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
		if err := checkFrontPayload(s, fields); err != nil {
			return err
		}
		if err := checkDiscussedPayload(s, fields); err != nil {
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
		// The snapshot already holds the incoming entry, so what the entry held before the batch is read from the store.
		var prior *Task
		if i, found := s.index(slugStr); found {
			prior = &s.tasks[i]
		}
		if err := checkOfferedRecipe(incoming, prior); err != nil {
			return err
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

// firstRemovedProject returns the project of the first entry in removeSlugs that the store holds.
// It returns "" when that entry names no project, or when the store holds none of them.
func (s *Store) firstRemovedProject(removeSlugs []string) string {
	for _, slug := range removeSlugs {
		if removed, held := s.GetTask(slug); held {
			return removed.Project
		}
	}
	return ""
}

// checkRemovedProjectsUnnamed refuses a merge that removes a project while a surviving entry still names it in project.
// The upserted entry counts as surviving under its own slug,
// and the error lists the entries that still name the project.
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
			return fmt.Errorf("cannot remove project %q: %s still name it in project: move them to another project, or drop their project, with `lym board upsert` first", slug, strings.Join(members, ", "))
		}
	}
	return nil
}

// mergeOrders rewrites each order naming a removed slug to name into once, in place of the first removed slug.
// An upserted slug that also appears in removeSlugs is not removed, so an order naming it keeps it.
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

// MergeTasks removes the removeSlugs entries, upserts one entry, and optionally sets a status.
// The upserted entry's issues are its own, then those of each removed entry; a merge never loses a recorded issue.
// Its creation time is the oldest known among the upserted and removed entries.
// An upserted entry with no project of its own takes the project of the first removed entry the store holds.
// Every order that names a removed slug names the upserted slug once, in its place.
// A merge that removes a project another surviving entry names in project is refused.
// A nil setStatus skips the status update.
// If setStatus targets a missing task, MergeTasks returns an error after it has changed the store in memory,
// so the caller must not save the store.
// The caller's upsert map is never changed, so callers may share one payload across concurrent writes.
func (s *Store) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, error) {
	upsert = maps.Clone(upsert)
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
	if err := checkFrontPayload(s, upsert); err != nil {
		return Task{}, err
	}
	if err := checkDiscussedPayload(s, upsert); err != nil {
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

	// A removed entry the merge upserts again keeps its discussed flag and record stamp, as it keeps its record file.
	removedEntry, upsertedAgain := s.GetTask(slugStr)
	keepDiscussed := upsertedAgain && removedEntry.Discussed && slices.Contains(removeSlugs, slugStr)

	for _, slug := range removeSlugs {
		s.removeEntry(slug, slug != slugStr)
	}

	upserted, err := s.UpsertTask(upsert)
	if err != nil {
		return Task{}, err
	}
	if keepDiscussed {
		i, _ := s.index(upserted.Slug)
		s.tasks[i].Discussed = true
		s.tasks[i].RecordBodySHA256 = removedEntry.RecordBodySHA256
		upserted.Discussed = true
		upserted.RecordBodySHA256 = removedEntry.RecordBodySHA256
	}
	s.mergeOrders(removeSlugs, upserted.Slug)

	if setStatus != nil {
		if err := s.SetStatus(setStatus.Selector, setStatus.Status); err != nil {
			return Task{}, err
		}
	}

	return upserted, nil
}
