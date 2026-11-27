// view_test.go tests the view projection: board-data.js as Render writes it, the revisions and Board.View.

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

// viewFixture is a board of projects, among them a sub-project and a done project that holds a sub-project and a task.
// It also holds tasks in several layers, notes of two types and a done entry.
func viewFixture() []boardengine.Task {
	task, note, project := boardengine.KindTask, boardengine.KindNote, boardengine.KindProject
	bug := []string{"bug"}
	created, revised := renderTime.AddDate(0, 0, -9), renderTime.AddDate(0, 0, -2)
	return []boardengine.Task{
		{ID: 1, Handle: "P1", Slug: "alpha", Title: "Alpha", Kind: project, Labels: bug, Brief: "First project.", Body: "Project design."},
		{ID: 2, Handle: "P2", Slug: "beta", Title: "Beta", Kind: project, Labels: bug, Project: "alpha"},
		{ID: 3, Handle: "P3", Slug: "gone", Title: "Gone", Kind: project, Labels: bug, Status: stringPtr("done")},
		{ID: 4, Handle: "P4", Slug: "orphan", Title: "Orphan", Kind: project, Labels: bug, Project: "gone"},
		{ID: 5, Handle: "T1", Slug: "base", Title: "Base", Kind: task, Labels: []string{"bug", "ui"}, Body: "Holds </script> and <b>bold</b>.", Created: created, Revised: revised},
		{ID: 6, Handle: "T2", Slug: "mid", Title: "Mid", Kind: task, Labels: bug, Project: "alpha", DependsOn: []string{"base", "shipped"}},
		{ID: 7, Handle: "T3", Slug: "busy", Title: "Busy", Kind: task, Labels: bug, Project: "beta", Status: stringPtr(boardengine.RunStatus("running", "Webster"))},
		{ID: 8, Handle: "T4", Slug: "later", Title: "Later", Kind: task, Labels: bug, Status: stringPtr(boardengine.StatusDraft), Discussed: true},
		{ID: 9, Handle: "T5", Slug: "solo", Title: "Solo", Kind: task, Labels: bug, Isolated: true},
		{ID: 10, Handle: "T6", Slug: "shipped", Title: "Shipped", Kind: task, Labels: bug, Status: stringPtr("done")},
		{ID: 11, Handle: "T7", Slug: "leftover", Title: "Leftover", Kind: task, Labels: bug, Project: "gone"},
		{ID: 12, Handle: "B1", Slug: "flaw", Title: "Flaw", Kind: note, Labels: bug, Project: "orphan"},
		{ID: 13, Handle: "R1", Slug: "measure", Title: "Measure", Kind: note, Labels: []string{"research"}},
	}
}

// readmePlace is where a README puts one entry; after is nil when the entry's table has no After column.
type readmePlace struct {
	project, section string
	after            []string
}

// readmePlaces reads the place of each task and note row of a README, by handle.
func readmePlaces(readme string) map[string]readmePlace {
	projectHandles := map[string]string{"project-root.md": "P0", "project-alpha.md": "P1", "project-beta.md": "P2", "project-orphan.md": "P4"}
	places := map[string]readmePlace{}
	byLevel := map[int]string{}
	project, section, afterColumn := "", "", -1
	for _, line := range strings.Split(readme, "\n") {
		level := len(line) - len(strings.TrimLeft(line, "#"))
		switch {
		case strings.HasPrefix(line, "<summary>"):
			_, rest, _ := strings.Cut(line, `href="`)
			file, _, _ := strings.Cut(rest, `"`)
			project = projectHandles[file]
			byLevel = map[int]string{2: project}
		case level > 0:
			name, _, _ := strings.Cut(strings.TrimSpace(line[level:]), " (")
			if _, file, linked := strings.Cut(line, "]("); linked {
				file, _, _ = strings.Cut(file, ")")
				project = projectHandles[file]
				byLevel[level] = project
			} else if name == "Own entries" {
				project = byLevel[level-1]
			} else {
				section = name
			}
		case strings.HasPrefix(line, "| # |"):
			afterColumn = slices.Index(strings.Split(line, " | "), "After")
		case strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| ---"):
			cells := strings.Split(line, " | ")
			place := readmePlace{project: project, section: section}
			if afterColumn >= 0 {
				place.after = []string{}
				if cells[afterColumn] != "" {
					place.after = strings.Split(cells[afterColumn], "<br>")
				}
			}
			places[strings.TrimPrefix(cells[0], "| ")] = place
		}
	}
	return places
}

