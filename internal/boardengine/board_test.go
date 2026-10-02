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

// TestUpsertTask tests the facade persistence wiring: creating a task writes both board.json and
// Home.md.
// Drop: store-layer assertion "update preserves fields" (owned by
// store_test.go:TestUpsertTaskPreservesFields).
func TestUpsertTask(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

	// Creates task, board.json written, Home.md written
	task, err := w.UpsertTask(map[string]any{
		"slug":  "test-task",
		"title": "Test Task",
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

// TestUpsertTaskUnconfiguredOutputsFailsBeforeWriting locks in boardCriticalSection's fail-fast guard: a Board
// built without output filenames (the --board-path shape) must reject a write before touching disk,
// never save board.json and then fail the render on an empty filename.
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
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
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

func TestHealthCheckPasses(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
	w := boardengine.New(cfg)

	// Create a task to initialize the board directory and tasks.json
	_, err := w.UpsertTask(map[string]any{
		"slug":  "test-task",
		"title": "Test Task",
	})
	if err != nil {
		t.Fatalf("UpsertTask failed: %v", err)
	}

	// HealthCheck should pass for a healthy board
	err = w.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed for healthy board: %v", err)
	}
}

func TestHealthCheckFailsNoBoardDir(t *testing.T) {
	boardPath := filepath.Join(t.TempDir(), "nonexistent")
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// HealthCheck should fail when board directory does not exist
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when board directory is absent")
	}
}

func TestHealthCheckFailsNoStoreFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// HealthCheck should fail when neither board.json nor a legacy file exists
	err := w.HealthCheck()
	if err == nil {
		t.Fatalf("HealthCheck should fail when no store file is present")
	}
}

func TestHealthCheckPassesLegacyFileAlone(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	if err := os.WriteFile(filepath.Join(boardPath, "notes.json"), []byte("[]"), 0o644); err != nil {
		t.Fatalf("write notes.json: %v", err)
	}
	if err := w.HealthCheck(); err != nil {
		t.Fatalf("HealthCheck failed on a legacy file alone: %v", err)
	}
}

func TestHealthCheckPassesCorruptFile(t *testing.T) {
	boardPath := t.TempDir()
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// Create a corrupt but readable board.json
	storePath := filepath.Join(boardPath, "board.json")
	err := os.WriteFile(storePath, []byte("{invalid json"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write corrupt board.json: %v", err)
	}

	// HealthCheck should pass even if JSON is corrupt, as long as it's readable
	err = w.HealthCheck()
	if err != nil {
		t.Fatalf("HealthCheck failed for corrupt but readable board.json: %v", err)
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
	cfg := boardengine.Config{Path: boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true}
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
	if got["alpha"].Tier != 1 || got["alpha"].Recipe != "loom" {
		t.Errorf("alpha: tier=%d recipe=%q; want tier 1 recipe loom", got["alpha"].Tier, got["alpha"].Recipe)
	}
	if got["idea"].Tier != 3 {
		t.Errorf("idea: tier=%d; want 3", got["idea"].Tier)
	}
	if got["alpha"].ID == got["idea"].ID {
		t.Errorf("migrated entries share id %d", got["alpha"].ID)
	}
	if _, found, err := w.GetTask("idea"); err != nil || !found {
		t.Errorf("GetTask(idea) found=%v err=%v; want found", found, err)
	}
	if _, err := w.ListTasksBrief(); err != nil {
		t.Errorf("ListTasksBrief: %v", err)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
	if _, err := os.Stat(filepath.Join(boardPath, "board.json")); !os.IsNotExist(err) {
		t.Errorf("board.json must stay absent after a read; stat err = %v", err)
	}
}

func TestFirstWriteCreatesBoardJSONAndKeepsLegacyFiles(t *testing.T) {
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON, "notes.json": legacyNotesJSON})

	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta"}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	files := dataFiles(t, boardPath)
	if _, ok := files["board.json"]; !ok {
		t.Fatalf("board.json not created")
	}
	if files["tasks.json"] != legacyTasksJSON || files["notes.json"] != legacyNotesJSON {
		t.Errorf("legacy files must be left in place byte-identical")
	}
	tasks, err := w.ListTasksFull()
	if err != nil || len(tasks) != 3 {
		t.Errorf("ListTasksFull = %d entries, err %v; want 3", len(tasks), err)
	}
}

func TestWriteOnFreshBoardCreatesNoLegacyFiles(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)

	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta"}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	files := dataFiles(t, boardPath)
	if _, ok := files["board.json"]; !ok {
		t.Errorf("board.json not created")
	}
	for _, legacy := range []string{"tasks.json", "notes.json"} {
		if _, ok := files[legacy]; ok {
			t.Errorf("%s must not be created on a board that had none", legacy)
		}
	}
}

