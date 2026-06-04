package wiki

import (
	"encoding/json"
	"fmt"
)

type Task struct {
	ID        int       `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	DependsOn []string  `json:"depends_on"`
	Isolated  bool      `json:"isolated"`
	Deferred  bool      `json:"deferred"`
	Brief     string    `json:"brief"`
	Body      string    `json:"body"`
	Status    *string   `json:"status,omitempty"`
}

func newTask(fields map[string]interface{}, nextID int) (Task, error) {
	if fields["group"] != nil {
		return Task{}, fmt.Errorf("group key is not allowed; use depends_on, isolated, deferred instead")
	}

	task := Task{
		ID:        nextID,
		DependsOn: []string{},
		Isolated:  false,
		Deferred:  false,
		Brief:     "",
		Body:      "",
		Status:    nil,
	}

	// Extract slug from fields
	slugVal, hasSlug := fields["slug"]
	if !hasSlug {
		return Task{}, fmt.Errorf("slug key is missing")
	}

	slugStr, ok := slugVal.(string)
	if !ok || slugStr == "" {
		return Task{}, fmt.Errorf("slug must be a non-empty string")
	}
	task.Slug = slugStr

	// JSON round-trip to merge remaining fields
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return Task{}, fmt.Errorf("marshal fields: %w", err)
	}

	var merged Task
	err = json.Unmarshal(fieldsJSON, &merged)
	if err != nil {
		return Task{}, fmt.Errorf("unmarshal fields: %w", err)
	}

	// Overlay the merged values onto defaults, preserving ID and Slug
	if merged.Title != "" {
		task.Title = merged.Title
	}
	if merged.DependsOn != nil {
		task.DependsOn = merged.DependsOn
	}
	if merged.Isolated {
		task.Isolated = merged.Isolated
	}
	if merged.Deferred {
		task.Deferred = merged.Deferred
	}
	if merged.Brief != "" {
		task.Brief = merged.Brief
	}
	if merged.Body != "" {
		task.Body = merged.Body
	}
	if merged.Status != nil {
		task.Status = merged.Status
	}

	return task, nil
}

func applyPatch(existing Task, fields map[string]interface{}) (Task, error) {
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
