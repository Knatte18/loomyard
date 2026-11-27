// view_test.go covers the view projection: board-data.js, the revisions and Board.View.

package boardengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewFixture returns a board with projects, a sub-project and a done project holding an open sub-project and a task.
// It also holds tasks in several layers, a draft, notes of two types, a done entry and a discussed entry.
// Its entries carry type and area labels.
func viewFixture() []boardengine.Task {
	task, note, project := boardengine.KindTask, boardengine.KindNote, boardengine.KindProject
	return []boardengine.Task{
		{ID: 1, Handle: "P1", Slug: "ship", Title: "Ship it", Kind: project, Body: "Ship design."},
		{ID: 2, Handle: "P2", Slug: "inner", Title: "Inner", Kind: project, Project: "ship"},
		{ID: 3, Handle: "P3", Slug: "old", Title: "Old project", Kind: project, Status: stringPtr("done")},
		{ID: 4, Handle: "P4", Slug: "orphan", Title: "Orphan", Kind: project, Project: "old"},
		{ID: 5, Handle: "T1", Slug: "base", Title: "Base", Kind: task, Labels: []string{"enhancement", "board"}, Body: "</script><b>bold</b>", Created: renderTime.AddDate(0, 0, -4), Revised: renderTime.AddDate(0, 0, -1)},
		{ID: 6, Handle: "T2", Slug: "next", Title: "Next", Kind: task, Labels: []string{"bug"}, DependsOn: []string{"base", "shipped", "ghost"}},
		{ID: 7, Handle: "T3", Slug: "held", Title: "Held", Kind: task, Project: "ship", Status: stringPtr(boardengine.RunStatus("running", "Webster")), Discussed: true},
		{ID: 8, Handle: "T4", Slug: "drafted", Title: "Drafted", Kind: task, Project: "inner", Status: stringPtr(boardengine.StatusDraft)},
		{ID: 9, Handle: "T5", Slug: "kept", Title: "Kept", Kind: task, Project: "old"},
		{ID: 10, Handle: "E1", Slug: "idea", Title: "Idea", Kind: note, Labels: []string{"enhancement"}},
		{ID: 11, Handle: "R1", Slug: "measure", Title: "Measure", Kind: note, Labels: []string{"research"}},
		{ID: 12, Handle: "T6", Slug: "shipped", Title: "Shipped", Kind: task, Status: stringPtr("done")},
	}
}

// decodeBoardData returns the projection board-data.js assigns to window.BOARD.
func decodeBoardData(t *testing.T, script string) boardengine.BoardView {
	t.Helper()
	body, hasPrefix := strings.CutPrefix(script, "window.BOARD = ")
	body, hasSuffix := strings.CutSuffix(body, ";\n")
	if !hasPrefix || !hasSuffix {
		t.Fatalf("board-data.js = %q; want a window.BOARD assignment ending in a newline", script)
	}
	var view boardengine.BoardView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode board-data.js: %v", err)
	}
	return view
}

// viewEntryBySlug returns the entry of view with slug.
func viewEntryBySlug(t *testing.T, view boardengine.BoardView, slug string) boardengine.ViewEntry {
	t.Helper()
	for _, entry := range view.Entries {
		if entry.Slug == slug {
			return entry
		}
	}
	t.Fatalf("view holds no entry %q", slug)
	return boardengine.ViewEntry{}
}

// entryRevision returns the revision of task, failing the test when it cannot be taken.
func entryRevision(t *testing.T, task boardengine.Task) string {
	t.Helper()
	revision, err := boardengine.EntryRevision(task)
	if err != nil {
		t.Fatalf("EntryRevision(%q): %v", task.Slug, err)
	}
	return revision
}

