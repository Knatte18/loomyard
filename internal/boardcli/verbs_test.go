//go:build integration

// verbs_test.go — tests for the verbs added with the tiered board: promote, prune, find,
// retire-legacy, and the --text listing on list and find.
// seedCwd is defined in cli_test.go and runCLI in cli_unit_test.go, same package.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// runJSON runs the CLI and decodes its stdout as one JSON object.
func runJSON(t *testing.T, wantExit int, args ...string) map[string]any {
	t.Helper()
	exitCode, stdout := runCLI(t, args...)
	if exitCode != wantExit {
		t.Fatalf("%v: exit %d, want %d; stdout: %s", args, exitCode, wantExit, stdout)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("%v: parse output: %v; stdout: %s", args, err, stdout)
	}
	return result
}

// mustUpsert upserts one entry from a JSON payload.
func mustUpsert(t *testing.T, payload string) {
	t.Helper()
	runJSON(t, 0, "upsert", payload)
}

// slugsOf returns the slug of each element of result["tasks"], in order.
func slugsOf(t *testing.T, result map[string]any) []string {
	t.Helper()
	tasks, ok := result["tasks"].([]any)
	if !ok {
		t.Fatalf("expected tasks array, got %v", result)
	}
	slugs := make([]string, len(tasks))
	for i, v := range tasks {
		slugs[i], _ = v.(map[string]any)["slug"].(string)
	}
	return slugs
}

func TestCLIPromote(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"p","title":"P","tier":3}`)

	// Default: one tier lower.
	task := runJSON(t, 0, "promote", `{"slug":"p"}`)["task"].(map[string]any)
	if tier, _ := task["tier"].(float64); tier != 2 {
		t.Fatalf("default promote: tier = %v, want 2", task["tier"])
	}

	// Explicit tier.
	task = runJSON(t, 0, "promote", `{"slug":"p","tier":1}`)["task"].(map[string]any)
	if tier, _ := task["tier"].(float64); tier != 1 {
		t.Fatalf("explicit promote: tier = %v, want 1", task["tier"])
	}
}

func TestCLIPromote_Refusals(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"p","title":"P","tier":2}`)

	for name, payload := range map[string]string{
		"no payload key slug": `{"tier":1}`,
		"unknown key":         `{"slug":"p","tiers":1}`,
		"non-integer tier":    `{"slug":"p","tier":1.5}`,
		"string tier":         `{"slug":"p","tier":"1"}`,
		"not lower":           `{"slug":"p","tier":2}`,
		"demotion":            `{"slug":"p","tier":3}`,
		"unknown slug":        `{"slug":"missing"}`,
		"invalid json":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			result := runJSON(t, 1, "promote", payload)
			if ok, _ := result["ok"].(bool); ok {
				t.Fatalf("expected ok=false, got %v", result)
			}
		})
	}

	if exitCode, _ := runCLI(t, "promote"); exitCode != 1 {
		t.Fatalf("promote with no payload: exit %d, want 1", exitCode)
	}
}

func TestCLIPrune(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"keep","title":"Keep"}`)
	mustUpsert(t, `{"slug":"gone","title":"Gone"}`)
	runJSON(t, 0, "set-status", `{"slug":"gone","status":"done"}`)

	result := runJSON(t, 0, "prune")
	removed, _ := result["removed"].([]any)
	if len(removed) != 1 || removed[0] != "gone" {
		t.Fatalf("removed = %v, want [gone]", result["removed"])
	}

	if got := slugsOf(t, runJSON(t, 0, "list")); len(got) != 1 || got[0] != "keep" {
		t.Fatalf("after prune: slugs = %v, want [keep]", got)
	}

	// A second prune has nothing to remove and reports an empty array.
	result = runJSON(t, 0, "prune")
	if removed, ok := result["removed"].([]any); !ok || len(removed) != 0 {
		t.Fatalf("second prune: removed = %v, want []", result["removed"])
	}
}

func TestCLIFind(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"needle-slug","title":"A"}`)
	mustUpsert(t, `{"slug":"b","title":"has Needle title"}`)
	mustUpsert(t, `{"slug":"c","title":"C","brief":"needle in brief"}`)
	mustUpsert(t, `{"slug":"d","title":"D","body":"body mentions needle"}`)
	mustUpsert(t, `{"slug":"e","title":"E","body":"needle in a finished entry"}`)
	runJSON(t, 0, "set-status", `{"slug":"e","status":"done"}`)
	mustUpsert(t, `{"slug":"f","title":"unrelated"}`)

	got := slugsOf(t, runJSON(t, 0, "find", "needle"))
	if len(got) != 5 {
		t.Fatalf("find needle: slugs = %v, want the five matching entries including the done one", got)
	}
	for _, slug := range got {
		if slug == "f" {
			t.Fatalf("find needle matched the unrelated entry: %v", got)
		}
	}

	// Several arguments join with single spaces into one search text.
	if got := slugsOf(t, runJSON(t, 0, "find", "needle", "in", "brief")); len(got) != 1 || got[0] != "c" {
		t.Fatalf("find needle in brief: slugs = %v, want [c]", got)
	}
}

