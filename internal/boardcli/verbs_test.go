// verbs_test.go tests the board verbs beyond the store verbs:
// promote, rename, prune, find, retire-legacy, the --text listing on list and find, and list's --sort.
// The steps run from TestCLI in cli_test.go, which also defines cliFixture; runCLI is in cli_unit_test.go, same package.

package boardcli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
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

	// --recipe sets the recipe in the promoting write.
	mustUpsert(t, `{"slug":"r","title":"R","labels":["bug"]}`)
	task = runJSON(t, 0, "promote", `{"slug":"r"}`, "--recipe", "darn")["task"].(map[string]any)
	if task["kind"] != "task" || task["recipe"] != "darn" {
		t.Fatalf("promote --recipe darn: task = %v, want r as a darn task", task)
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

// stepRaddle pins that raddle prints the stored proposal of a decision record or a filed slug verbatim.
// It also pins that raddle errors for a slug the raddle store does not hold, naming lym report raddle.
func stepRaddle(t *testing.T, f *cliFixture) {
	storeDir := boardengine.RaddleStoreDir(fabricengine.BoardDir(filepath.Dir(f.cwd)), "plan")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("create store directory: %v", err)
	}
	for name, content := range map[string]string{"store.json": `{"hash":"h","filed":["part"]}`, "proposal.yaml": "proposal: approved\n"} {
		if err := os.WriteFile(filepath.Join(storeDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	for _, slug := range []string{"plan", "part"} {
		if exitCode, stdout := runCLI(t, "raddle", slug); exitCode != 0 || stdout != "proposal: approved\n" {
			t.Errorf("raddle %s = exit %d, %q; want exit 0 and the stored proposal verbatim", slug, exitCode, stdout)
		}
	}

	result := runJSON(t, 1, "raddle", "unheld")
	if message, _ := result["error"].(string); !strings.Contains(message, "lym report raddle") {
		t.Errorf("raddle unheld = %v; want an error naming lym report raddle", result)
	}
}

// stepTaskSessionFilesNotesOnly pins that the verbs carry the caller's strand name to the board: a run agent's new task is refused and its note is accepted.
func stepTaskSessionFilesNotesOnly(t *testing.T, f *cliFixture) {
	t.Setenv(agentname.StrandNameEnv, "ly:task-a:loom")

	result := runJSON(t, 1, "upsert", `{"slug":"follow-up","title":"F","kind":"task","labels":["bug"]}`)
	if message, _ := result["error"].(string); !strings.Contains(message, "ly:task-a:loom") || !strings.Contains(message, `"kind":"note"`) {
		t.Errorf("error %q does not name the session and the note way forward", message)
	}
	runJSON(t, 0, "upsert", `{"slug":"follow-up","title":"F","kind":"note","labels":["bug"]}`)
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

// stepNotifyOnLandOnALandedEntry pins notify_on_land set after the run landed.
// The write on a done entry says so, prune keeps the entry while it carries the request,
// and a bare request on a slug the board does not hold is refused.
func stepNotifyOnLandOnALandedEntry(t *testing.T, f *cliFixture) {
	mustUpsert(t, `{"slug":"landed","title":"Landed","kind":"task","labels":["bug"]}`)
	runJSON(t, 0, "set-status", `{"slug":"landed","status":"done"}`)

	result := runJSON(t, 0, "upsert", `{"slug":"landed","notify_on_land":true}`)
	const wantNotice = `"landed" is done, so its run already landed: its parent's dobby sends the landing line at its next tick, or only clears notify_on_land when land_notice is off`
	if notice, _ := result["notice"].(string); notice != wantNotice {
		t.Errorf("notice = %q; want %q", notice, wantNotice)
	}

	result = runJSON(t, 0, "upsert", `{"slug":"landed","title":"Landed again"}`)
	if notice, _ := result["notice"].(string); notice != "" {
		t.Errorf("notice of a title-only upsert = %q; want none", notice)
	}

	result = runJSON(t, 0, "prune", "--apply")
	if removed, ok := result["removed"].([]any); !ok || len(removed) != 0 {
		t.Errorf("prune removed = %v; want [] while the done entry carries notify_on_land", result["removed"])
	}

	result = runJSON(t, 1, "upsert", `{"slug":"pruned","notify_on_land":true}`)
	if message, _ := result["error"].(string); !strings.Contains(message, `no entry "pruned" is on the board`) || !strings.Contains(message, "lym batten status pruned") {
		t.Errorf("error %q does not name the absent entry and lym batten status pruned", message)
	}
	if got := slugsOf(t, runJSON(t, 0, "list")); !slices.Equal(got, []string{"landed"}) {
		t.Errorf("slugs = %v; want [landed], the refused write filing nothing", got)
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

// seedUndecidedLabel rewrites the seeded board.yaml of cwd to configure the undecided label.
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

	wantList := "prime owner: none\n" +
		"T1  task  0m  0m  soon   Soon thing   bug                    [active]\n" +
		"E1  note  0m  0m  later  Later thing  enhancement,undecided\n"
	exitCode, stdout := runCLI(t, "list", "--text")
	if exitCode != 0 || stdout != wantList {
		t.Fatalf("list --text: exit %d, stdout %q, want %q", exitCode, stdout, wantList)
	}
	if got, present := runJSON(t, 0, "list")["prime_owner"]; !present || got != "" {
		t.Errorf("list: prime_owner = %v (present %t), want an empty string", got, present)
	}

	wantFind := "E1  note  0m  0m  later  Later thing  enhancement,undecided\n"
	exitCode, stdout = runCLI(t, "find", "--text", "later")
	if exitCode != 0 || stdout != wantFind {
		t.Fatalf("find --text: exit %d, stdout %q, want %q", exitCode, stdout, wantFind)
	}

	// A backfilled creation date reorders --sort age, oldest first, and leaves --sort revised in README order.
	// Both entries get the same revised stamp, since the upserts above can fall in different whole seconds.
	mustUpsert(t, `{"slug":"soon","revised":"2026-02-03T04:05:06Z"}`)
	mustUpsert(t, `{"slug":"later","created":"2026-01-02T03:04:05Z","revised":"2026-02-03T04:05:06Z"}`)
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

// stepListShowsASketch asserts the hub's board takes a note of the built-in sketch type, whatever its board.yaml lists,
// and that list --text lists it last under the sketches: line with handle S1.
func stepListShowsASketch(t *testing.T, f *cliFixture) {
	mustUpsert(t, `{"slug":"sketched","title":"Sketched thing","kind":"note","labels":["sketch"]}`)

	exitCode, stdout := runCLI(t, "list", "--text")
	if exitCode != 0 {
		t.Fatalf("list --text: exit %d, stdout %q", exitCode, stdout)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 2 || lines[len(lines)-2] != "sketches:" || !strings.HasPrefix(lines[len(lines)-1], "S1 ") || !strings.Contains(lines[len(lines)-1], "sketched") {
		t.Errorf("list --text = %q; want it to end with a sketches: line and the S1 sketched line", stdout)
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
	mustUpsert(t, `{"slug":"by-id","title":"By ID","kind":"task","labels":["bug"],"recipe":"tapestry"}`)

	task := runJSON(t, 0, "get", `{"slug":"by-id"}`)["task"].(map[string]any)
	if task["kind"] != "task" {
		t.Fatalf("get: kind = %v, want task", task["kind"])
	}
	if labels, _ := task["labels"].([]any); len(labels) != 1 || labels[0] != "bug" || task["recipe"] != "tapestry" {
		t.Fatalf("get: labels/recipe = %v/%v, want [bug]/tapestry", task["labels"], task["recipe"])
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

// stepOwners seeds a board.json that records a prime owner and a beam project's owner.
// It writes the file directly, since the owner setter is not the subject.
// It reads the owners back through get and list.
// A task outside every beam reads as the prime's owner, a member of the beam as the beam's, and a note carries none;
// list prints the prime owner as prime_owner and opens its --text listing with it.
func stepOwners(t *testing.T, f *cliFixture) {
	seedBoardFile(t, f.cwd, `{"version":1,"prime_owner":"neve","entries":[`+
		`{"id":0,"slug":"plain","title":"P","kind":"task","labels":["bug"]},`+
		`{"id":1,"slug":"owned","title":"O","kind":"task","labels":["bug"],"project":"beamed"},`+
		`{"id":2,"slug":"idea","title":"I","kind":"note","labels":["bug"]},`+
		`{"id":3,"slug":"beamed","title":"B","kind":"project","labels":["bug"],"beam":true,"owner_machine":"box"}]}`)
	want := map[string]any{"plain": "neve", "owned": "box", "idea": nil, "beamed": "box"}

	for slug, owner := range want {
		task := runJSON(t, 0, "get", `{"slug":"`+slug+`"}`)["task"].(map[string]any)
		if task["owner_machine"] != owner {
			t.Errorf("get %s: owner_machine = %v, want %v", slug, task["owner_machine"], owner)
		}
	}
	listed := runJSON(t, 0, "list")
	for _, entry := range listed["tasks"].([]any) {
		task := entry.(map[string]any)
		if got, slug := task["owner_machine"], task["slug"].(string); got != want[slug] {
			t.Errorf("list %s: owner_machine = %v, want %v", slug, got, want[slug])
		}
	}
	if got := listed["prime_owner"]; got != "neve" {
		t.Errorf("list: prime_owner = %v, want neve", got)
	}
	if exitCode, stdout := runCLI(t, "list", "--text"); exitCode != 0 || !strings.HasPrefix(stdout, "prime owner: neve\n") {
		t.Errorf("list --text: exit %d, stdout %q, want it to open with %q", exitCode, stdout, "prime owner: neve\n")
	}
}

// stepFrontTask drives the front and on_land guards through a seeded released front task.
// Every payload verb refuses a value other than the stored one and accepts the stored one unchanged.
// A change of the front task's project, kind or slug is refused with a removal command the remove verb accepts.
func stepFrontTask(t *testing.T, f *cliFixture) {
	seedBoardFile(t, f.cwd, `{"version":1,"draft_status":true,"entries":[`+
		`{"id":0,"slug":"fix","title":"Fix","kind":"task","labels":["bug"],"front":true,"on_land":"adopt-check"}]}`)

	refuse := func(t *testing.T, want string, args ...string) {
		t.Helper()
		result := runJSON(t, 1, args...)
		if msg, _ := result["error"].(string); !strings.Contains(msg, want) {
			t.Errorf("%v: error = %q, want it to carry %q", args, msg, want)
		}
	}
	const dropKeys = "drop the front and on_land keys from the payload"
	for _, key := range []string{`"front":false`, `"on_land":"x"`} {
		refuse(t, dropKeys, "upsert", `{"slug":"fix",`+key+`}`)
		refuse(t, dropKeys, "upsert-batch", `{"tasks":[{"slug":"fix",`+key+`}]}`)
		refuse(t, dropKeys, "merge", `{"upsert":{"slug":"fix",`+key+`}}`)
	}
	refuse(t, dropKeys, "upsert", `{"slug":"fix","front":"yes"}`)
	refuse(t, dropKeys, "upsert", `{"slug":"fix","on_land":7}`)

	runJSON(t, 0, "upsert", `{"slug":"fix","front":true,"on_land":"adopt-check"}`)
	if got := runJSON(t, 0, "get", `{"slug":"fix"}`)["task"].(map[string]any); got["front"] != true || got["on_land"] != "adopt-check" {
		t.Errorf("get after an echoing upsert: front, on_land = %v, %v; want true, adopt-check", got["front"], got["on_land"])
	}
	runJSON(t, 0, "upsert", `{"slug":"plain","title":"Plain","kind":"task","labels":["bug"],"front":false,"on_land":""}`)
	if got := runJSON(t, 0, "get", `{"slug":"plain"}`)["task"].(map[string]any); got["front"] != nil || got["on_land"] != nil {
		t.Errorf("get a new task filed with false and empty: front, on_land = %v, %v; want neither stored", got["front"], got["on_land"])
	}
	runJSON(t, 0, "upsert", `{"slug":"fix","title":"Fix again"}`)

	// The refusal's way forward is the removal below, so the payload it names is the one the remove verb is run with.
	const removePayload = `{"slug":"fix"}`
	const fixed = `lym board remove '` + removePayload + `'`
	refuse(t, fixed, "upsert", `{"slug":"fix","project":"other"}`)
	refuse(t, fixed, "upsert", `{"slug":"fix","kind":"note"}`)
	refuse(t, fixed, "rename", `{"slug":"fix","to":"moved"}`)

	runJSON(t, 0, "remove", removePayload)
	if got := runJSON(t, 0, "get", `{"slug":"fix"}`)["task"]; got != nil {
		t.Errorf("get after remove: task = %v; want none", got)
	}
}

// stepRemoveCommandParses runs the remove verb with the payload of each command boardengine.RemoveCommand renders, and expects it to remove the entry the slug names.
func stepRemoveCommandParses(t *testing.T, f *cliFixture) {
	slugs := []string{"fix", "adopt-fix-alpha-t1", "a-b-c-d"}
	entries := make([]string, len(slugs))
	for i, slug := range slugs {
		entries[i] = fmt.Sprintf(`{"id":%d,"slug":%q,"title":"T","kind":"task","labels":["bug"]}`, i, slug)
	}
	seedBoardFile(t, f.cwd, `{"version":1,"draft_status":true,"entries":[`+strings.Join(entries, ",")+`]}`)

	const prefix = "lym board remove '"
	for _, slug := range slugs {
		command := boardengine.RemoveCommand(slug)
		if !strings.HasPrefix(command, prefix) || !strings.HasSuffix(command, "'") {
			t.Fatalf("RemoveCommand(%q) = %q; want %s<payload>'", slug, command, prefix)
		}
		payload := strings.TrimSuffix(strings.TrimPrefix(command, prefix), "'")
		runJSON(t, 0, "remove", payload)
		if got := runJSON(t, 0, "get", fmt.Sprintf(`{"slug":%q}`, slug))["task"]; got != nil {
			t.Errorf("get %s after running %s: task = %v; want none", slug, command, got)
		}
	}
}

// stepProjects drives the project kind through a seeded hub:
// list and get carry parent_beam.
// It is empty under a grouping project, and it names the beam once the project is hand-marked a beam.
// A task slug ending in the records suffix, or one equal to a configured junction name, is refused with fabric's reason.
// A member's project change is refused while a worktree stands at its pair path;
// the refusal names `lym batten cancel <slug> --remove`.
// A plain directory at another entry's pair path is no pair, so its project change and rename go through.
func stepProjects(t *testing.T, f *cliFixture) {
	mustUpsert(t, `{"slug":"proj","title":"Proj","kind":"project","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"proj"}`)

	parentBeamOfListed := func(slug string) any {
		for _, entry := range runJSON(t, 0, "list")["tasks"].([]any) {
			if task := entry.(map[string]any); task["slug"] == slug {
				return task["parent_beam"]
			}
		}
		t.Fatalf("list: no entry %q", slug)
		return nil
	}
	if got := runJSON(t, 0, "get", `{"slug":"member"}`)["parent_beam"]; got != "" {
		t.Errorf("get member under a grouping project: parent_beam = %v, want empty", got)
	}
	if got := parentBeamOfListed("member"); got != nil {
		t.Errorf("list member under a grouping project: parent_beam = %v, want none", got)
	}

	seedBoardFile(t, f.cwd, `{"version":1,"entries":[`+
		`{"id":0,"slug":"proj","title":"Proj","kind":"project","labels":["bug"],"beam":true},`+
		`{"id":1,"slug":"other","title":"Other","kind":"project","labels":["bug"]},`+
		`{"id":2,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"proj"}]}`)
	if got := runJSON(t, 0, "get", `{"slug":"member"}`)["parent_beam"]; got != "proj" {
		t.Errorf("get member of a beam: parent_beam = %v, want proj", got)
	}
	if got := parentBeamOfListed("member"); got != "proj" {
		t.Errorf("list member of a beam: parent_beam = %v, want proj", got)
	}
	if got := runJSON(t, 0, "get", `{"slug":"proj"}`)["parent_beam"]; got != "" {
		t.Errorf("get the beam itself: parent_beam = %v, want empty", got)
	}

	if err := os.WriteFile(configengine.ConfigFile(fabricengine.BoardDir(filepath.Dir(f.cwd)), "fabric"), []byte("branch_prefix: \"\"\npathspec: docs\n"), 0o644); err != nil {
		t.Fatalf("write fabric.yaml: %v", err)
	}
	refusals := []struct {
		slug string
		want string
	}{
		{fabricengine.RecordsBranchName("made"), "must not end in"},
		{"docs", "reserved for lym hub geometry"},
	}
	for _, tt := range refusals {
		result := runJSON(t, 1, "upsert", `{"slug":"`+tt.slug+`","title":"T","kind":"task","labels":["bug"]}`)
		if msg, _ := result["error"].(string); !strings.Contains(msg, tt.want) {
			t.Errorf("upsert slug %q: error = %q, want it to carry %q", tt.slug, msg, tt.want)
		}
	}

	pairDir := filepath.Join(filepath.Dir(f.cwd), "member")
	for _, args := range [][]string{
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--allow-empty", "-m", "init"},
		{"worktree", "add", "--detach", pairDir},
	} {
		if out, err := exec.Command("git", append([]string{"-C", f.cwd}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	result := runJSON(t, 1, "upsert", `{"slug":"member","project":"other"}`)
	if msg, _ := result["error"].(string); !strings.Contains(msg, "lym batten cancel member --remove") {
		t.Errorf("upsert a project change on an entry with a pair: error = %q, want it to name `lym batten cancel member --remove`", msg)
	}

	strayDir := filepath.Join(filepath.Dir(f.cwd), "strayed", "obj")
	if err := os.MkdirAll(strayDir, 0o755); err != nil {
		t.Fatalf("create a plain directory at a pair's path: %v", err)
	}
	mustUpsert(t, `{"slug":"strayed","title":"Strayed","kind":"task","labels":["bug"]}`)
	if task, _ := runJSON(t, 0, "upsert", `{"slug":"strayed","project":"other"}`)["task"].(map[string]any); task["project"] != "other" {
		t.Errorf("upsert a project change on an entry whose pair path holds a plain directory: task = %v, want project other", task)
	}
	if task, _ := runJSON(t, 0, "rename", `{"slug":"strayed","to":"strayed-again"}`)["task"].(map[string]any); task["slug"] != "strayed-again" {
		t.Errorf("rename an entry whose pair path holds a plain directory: task = %v, want slug strayed-again", task)
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

// stepGetRecord pins the discussed flag in the get envelope and the record that get --record prints.
// The record prints byte for byte, and each case with no record to print is refused.
// The record is stored through the board engine on the fixture's hub board.
// The engine honours the BOARD_SKIP_GIT the test sets and spawns no sync.
func stepGetRecord(t *testing.T, f *cliFixture) {
	hub := filepath.Dir(f.cwd)
	mustUpsert(t, `{"slug":"talked","title":"T","labels":["bug"]}`)
	mustUpsert(t, `{"slug":"plain","title":"P","labels":["bug"]}`)
	board, err := boardengine.OpenHub(hub)
	if err != nil {
		t.Fatalf("OpenHub: %v", err)
	}
	record := "# Decision record\n\nno trailing newline"
	if err := board.StoreDecisionRecord("talked", []byte(record)); err != nil {
		t.Fatalf("StoreDecisionRecord: %v", err)
	}

	if task := runJSON(t, 0, "get", `{"slug":"talked"}`)["task"].(map[string]any); task["discussed"] != true {
		t.Errorf("get of a discussed entry: discussed = %v; want true", task["discussed"])
	}
	if task := runJSON(t, 0, "get", `{"slug":"plain"}`)["task"].(map[string]any); task["discussed"] != nil {
		t.Errorf("get of an undiscussed entry: discussed = %v; want no key", task["discussed"])
	}
	if exitCode, stdout := runCLI(t, "get", `{"slug":"talked"}`, "--record"); exitCode != 0 || stdout != record {
		t.Fatalf("get --record: exit %d, stdout %q; want 0 and %q", exitCode, stdout, record)
	}

	if err := os.Remove(filepath.Join(fabricengine.BoardDir(hub), "decision-talked.md")); err != nil {
		t.Fatalf("remove decision-talked.md: %v", err)
	}
	refusals := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"UndiscussedEntry", []string{"get", `{"slug":"plain"}`, "--record"}, "lym loom decision store"},
		{"AbsentEntry", []string{"get", `{"slug":"absent"}`, "--record"}, "check the slug, id or handle with lym board list"},
		{"FlaggedEntryWhoseFileWasDeleted", []string{"get", `{"slug":"talked"}`, "--record"}, "has no decision-talked.md"},
		{"BesideBody", []string{"get", `{"slug":"talked"}`, "--record", "--body"}, "pass one of them"},
	}
	for _, tt := range refusals {
		result := runJSON(t, 1, tt.args...)
		if msg, _ := result["error"].(string); !strings.Contains(msg, tt.wantErr) {
			t.Errorf("%s: error = %q; want one containing %q", tt.name, msg, tt.wantErr)
		}
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

// sketchPair returns the first type label the labels verb printed.
// It fails t unless that label is the built-in sketch type with a description, which leads every board's types.
func sketchPair(t *testing.T, result map[string]any) [2]string {
	t.Helper()
	pairs := labelPairs(t, result, "types")
	if len(pairs) == 0 || pairs[0][0] != "sketch" || pairs[0][1] == "" {
		t.Fatalf("types = %v; want the described sketch type label first", pairs)
	}
	return pairs[0]
}

func stepLabelsMapShapedPrintsFileOrderWithDescriptions(t *testing.T, f *cliFixture) {
	cwd := f.cwd
	content := "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  enhancement: A new capability\n  bug: Something broken\nlabels:\n  quarry: The code index\n  board: The task board\n"
	seedHubBoardConfig(t, filepath.Dir(cwd), content)

	result := runJSON(t, 0, "labels")
	wantTypes := [][2]string{sketchPair(t, result), {"enhancement", "A new capability"}, {"bug", "Something broken"}}
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
	wantTypes := [][2]string{sketchPair(t, result), {"bug", ""}, {"enhancement", ""}}
	wantLabels := [][2]string{{"quarry", ""}}
	if got := labelPairs(t, result, "types"); !slices.Equal(got, wantTypes) {
		t.Fatalf("types = %v, want %v", got, wantTypes)
	}
	if got := labelPairs(t, result, "labels"); !slices.Equal(got, wantLabels) {
		t.Fatalf("labels = %v, want %v", got, wantLabels)
	}
}

// stepProfiles pins the profiles verb.
// With no profiles.yaml it prints the template's starting profiles in name order, each with its settings.
// It also prints the template's defaults.
// A hub file without profiles prints an empty list.
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
	if want := []string{"docs", "light", "solid", "speed", "wide"}; !slices.Equal(names, want) {
		t.Errorf("profile names = %v, want %v", names, want)
	}
	if want := map[string]any{"tapestry": "solid", "motif": "solid", "darn": "light", "pawl": "light", "raddle": "light"}; !reflect.DeepEqual(result["defaults"], want) {
		t.Errorf("defaults = %v, want %v", result["defaults"], want)
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

// stepRunSettings pins run_settings and profile on the upsert verbs.
// A valid setting or known profile is stored, and each refusal names the module, the key or the profile.
// A task session's change is refused, while an identical re-write passes.
// A beam's orch changes the entries of its own beam and no other.
// A change on an entry past Seed-Child returns the notice.
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
		{name: "an unknown profile", payload: `{"slug":"bad","profile":"heavy"}`, wantErr: []string{`"heavy"`, "known: docs, light, solid, speed, wide", "lym config profiles --set heavy="}},
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
			for _, want := range []string{`"heavy"`, "known: docs, light, solid, speed, wide"} {
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

	t.Run("a beam's orch changes the entries of its own beam only", func(t *testing.T) {
		f.reset(t)
		seedBoardFile(t, f.cwd, `{"version":1,"entries":[`+
			`{"id":0,"slug":"bm","title":"Beam","kind":"project","labels":["bug"],"beam":true},`+
			`{"id":1,"slug":"grp","title":"Group","kind":"project","labels":["bug"],"project":"bm"},`+
			`{"id":2,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"bm"},`+
			`{"id":3,"slug":"outside","title":"Outside","kind":"task","labels":["bug"]}]}`)
		t.Setenv(agentname.StrandNameEnv, "ly:bm:orch")
		runJSON(t, 0, "upsert", `{"slug":"new","title":"New","kind":"task","labels":["bug"],"project":"bm","profile":"docs"}`)
		runJSON(t, 0, "upsert", `{"slug":"deep","title":"Deep","kind":"task","labels":["bug"],"project":"grp","profile":"docs"}`)
		runJSON(t, 0, "upsert", `{"slug":"member","profile":"light"}`)
		if task, _ := runJSON(t, 0, "get", `{"slug":"new"}`)["task"].(map[string]any); task["profile"] != "docs" {
			t.Errorf("stored task = %v, want profile docs", task)
		}
		t.Setenv(agentname.StrandNameEnv, "ly:bm:orch-2")
		runJSON(t, 0, "upsert", `{"slug":"member","profile":"speed"}`)

		refusals := []struct {
			name    string
			strand  string
			payload string
		}{
			{name: "an entry outside the beam", strand: "ly:bm:orch", payload: `{"slug":"outside","profile":"docs"}`},
			{name: "a new entry under no project", strand: "ly:bm:orch", payload: `{"slug":"loose","title":"Loose","kind":"task","labels":["bug"],"profile":"docs"}`},
			{name: "a member moved out of the beam", strand: "ly:bm:orch", payload: `{"slug":"member","project":"","profile":"solid"}`},
			{name: "an entry moved into the beam", strand: "ly:bm:orch", payload: `{"slug":"outside","project":"bm","profile":"docs"}`},
			{name: "a project link that is not a string", strand: "ly:bm:orch", payload: `{"slug":"member","project":5,"profile":"docs"}`},
			{name: "the beam itself", strand: "ly:bm:orch", payload: `{"slug":"bm","profile":"docs"}`},
			{name: "another role in the beam", strand: "ly:bm:loom", payload: `{"slug":"member","profile":"solid"}`},
			{name: "the orch of another beam", strand: "ly:other:orch", payload: `{"slug":"member","profile":"solid"}`},
		}
		for _, tt := range refusals {
			t.Run("refuses "+tt.name, func(t *testing.T) {
				t.Setenv(agentname.StrandNameEnv, tt.strand)
				result := runJSON(t, 1, "upsert", tt.payload)
				if message, _ := result["error"].(string); !strings.Contains(message, tt.strand) || !strings.Contains(message, "the orch of the entry's beam") {
					t.Errorf("error %q does not name the session and the way forward", message)
				}
			})
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
		result := runJSON(t, 0, "upsert", `{"slug":"rs","profile":"solid"}`)
		if notice, _ := result["notice"].(string); !strings.Contains(notice, "--layer local") {
			t.Errorf("notice = %q, want it to name --layer local", notice)
		}
		result = runJSON(t, 0, "upsert", `{"slug":"rs","profile":"solid"}`)
		if notice, has := result["notice"]; has {
			t.Errorf("unchanged profile returned notice %v, want none", notice)
		}
	})
}
