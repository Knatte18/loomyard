// view_test.go checks the view projection a render writes as board-data.js against the README of the same render.
// It also checks the revisions the projection carries.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewFixtureCreated and viewFixtureRevised are the dates the fixture entry that carries dates stores.
var (
	viewFixtureCreated = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	viewFixtureRevised = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
)

// viewFixture is a board of nested projects, including a done project that holds an open sub-project and a task.
// It also holds tasks in several layers, a draft, a running task, notes of two types and a done entry.
func viewFixture() []boardengine.Task {
	status := func(s string) *string { return &s }
	task := func(id int, handle, slug, project string, deps ...string) boardengine.Task {
		return boardengine.Task{ID: id, Handle: handle, Slug: slug, Title: slug, Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Project: project, DependsOn: deps}
	}
	project := func(id int, handle, slug, parent string) boardengine.Task {
		return boardengine.Task{ID: id, Handle: handle, Slug: slug, Title: slug, Kind: boardengine.KindProject, Labels: []string{"enhancement"}, Project: parent}
	}
	alpha := project(1, "P1", "alpha", "")
	alpha.Body = "why alpha"
	old := project(3, "P3", "old", "")
	old.Status = status("done")

	body := task(9, "T6", "bodied", "")
	body.Body = "</script> <b>bold</b>"
	body.Labels = []string{"enhancement", "undecided"}
	body.Created, body.Revised = viewFixtureCreated, viewFixtureRevised

	drafty := task(8, "T5", "drafty", "alpha-sub")
	drafty.Status = status(boardengine.StatusDraft)
	running := task(10, "T7", "running", "alpha")
	running.Status = status("running · Webster")
	shipped := task(11, "T8", "shipped", "alpha")
	shipped.Status = status("done")
	sketched := task(12, "T9", "sketched", "")
	sketched.Discussed = true

	note := func(id int, handle, slug, typeLabel, project string) boardengine.Task {
		return boardengine.Task{ID: id, Handle: handle, Slug: slug, Title: slug, Kind: boardengine.KindNote, Labels: []string{typeLabel}, Project: project}
	}
	return []boardengine.Task{
		alpha,
		project(2, "P2", "alpha-sub", "alpha"),
		old,
		project(4, "P4", "orphan-sub", "old"),
		task(5, "T1", "old-task", "old"),
		task(6, "T2", "base", "alpha"),
		task(7, "T3", "mid", "alpha", "base"),
		task(13, "T10", "top", "alpha", "mid", "nofind"),
		drafty,
		body,
		running,
		shipped,
		sketched,
		note(14, "E1", "idea", "enhancement", "alpha-sub"),
		note(15, "B1", "defect", "bug", ""),
	}
}

// readmePlacement is where a README shows one entry: its project's handle, its section and its After cell.
type readmePlacement struct {
	project string
	section string
	after   []string
}

// readmePlacements reads each entry row of a README by handle, with the section headings it holds.
func readmePlacements(t *testing.T, readme string) (map[string]readmePlacement, []string) {
	t.Helper()
	summary := regexp.MustCompile(`^<summary><strong><a href="[^"]+">(P\d+) `)
	projectHeading := regexp.MustCompile(`^(#+) \[(P\d+) `)
	ownHeading := regexp.MustCompile(`^(#+) Own entries$`)
	section := regexp.MustCompile(`^#+ (.+) \(\d+\)$`)

	placements := map[string]readmePlacement{}
	var sections []string
	open := map[int]string{}
	var project, current string
	afterColumn := -1
	for _, line := range strings.Split(readme, "\n") {
		if m := summary.FindStringSubmatch(line); m != nil {
			project, open[2] = m[1], m[1]
		} else if m := projectHeading.FindStringSubmatch(line); m != nil {
			project, open[len(m[1])] = m[2], m[2]
		} else if m := ownHeading.FindStringSubmatch(line); m != nil {
			project = open[len(m[1])-1]
		} else if m := section.FindStringSubmatch(line); m != nil {
			current = m[1]
			if current != "Tasks" && current != "Notes" && !slices.Contains(sections, current) {
				sections = append(sections, current)
			}
		} else if strings.HasPrefix(line, "| # |") {
			afterColumn = slices.Index(strings.Split(line, " | "), "After")
		} else if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| --- ") {
			cells := strings.Split(strings.TrimPrefix(line, "| "), " | ")
			placed := readmePlacement{project: project, section: current, after: []string{}}
			if afterColumn >= 0 {
				// The header split keeps the leading pipe in its first cell, so the indexes line up.
				for _, dep := range strings.Split(cells[afterColumn], "<br>") {
					if dep != "" {
						placed.after = append(placed.after, strings.Trim(dep, "`"))
					}
				}
			}
			placements[cells[0]] = placed
		}
	}
	return placements, sections
}

