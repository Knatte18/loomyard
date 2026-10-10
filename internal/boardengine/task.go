// task.go — the Task record stored in board.json.
//
// Defines the Task struct plus NewTask and ApplyPatch, which build/patch a Task from a raw field
// map via JSON round-trip so field types are validated exactly as they would be on disk.

package boardengine

import (
	"encoding/json"
	"fmt"
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

// normalizeTask stores a normal priority as the empty value, then checks that the Kind is one of the two kinds and the Priority one of the accepted values.
func normalizeTask(t *Task) error {
	if t.Kind != KindTask && t.Kind != KindNote {
		return fmt.Errorf("kind %q is not one of %s, %s", t.Kind, KindTask, KindNote)
	}
	if t.Priority == PriorityNormal {
		t.Priority = ""
	}
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
// Uses JSON round-trip: existing → map → overlay fields → Task, preserving fields not in the patch.
// Unknown-field validation is the caller's responsibility (store.validateUpsertFields).
func ApplyPatch(existing Task, fields map[string]any) (Task, error) {
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return Task{}, fmt.Errorf("marshal existing: %w", err)
	}

	var existingMap map[string]any
	err = json.Unmarshal(existingJSON, &existingMap)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal existing: %w", err)
	}

	for k, v := range fields {
		existingMap[k] = v
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