// decodeBoardData strips the script statement around board-data.js and decodes its JSON.
func decodeBoardData(t *testing.T, script string) boardengine.BoardView {
	t.Helper()
	body, prefixed := strings.CutPrefix(script, "window.BOARD = ")
	body, suffixed := strings.CutSuffix(body, ";\n")
	if !prefixed || !suffixed {
		t.Fatalf("board-data.js = %q; want a window.BOARD statement", script)
	}
	var view boardengine.BoardView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode board-data.js: %v", err)
	}
	return view
}

func viewEntry(t *testing.T, view boardengine.BoardView, slug string) boardengine.ViewEntry {
	t.Helper()
	index := slices.IndexFunc(view.Entries, func(e boardengine.ViewEntry) bool { return e.Slug == slug })
	if index < 0 {
		t.Fatalf("the view holds no entry %q", slug)
	}
	return view.Entries[index]
}

func entryRevision(t *testing.T, task boardengine.Task) string {
	t.Helper()
	revision, err := boardengine.EntryRevision(task)
	if err != nil {
		t.Fatalf("EntryRevision(%s): %v", task.Slug, err)
	}
	return revision
}

func TestRenderBoardData(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tasks := viewFixture()
	out := boardengine.Outputs{
		Readme:        "README.md",
		DesignPrefix:  "design-",
		DefaultRecipe: "darn",
		Types:         []string{"sketch", "research", "bug"},
		Labels:        []string{"ui", "cli"},
	}
	files, err := boardengine.Render(tasks, "neve", out, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	script := files[boardengine.BoardDataFile]
	view := decodeBoardData(t, script)

	t.Run("PlacesEveryEntryAsTheReadmeDoes", func(t *testing.T) {
		t.Parallel()
		places := readmePlaces(files["README.md"])
		for _, entry := range view.Entries {
			// A done project has no README section, so its task is asserted on its own below.
			if entry.Slug == "leftover" {
				continue
			}
			place, listed := places[entry.Handle]
			if !listed {
				t.Errorf("entry %s is in no README row", entry.Slug)
				continue
			}
			if entry.Project != place.project || entry.Section != place.section {
				t.Errorf("entry %s = project %s section %q; want %s %q", entry.Slug, entry.Project, entry.Section, place.project, place.section)
			}
			if place.after != nil && !slices.Equal(entry.WaitsOn, place.after) {
				t.Errorf("entry %s waits_on = %q; want %q", entry.Slug, entry.WaitsOn, place.after)
			}
		}
		if got := viewEntry(t, view, "mid").WaitsOn; !slices.Equal(got, []string{"T1"}) {
			t.Errorf("mid waits_on = %q; want [T1]", got)
		}
		if got := viewEntry(t, view, "leftover").Project; got != "P3" {
			t.Errorf("the done project's task has project %s; want P3", got)
		}
	})

	t.Run("NamesADependencyTheBoardDoesNotHoldByItsSlug", func(t *testing.T) {
		t.Parallel()
		waiting := []boardengine.Task{{ID: 1, Handle: "T1", Slug: "waiter", Title: "Waiter", Kind: boardengine.KindTask, Labels: []string{"bug"}, DependsOn: []string{"unheld"}}}
		files, err := boardengine.Render(waiting, "neve", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if got := viewEntry(t, decodeBoardData(t, files[boardengine.BoardDataFile]), "waiter").WaitsOn; !slices.Equal(got, []string{"unheld"}) {
			t.Errorf("waits_on = %q; want [unheld]", got)
		}
	})

	t.Run("ListsProjectsWithTheirParents", func(t *testing.T) {
		t.Parallel()
		var got []string
		for _, project := range view.Projects {
			got = append(got, project.Handle+" "+project.Slug+" under "+project.Parent+" "+project.DesignFile)
		}
		want := []string{"P0 root under  ", "P1 alpha under P0 design-alpha.md", "P2 beta under P1 ", "P4 orphan under P0 "}
		if !slices.Equal(got, want) {
			t.Errorf("projects = %q; want %q", got, want)
		}
	})

	t.Run("CarriesTheBoardFacts", func(t *testing.T) {
		t.Parallel()
		wantSections := []string{"Running", "Ready", "Layer A", "Independent", "Draft", "Research", "Bugs", "Done"}
		if !slices.Equal(view.Sections, wantSections) {
			t.Errorf("sections = %q; want %q", view.Sections, wantSections)
		}
		if !slices.Equal(view.Types, out.Types) || !slices.Equal(view.Areas, out.Labels) {
			t.Errorf("types, areas = %q, %q; want %q, %q", view.Types, view.Areas, out.Types, out.Labels)
		}
		if view.PrimeOwner != "neve" || view.Rendered != "2026-10-12T09:30:00Z" {
			t.Errorf("prime_owner, rendered = %q, %q", view.PrimeOwner, view.Rendered)
		}
	})

	t.Run("CarriesEachEntrysFields", func(t *testing.T) {
		t.Parallel()
		base, note := viewEntry(t, view, "base"), viewEntry(t, view, "measure")
		if base.DesignFile != "design-base.md" || note.DesignFile != "" {
			t.Errorf("design files = %q, %q; want design-base.md and none", base.DesignFile, note.DesignFile)
		}
		if base.Recipe != "darn" || note.Recipe != "" {
			t.Errorf("recipes = %q, %q; want darn and none", base.Recipe, note.Recipe)
		}
		if base.Owner != "neve" || !base.Created.Equal(tasks[4].Created) || !base.Revised.Equal(tasks[4].Revised) {
			t.Errorf("base owner, created, revised = %q, %v, %v", base.Owner, base.Created, base.Revised)
		}
		if !slices.Equal(base.Types, []string{"bug"}) || !slices.Equal(base.Areas, []string{"ui"}) {
			t.Errorf("base types, areas = %q, %q; want [bug] [ui]", base.Types, base.Areas)
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
			if got, want := viewEntry(t, view, task.Slug).Revision, entryRevision(t, task); got != want {
				t.Errorf("%s revision = %s; want %s", task.Slug, got, want)
			}
		}
	})

	t.Run("AMissingRecordNamesNoDecisionFileAndKeepsTheStoredRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := boardengine.RenderToDisk(dir, tasks, "neve", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		written, err := os.ReadFile(filepath.Join(dir, boardengine.BoardDataFile))
		if err != nil {
			t.Fatalf("read board-data.js: %v", err)
		}
		later := viewEntry(t, decodeBoardData(t, string(written)), "later")
		if later.DecisionFile != "" {
			t.Errorf("decision_file = %q; want none", later.DecisionFile)
		}
		if want := entryRevision(t, tasks[7]); later.Revision != want {
			t.Errorf("revision = %s; want the stored entry's %s", later.Revision, want)
		}
	})

	t.Run("BoardRevisionHashesTheStoreFile", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := boardengine.BoardRevision(dir); got != "" || err != nil {
			t.Errorf("BoardRevision of an absent store = %q, %v; want empty", got, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "board.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte("{}"))
		if got, err := boardengine.BoardRevision(dir); got != hex.EncodeToString(sum[:]) || err != nil {
			t.Errorf("BoardRevision = %q, %v", got, err)
		}
	})

	t.Run("ViewRevisesTheMigratedEntryAndWritesNothing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		stored := []byte(`{"version":1,"entries":[{"id":0,"handle":"T1","slug":"a","title":"A","kind":"task","labels":["bug"],"issues":[],"depends_on":[],"isolated":false,"brief":"","body":""}]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), stored, 0o600); err != nil {
			t.Fatal(err)
		}
		board := boardengine.New(boardengine.Config{Path: dir, Readme: "README.md", DesignPrefix: "design-", Types: testTypes, SkipGit: true})
		got, err := board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		migrated := boardengine.Task{Handle: "T1", Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"bug"}, Issues: []int{}, DependsOn: []string{}, Status: stringPtr(boardengine.StatusDraft)}
		if revision, want := viewEntry(t, got, "a").Revision, entryRevision(t, migrated); revision != want {
			t.Errorf("revision = %s; want the migrated entry's %s", revision, want)
		}
		if after, err := os.ReadFile(filepath.Join(dir, "board.json")); err != nil || string(after) != string(stored) {
			t.Errorf("board.json changed: %q, %v", after, err)
		}
	})
}