// decodeBoardData decodes the projection a board-data.js file holds.
func decodeBoardData(t *testing.T, content string) boardengine.BoardView {
	t.Helper()
	rest, found := strings.CutPrefix(content, "window.BOARD = ")
	rest, ended := strings.CutSuffix(rest, ";\n")
	if !found || !ended {
		t.Fatalf("board-data.js = %q; want `window.BOARD = ...;`", content)
	}
	var view boardengine.BoardView
	if err := json.Unmarshal([]byte(rest), &view); err != nil {
		t.Fatalf("decode board-data.js: %v", err)
	}
	return view
}

// entryOf returns the entry of view with slug.
func entryOf(t *testing.T, view boardengine.BoardView, slug string) boardengine.ViewEntry {
	t.Helper()
	for _, entry := range view.Entries {
		if entry.Slug == slug {
			return entry
		}
	}
	t.Fatalf("view holds no entry %q", slug)
	return boardengine.ViewEntry{}
}

// revisionOf returns the revision of task.
func revisionOf(t *testing.T, task boardengine.Task) string {
	t.Helper()
	revision, err := boardengine.EntryRevision(task)
	if err != nil {
		t.Fatalf("EntryRevision(%q): %v", task.Slug, err)
	}
	return revision
}

func TestRenderBoardData(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-", DefaultRecipe: "darn", Types: []string{"bug", "enhancement"}, Labels: []string{"undecided"}}
	tasks := viewFixture()

	t.Run("Projection", func(t *testing.T) {
		t.Parallel()
		files, err := boardengine.Render(tasks, "neve", out, renderTime)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		content := files[boardengine.BoardDataFile]
		if strings.Contains(content, "</script>") || strings.Contains(content, "<b>") {
			t.Errorf("board-data.js holds an unescaped tag: %s", content)
		}
		view := decodeBoardData(t, content)

		placements, headings := readmePlacements(t, files["Home.md"])
		for _, entry := range view.Entries {
			got := readmePlacement{project: entry.Project, section: entry.Section, after: entry.WaitsOn}
			// The README has no section for a done project, so its task has no row there.
			if entry.Slug == "old-task" {
				if entry.Project != "P3" {
					t.Errorf("old-task project = %q; want P3", entry.Project)
				}
				continue
			}
			want, listed := placements[entry.Handle]
			if !listed {
				t.Errorf("README has no row for %s", entry.Handle)
				continue
			}
			if got.project != want.project || got.section != want.section {
				t.Errorf("%s placed at %s/%s; want %s/%s", entry.Handle, got.project, got.section, want.project, want.section)
			}
			if want.section != "Done" && want.section != "Running" && entry.Kind == boardengine.KindTask && !slices.Equal(got.after, want.after) {
				t.Errorf("%s waits_on = %v; want %v", entry.Handle, got.after, want.after)
			}
		}
		if got := entryOf(t, view, "top").WaitsOn; !slices.Equal(got, []string{"T3", "nofind"}) {
			t.Errorf("top waits_on = %v; want [T3 nofind]", got)
		}

		var parents []string
		for _, project := range view.Projects {
			parents = append(parents, project.Handle+"<"+project.Parent)
		}
		if want := []string{"P0<", "P1<P0", "P2<P1", "P4<P0"}; !slices.Equal(parents, want) {
			t.Errorf("projects = %v; want %v", parents, want)
		}

		wantSections := []string{"Running", "Ready", "Layer A", "Layer B", "Draft", "Bugs", "Enhancements", "Done"}
		if !slices.Equal(view.Sections, wantSections) {
			t.Errorf("sections = %v; want %v", view.Sections, wantSections)
		}
		for _, heading := range headings {
			if !slices.Contains(view.Sections, heading) {
				t.Errorf("README section %q is missing from sections %v", heading, view.Sections)
			}
		}
		if !slices.Equal(view.Types, out.Types) || !slices.Equal(view.Areas, out.Labels) {
			t.Errorf("types, areas = %v, %v; want %v, %v", view.Types, view.Areas, out.Types, out.Labels)
		}
		if view.PrimeOwner != "neve" || view.Rendered != "2026-10-12T09:30:00Z" {
			t.Errorf("prime_owner, rendered = %q, %q; want neve, 2026-10-12T09:30:00Z", view.PrimeOwner, view.Rendered)
		}

		bodied := entryOf(t, view, "bodied")
		if bodied.DesignFile != "proposal-bodied.md" || bodied.Body != "</script> <b>bold</b>" {
			t.Errorf("bodied design_file, body = %q, %q", bodied.DesignFile, bodied.Body)
		}
		if !slices.Equal(bodied.Types, []string{"enhancement"}) || !slices.Equal(bodied.Areas, []string{"undecided"}) {
			t.Errorf("bodied types, areas = %v, %v; want [enhancement], [undecided]", bodied.Types, bodied.Areas)
		}
		if bodied.Owner != "neve" || !bodied.Created.Equal(viewFixtureCreated) || !bodied.Revised.Equal(viewFixtureRevised) || bodied.Recipe != "darn" {
			t.Errorf("bodied owner, created, revised, recipe = %q, %v, %v, %q", bodied.Owner, bodied.Created, bodied.Revised, bodied.Recipe)
		}
		if base := entryOf(t, view, "base"); base.DesignFile != "" {
			t.Errorf("base design_file = %q; want none", base.DesignFile)
		}
		if note := entryOf(t, view, "idea"); note.Recipe != "" {
			t.Errorf("note recipe = %q; want empty", note.Recipe)
		}
		if view.Projects[1].DesignFile != "proposal-alpha.md" || view.Projects[2].DesignFile != "" {
			t.Errorf("project design files = %q, %q; want proposal-alpha.md, none", view.Projects[1].DesignFile, view.Projects[2].DesignFile)
		}
		if got := entryOf(t, view, "sketched").DecisionFile; got != "decision-sketched.md" {
			t.Errorf("sketched decision_file = %q; want decision-sketched.md", got)
		}
		for _, task := range tasks {
			if task.Kind == boardengine.KindProject {
				continue
			}
			if got, want := entryOf(t, view, task.Slug).Revision, revisionOf(t, task); got != want {
				t.Errorf("%s revision = %q; want %q", task.Slug, got, want)
			}
		}
	})

	t.Run("MissingRecordKeepsStoredRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := boardengine.RenderToDisk(dir, tasks, "neve", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(dir, boardengine.BoardDataFile))
		if err != nil {
			t.Fatalf("read board-data.js: %v", err)
		}
		got := entryOf(t, decodeBoardData(t, string(content)), "sketched")
		stored := tasks[slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == "sketched" })]
		if got.DecisionFile != "" || got.Revision != revisionOf(t, stored) {
			t.Errorf("sketched decision_file, revision = %q, %q; want none and the stored entry's", got.DecisionFile, got.Revision)
		}
	})

	t.Run("BoardRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := boardengine.BoardRevision(dir); got != "" || err != nil {
			t.Errorf("BoardRevision(absent) = %q, %v; want empty", got, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "board.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		const sumOfBraces = "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
		if got, err := boardengine.BoardRevision(dir); got != sumOfBraces || err != nil {
			t.Errorf("BoardRevision() = %q, %v; want %q", got, err, sumOfBraces)
		}
	})

	t.Run("ViewMigratesWithoutSaving", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		raw := []byte(`{"version":1,"entries":[{"id":1,"handle":"T1","slug":"unmarked","title":"U","kind":"task","labels":["bug"]}]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		board := boardengine.New(boardengine.Config{Path: dir, Readme: "Home.md", DesignPrefix: "proposal-", Types: testTypes, Labels: testLabels, SkipGit: true})
		view, err := board.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		migrated, found, err := board.GetTask("unmarked")
		if err != nil || !found {
			t.Fatalf("GetTask: %v, %v", found, err)
		}
		draft := boardengine.StatusDraft
		migrated.Status = &draft
		if got, want := entryOf(t, view, "unmarked").Revision, revisionOf(t, migrated); got != want {
			t.Errorf("revision = %q; want the migrated entry's %q", got, want)
		}
		after, err := os.ReadFile(filepath.Join(dir, "board.json"))
		if err != nil || string(after) != string(raw) {
			t.Errorf("board.json = %s, %v; want it unchanged", after, err)
		}
	})
}