func TestCLIFind_NoArgumentRefused(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	result := runJSON(t, 1, "find")
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", result)
	}
}

func TestCLIListAndFindText(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"later","title":"Later thing","tier":3,"type":"chore"}`)
	mustUpsert(t, `{"slug":"soon","title":"Soon thing","tier":1,"type":"bug"}`)
	runJSON(t, 0, "set-status", `{"slug":"soon","status":"active"}`)

	wantList := "1  bug    soon   Soon thing   [active]\n" +
		"3  chore  later  Later thing\n"
	exitCode, stdout := runCLI(t, "list", "--text")
	if exitCode != 0 || stdout != wantList {
		t.Fatalf("list --text: exit %d, stdout %q, want %q", exitCode, stdout, wantList)
	}

	wantFind := "3  chore  later  Later thing\n"
	exitCode, stdout = runCLI(t, "find", "--text", "later")
	if exitCode != 0 || stdout != wantFind {
		t.Fatalf("find --text: exit %d, stdout %q, want %q", exitCode, stdout, wantFind)
	}
}

func TestCLIRetireLegacy(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	cwd := seedCwd(t)
	boardDir := fabricengine.BoardDir(filepath.Dir(cwd))

	// A board with no legacy files refuses.
	mustUpsert(t, `{"slug":"x","title":"X"}`)
	result := runJSON(t, 1, "retire-legacy")
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected refusal without legacy files, got %v", result)
	}

	// Seeded legacy files are removed.
	for _, name := range []string{"tasks.json", "notes.json"} {
		if err := os.WriteFile(filepath.Join(boardDir, name), []byte(`[]`), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	runJSON(t, 0, "retire-legacy")
	for _, name := range []string{"tasks.json", "notes.json"} {
		if _, err := os.Stat(filepath.Join(boardDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still present after retire-legacy (stat err: %v)", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(boardDir, "board.json")); err != nil {
		t.Fatalf("board.json missing after retire-legacy: %v", err)
	}
}

func TestCLIGetAndRemoveByID(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"by-id","title":"By ID","tier":2,"type":"bug","recipe":"loom"}`)

	task := runJSON(t, 0, "get", `{"slug":"by-id"}`)["task"].(map[string]any)
	if tier, _ := task["tier"].(float64); tier != 2 {
		t.Fatalf("get: tier = %v, want 2", task["tier"])
	}
	if task["type"] != "bug" || task["recipe"] != "loom" {
		t.Fatalf("get: type/recipe = %v/%v, want bug/loom", task["type"], task["recipe"])
	}
	id := task["id"].(float64)

	byID := runJSON(t, 0, "get", `{"id":`+formatNumber(id)+`}`)["task"].(map[string]any)
	if byID["slug"] != "by-id" {
		t.Fatalf("get by id: slug = %v, want by-id", byID["slug"])
	}

	runJSON(t, 0, "remove", `{"id":`+formatNumber(id)+`}`)
	if got := runJSON(t, 0, "get", `{"slug":"by-id"}`)["task"]; got != nil {
		t.Fatalf("after remove by id: task = %v, want null", got)
	}
}

// formatNumber renders a decoded JSON integer without a fractional part.
func formatNumber(f float64) string {
	b, _ := json.Marshal(int(f))
	return string(b)
}
