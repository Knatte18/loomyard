// task.go — the Task record stored in board.json.
//
// Defines the Task struct plus NewTask and ApplyPatch, which build/patch a Task from a raw field
// map via JSON round-trip so field types are validated exactly as they would be on disk.

package boardengine

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Task is the canonical record stored in board.json.
type Task struct {
	ID        int      `json:"id"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`             // KindTask or KindNote
	Labels    []string `json:"labels"`           // type and plain labels, validated against the board's Vocabulary
	Issues    []int    `json:"issues"`           // numbers of the inbox issues this entry records; a number is not checked against GitHub
	Recipe    string   `json:"recipe,omitempty"` // recipe name for the task's child worktree; empty means "loom". Resolved (and validated against the recipe vocabulary) at the seeding site, not here.
	Priority  string   `json:"priority,omitempty"`
	DependsOn []string `json:"depends_on"`
	Isolated  bool     `json:"isolated"`
	Brief     string   `json:"brief"`
	Body      string   `json:"body"`
	Status    *string  `json:"status,omitempty"`     // pointer: nil → field omitted in JSON; non-nil → status value present
	ShortName string   `json:"short_name,omitempty"` // optional short display label; falls back to Slug via ShortNameOrSlug
	// Created is when the entry was added and Revised when its scope last changed, both in whole UTC seconds;
	// zero, stored as no key, means unknown.
	Created time.Time `json:"created,omitzero"`
	Revised time.Time `json:"revised,omitzero"`

	// RunSettings maps a module name to a mapping of that module's config keys, nested as in its YAML file; an empty value is stored as no field.
	RunSettings map[string]any `json:"run_settings,omitempty"`

	// Unknown holds the decoded entry's keys this binary does not know, each with its raw JSON value, nil when there are none.
	// Encoding writes them back after the known fields in key order, and no step decodes a value into a Go type.
	Unknown map[string]json.RawMessage `json:"-"`
}

// taskKeys is the set of JSON keys Task declares, which splitUnknown subtracts from a decoded entry.
var taskKeys = jsonKeys(reflect.TypeFor[Task]())

// jsonKeys returns the JSON key names of the exported fields of struct type t: the tag name, else the field name, skipping fields tagged "-".
func jsonKeys(t reflect.Type) []string {
	var keys []string
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		keys = append(keys, name)
	}
	return keys
}

// splitUnknown decodes the JSON object data and returns its members whose key matches none of known, nil when none is left.
// A key matches a known name case-insensitively, as encoding/json matches a key to a field, so "Title" fills Title and is not also kept.
func splitUnknown(data []byte, known []string) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	for key := range members {
		if slices.ContainsFunc(known, func(name string) bool { return strings.EqualFold(key, name) }) {
			delete(members, key)
		}
	}
	if len(members) == 0 {
		return nil, nil
	}
	return members, nil
}

// appendUnknown writes the unknown members into the encoded JSON object before its closing brace, in sorted key order.
// An empty unknown returns object unchanged.
func appendUnknown(object []byte, unknown map[string]json.RawMessage) ([]byte, error) {
	if len(unknown) == 0 {
		return object, nil
	}
	out := slices.Clone(object[:len(object)-1])
	needComma := string(object) != "{}"
	for _, key := range slices.Sorted(maps.Keys(unknown)) {
		name, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		if needComma {
			out = append(out, ',')
		}
		needComma = true
		out = append(out, name...)
		out = append(out, ':')
		out = append(out, unknown[key]...)
	}
	return append(out, '}'), nil
}

// UnmarshalJSON decodes the known fields over t's current values, so a caller's pre-set defaults survive, and keeps every other key in Unknown.
func (t *Task) UnmarshalJSON(data []byte) error {
	type plain Task
	decoded := plain(*t)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := splitUnknown(data, taskKeys)
	if err != nil {
		return err
	}
	*t = Task(decoded)
	t.Unknown = unknown
	return nil
}

// MarshalJSON encodes the known fields, then the Unknown members after them in key order.
func (t Task) MarshalJSON() ([]byte, error) {
	type plain Task
	object, err := json.Marshal(plain(t))
	if err != nil {
		return nil, err
	}
	return appendUnknown(object, t.Unknown)
}

const (
	// KindTask is an entry that can run; KindNote is an idea or observation.
	KindTask = "task"
	KindNote = "note"
)

// The priority values an entry accepts.
// Task.Priority holds PriorityHigh, PriorityLow or the empty value for normal, so a write of PriorityNormal stores no key.
const (
	PriorityHigh   = "high"
	PriorityNormal = "normal"
	PriorityLow    = "low"
)

