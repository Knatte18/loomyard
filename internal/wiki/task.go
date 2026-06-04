package wiki

import (
	"encoding/json"
	"fmt"
)

type Task struct {
	ID        int      `json:"id"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	DependsOn []string `json:"depends_on"`
	Isolated  bool     `json:"isolated"`
	Deferred  bool     `json:"deferred"`
	Brief     string   `json:"brief"`
	Body      string   `json:"body"`
	Status    *string  `json:"status,omitempty"`
}

func NewTask(fields map[string]interface{}, nextID int) (Task, error) {
	if fields["group"] != nil {
		return Task{}, fmt.Errorf("group key is not allowed; use depends_on, isolated, deferred instead")
	}

	// Validate slug upfront
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}

	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return Task{}, fmt.Errorf("slug must be a non-empty string")
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

func ApplyPatch(existing Task, fields map[string]interface{}) (Task, error) {
	if fields["group"] != nil {
		return Task{}, fmt.Errorf("group key is not allowed; use depends_on, isolated, deferred instead")
	}

	// Marshal existing task to map
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return Task{}, fmt.Errorf("marshal existing: %w", err)
	}

	var existingMap map[string]interface{}
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

	return result, nil
}