func TestDuplicateLegacySlugRefusesReadsAndWrites(t *testing.T) {
	dupNotes := `[{"id":5,"slug":"alpha","title":"Dup","depends_on":[]}]`
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON, "notes.json": dupNotes})
	before := dataFiles(t, boardPath)

	if _, _, err := w.GetTask("alpha"); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("GetTask error = %v; want one naming alpha", err)
	}
	if _, err := w.ListTasksBrief(); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("ListTasksBrief error = %v; want one naming alpha", err)
	}
	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta"}); err == nil || !strings.Contains(err.Error(), "alpha") {
		t.Errorf("UpsertTask error = %v; want one naming alpha", err)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
}

func TestReadFoldsLegacyDoneWithoutWriting(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha"}); err != nil {
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
	if _, err := w.UpsertTask(map[string]any{"slug": "beta", "title": "Beta"}); err != nil {
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

	if _, err := w.UpsertTask(map[string]any{"slug": "gamma", "title": "Gamma"}); err != nil {
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

func TestPromoteNoteMovesToTierOneAndIsIdempotent(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "idea", "title": "Idea", "tier": 3}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	promoted, err := w.PromoteNote("idea")
	if err != nil {
		t.Fatalf("PromoteNote: %v", err)
	}
	if promoted.Tier != 1 {
		t.Errorf("promoted tier = %d; want 1", promoted.Tier)
	}

	before := dataFiles(t, boardPath)
	again, err := w.PromoteNote("idea")
	if err != nil {
		t.Fatalf("second PromoteNote: %v", err)
	}
	if again.Tier != 1 || again.Slug != "idea" {
		t.Errorf("second PromoteNote = %+v; want idea unchanged at tier 1", again)
	}
	assertSameFiles(t, before, dataFiles(t, boardPath))

	if _, err := w.PromoteNote("ghost"); err == nil {
		t.Errorf("PromoteNote of a missing entry should fail")
	}
}

// TestPromoteNotePreservesRecipeAndType checks that moving an entry to tier 1 leaves its recipe and type alone.
func TestPromoteNotePreservesRecipeAndType(t *testing.T) {
	w, _ := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "typed", "title": "Typed", "tier": 3, "recipe": "batten", "type": "bug"}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	promoted, err := w.PromoteNote("typed")
	if err != nil {
		t.Fatalf("PromoteNote: %v", err)
	}
	if promoted.Recipe != "batten" || promoted.Type != "bug" || promoted.Tier != 1 {
		t.Errorf("promoted = recipe %q type %q tier %d; want batten bug 1", promoted.Recipe, promoted.Type, promoted.Tier)
	}
}

func TestPromoteThroughFacadePersistsTier(t *testing.T) {
	w, _ := newLegacyBoard(t, nil)
	if _, err := w.UpsertTask(map[string]any{"slug": "idea", "title": "Idea", "tier": 3}); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	two := 2
	promoted, err := w.Promote("idea", &two)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if promoted.Tier != 2 {
		t.Errorf("promoted tier = %d; want 2", promoted.Tier)
	}

	got, found, err := w.GetTask("idea")
	if err != nil || !found || got.Tier != 2 {
		t.Errorf("GetTask = %+v found=%v err=%v; want persisted tier 2", got, found, err)
	}

	again, err := w.Promote("idea", nil)
	if err != nil || again.Tier != 1 {
		t.Errorf("Promote(nil) = %+v err=%v; want tier 1", again, err)
	}
}

func TestPruneRemovesDoneEntryAndDesignDocKeepsAbandoned(t *testing.T) {
	w, boardPath := newLegacyBoard(t, nil)
	for _, slug := range []string{"finished", "dropped"} {
		if _, err := w.UpsertTask(map[string]any{"slug": slug, "title": slug, "body": "design of " + slug}); err != nil {
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

func TestFindOnUnmigratedBoardWritesNothing(t *testing.T) {
	w, boardPath := newLegacyBoard(t, map[string]string{"tasks.json": legacyTasksJSON, "notes.json": legacyNotesJSON})
	before := dataFiles(t, boardPath)

	found, err := w.Find("IDEA")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 || found[0].Slug != "idea" {
		t.Errorf("Find(IDEA) = %+v; want only idea", found)
	}

	assertSameFiles(t, before, dataFiles(t, boardPath))
	if _, err := os.Stat(filepath.Join(boardPath, "board.json")); !os.IsNotExist(err) {
		t.Errorf("board.json must stay absent after Find; stat err = %v", err)
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
	if _, err := w.UpsertTask(map[string]any{"slug": "alpha", "title": "Alpha"}); err != nil {
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