// priorityRank orders priority values for sorting: high before normal before low, with the empty value as normal.
func priorityRank(priority string) int {
	switch priority {
	case PriorityHigh:
		return 0
	case PriorityLow:
		return 2
	default:
		return 1
	}
}

// normalizeTask stores a normal priority and an empty run settings object as the empty value and the dates in whole UTC seconds, then checks that the Kind is one of the two kinds and the Priority one of the accepted values.
func normalizeTask(t *Task) error {
	if t.Kind != KindTask && t.Kind != KindNote {
		return fmt.Errorf("kind %q is not one of %s, %s", t.Kind, KindTask, KindNote)
	}
	if t.Priority == PriorityNormal {
		t.Priority = ""
	}
	if len(t.RunSettings) == 0 {
		t.RunSettings = nil
	}
	t.Created = entryTime(t.Created)
	t.Revised = entryTime(t.Revised)
	if t.Priority != "" && t.Priority != PriorityHigh && t.Priority != PriorityLow {
		return fmt.Errorf("entry %q has priority %q, and a priority is one of %s, %s, %s: correct it, or set %q to clear it",
			t.Slug, t.Priority, PriorityHigh, PriorityNormal, PriorityLow, PriorityNormal)
	}
	return nil
}

// ShortNameOrSlug returns t.ShortName when non-empty, otherwise t.Slug.
func (t Task) ShortNameOrSlug() string {
	if t.ShortName != "" {
		return t.ShortName
	}
	return t.Slug
}

// MaxSlugLength caps a slug's length to fit in directory names without MAX_PATH issues.
const MaxSlugLength = 32

// validateSlugLength returns an error when slug exceeds MaxSlugLength characters.
func validateSlugLength(slug string) error {
	if len(slug) > MaxSlugLength {
		return fmt.Errorf("slug exceeds max length of %d characters: %q (%d chars)", MaxSlugLength, slug, len(slug))
	}
	return nil
}

// NewTask builds a Task from a raw field map, assigning nextID.
// Uses JSON round-trip so field types are validated exactly as they would be on disk.
// Unknown-field validation is the caller's responsibility (store.validateUpsertFields).
func NewTask(fields map[string]any, nextID int) (Task, error) {
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}

	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return Task{}, fmt.Errorf("slug must be a non-empty string")
	}

	if err := validateSlugLength(slugStr); err != nil {
		return Task{}, err
	}

	task := Task{
		ID:        nextID,
		Kind:      KindNote,
		Labels:    []string{},
		Issues:    []int{},
		DependsOn: []string{},
		Isolated:  false,
		Brief:     "",
		Body:      "",
		Status:    nil,
	}

	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return Task{}, fmt.Errorf("marshal fields: %w", err)
	}

	err = json.Unmarshal(fieldsJSON, &task)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal fields: %w", err)
	}

	task.ID = nextID
	task.Slug = slugStr
	if task.Labels == nil {
		task.Labels = []string{}
	}
	if task.Issues == nil {
		task.Issues = []int{}
	}

	if err := normalizeTask(&task); err != nil {
		return Task{}, err
	}

	return task, nil
}

// ApplyPatch overlays fields onto existing and returns the updated Task.
// Uses JSON round-trip: existing → raw members → each patch field overlaid as its own encoding → Task, preserving fields not in the patch.
// The members stay raw, so the existing entry's kept keys come back with their exact digits.
// Unknown-field validation is the caller's responsibility (store.validateUpsertFields).
func ApplyPatch(existing Task, fields map[string]any) (Task, error) {
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return Task{}, fmt.Errorf("marshal existing: %w", err)
	}

	var existingMap map[string]json.RawMessage
	err = json.Unmarshal(existingJSON, &existingMap)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal existing: %w", err)
	}

	for k, v := range fields {
		encoded, err := json.Marshal(v)
		if err != nil {
			return Task{}, fmt.Errorf("marshal field %q: %w", k, err)
		}
		existingMap[k] = encoded
	}

	mergedJSON, err := json.Marshal(existingMap)
	if err != nil {
		return Task{}, fmt.Errorf("marshal merged: %w", err)
	}

	var result Task
	err = json.Unmarshal(mergedJSON, &result)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal merged: %w", err)
	}

	if result.Labels == nil {
		result.Labels = []string{}
	}
	if result.Issues == nil {
		result.Issues = []int{}
	}

	if result.Slug == "" {
		return Task{}, fmt.Errorf("slug key is missing or empty after patch")
	}

	if err := validateSlugLength(result.Slug); err != nil {
		return Task{}, err
	}

	if err := normalizeTask(&result); err != nil {
		return Task{}, err
	}

	return result, nil
}
