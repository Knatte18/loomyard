//go:build integration

// verbs_test.go — tests for the board verbs beyond the store verbs: promote, prune, find, retire-legacy, and the --text listing on list and find.
// seedCwd is defined in cli_test.go and runCLI in cli_unit_test.go, same package.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
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
	mustUpsert(t, `{"slug":"p","title":"P","labels":["bug"]}`)

	task := runJSON(t, 0, "promote", `{"slug":"p"}`)["task"].(map[string]any)
	if task["kind"] != "task" {
		t.Fatalf("promote: kind = %v, want task", task["kind"])
	}

	// A task comes back unchanged.
	task = runJSON(t, 0, "promote", `{"slug":"p"}`)["task"].(map[string]any)
	if task["kind"] != "task" || task["slug"] != "p" {
		t.Fatalf("second promote: task = %v, want p unchanged as a task", task)
	}

	// An id selects the entry as a slug does.
	mustUpsert(t, `{"slug":"q","title":"Q","labels":["bug"]}`)
	q := runJSON(t, 0, "get", `{"slug":"q"}`)["task"].(map[string]any)
	task = runJSON(t, 0, "promote", `{"id":`+formatNumber(q["id"].(float64))+`}`)["task"].(map[string]any)
	if task["slug"] != "q" || task["kind"] != "task" {
		t.Fatalf("promote by id: task = %v, want q as a task", task)
	}

	// Demotion goes through upsert.
	task = runJSON(t, 0, "upsert", `{"slug":"p","kind":"note"}`)["task"].(map[string]any)
	if task["kind"] != "note" {
		t.Fatalf("demotion via upsert: kind = %v, want note", task["kind"])
	}
}

