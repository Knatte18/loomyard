//go:build integration

// verbs_test.go — tests for the board verbs beyond the store verbs: promote, prune, find, retire-legacy, and the --text listing on list and find.
// The steps run from TestCLI in cli_test.go, which also defines cliFixture; runCLI is in cli_unit_test.go, same package.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
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

// stepRunSettings pins run_settings on the upsert verbs: a valid setting is stored migrated, each refusal names the module or the key, a task session's change is refused while an identical re-write passes, and a change on an entry past Seed-Child returns the notice.
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
		{name: "a closed key", payload: `{"slug":"bad","run_settings":{"loom":{"discussion_producer":"single"}}}`, wantErr: []string{"loom", "discussion_producer"}},
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

	t.Run("a task session's change is refused and an identical re-write passes", func(t *testing.T) {
		mustUpsert(t, loomBounces)
		t.Setenv(agentname.StrandNameEnv, "ly:task-a:loom")
		runJSON(t, 0, "upsert", loomBounces)
		result := runJSON(t, 1, "upsert", strings.Replace(loomBounces, `"review_max_bounces":2`, `"review_max_bounces":3`, 1))
		if message, _ := result["error"].(string); !strings.Contains(message, "ly:task-a:loom") {
			t.Errorf("error %q does not name the session", message)
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
}
