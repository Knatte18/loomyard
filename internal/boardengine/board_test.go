// board_test.go — unit tests for the Board facade (board.go).
//
// Upsert / remove / rerender against a temp board with git skipped.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	flock "github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
)

// testTypes and testLabels are the vocabulary every facade test's Config carries; bugLabels is a valid label set for an entry.
var (
	testTypes  = []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}}
	testLabels = []boardengine.Label{{Name: "undecided"}}
	bugLabels  = []string{"bug"}
)

// TestUpsertTask tests the facade persistence wiring:creating a task writes both board.json and Home.md.
// Drop: store-layer assertion "update preserves fields" (owned by
// store_test.go:TestUpsertTaskPreservesFields).
func TestUpsertTask(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
	w := boardengine.New(cfg)

	// Creates task, board.json written, Home.md written
	task, err := w.UpsertTask(map[string]any{
		"slug":   "test-task",
		"title":  "Test Task",
		"labels": bugLabels,
	})
	if err != nil {
		t.Fatalf("UpsertTask failed: %v", err)
	}

	if task.Slug != "test-task" || task.Title != "Test Task" {
		t.Fatalf("Task not created correctly: %+v", task)
	}

	// Check board.json exists
	boardJSONPath := filepath.Join(boardPath, "board.json")
	if _, err := os.Stat(boardJSONPath); err != nil {
		t.Fatalf("board.json not created: %v", err)
	}

	// Check Home.md exists
	homePath := filepath.Join(boardPath, "Home.md")
	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}

// TestUpsertTaskUnconfiguredOutputsFailsBeforeWriting locks in boardCriticalSection's fail-fast guard:
// a Board built without output filenames (the --board-path shape) must reject a write before touching disk, never save board.json and then fail the render on an empty filename.
func TestUpsertTaskUnconfiguredOutputsFailsBeforeWriting(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, SkipGit: true}
	w := boardengine.New(cfg)

	_, err := w.UpsertTask(map[string]any{"slug": "test-task", "title": "Test Task"})
	if err == nil {
		t.Fatalf("UpsertTask without configured outputs should error")
	}

	// The half-applied failure mode this guards against saved board.json first;
	// the guard must fire before any disk mutation.
	if _, statErr := os.Stat(filepath.Join(boardPath, "board.json")); !os.IsNotExist(statErr) {
		t.Fatalf("board.json must not be created on a rejected write; stat err = %v", statErr)
	}
}

func TestRerender(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
	w := boardengine.New(cfg)

	// (d) Writes all output files without error on empty store
	err := w.Rerender()
	if err != nil {
		t.Fatalf("Rerender failed: %v", err)
	}

	// Check that Home.md exists
	homePath := filepath.Join(boardPath, "Home.md")

	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("Home.md not created: %v", err)
	}
}

