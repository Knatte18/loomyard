// task.go — the Task record stored in tasks.json.
//
// Defines the Task struct plus NewTask and ApplyPatch, which build/patch a Task
// from a raw field map via JSON round-trip so field types are validated exactly
// as they would be on disk.

package boardengine

import (
	"encoding/json"
	"fmt"
)

// Task is the canonical record stored in tasks.json.
type Task struct {
	ID        int      `json:"id"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	DependsOn []string `json:"depends_on"`
	Isolated  bool     `json:"isolated"`
	Deferred  bool     `json:"deferred"`
	Brief     string   `json:"brief"`
	Body      string   `json:"body"`
	Status    *string  `json:"status,omitempty"`     // pointer: nil → field omitted in JSON; non-nil → status value present
	ShortName string   `json:"short_name,omitempty"` // optional short display label; falls back to Slug via ShortNameOrSlug
}

// ShortNameOrSlug returns t.ShortName when non-empty, otherwise t.Slug. This is
// the fallback a future display-label consumer (a VS Code window-title builder)
// will use, mirroring mill-config.yaml's repo.short_name -> window.title pattern
// one layer up; this method only adds and round-trips the field — the actual
// consumer is future work.
func (t Task) ShortNameOrSlug() string {
	if t.ShortName != "" {
		return t.ShortName
	}
	return t.Slug
}

// maxSlugLength caps a slug's length. Slugs seed worktree/portal/launcher
// directory names, and 32 chars gives headroom against Windows MAX_PATH issues
// in nested paths while comfortably fitting this repo's own real slugs
// (board, pattern-wiring, fabric-unified-view, codeintel-v1).
const maxSlugLength = 32

// validateSlugLength returns an error when slug exceeds maxSlugLength characters.
func validateSlugLength(slug string) error {
	if len(slug) > maxSlugLength {
		return fmt.Errorf("slug exceeds max length of %d characters: %q (%d chars)", maxSlugLength, slug, len(slug))
	}
	return nil
}

// NewTask builds a Task from a raw field map, assigning nextID.
// Uses JSON round-trip so field types are validated exactly as they would be on disk.
// Unknown-field validation is the caller's responsibility (store.validateUpsertFields).
func NewTask(fields map[string]any, nextID int) (Task, error) {
	// Validate slug upfront
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

	// Create default task with all fields initialized
	task := Task{
		ID:        nextID,
		DependsOn: []string{},
		Isolated:  false,
		Deferred:  false,
		Brief:     "",
		Body:      "",
		Status:    nil,
	}

	// Marshal fields to JSON and unmarshal directly into task
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return Task{}, fmt.Errorf("marshal fields: %w", err)
	}

	err = json.Unmarshal(fieldsJSON, &task)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal fields: %w", err)
	}

	// Force ID and slug to their intended values
	task.ID = nextID
	task.Slug = slugStr

	return task, nil
}

// ApplyPatch overlays fields onto existing and returns the updated Task.
// Uses JSON round-trip: existing → map → overlay fields → Task, preserving fields not in the patch.
// Unknown-field validation is the caller's responsibility (store.validateUpsertFields).
func ApplyPatch(existing Task, fields map[string]any) (Task, error) {
	// Marshal existing task to map
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return Task{}, fmt.Errorf("marshal existing: %w", err)
	}

	var existingMap map[string]any
	err = json.Unmarshal(existingJSON, &existingMap)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal existing: %w", err)
	}

	// Overlay fields onto existing map
	for k, v := range fields {
		existingMap[k] = v
	}

	// Marshal back to JSON and unmarshal into Task
	mergedJSON, err := json.Marshal(existingMap)
	if err != nil {
		return Task{}, fmt.Errorf("marshal merged: %w", err)
	}

	var result Task
	err = json.Unmarshal(mergedJSON, &result)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal merged: %w", err)
	}

	// Check slug is still present
	if result.Slug == "" {
		return Task{}, fmt.Errorf("slug key is missing or empty after patch")
	}

	if err := validateSlugLength(result.Slug); err != nil {
		return Task{}, err
	}

	return result, nil
}
