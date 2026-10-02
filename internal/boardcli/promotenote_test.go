//go:build integration

// promotenote_test.go — tests for the top-level "promote-note" command
// (cli.go), which moves an entry to tier 1 via Board.PromoteNote.
// seedCwd and runCLI are defined in cli_test.go and
// cli_unit_test.go respectively, same package, directly callable.

package boardcli_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCLIPromoteNote seeds an entry via "notes upsert", promotes it, and asserts the promoted
// entry's tier is 1 and its fields round-trip, and that a second promote succeeds unchanged.
func TestCLIPromoteNote(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	if exitCode, stdout := runCLI(t, "notes", "upsert", `{"slug":"promote-me","title":"Promote Me","brief":"a brief"}`); exitCode != 0 {
		t.Fatalf("notes upsert: exit %d; stdout: %s", exitCode, stdout)
	}

	promote := func() map[string]any {
		t.Helper()
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
		return task
	}

	for _, call := range []string{"first", "second"} {
		task := promote()
		if slug, _ := task["slug"].(string); slug != "promote-me" {
			t.Fatalf("%s call: expected task.slug = %q, got %v", call, "promote-me", task["slug"])
		}
		if title, _ := task["title"].(string); title != "Promote Me" {
			t.Fatalf("%s call: expected task.title = %q, got %v", call, "Promote Me", task["title"])
		}
		if brief, _ := task["brief"].(string); brief != "a brief" {
			t.Fatalf("%s call: expected task.brief = %q, got %v", call, "a brief", task["brief"])
		}
		if tier, _ := task["tier"].(float64); tier != 1 {
			t.Fatalf("%s call: expected task.tier = 1, got %v", call, task["tier"])
		}
	}

	exitCode, stdout := runCLI(t, "get", `{"slug":"promote-me"}`)
	if exitCode != 0 {
		t.Fatalf("get after promote: exit %d; stdout: %s", exitCode, stdout)
	}
	var getResult map[string]any
	if err := json.Unmarshal([]byte(stdout), &getResult); err != nil {
		t.Fatalf("failed to parse get output: %v; stdout: %s", err, stdout)
	}
	getTask, ok := getResult["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task object after promote, got %v", getResult)
	}
	if tier, _ := getTask["tier"].(float64); tier != 1 {
		t.Fatalf("expected stored tier 1, got %v", getTask["tier"])
	}
}

// TestCLIPromoteNote_TierOneUnchanged asserts that promote-note on an entry already at tier 1
// succeeds and leaves it unchanged.
func TestCLIPromoteNote_TierOneUnchanged(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	if exitCode, stdout := runCLI(t, "upsert", `{"slug":"already-one","title":"Already One","tier":1}`); exitCode != 0 {
		t.Fatalf("upsert: exit %d; stdout: %s", exitCode, stdout)
	}

	exitCode, stdout := runCLI(t, "promote-note", `{"slug":"already-one"}`)
	if exitCode != 0 {
		t.Fatalf("promote-note: exit %d; stdout: %s", exitCode, stdout)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse promote-note output: %v; stdout: %s", err, stdout)
	}
	task, ok := result["task"].(map[string]any)
	if !ok {
		t.Fatalf("expected task object, got %v", result)
	}
	if tier, _ := task["tier"].(float64); tier != 1 {
		t.Fatalf("expected tier 1, got %v", task["tier"])
	}
	if title, _ := task["title"].(string); title != "Already One" {
		t.Fatalf("expected title unchanged, got %v", task["title"])
	}
}

// TestCLIPromoteNote_NeverANote asserts that promoting a slug absent from the board errors with a message containing "task not found".
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
	if !strings.Contains(errMsg, "task not found") {
		t.Fatalf("expected error to contain %q, got %q", "task not found", errMsg)
	}
}