// TestHealthCheck asserts HealthCheck passes while a store file is readable, whatever its content, and fails for an absent board dir or no store file.
func TestHealthCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// boardDirAbsent points the board at a directory that does not exist.
		boardDirAbsent bool
		// upsert writes a task through the facade, which creates board.json.
		upsert  bool
		files   map[string]string
		wantErr bool
	}{
		{name: "healthy board", upsert: true},
		{name: "board dir absent", boardDirAbsent: true, wantErr: true},
		{name: "no store file", wantErr: true},
		{name: "legacy file alone", files: map[string]string{"notes.json": "[]"}},
		{name: "corrupt but readable board.json", files: map[string]string{"board.json": "{invalid json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardPath := t.TempDir()
			if tt.boardDirAbsent {
				boardPath = filepath.Join(boardPath, "nonexistent")
			}
			w := boardengine.New(boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
			if tt.upsert {
				if _, err := w.UpsertTask(map[string]any{"slug": "test-task", "title": "Test Task", "labels": bugLabels}); err != nil {
					t.Fatalf("UpsertTask failed: %v", err)
				}
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(boardPath, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			if err := w.HealthCheck(); (err != nil) != tt.wantErr {
				t.Fatalf("HealthCheck error = %v; wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// legacyTasksJSON and legacyNotesJSON are pre-upgrade store files: one task and one note, both with id 0.
const (
	legacyTasksJSON = `[{"id":0,"slug":"alpha","title":"Alpha","depends_on":[],"isolated":false,"deferred":false,"brief":"","body":"","type":"loom"}]`
	legacyNotesJSON = `[{"id":0,"slug":"idea","title":"Idea","depends_on":[],"isolated":false,"deferred":false,"brief":"","body":""}]`
)

func newLegacyBoard(t *testing.T, files map[string]string) (*boardengine.Board, string) {
	t.Helper()
	boardPath := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(boardPath, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
	return boardengine.New(cfg), boardPath
}

// dataFiles returns every file in dir except the *.swaplock and *.lock scratch files a locked read or write creates.
func dataFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".swaplock") || strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func assertSameFiles(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("file set changed: before %d files, after %d files", len(before), len(after))
	}
	for name, content := range before {
		if after[name] != content {
			t.Errorf("%s changed\nbefore: %s\nafter:  %s", name, content, after[name])
		}
	}
}

func TestLegacyBoardReadsMigratedWithoutWriting(t *testing.T) {
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON, "notes.json": legacyNotesJSON})
	before := dataFiles(t, boardPath)

	tasks, err := w.ListTasksFull()
	if err != nil {
		t.Fatalf("ListTasksFull: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 migrated entries, got %d: %+v", len(tasks), tasks)
	}
	got := map[string]boardengine.Task{}
	for _, task := range tasks {
		got[task.Slug] = task
	}
	if got["alpha"].Kind != boardengine.KindTask || got["alpha"].Recipe != "loom" {
		t.Errorf("alpha: kind=%q recipe=%q; want task recipe loom", got["alpha"].Kind, got["alpha"].Recipe)
	}
	if got["idea"].Kind != boardengine.KindNote {
		t.Errorf("idea: kind=%q; want note", got["idea"].Kind)
	}
	if got["alpha"].ID == got["idea"].ID {
		t.Errorf("migrated entries share id %d", got["alpha"].ID)
	}
	if _, found, err := w.GetTask("idea"); err != nil || !found {
		t.Errorf("GetTask(idea) found=%v err=%v; want found", found, err)
	}
	if _, err := w.ListTasksBrief(nil); err != nil {
		t.Errorf("ListTasksBrief: %v", err)
	}
	found, err := w.Find("IDEA", nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 || found[0].Slug != "idea" {
		t.Errorf("Find(IDEA) = %+v; want only idea", found)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
	if _, err := os.Stat(filepath.Join(boardPath, "board.json")); !os.IsNotExist(err) {
		t.Errorf("board.json must stay absent after a read; stat err = %v", err)
	}
}

// TestFirstWriteCreatesBoardJSONAndLeavesLegacyFilesAlone asserts a first write creates board.json, keeps the legacy files byte-identical, and creates none on a board that had none.
func TestFirstWriteCreatesBoardJSONAndLeavesLegacyFilesAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		files       map[string]string
		wantEntries int
	}{
		{"legacy files present", map[string]string{"tasks.json": legacyTasksJSON, "notes.json": legacyNotesJSON}, 3},
		{"fresh board", nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w, boardPath := newLegacyBoard(t, tt.files)

			if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta", "labels": bugLabels}); err != nil {
				t.Fatalf("UpsertTask: %v", err)
			}

			files := dataFiles(t, boardPath)
			if _, ok := files["board.json"]; !ok {
				t.Fatalf("board.json not created")
			}
			for _, legacy := range []string{"tasks.json", "notes.json"} {
				want, existed := tt.files[legacy]
				got, ok := files[legacy]
				if ok != existed || got != want {
					t.Errorf("%s = %q (present %v); want %q (present %v), left as it was", legacy, got, ok, want, existed)
				}
			}
			tasks, err := w.ListTasksFull()
			if err != nil || len(tasks) != tt.wantEntries {
				t.Errorf("ListTasksFull = %d entries, err %v; want %d", len(tasks), err, tt.wantEntries)
			}
		})
	}
}

func TestDuplicateLegacySlugRefusesReadsAndWrites(t *testing.T) {
	dupNotes := `[{"id":5,"slug":"alpha","title":"Dup","depends_on":[]}]`
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON, "notes.json": dupNotes})
	before := dataFiles(t, boardPath)

	if _, _, err := w.GetTask("alpha"); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("GetTask error = %v; want one naming alpha", err)
	}
	if _, err := w.ListTasksBrief(nil); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("ListTasksBrief error = %v; want one naming alpha", err)
	}
	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta", "labels": bugLabels}); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("UpsertTask error = %v; want one naming alpha", err)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
}

