//go:build integration

// promotenote_test.go — tests for the top-level "promote-note" command
// (cli.go), which moves a note from notes.json into tasks.json via
// Board.PromoteNote. seedCwd and runCLI are defined in cli_test.go and
// cli_unit_test.go respectively, same package, directly callable.

package boardcli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCLIPromoteNote seeds a note via "notes upsert", promotes it, and
// asserts the move landed: the promoted task's fields round-trip, the slug
// is gone from notes.json (notes get returns task:null), and the same slug
// is now present via the task-verb "get" (tasks.json).
func TestCLIPromoteNote(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	if exitCode, stdout := runCLI(t, "notes", "upsert", `{"slug":"promote-me","title":"Promote Me","brief":"a brief"}`); exitCode != 0 {
		t.Fatalf("notes upsert: exit %d; stdout: %s", exitCode, stdout)
	}

	exitCode, stdout := runCLI(t, "promote-note", `{"slug":"promote-me"}`)
	if exitCode != 0 {
		t.Fatalf("promote-note: exit %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse promote-note output: %v; stdout: %s", err, stdout)
	}
	if ok, exists := result["ok"].(bool); !exists || !ok {
		t.Fatalf("expected ok=true, got %v", result)
	}
	task, ok := result["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task object in result, got %v", result)
	}
	if slug, _ := task["slug"].(string); slug != "promote-me" {
		t.Fatalf("expected task.slug = %q, got %v", "promote-me", task["slug"])
	}
	if title, _ := task["title"].(string); title != "Promote Me" {
		t.Fatalf("expected task.title = %q, got %v", "Promote Me", task["title"])
	}

	// The note must be gone from notes.json.
	exitCode, stdout = runCLI(t, "notes", "get", `{"slug":"promote-me"}`)
	if exitCode != 0 {
		t.Fatalf("notes get after promote: exit %d; stdout: %s", exitCode, stdout)
	}
	var notesGetResult map[string]any
	if err := json.Unmarshal([]byte(stdout), &notesGetResult); err != nil {
		t.Fatalf("failed to parse notes get output: %v; stdout: %s", err, stdout)
	}
	if taskField, exists := notesGetResult["task"]; !exists || taskField != nil {
		t.Fatalf("expected promoted note to be gone from notes.json, got %v", notesGetResult)
	}

	// The task-verb "get" must now return the promoted task with matching fields.
	exitCode, stdout = runCLI(t, "get", `{"slug":"promote-me"}`)
	if exitCode != 0 {
		t.Fatalf("get after promote: exit %d; stdout: %s", exitCode, stdout)
	}
	var getResult map[string]any
	if err := json.Unmarshal([]byte(stdout), &getResult); err != nil {
		t.Fatalf("failed to parse get output: %v; stdout: %s", err, stdout)
	}
	getTask, ok := getResult["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task object promoted into tasks.json, got %v", getResult)
	}
	if slug, _ := getTask["slug"].(string); slug != "promote-me" {
		t.Fatalf("expected promoted task.slug = %q, got %v", "promote-me", getTask["slug"])
	}
	if title, _ := getTask["title"].(string); title != "Promote Me" {
		t.Fatalf("expected promoted task.title = %q, got %v", "Promote Me", getTask["title"])
	}
	if brief, _ := getTask["brief"].(string); brief != "a brief" {
		t.Fatalf("expected promoted task.brief = %q, got %v", "a brief", getTask["brief"])
	}
}

// TestCLIPromoteNote_NeverANote asserts that promoting a slug that was never
// a note errors with a message containing "note not found".
func TestCLIPromoteNote_NeverANote(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	exitCode, stdout := runCLI(t, "promote-note", `{"slug":"never-a-note"}`)
	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d; stdout: %s", exitCode, stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse output: %v; stdout: %s", err, stdout)
	}
	if ok, exists := result["ok"].(bool); !exists || ok {
		t.Fatalf("expected ok=false, got %v", result)
	}
	errMsg, exists := result["error"].(string)
	if !exists {
		t.Fatalf("expected error message, got %v", result)
	}
	if !strings.Contains(errMsg, "note not found") {
		t.Fatalf("expected error to contain %q, got %q", "note not found", errMsg)
	}
}