// TestRenderBoardData pins the view projection against the README of the same render, and the revisions it carries.
func TestRenderBoardData(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	out := boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, Labels: []string{"board"}, DefaultRecipe: "tapestry"}

	t.Run("MatchesTheReadme", func(t *testing.T) {
		t.Parallel()
		tasks := viewFixture()
		files, err := boardengine.Render(tasks, "neve", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		script := files[boardengine.BoardDataFile]
		view := decodeBoardData(t, script)

		var parents []string
		for _, project := range view.Projects {
			parents = append(parents, project.Handle+"<"+project.Parent+" "+project.DesignFile)
		}
		wantParents := []string{"P0< ", "P1<P0 design-ship.md", "P2<P1 ", "P4<P0 "}
		if !slices.Equal(parents, wantParents) {
			t.Errorf("projects = %q; want %q", parents, wantParents)
		}

		placements := []struct {
			slug, section, project string
			waitsOn                []string
		}{
			{"base", "Ready", "P0", []string{}},
			{"next", "Layer A", "P0", []string{"T1", "ghost"}},
			{"held", "Running", "P1", []string{}},
			{"drafted", "Draft", "P2", []string{}},
			{"kept", "Ready", "P3", []string{}},
			{"idea", "Enhancements", "P0", []string{}},
			{"measure", "Research", "P0", []string{}},
			{"shipped", "Done", "P0", []string{}},
		}
		for _, want := range placements {
			entry := viewEntryBySlug(t, view, want.slug)
			if entry.Section != want.section || entry.Project != want.project || !slices.Equal(entry.WaitsOn, want.waitsOn) {
				t.Errorf("entry %s = section %q project %q waits_on %q; want %q %q %q", want.slug, entry.Section, entry.Project, entry.WaitsOn, want.section, want.project, want.waitsOn)
			}
		}
		readme := files["README.md"]
		for _, row := range []string{"| T2 | **Next** | `next` | tapestry | T1<br>`ghost` |", "#### Layer A (1)", "#### Research (1)"} {
			if !strings.Contains(readme, row) {
				t.Errorf("README lacks %q:\n%s", row, readme)
			}
		}

		wantSections := []string{"Running", "Ready", "Layer A", "Draft", "Research", "Enhancements", "Done"}
		if !slices.Equal(view.Sections, wantSections) {
			t.Errorf("sections = %q; want %q", view.Sections, wantSections)
		}
		if !slices.Equal(view.Types, readmeTypes) || !slices.Equal(view.Areas, []string{"board"}) {
			t.Errorf("types, areas = %q, %q; want %q, [board]", view.Types, view.Areas, readmeTypes)
		}
		if view.PrimeOwner != "neve" || view.Rendered != "2026-10-12T09:30:00Z" {
			t.Errorf("prime_owner, rendered = %q, %q; want neve, 2026-10-12T09:30:00Z", view.PrimeOwner, view.Rendered)
		}

		base := viewEntryBySlug(t, view, "base")
		if base.DesignFile != "design-base.md" || base.Recipe != "tapestry" || base.Owner != "neve" {
			t.Errorf("base design_file, recipe, owner = %q, %q, %q; want design-base.md, tapestry, neve", base.DesignFile, base.Recipe, base.Owner)
		}
		if !base.Created.Equal(renderTime.AddDate(0, 0, -4)) || !base.Revised.Equal(renderTime.AddDate(0, 0, -1)) {
			t.Errorf("base created, revised = %v, %v; want the stored dates", base.Created, base.Revised)
		}
		if !slices.Equal(base.Types, []string{"enhancement"}) || !slices.Equal(base.Areas, []string{"board"}) {
			t.Errorf("base types, areas = %q, %q; want [enhancement], [board]", base.Types, base.Areas)
		}
		idea := viewEntryBySlug(t, view, "idea")
		if idea.DesignFile != "" || idea.Recipe != "" {
			t.Errorf("idea design_file, recipe = %q, %q; want both empty", idea.DesignFile, idea.Recipe)
		}
		for _, literal := range []string{"</script>", "<b>"} {
			if strings.Contains(script, literal) {
				t.Errorf("board-data.js holds a literal %q", literal)
			}
		}
		for _, task := range tasks {
			if task.Kind == boardengine.KindProject {
				continue
			}
			if got, want := viewEntryBySlug(t, view, task.Slug).Revision, entryRevision(t, task); got != want {
				t.Errorf("revision of %s = %q; want %q", task.Slug, got, want)
			}
		}
	})

	t.Run("MissingDecisionRecordKeepsTheStoredRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		tasks := viewFixture()
		if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		script, err := os.ReadFile(filepath.Join(dir, boardengine.BoardDataFile))
		if err != nil {
			t.Fatalf("read board-data.js: %v", err)
		}
		held := viewEntryBySlug(t, decodeBoardData(t, string(script)), "held")
		stored := tasks[slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == "held" })]
		if held.DecisionFile != "" || held.Revision != entryRevision(t, stored) {
			t.Errorf("held decision_file, revision = %q, %q; want none and the revision of the stored entry", held.DecisionFile, held.Revision)
		}
	})

	t.Run("BoardRevisionHashesTheStoreBytes", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := boardengine.BoardRevision(dir); err != nil || got != "" {
			t.Errorf("BoardRevision(no board.json) = %q, %v; want \"\", nil", got, err)
		}
		data := []byte(`{"version":1,"entries":[]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got, err := boardengine.BoardRevision(dir); err != nil || got != hex.EncodeToString(sum[:]) {
			t.Errorf("BoardRevision() = %q, %v; want %q", got, err, hex.EncodeToString(sum[:]))
		}
	})

	t.Run("ViewReadsTheMigratedEntryAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		data := []byte(`{"version":1,"entries":[{"id":1,"handle":"T1","slug":"loose","title":"Loose","kind":"task","labels":["enhancement"],"issues":[],"depends_on":[],"isolated":false,"brief":"","body":""}]}`)
		path := filepath.Join(dir, "board.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		board := boardengine.New(boardengine.Config{Path: dir, Readme: "README.md", DesignPrefix: "design-", Types: testTypes, Labels: testLabels, SkipGit: true})
		view, err := board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		migrated, err := boardengine.ParseBoardTasks(data)
		if err != nil {
			t.Fatalf("ParseBoardTasks: %v", err)
		}
		migrated[0].Status = stringPtr(boardengine.StatusDraft)
		if got, want := viewEntryBySlug(t, view, "loose").Revision, entryRevision(t, migrated[0]); got != want {
			t.Errorf("View() revision = %q; want the migrated entry's %q", got, want)
		}
		if after, err := os.ReadFile(path); err != nil || string(after) != string(data) {
			t.Errorf("board.json after View = %q, %v; want it unchanged", after, err)
		}
	})
}