func TestReadFoldsLegacyDoneWithoutWriting(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha", "labels": bugLabels}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}
	doneTasks := `[{"id":0,"slug":"alpha","title":"Alpha","depends_on":[],"status":"done"}]`
	if err := os.WriteFile(filepath.Join(boardPath, "tasks.json"), []byte(doneTasks), 0o644); err != nil {
		t.Fatalf("write tasks.json: %v", err)
	}
	before := dataFiles(t, boardPath)

	task, found, err := w.GetTask("alpha")
	if err != nil || !found {
		t.Fatalf("GetTask found=%v err=%v", found, err)
	}
	if task.Status == nil || *task.Status != "done" {
		t.Errorf("alpha status = %v; want done", task.Status)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
}

// TestPreUpgradeDoneMarkSurvivesNewWrite plays a pre-upgrade binary's SetStatus (rewrite tasks.json under board.lock)
// and checks the next new-binary write keeps the entry done in board.json.
func TestPreUpgradeDoneMarkSurvivesNewWrite(t *testing.T) {
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON})
	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta", "labels": bugLabels}); err != nil {
		t.Fatalf("UpsertTask beta: %v", err)
	}

	type legacyRec struct {
		ID        int      `json:"id"`
		Slug      string   `json:"slug"`
		Title     string   `json:"title"`
		DependsOn []string `json:"depends_on"`
		Status    string   `json:"status,omitempty"`
	}
	l, err := flock.AcquireWriteLock(filepath.Join(boardPath, "board.lock"))
	if err != nil {
		t.Fatalf("acquire board.lock: %v", err)
	}
	tasksPath := filepath.Join(boardPath, "tasks.json")
	err = state.WriteJSON(tasksPath, tasksPath+".swaplock", []legacyRec{{ID: 0, Slug: "alpha", Title: "Alpha", DependsOn: []string{}, Status: "done"}})
	l.Release()
	if err != nil {
		t.Fatalf("pre-upgrade SetStatus: %v", err)
	}

	if _, err := w.UpsertTask(map[string]any{"slug": "gamma", "title": "Gamma", "labels": bugLabels}); err != nil {
		t.Fatalf("UpsertTask gamma: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	var file struct {
		Entries []struct {
			Slug   string  `json:"slug"`
			Status *string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal board.json: %v", err)
	}
	for _, e := range file.Entries {
		if e.Slug == "alpha" {
			if e.Status == nil || *e.Status != "done" {
				t.Errorf("alpha status in board.json = %v; want done", e.Status)
			}
			return
		}
	}
	t.Errorf("alpha missing from board.json: %s", raw)
}

func TestPromoteMakesNoteATaskAndIsIdempotent(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "idea", "title": "Idea", "labels": bugLabels}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	promoted, err := w.Promote("idea")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if promoted.Kind != boardengine.KindTask {
		t.Errorf("promoted kind = %q; want task", promoted.Kind)
	}
	got, found, err := w.GetTask("idea")
	if err != nil || !found || got.Kind != boardengine.KindTask {
		t.Errorf("GetTask = %+v found=%v err=%v; want persisted kind task", got, found, err)
	}

	before := dataFiles(t, boardPath)
	boardJSON := filepath.Join(boardPath, "board.json")
	infoBefore, err := os.Stat(boardJSON)
	if err != nil {
		t.Fatalf("stat board.json: %v", err)
	}
	again, err := w.Promote("idea")
	if err != nil {
		t.Fatalf("second Promote: %v", err)
	}
	if again.Kind != boardengine.KindTask || again.Slug != "idea" {
		t.Errorf("second Promote = %+v; want idea unchanged as a task", again)
	}
	assertSameFiles(t, before, dataFiles(t, boardPath))
	infoAfter, err := os.Stat(boardJSON)
	if err != nil || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Errorf("board.json was rewritten by promoting a task: before %v, after %v, err %v", infoBefore.ModTime(), infoAfter.ModTime(), err)
	}

	if _, err := w.Promote("ghost"); err == nil {
		t.Errorf("Promote of a missing entry should fail")
	}
}

// TestPromotePreservesRecipeAndLabels checks that making an entry a task leaves its recipe and labels alone, and that an id selects it as a slug does.
func TestPromotePreservesRecipeAndLabels(t *testing.T) {
	w, _ := newLegacyBoard(t, nil)
	created, err := w.UpsertTask(map[string]any{"slug": "typed", "title": "Typed", "recipe": "batten", "labels": []string{"bug", "undecided"}})
	if err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	promoted, err := w.Promote(created.ID)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if promoted.Recipe != "batten" || promoted.Kind != boardengine.KindTask || len(promoted.Labels) != 2 || promoted.Labels[0] != "bug" || promoted.Labels[1] != "undecided" {
		t.Errorf("promoted = recipe %q kind %q labels %v; want batten task [bug undecided]", promoted.Recipe, promoted.Kind, promoted.Labels)
	}
}