func TestCLIPromote_Refusals(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)
	mustUpsert(t, `{"slug":"p","title":"P","labels":["bug"]}`)

	for name, payload := range map[string]string{
		"no payload key slug":    `{}`,
		"tier is an unknown key": `{"slug":"p","tier":1}`,
		"unknown key":            `{"slug":"p","kind":"task"}`,
		"slug and id":            `{"slug":"p","id":0}`,
		"unknown slug":           `{"slug":"missing"}`,
		"invalid json":           `{`,
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
	mustUpsert(t, `{"slug":"keep","title":"Keep","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"gone","title":"Gone","labels":["bug"]}`)
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
	mustUpsert(t, `{"slug":"needle-slug","title":"A","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"b","title":"has Needle title","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"c","title":"C","labels":["bug"],"brief":"needle in brief"}`)
	mustUpsert(t, `{"slug":"d","title":"D","labels":["bug"],"body":"body mentions needle"}`)
	mustUpsert(t, `{"slug":"e","title":"E","labels":["bug"],"body":"needle in a finished entry"}`)
	runJSON(t, 0, "set-status", `{"slug":"e","status":"done"}`)
	mustUpsert(t, `{"slug":"f","title":"unrelated","labels":["bug"]}`)

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

// seedUndecidedLabel rewrites the seeded board.yaml of cwd to configure the undecided label, which the template no longer does.
func seedUndecidedLabel(t *testing.T, cwd string) {
	t.Helper()
	content := "readme: Home.md\ndesign_prefix: proposal-\nlabels:\n  undecided: Not yet triaged\n"
	if err := os.WriteFile(configengine.ConfigFile(cwd, "board"), []byte(content), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}
}

func TestCLIListAndFindLabelFilter(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedUndecidedLabel(t, seedCwd(t))
	mustUpsert(t, `{"slug":"one","title":"One","kind":"note","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"two","title":"Two","kind":"note","labels":["bug","undecided"]}`)
	mustUpsert(t, `{"slug":"three","title":"Three","kind":"note","labels":["enhancement","undecided"]}`)

	if got := slugsOf(t, runJSON(t, 0, "list", "--label", "bug", "--label", "undecided")); len(got) != 1 || got[0] != "two" {
		t.Fatalf("list --label bug --label undecided: slugs = %v, want [two]", got)
	}
	if got := slugsOf(t, runJSON(t, 0, "find", "--label", "undecided", "t")); len(got) != 2 {
		t.Fatalf("find --label undecided t: slugs = %v, want two and three", got)
	}

	for _, args := range [][]string{{"list", "--label", "mystery"}, {"find", "--label", "mystery", "t"}} {
		result := runJSON(t, 1, args...)
		if ok, _ := result["ok"].(bool); ok {
			t.Fatalf("%v: expected ok=false, got %v", args, result)
		}
		if msg, _ := result["error"].(string); !strings.Contains(msg, "mystery") || !strings.Contains(msg, "board.yaml") {
			t.Fatalf("%v: error = %q, want one naming the label and board.yaml", args, msg)
		}
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
	seedUndecidedLabel(t, seedCwd(t))
	mustUpsert(t, `{"slug":"later","title":"Later thing","kind":"note","labels":["enhancement","undecided"]}`)
	mustUpsert(t, `{"slug":"soon","title":"Soon thing","kind":"task","labels":["bug"]}`)
	runJSON(t, 0, "set-status", `{"slug":"soon","status":"active"}`)

	wantList := "task  soon   Soon thing   bug                    [active]\n" +
		"note  later  Later thing  enhancement,undecided\n"
	exitCode, stdout := runCLI(t, "list", "--text")
	if exitCode != 0 || stdout != wantList {
		t.Fatalf("list --text: exit %d, stdout %q, want %q", exitCode, stdout, wantList)
	}

	wantFind := "note  later  Later thing  enhancement,undecided\n"
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
	mustUpsert(t, `{"slug":"x","title":"X","labels":["bug"]}`)
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
	mustUpsert(t, `{"slug":"by-id","title":"By ID","kind":"task","labels":["bug"],"recipe":"loom"}`)

	task := runJSON(t, 0, "get", `{"slug":"by-id"}`)["task"].(map[string]any)
	if task["kind"] != "task" {
		t.Fatalf("get: kind = %v, want task", task["kind"])
	}
	if labels, _ := task["labels"].([]any); len(labels) != 1 || labels[0] != "bug" || task["recipe"] != "loom" {
		t.Fatalf("get: labels/recipe = %v/%v, want [bug]/loom", task["labels"], task["recipe"])
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

// TestCLIUpsertBodyFile drives --body-file with a path and with stdin, then reads the body back with get.
func TestCLIUpsertBodyFile(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	path := filepath.Join(t.TempDir(), "body.md")
	fileBody := "# Heading\n\nA \"quoted\" line.\n"
	if err := os.WriteFile(path, []byte(fileBody), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	runJSON(t, 0, "upsert", `{"slug":"from-file","title":"F","labels":["bug"]}`, "--body-file", path)
	got := runJSON(t, 0, "get", `{"slug":"from-file"}`)["task"].(map[string]any)
	if got["body"] != fileBody {
		t.Fatalf("body from file = %q, want %q", got["body"], fileBody)
	}

	stdinBody := "piped body\n"
	pipeStdin(t, stdinBody)
	runJSON(t, 0, "upsert", `{"slug":"from-stdin","title":"S","labels":["bug"]}`, "--body-file", "-")
	got = runJSON(t, 0, "get", `{"slug":"from-stdin"}`)["task"].(map[string]any)
	if got["body"] != stdinBody {
		t.Fatalf("body from stdin = %q, want %q", got["body"], stdinBody)
	}

	// Refusals leave the board unchanged and exit non-zero.
	runJSON(t, 1, "upsert", `{"slug":"both","title":"B","body":"x","labels":["bug"]}`, "--body-file", path)
	runJSON(t, 1, "upsert", "-", "--body-file", "-")
}

// TestCLIMergeBodyFile drives merge --body-file with a path and with stdin, and refuses a body given twice.
func TestCLIMergeBodyFile(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	path := filepath.Join(t.TempDir(), "body.md")
	fileBody := "# Merged\n\nA \"quoted\" line.\n"
	if err := os.WriteFile(path, []byte(fileBody), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	mustUpsert(t, `{"slug":"old","title":"Old","labels":["bug"]}`)
	runJSON(t, 0, "merge", `{"remove_slugs":["old"],"upsert":{"slug":"merged","title":"M","labels":["bug"]}}`, "--body-file", path)
	got := runJSON(t, 0, "get", `{"slug":"merged"}`)["task"].(map[string]any)
	if got["body"] != fileBody {
		t.Fatalf("merged body from file = %q, want %q", got["body"], fileBody)
	}

	stdinBody := "piped body\n"
	pipeStdin(t, stdinBody)
	runJSON(t, 0, "merge", `{"upsert":{"slug":"from-stdin","title":"S","labels":["bug"]}}`, "--body-file", "-")
	got = runJSON(t, 0, "get", `{"slug":"from-stdin"}`)["task"].(map[string]any)
	if got["body"] != stdinBody {
		t.Fatalf("merged body from stdin = %q, want %q", got["body"], stdinBody)
	}

	// Refusals write nothing.
	mustUpsert(t, `{"slug":"keep","title":"Keep","labels":["bug"]}`)
	runJSON(t, 1, "merge", `{"remove_slugs":["keep"],"upsert":{"slug":"both","title":"B","body":"x","labels":["bug"]}}`, "--body-file", path)
	runJSON(t, 1, "merge", "-", "--body-file", "-")
	if got := runJSON(t, 0, "get", `{"slug":"keep"}`); got["task"] == nil {
		t.Fatal("refused merge removed keep")
	}
	if got := runJSON(t, 0, "get", `{"slug":"both"}`); got["task"] != nil {
		t.Fatalf("refused merge wrote both: %v", got)
	}
}

// TestCLIGetBody prints bodies byte-for-byte, an empty body as nothing, and refuses an absent target.
func TestCLIGetBody(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	for slug, body := range map[string]string{
		"multi":    "# Heading\n\nline one\nline two\n",
		"no-final": "no trailing newline",
	} {
		path := filepath.Join(t.TempDir(), "body.md")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write body file: %v", err)
		}
		runJSON(t, 0, "upsert", `{"slug":"`+slug+`","title":"T","labels":["bug"]}`, "--body-file", path)
		exitCode, stdout := runCLI(t, "get", `{"slug":"`+slug+`"}`, "--body")
		if exitCode != 0 || stdout != body {
			t.Fatalf("get --body %s: exit %d, stdout %q, want %q", slug, exitCode, stdout, body)
		}
	}

	mustUpsert(t, `{"slug":"empty","title":"E","labels":["bug"]}`)
	exitCode, stdout := runCLI(t, "get", `{"slug":"empty"}`, "--body")
	if exitCode != 0 || stdout != "" {
		t.Fatalf("get --body of an empty body: exit %d, stdout %q, want nothing", exitCode, stdout)
	}

	result := runJSON(t, 1, "get", `{"slug":"absent"}`, "--body")
	if msg, _ := result["error"].(string); !strings.Contains(msg, "absent") {
		t.Fatalf("absent slug: error = %q, want it to name the slug", msg)
	}
	if got := runJSON(t, 0, "get", `{"slug":"absent"}`); got["task"] != nil {
		t.Fatalf("flagless get of an absent slug: %v, want task:null", got)
	}
}

// TestCLIGetBodyRoundTrip feeds the bytes of get --body back through upsert --body-file and finds the body unchanged.
func TestCLIGetBodyRoundTrip(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	seedCwd(t)

	body := "# Heading\n\n- item \"one\"\n- item two\n\ntail\n"
	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write body file: %v", err)
	}
	runJSON(t, 0, "upsert", `{"slug":"rt","title":"R","labels":["bug"]}`, "--body-file", path)

	_, printed := runCLI(t, "get", `{"slug":"rt"}`, "--body")
	if err := os.WriteFile(path, []byte(printed), 0o644); err != nil {
		t.Fatalf("write printed body: %v", err)
	}
	runJSON(t, 0, "upsert", `{"slug":"rt"}`, "--body-file", path)
	got := runJSON(t, 0, "get", `{"slug":"rt"}`)["task"].(map[string]any)
	if got["body"] != body {
		t.Fatalf("body after round trip = %q, want %q", got["body"], body)
	}
}

// pipeStdin replaces os.Stdin with a file holding content until the test ends.
func pipeStdin(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write stdin file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open stdin file: %v", err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = orig
		f.Close()
	})
}

// labelPairs returns the label/description pairs of result[key], in order.
func labelPairs(t *testing.T, result map[string]any, key string) [][2]string {
	t.Helper()
	list, ok := result[key].([]any)
	if !ok {
		t.Fatalf("expected %s array, got %v", key, result)
	}
	pairs := make([][2]string, len(list))
	for i, v := range list {
		m := v.(map[string]any)
		pairs[i] = [2]string{m["label"].(string), m["description"].(string)}
	}
	return pairs
}

func TestCLILabelsMapShapedPrintsFileOrderWithDescriptions(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	cwd := seedCwd(t)
	content := "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  enhancement: A new capability\n  bug: Something broken\nlabels:\n  quarry: The code index\n  board: The task board\n"
	if err := os.WriteFile(configengine.ConfigFile(cwd, "board"), []byte(content), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	result := runJSON(t, 0, "labels")
	wantTypes := [][2]string{{"enhancement", "A new capability"}, {"bug", "Something broken"}}
	wantLabels := [][2]string{{"quarry", "The code index"}, {"board", "The task board"}}
	if got := labelPairs(t, result, "types"); !slices.Equal(got, wantTypes) {
		t.Fatalf("types = %v, want %v", got, wantTypes)
	}
	if got := labelPairs(t, result, "labels"); !slices.Equal(got, wantLabels) {
		t.Fatalf("labels = %v, want %v", got, wantLabels)
	}
}

func TestCLILabelsListShapedPrintsNamesWithEmptyDescriptions(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	cwd := seedCwd(t)
	content := "readme: Home.md\ndesign_prefix: proposal-\ntypes: [bug, enhancement]\nlabels: [quarry]\n"
	if err := os.WriteFile(configengine.ConfigFile(cwd, "board"), []byte(content), 0o644); err != nil {
		t.Fatalf("write board.yaml: %v", err)
	}

	result := runJSON(t, 0, "labels")
	wantTypes := [][2]string{{"bug", ""}, {"enhancement", ""}}
	wantLabels := [][2]string{{"quarry", ""}}
	if got := labelPairs(t, result, "types"); !slices.Equal(got, wantTypes) {
		t.Fatalf("types = %v, want %v", got, wantTypes)
	}
	if got := labelPairs(t, result, "labels"); !slices.Equal(got, wantLabels) {
		t.Fatalf("labels = %v, want %v", got, wantLabels)
	}
}
