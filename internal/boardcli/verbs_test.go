//go:build integration

// verbs_test.go — tests for the board verbs beyond the store verbs: promote, rename, prune, find, retire-legacy, the --text listing on list and find, and list's --sort.
// The steps run from TestCLI in cli_test.go, which also defines cliFixture; runCLI is in cli_unit_test.go, same package.

package boardcli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
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

func stepPromote(t *testing.T, f *cliFixture) {
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

func stepPromote_Refusals(t *testing.T, f *cliFixture) {
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

// stepRename pins the rename envelope, the rewritten depends_on and the payload refusals.
func stepRename(t *testing.T, f *cliFixture) {
	mustUpsert(t, `{"slug":"before","title":"Before","kind":"task","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"after","title":"After","kind":"task","labels":["bug"],"depends_on":["before"],"body":"Waits on before."}`)

	result := runJSON(t, 0, "rename", `{"slug":"before","to":"renamed"}`)
	task := result["task"].(map[string]any)
	if task["slug"] != "renamed" || task["title"] != "Before" {
		t.Errorf("rename: task = %v, want slug renamed with title Before", task)
	}
	if mentions, _ := json.Marshal(result["mentions"]); string(mentions) != `["after"]` {
		t.Errorf("rename: mentions = %s, want [\"after\"]", mentions)
	}
	after := runJSON(t, 0, "get", `{"slug":"after"}`)["task"].(map[string]any)
	if deps, _ := json.Marshal(after["depends_on"]); string(deps) != `["renamed"]` {
		t.Errorf("rename: after.depends_on = %s, want [\"renamed\"]", deps)
	}

	renamed := runJSON(t, 0, "get", `{"slug":"renamed"}`)["task"].(map[string]any)
	result = runJSON(t, 0, "rename", `{"id":`+formatNumber(renamed["id"].(float64))+`,"to":"again"}`)
	if task := result["task"].(map[string]any); task["slug"] != "again" {
		t.Errorf("rename by id: task = %v, want slug again", task)
	}
	if mentions, _ := json.Marshal(result["mentions"]); string(mentions) != `[]` {
		t.Errorf("rename with no mention: mentions = %s, want []", mentions)
	}

	for name, payload := range map[string]string{
		"missing to":     `{"slug":"again"}`,
		"to not string":  `{"slug":"again","to":1}`,
		"unknown key":    `{"slug":"again","to":"x","title":"X"}`,
		"target exists":  `{"slug":"again","to":"after"}`,
		"unknown source": `{"slug":"missing","to":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if ok, _ := runJSON(t, 1, "rename", payload)["ok"].(bool); ok {
				t.Fatalf("rename %s: expected ok=false", payload)
			}
		})
	}
}

func stepPrune(t *testing.T, f *cliFixture) {
	mustUpsert(t, `{"slug":"keep","title":"Keep","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"gone","title":"Gone","labels":["bug"]}`)
	runJSON(t, 0, "set-status", `{"slug":"gone","status":"done"}`)

	// A bare prune is a dry run: it reports the done entry and leaves it listed.
	result := runJSON(t, 0, "prune")
	removed, _ := result["removed"].([]any)
	if len(removed) != 1 || removed[0] != "gone" || result["applied"] != false {
		t.Fatalf("dry run: removed = %v applied = %v, want [gone] false", result["removed"], result["applied"])
	}
	if got := slugsOf(t, runJSON(t, 0, "list")); len(got) != 2 {
		t.Fatalf("after dry run: slugs = %v, want both entries still listed", got)
	}

	result = runJSON(t, 0, "prune", "--apply")
	removed, _ = result["removed"].([]any)
	if len(removed) != 1 || removed[0] != "gone" || result["applied"] != true {
		t.Fatalf("removed = %v applied = %v, want [gone] true", result["removed"], result["applied"])
	}

	if got := slugsOf(t, runJSON(t, 0, "list")); len(got) != 1 || got[0] != "keep" {
		t.Fatalf("after prune: slugs = %v, want [keep]", got)
	}

	// A second prune has nothing to remove and reports an empty array.
	result = runJSON(t, 0, "prune", "--apply")
	if removed, ok := result["removed"].([]any); !ok || len(removed) != 0 {
		t.Fatalf("second prune: removed = %v, want []", result["removed"])
	}
}

func stepFind(t *testing.T, f *cliFixture) {
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
	seedHubBoardConfig(t, filepath.Dir(cwd), "readme: Home.md\ndesign_prefix: proposal-\nlabels:\n  undecided: Not yet triaged\n")
}

func stepListAndFindLabelFilter(t *testing.T, f *cliFixture) {
	seedUndecidedLabel(t, f.cwd)
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

func stepFind_NoArgumentRefused(t *testing.T, f *cliFixture) {

	result := runJSON(t, 1, "find")
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", result)
	}
}

func stepListAndFindText(t *testing.T, f *cliFixture) {
	seedUndecidedLabel(t, f.cwd)
	mustUpsert(t, `{"slug":"later","title":"Later thing","kind":"note","labels":["enhancement","undecided"]}`)
	mustUpsert(t, `{"slug":"soon","title":"Soon thing","kind":"task","labels":["bug"]}`)
	runJSON(t, 0, "set-status", `{"slug":"soon","status":"active"}`)

	wantList := "task  0m  0m  soon   Soon thing   bug                    [active]\n" +
		"note  0m  0m  later  Later thing  enhancement,undecided\n"
	exitCode, stdout := runCLI(t, "list", "--text")
	if exitCode != 0 || stdout != wantList {
		t.Fatalf("list --text: exit %d, stdout %q, want %q", exitCode, stdout, wantList)
	}

	wantFind := "note  0m  0m  later  Later thing  enhancement,undecided\n"
	exitCode, stdout = runCLI(t, "find", "--text", "later")
	if exitCode != 0 || stdout != wantFind {
		t.Fatalf("find --text: exit %d, stdout %q, want %q", exitCode, stdout, wantFind)
	}

	// A backfilled creation date reorders --sort age, oldest first, and leaves --sort revised in README order.
	mustUpsert(t, `{"slug":"later","created":"2026-01-02T03:04:05Z"}`)
	if got := slugsOf(t, runJSON(t, 0, "list", "--sort", "age")); !slices.Equal(got, []string{"later", "soon"}) {
		t.Fatalf("list --sort age: slugs = %v, want [later soon]", got)
	}
	if got := slugsOf(t, runJSON(t, 0, "list", "--sort", "revised")); !slices.Equal(got, []string{"soon", "later"}) {
		t.Fatalf("list --sort revised: slugs = %v, want [soon later]", got)
	}
	if result := runJSON(t, 1, "list", "--sort", "newest"); !strings.Contains(fmt.Sprint(result["error"]), "newest") {
		t.Fatalf("list --sort newest: result = %v, want a refusal naming the value", result)
	}
}

func stepRetireLegacy(t *testing.T, f *cliFixture) {
	cwd := f.cwd
	boardDir := fabricengine.BoardDir(filepath.Dir(cwd))

	// A board with no legacy files refuses.
	mustUpsert(t, `{"slug":"x","title":"X","labels":["bug"]}`)
	for _, args := range [][]string{{"retire-legacy"}, {"retire-legacy", "--apply"}} {
		result := runJSON(t, 1, args...)
		if ok, _ := result["ok"].(bool); ok {
			t.Fatalf("%v: expected refusal without legacy files, got %v", args, result)
		}
	}

	// Seeded legacy files are reported by a bare run and left on disk.
	for _, name := range []string{"tasks.json", "notes.json"} {
		if err := os.WriteFile(filepath.Join(boardDir, name), []byte(`[]`), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	dry := runJSON(t, 0, "retire-legacy")
	if files, _ := dry["legacy_files"].([]any); len(files) != 2 || files[0] != "tasks.json" || files[1] != "notes.json" || dry["applied"] != false {
		t.Fatalf("dry run: legacy_files = %v applied = %v, want [tasks.json notes.json] false", dry["legacy_files"], dry["applied"])
	}
	for _, name := range []string{"tasks.json", "notes.json"} {
		if _, err := os.Stat(filepath.Join(boardDir, name)); err != nil {
			t.Fatalf("%s removed by a dry run: %v", name, err)
		}
	}

	// --apply reports them again and removes them.
	applied := runJSON(t, 0, "retire-legacy", "--apply")
	if files, _ := applied["legacy_files"].([]any); len(files) != 2 || applied["applied"] != true {
		t.Fatalf("retire-legacy --apply: legacy_files = %v applied = %v, want two files and true", applied["legacy_files"], applied["applied"])
	}
	for _, name := range []string{"tasks.json", "notes.json"} {
		if _, err := os.Stat(filepath.Join(boardDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still present after retire-legacy (stat err: %v)", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(boardDir, "board.json")); err != nil {
		t.Fatalf("board.json missing after retire-legacy: %v", err)
	}
}

func stepGetAndRemoveByID(t *testing.T, f *cliFixture) {
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

// stepUpsertBodyFile drives --body-file with a path and with stdin, then reads the body back with get.
func stepUpsertBodyFile(t *testing.T, f *cliFixture) {

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

// stepMergeBodyFile drives merge --body-file with a path and with stdin, and refuses a body given twice.
func stepMergeBodyFile(t *testing.T, f *cliFixture) {

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

// stepGetBody prints bodies byte-for-byte, an empty body as nothing, and refuses an absent target.
func stepGetBody(t *testing.T, f *cliFixture) {

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

// stepGetBodyRoundTrip feeds the bytes of get --body back through upsert --body-file and finds the body unchanged.
func stepGetBodyRoundTrip(t *testing.T, f *cliFixture) {

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

func stepLabelsMapShapedPrintsFileOrderWithDescriptions(t *testing.T, f *cliFixture) {
	cwd := f.cwd
	content := "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  enhancement: A new capability\n  bug: Something broken\nlabels:\n  quarry: The code index\n  board: The task board\n"
	seedHubBoardConfig(t, filepath.Dir(cwd), content)

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

func stepLabelsListShapedPrintsNamesWithEmptyDescriptions(t *testing.T, f *cliFixture) {
	cwd := f.cwd
	content := "readme: Home.md\ndesign_prefix: proposal-\ntypes: [bug, enhancement]\nlabels: [quarry]\n"
	seedHubBoardConfig(t, filepath.Dir(cwd), content)

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

// stepProfiles pins the profiles verb: with no profiles.yaml it prints the template's starting profiles with their settings in name order, and a hub file without profiles prints an empty list.
func stepProfiles(t *testing.T, f *cliFixture) {
	result := runJSON(t, 0, "profiles")
	list, _ := result["profiles"].([]any)
	var names []string
	for _, entry := range list {
		profile, _ := entry.(map[string]any)
		name, _ := profile["name"].(string)
		names = append(names, name)
		if settings, _ := profile["settings"].(map[string]any); len(settings) == 0 {
			t.Errorf("profile %q has no settings; got %v", name, profile)
		}
	}
	if want := []string{"budget", "docs", "light"}; !slices.Equal(names, want) {
		t.Errorf("profile names = %v, want %v", names, want)
	}

	boardDir := fabricengine.BoardDir(filepath.Dir(f.cwd))
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "profiles"), []byte("# none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result = runJSON(t, 0, "profiles")
	if list, isList := result["profiles"].([]any); !isList || len(list) != 0 {
		t.Errorf("profiles = %v, want []", result["profiles"])
	}
}

// stepRunSettings pins run_settings and profile on the upsert verbs: a valid setting or known profile is stored, each refusal names the module, the key or the profile, a task session's change is refused while an identical re-write passes, and a change on an entry past Seed-Child returns the notice.
func stepRunSettings(t *testing.T, f *cliFixture) {
	const loomBounces = `{"slug":"rs","title":"RS","kind":"task","labels":["bug"],"run_settings":{"loom":{"review_max_bounces":2}}}`

	t.Run("a valid setting is stored", func(t *testing.T) {
		result := runJSON(t, 0, "upsert", loomBounces)
		task, _ := result["task"].(map[string]any)
		settings, _ := task["run_settings"].(map[string]any)
		loom, _ := settings["loom"].(map[string]any)
		if loom["review_max_bounces"] != float64(2) {
			t.Errorf("stored run_settings = %v, want loom.review_max_bounces 2", task["run_settings"])
		}
	})

	refusals := []struct {
		name    string
		payload string
		wantErr []string
	}{
		{name: "a non-object value", payload: `{"slug":"bad","run_settings":3}`, wantErr: []string{"run_settings"}},
		{name: "an unknown module", payload: `{"slug":"bad","run_settings":{"bogus":{"x":1}}}`, wantErr: []string{"bogus"}},
		{name: "a key absent from the template", payload: `{"slug":"bad","run_settings":{"loom":{"no_such_key":1}}}`, wantErr: []string{"loom", "no_such_key"}},
		{name: "a shape mismatch", payload: `{"slug":"bad","run_settings":{"loom":{"review_max_bounces":{"a":1}}}}`, wantErr: []string{"loom", "review_max_bounces"}},
		{name: "a closed key", payload: `{"slug":"bad","run_settings":{"loom":{"discussion_advisors":[{"name":"critic","model":"sonnet[low]"}]}}}`, wantErr: []string{"loom", "discussion_advisors"}},
		{name: "a profile that is not a string", payload: `{"slug":"bad","profile":3}`, wantErr: []string{"profile must be a string"}},
		{name: "an unknown profile", payload: `{"slug":"bad","profile":"heavy"}`, wantErr: []string{`"heavy"`, "known: budget, docs, light", "lyx config profiles --set heavy="}},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			result := runJSON(t, 1, "upsert", tt.payload)
			message, _ := result["error"].(string)
			for _, want := range tt.wantErr {
				if !strings.Contains(message, want) {
					t.Errorf("error %q lacks %q", message, want)
				}
			}
		})
	}

	batchAndMerge := []struct {
		name    string
		verb    string
		payload string
	}{
		{name: "upsert-batch", verb: "upsert-batch", payload: `{"tasks":[{"slug":"bad","profile":"heavy"}]}`},
		{name: "merge", verb: "merge", payload: `{"upsert":{"slug":"bad","title":"B","labels":["bug"],"profile":"heavy"}}`},
	}
	for _, tt := range batchAndMerge {
		t.Run(tt.name+" refuses an unknown profile", func(t *testing.T) {
			result := runJSON(t, 1, tt.verb, tt.payload)
			message, _ := result["error"].(string)
			for _, want := range []string{`"heavy"`, "known: budget, docs, light"} {
				if !strings.Contains(message, want) {
					t.Errorf("error %q lacks %q", message, want)
				}
			}
		})
	}

	t.Run("a task session's change is refused and an identical re-write passes", func(t *testing.T) {
		mustUpsert(t, loomBounces)
		t.Setenv(agentname.StrandNameEnv, "ly:task-a:loom")
		runJSON(t, 0, "upsert", loomBounces)
		result := runJSON(t, 1, "upsert", strings.Replace(loomBounces, `"review_max_bounces":2`, `"review_max_bounces":3`, 1))
		if message, _ := result["error"].(string); !strings.Contains(message, "ly:task-a:loom") {
			t.Errorf("error %q does not name the session", message)
		}
		result = runJSON(t, 1, "upsert", `{"slug":"rs","profile":"light"}`)
		if message, _ := result["error"].(string); !strings.Contains(message, "ly:task-a:loom") {
			t.Errorf("profile change error %q does not name the session", message)
		}
	})

	t.Run("a task session re-sending the stored profile passes", func(t *testing.T) {
		f.reset(t)
		mustUpsert(t, `{"slug":"rs","title":"RS","kind":"task","labels":["bug"],"profile":"light"}`)
		t.Setenv(agentname.StrandNameEnv, "ly:task-a:loom")
		runJSON(t, 0, "upsert", `{"slug":"rs","profile":"light"}`)
	})

	t.Run("a known profile is stored and an empty one clears it", func(t *testing.T) {
		f.reset(t)
		result := runJSON(t, 0, "upsert", `{"slug":"rs","title":"RS","kind":"task","labels":["bug"],"profile":"light"}`)
		if task, _ := result["task"].(map[string]any); task["profile"] != "light" {
			t.Errorf("stored task = %v, want profile light", task)
		}
		result = runJSON(t, 0, "upsert", `{"slug":"rs","profile":""}`)
		if task, _ := result["task"].(map[string]any); task["profile"] != nil {
			t.Errorf("stored task = %v, want no profile", task)
		}
	})

	notices := []struct {
		name       string
		status     string
		wantNotice bool
	}{
		{name: "past Seed-Child", status: "running · Discussion-Write", wantNotice: true},
		{name: "at Seed-Child", status: "running · Seed-Child"},
	}
	for _, tt := range notices {
		t.Run("a change "+tt.name+" returns the notice accordingly", func(t *testing.T) {
			f.reset(t)
			mustUpsert(t, loomBounces)
			runJSON(t, 0, "set-status", `{"slug":"rs","status":"`+tt.status+`"}`)
			result := runJSON(t, 0, "upsert", `{"slug":"rs","run_settings":{"loom":{"review_max_bounces":4}}}`)
			notice, hasNotice := result["notice"].(string)
			if hasNotice != tt.wantNotice || (tt.wantNotice && !strings.Contains(notice, "--layer local")) {
				t.Errorf("notice = %q (present %v), want present %v naming --layer local", notice, hasNotice, tt.wantNotice)
			}
		})
	}

	t.Run("a profile change past Seed-Child returns the notice", func(t *testing.T) {
		f.reset(t)
		mustUpsert(t, `{"slug":"rs","title":"RS","kind":"task","labels":["bug"],"profile":"light"}`)
		runJSON(t, 0, "set-status", `{"slug":"rs","status":"running · Discussion-Write"}`)
		result := runJSON(t, 0, "upsert", `{"slug":"rs","profile":"budget"}`)
		if notice, _ := result["notice"].(string); !strings.Contains(notice, "--layer local") {
			t.Errorf("notice = %q, want it to name --layer local", notice)
		}
		result = runJSON(t, 0, "upsert", `{"slug":"rs","profile":"budget"}`)
		if notice, has := result["notice"]; has {
			t.Errorf("unchanged profile returned notice %v, want none", notice)
		}
	})
}