// TestBoardRefusesUnconfiguredLabel checks a write through a config-built Board validates labels, including a status write.
func TestBoardRefusesUnconfiguredLabel(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	_, err := w.UpsertTask(map[string]any{"slug": "x", "title": "X", "labels": []string{"bug", "mystery"}})
	if err == nil || !strings.Contains(err.Error(), "mystery") || !strings.Contains(err.Error(), "board.yaml") {
		t.Errorf("expected refusal naming mystery and board.yaml, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(boardPath, "board.json")); !os.IsNotExist(statErr) {
		t.Errorf("a refused write must not create board.json; stat err = %v", statErr)
	}

	// An entry whose label left board.yaml cannot take a status write either.
	stale, staleDir := newLegacyBoard(t, nil)
	if _, err := stale.UpsertTask(map[string]any{"slug": "x", "title": "X", "labels": bugLabels}); err != nil {
		t.Fatal(err)
	}
	narrowed := boardengine.New(boardengine.Config{Path: staleDir, Readme: "Home.md", DesignPrefix: "proposal-", Types: []boardengine.Label{{Name: "enhancement"}}, SkipGit: true})
	done := "done"
	if err := narrowed.SetStatus("x", &done); err == nil || !strings.Contains(err.Error(), `"bug"`) {
		t.Errorf("expected status write refused naming bug, got %v", err)
	}
}

func TestPruneRemovesDoneEntryAndDesignDocKeepsAbandoned(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	for _, slug := range []string{"finished", "dropped"} {
		if _, err := w.UpsertTask(map[string]any{"slug": slug, "title": slug, "labels": bugLabels, "body": "design of " + slug}); err != nil {
			t.Fatalf("UpsertTask %s: %v", slug, err)
		}
	}
	done, abandoned := "done", "abandoned"
	if err := w.SetStatus("finished", &done); err != nil {
		t.Fatalf("SetStatus done: %v", err)
	}
	if err := w.SetStatus("dropped", &abandoned); err != nil {
		t.Fatalf("SetStatus abandoned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(boardPath, "proposal-finished.md")); err != nil {
		t.Fatalf("design doc missing before prune: %v", err)
	}

	removed, err := w.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 1 || removed[0] != "finished" {
		t.Errorf("removed = %v; want [finished]", removed)
	}
	if _, err := os.Stat(filepath.Join(boardPath, "proposal-finished.md")); !os.IsNotExist(err) {
		t.Errorf("design doc of pruned entry must be gone; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(boardPath, "proposal-dropped.md")); err != nil {
		t.Errorf("design doc of abandoned entry must stay: %v", err)
	}
	if _, found, _ := w.GetTask("dropped"); !found {
		t.Errorf("abandoned entry must survive prune")
	}
}

func TestRetireLegacyFoldsDoneMarkAndRemovesLegacyFiles(t *testing.T) {
	doneTasks := `[{"id":0,"slug":"alpha","title":"Alpha","depends_on":[],"status":"done"}]`
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": doneTasks, "notes.json": legacyNotesJSON})
	// A locked read or write leaves swap locks beside the legacy files.
	for _, name := range []string{"tasks.json.swaplock", "notes.json.swaplock"} {
		if err := os.WriteFile(filepath.Join(boardPath, name), nil, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := w.RetireLegacy(); err != nil {
		t.Fatalf("RetireLegacy: %v", err)
	}

	for _, name := range []string{"tasks.json", "notes.json", "tasks.json.swaplock", "notes.json.swaplock"} {
		if _, err := os.Stat(filepath.Join(boardPath, name)); !os.IsNotExist(err) {
			t.Errorf("%s must be removed; stat err = %v", name, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(boardPath, "board.json"))
	if err != nil {
		t.Fatalf("read board.json: %v", err)
	}
	if strings.Contains(string(raw), "legacy_done") {
		t.Errorf("board.json must not carry legacy_done: %s", raw)
	}
	alpha, found, err := w.GetTask("alpha")
	if err != nil || !found || alpha.Status == nil || *alpha.Status != "done" {
		t.Errorf("alpha = %+v found=%v err=%v; want the folded done mark kept", alpha, found, err)
	}
}

func TestRetireLegacyWithoutLegacyFilesRefusesAndWritesNothing(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha", "labels": bugLabels}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}
	before := dataFiles(t, boardPath)

	err := w.RetireLegacy()
	if err == nil || !strings.Contains(err.Error(), "nothing to retire") {
		t.Fatalf("RetireLegacy error = %v; want one saying nothing to retire", err)
	}
	assertSameFiles(t, before, dataFiles(t, boardPath))
}

// TestRetireLegacyFailedDeletionKeepsLegacyDone forces the deletion to fail after board.json is saved and checks that the surviving legacy file folds nothing twice:
// an entry done at migration and reopened since must stay reopened.
func TestRetireLegacyFailedDeletionKeepsLegacyDone(t *testing.T) {
	doneNotes := `[{"id":0,"slug":"beta","title":"Beta","depends_on":[],"status":"done"}]`
	w, boardPath := newLegacyBoard(t, map[string]string{"notes.json": doneNotes})
	active := "active"
	if err := w.SetStatus("beta", &active); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	// tasks.json is absent, so no load locks its swap lock, but a non-empty directory there makes its removal fail before notes.json is reached.
	blocker := filepath.Join(boardPath, "tasks.json.swaplock")
	if err := os.MkdirAll(filepath.Join(blocker, "keep"), 0o755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}

	if err := w.RetireLegacy(); err == nil {
		t.Fatal("RetireLegacy succeeded; want the blocked deletion to fail")
	}

	if _, err := os.Stat(filepath.Join(boardPath, "notes.json")); err != nil {
		t.Fatalf("notes.json must survive the failed deletion; stat err = %v", err)
	}
	beta, found, err := w.GetTask("beta")
	if err != nil || !found || beta.Status == nil || *beta.Status != active {
		t.Errorf("beta = %+v found=%v err=%v; want it to stay %q", beta, found, err, active)
	}
}

// TestFilterLabelValidation covers D6: a config-built Board refuses a filter label that is unconfigured and carried by no entry,
// accepts one removed from config that an entry still carries, and a path-only Board filters without validating.
func TestFilterLabelValidation(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true}
	cfg.Labels = []boardengine.Label{{Name: "undecided"}, {Name: "retired"}}
	w := boardengine.New(cfg)
	for _, f := range []map[string]any{
		{"slug": "old", "kind": "note", "labels": []string{"bug", "retired"}},
		{"slug": "new", "kind": "note", "labels": []string{"bug"}},
	} {
		if _, err := w.UpsertTask(f); err != nil {
			t.Fatalf("UpsertTask %v: %v", f, err)
		}
	}

	// "retired" is configured only while the entry is written; the narrowed config no longer lists it.
	cfg.Labels = []boardengine.Label{{Name: "undecided"}}
	narrowed := boardengine.New(cfg)

	t.Run("unknown label refused", func(t *testing.T) {
		for name, call := range map[string]func() error{
			"list": func() error { _, err := narrowed.ListTasksBrief([]string{"mystery"}); return err },
			"find": func() error { _, err := narrowed.Find("o", []string{"mystery"}); return err },
		} {
			err := call()
			if err == nil || !strings.Contains(err.Error(), `"mystery"`) || !strings.Contains(err.Error(), "board.yaml") {
				t.Errorf("%s: error = %v; want one naming the label and board.yaml", name, err)
			}
		}
	})

	t.Run("label removed from config but carried is accepted", func(t *testing.T) {
		got, err := narrowed.ListTasksBrief([]string{"retired"})
		if err != nil {
			t.Fatalf("ListTasksBrief: %v", err)
		}
		if len(got) != 1 || got[0].Slug != "old" {
			t.Errorf("got %+v; want only old", got)
		}
	})

	t.Run("path-only board filters without refusal", func(t *testing.T) {
		pathOnly := boardengine.New(boardengine.Config{Path: boardPath})
		got, err := pathOnly.ListTasksBrief([]string{"mystery"})
		if err != nil || len(got) != 0 {
			t.Errorf("list = %+v, %v; want empty and no error", got, err)
		}
		got, err = pathOnly.Find("", []string{"bug"})
		if err != nil || len(got) != 2 {
			t.Errorf("find = %+v, %v; want both entries and no error", got, err)
		}
	})
}
