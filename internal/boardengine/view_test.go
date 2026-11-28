// view_test.go covers the view projection of view.go: board-data.js, Board.View and the revisions.

package boardengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewFixture returns projects, a sub-project and a done project with an open sub-project.
// It also returns tasks in several layers, notes of two types and a done entry.
func viewFixture() []boardengine.Task {
	task, note, project := boardengine.KindTask, boardengine.KindNote, boardengine.KindProject
	bug := []string{"bug"}
	return []boardengine.Task{
		{ID: 1, Handle: "P1", Slug: "alpha", Title: "Alpha", Kind: project, Body: "Alpha body."},
		{ID: 2, Handle: "P2", Slug: "sub", Title: "Sub", Kind: project, Project: "alpha"},
		{ID: 3, Handle: "P3", Slug: "old", Title: "Old", Kind: project, Status: stringPtr("done")},
		{ID: 4, Handle: "P4", Slug: "orphan", Title: "Orphan", Kind: project, Project: "old"},
		{ID: 5, Handle: "T1", Slug: "old-task", Title: "Old task", Kind: task, Labels: bug, Project: "old"},
		{ID: 6, Handle: "T2", Slug: "base", Title: "Base", Kind: task, Labels: []string{"bug", "ui"}, Body: "see </script> <b>bold</b>", Created: renderTime.AddDate(0, 0, -4), Revised: renderTime.AddDate(0, 0, -2)},
		{ID: 7, Handle: "T3", Slug: "next", Title: "Next", Kind: task, Labels: bug, Project: "alpha", DependsOn: []string{"base"}},
		{ID: 8, Handle: "T4", Slug: "later", Title: "Later", Kind: task, Labels: bug, DependsOn: []string{"next", "gone"}},
		{ID: 9, Handle: "T5", Slug: "drafted", Title: "Drafted", Kind: task, Labels: bug, Project: "sub", Status: stringPtr(boardengine.StatusDraft)},
		{ID: 10, Handle: "T6", Slug: "alone", Title: "Alone", Kind: task, Labels: bug, Project: "orphan", Isolated: true},
		{ID: 11, Handle: "T7", Slug: "busy", Title: "Busy", Kind: task, Labels: bug, Project: "alpha", Status: stringPtr("running · Build")},
		{ID: 12, Handle: "T8", Slug: "finished", Title: "Finished", Kind: task, Labels: bug, Status: stringPtr("done")},
		{ID: 13, Handle: "T9", Slug: "sketched", Title: "Sketched", Kind: task, Labels: bug, Discussed: true},
		{ID: 14, Handle: "R1", Slug: "study", Title: "Study", Kind: note, Labels: []string{"research"}, Project: "alpha"},
		{ID: 15, Handle: "B1", Slug: "defect", Title: "Defect", Kind: note, Labels: bug, Project: "sub"},
	}
}

// readmePlacement is where the README shows one entry's row.
type readmePlacement struct {
	project string
	section string
	waitsOn []string
}

// readmeLayout is the placement the README gives entries, projects and sections.
type readmeLayout struct {
	entries map[string]readmePlacement
	parents map[string]string
	// sections lists each project's section headings in the order the README shows them.
	sections map[string][]string
}

var (
	readmeSummaryLine    = regexp.MustCompile(`^<summary><strong><a href="[^"]*">(P\d+) `)
	readmeProjectHeading = regexp.MustCompile(`^(#+) \[(P\d+) [^\]]*\]\([^)]*\) \(\d+\)$`)
	readmeOwnHeading     = regexp.MustCompile(`^(#+) Own entries$`)
	readmeCountedHeading = regexp.MustCompile(`^#+ ([^\[\]]+) \(\d+\)$`)
	readmeEntryRow       = regexp.MustCompile(`^\| ([A-OQ-Z]\d+) \|`)
)

// parseReadmeLayout reads the placement of every entry row, project heading and section heading of readme.
func parseReadmeLayout(readme string) readmeLayout {
	layout := readmeLayout{entries: map[string]readmePlacement{}, parents: map[string]string{}, sections: map[string][]string{}}
	type open struct {
		level  int
		handle string
	}
	var stack []open
	closeFrom := func(level int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
	}
	section, afterColumn := "", -1
	cells := func(line string) []string {
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	for _, line := range strings.Split(readme, "\n") {
		if m := readmeSummaryLine.FindStringSubmatch(line); m != nil {
			// A README section's summary stands in for a heading at level two.
			stack = []open{{2, m[1]}}
			if m[1] != "P0" {
				layout.parents[m[1]] = "P0"
			}
		} else if m := readmeProjectHeading.FindStringSubmatch(line); m != nil {
			closeFrom(len(m[1]))
			layout.parents[m[2]] = stack[len(stack)-1].handle
			stack = append(stack, open{len(m[1]), m[2]})
		} else if m := readmeOwnHeading.FindStringSubmatch(line); m != nil {
			closeFrom(len(m[1]))
		} else if m := readmeCountedHeading.FindStringSubmatch(line); m != nil {
			if m[1] == "Tasks" || m[1] == "Notes" {
				section = ""
				continue
			}
			section = m[1]
			project := stack[len(stack)-1].handle
			layout.sections[project] = append(layout.sections[project], section)
		} else if strings.HasPrefix(line, "| # |") {
			afterColumn = slices.Index(cells(line), "After")
		} else if m := readmeEntryRow.FindStringSubmatch(line); m != nil {
			placement := readmePlacement{project: stack[len(stack)-1].handle, section: section, waitsOn: []string{}}
			if afterColumn >= 0 {
				if after := cells(line)[afterColumn]; after != "" {
					placement.waitsOn = strings.Split(strings.ReplaceAll(after, "`", ""), "<br>")
				}
			}
			layout.entries[m[1]] = placement
		}
	}
	return layout
}

// decodeBoardData decodes the projection a board-data.js holds.
func decodeBoardData(t *testing.T, script string) boardengine.BoardView {
	t.Helper()
	body, prefixed := strings.CutPrefix(script, "window.BOARD = ")
	body, suffixed := strings.CutSuffix(body, ";\n")
	if !prefixed || !suffixed {
		t.Fatalf("board-data.js = %q; want `window.BOARD = <json>;` and a newline", script)
	}
	var view boardengine.BoardView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode board-data.js: %v", err)
	}
	return view
}

// viewEntries indexes the entries of view by slug.
func viewEntries(view boardengine.BoardView) map[string]boardengine.ViewEntry {
	entries := map[string]boardengine.ViewEntry{}
	for _, entry := range view.Entries {
		entries[entry.Slug] = entry
	}
	return entries
}

// revisionOf is the entry revision of task, failing the test when it has none.
func revisionOf(t *testing.T, task boardengine.Task) string {
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
	out := boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", DefaultRecipe: "darn", Types: []string{"bug", "enhancement", "research"}, Labels: []string{"ui"}}
	tasks := viewFixture()
	files, err := boardengine.Render(tasks, "neve", out, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	script := files[boardengine.BoardDataFile]
	view := decodeBoardData(t, script)
	entries := viewEntries(view)
	layout := parseReadmeLayout(files["README.md"])

	t.Run("PlacementMatchesTheReadme", func(t *testing.T) {
		t.Parallel()
		var absent []string
		for _, entry := range view.Entries {
			want, shown := layout.entries[entry.Handle]
			if !shown {
				absent = append(absent, entry.Slug)
				continue
			}
			got := readmePlacement{project: entry.Project, section: entry.Section, waitsOn: entry.WaitsOn}
			if got.project != want.project || got.section != want.section || !slices.Equal(got.waitsOn, want.waitsOn) {
				t.Errorf("entry %s = %+v; want %+v as the README places it", entry.Slug, got, want)
			}
		}
		// A done project has no README section, so its task has no row.
		if !slices.Equal(absent, []string{"old-task"}) {
			t.Errorf("entries with no README row = %v; want [old-task]", absent)
		}
		if got := entries["old-task"].Project; got != "P3" {
			t.Errorf("old-task project = %q; want the done project's handle P3", got)
		}
	})

	t.Run("ProjectsNameTheirParents", func(t *testing.T) {
		t.Parallel()
		var handles []string
		parents := map[string]string{}
		for _, project := range view.Projects {
			handles = append(handles, project.Handle)
			if project.Handle != "P0" {
				parents[project.Handle] = project.Parent
			}
		}
		if want := []string{"P0", "P1", "P2", "P4"}; !slices.Equal(handles, want) {
			t.Errorf("projects = %v; want %v", handles, want)
		}
		if !maps.Equal(parents, layout.parents) {
			t.Errorf("project parents = %v; want %v as the README nests them", parents, layout.parents)
		}
		if parents["P4"] != "P0" || parents["P2"] != "P1" {
			t.Errorf("project parents = %v; want P4 under P0 and P2 under P1", parents)
		}
		if root := view.Projects[0]; root.Slug != "root" || root.Parent != "" || root.DesignFile != "" {
			t.Errorf("root project = %+v; want slug root with no parent and no design file", root)
		}
	})

	t.Run("SectionsFollowTheReadme", func(t *testing.T) {
		t.Parallel()
		want := []string{"Running", "Ready", "Layer A", "Layer B", "Independent", "Draft", "Research", "Bugs", "Done"}
		if !slices.Equal(view.Sections, want) {
			t.Errorf("sections = %v; want %v", view.Sections, want)
		}
		shown := map[string]bool{}
		for project, sections := range layout.sections {
			last := -1
			for _, name := range sections {
				shown[name] = true
				at := slices.Index(view.Sections, name)
				if at <= last {
					t.Errorf("README sections of %s = %v; want them in the order of %v", project, sections, view.Sections)
				}
				last = at
			}
		}
		if len(shown) != len(view.Sections) {
			t.Errorf("README section names = %v; want those of %v", slices.Collect(maps.Keys(shown)), view.Sections)
		}
	})

	t.Run("FactsCarryTheStoredValues", func(t *testing.T) {
		t.Parallel()
		if view.Rendered != "2026-10-12T09:30:00Z" || view.PrimeOwner != "neve" {
			t.Errorf("rendered, prime_owner = %q, %q; want 2026-10-12T09:30:00Z, neve", view.Rendered, view.PrimeOwner)
		}
		if !slices.Equal(view.Types, out.Types) || !slices.Equal(view.Areas, out.Labels) {
			t.Errorf("types, areas = %v, %v; want %v, %v", view.Types, view.Areas, out.Types, out.Labels)
		}
		base := entries["base"]
		if base.DesignFile != "design-base.md" || entries["next"].DesignFile != "" {
			t.Errorf("design files of base, next = %q, %q; want design-base.md and none", base.DesignFile, entries["next"].DesignFile)
		}
		if view.Projects[1].DesignFile != "design-alpha.md" || view.Projects[2].DesignFile != "" {
			t.Errorf("project design files = %q, %q; want design-alpha.md and none", view.Projects[1].DesignFile, view.Projects[2].DesignFile)
		}
		if base.Recipe != "darn" || entries["study"].Recipe != "" {
			t.Errorf("recipes of a task, a note = %q, %q; want darn and none", base.Recipe, entries["study"].Recipe)
		}
		if base.Owner != "neve" || !base.Created.Equal(tasks[5].Created) || !base.Revised.Equal(tasks[5].Revised) {
			t.Errorf("base owner, created, revised = %q, %v, %v; want neve and the stored dates", base.Owner, base.Created, base.Revised)
		}
		if got := entries["sketched"].DecisionFile; got != "decision-sketched.md" {
			t.Errorf("sketched decision_file = %q; want decision-sketched.md", got)
		}
		if !slices.Equal(base.Types, []string{"bug"}) || !slices.Equal(base.Areas, []string{"ui"}) {
			t.Errorf("base types, areas = %v, %v; want [bug], [ui]", base.Types, base.Areas)
		}
		for _, entry := range view.Entries {
			for _, area := range entry.Areas {
				if slices.Contains(out.Types, area) {
					t.Errorf("entry %s lists type label %q among its areas", entry.Slug, area)
				}
			}
		}
		for _, task := range tasks {
			if task.Kind != boardengine.KindProject && entries[task.Slug].Revision != revisionOf(t, task) {
				t.Errorf("revision of %s = %q; want that of the stored entry", task.Slug, entries[task.Slug].Revision)
			}
		}
	})

	t.Run("BodyCannotEndTheScript", func(t *testing.T) {
		t.Parallel()
		for _, markup := range []string{"</script>", "<b>"} {
			if strings.Contains(script, markup) {
				t.Errorf("board-data.js holds a literal %q", markup)
			}
		}
		if got := entries["base"].Body; got != "see </script> <b>bold</b>" {
			t.Errorf("decoded body = %q; want the stored body", got)
		}
	})

	t.Run("MissingDecisionRecordIsNotNamed", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := boardengine.RenderToDisk(dir, tasks, "neve", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, boardengine.BoardDataFile))
		if err != nil {
			t.Fatalf("read board-data.js: %v", err)
		}
		got := viewEntries(decodeBoardData(t, string(data)))["sketched"]
		stored := tasks[slices.IndexFunc(tasks, func(task boardengine.Task) bool { return task.Slug == "sketched" })]
		if got.DecisionFile != "" || got.Revision != revisionOf(t, stored) {
			t.Errorf("sketched decision_file, revision = %q, %q; want none and the revision of the stored, discussed entry", got.DecisionFile, got.Revision)
		}
	})

	t.Run("BoardRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := boardengine.BoardRevision(dir); err != nil || got != "" {
			t.Errorf("BoardRevision(no board.json) = %q, %v; want empty and no error", got, err)
		}
		content := []byte(`{"version":1,"entries":[]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), content, 0o644); err != nil {
			t.Fatalf("write board.json: %v", err)
		}
		sum := sha256.Sum256(content)
		if got, err := boardengine.BoardRevision(dir); err != nil || got != hex.EncodeToString(sum[:]) {
			t.Errorf("BoardRevision() = %q, %v; want %x", got, err, sum)
		}
	})

	t.Run("ViewMigratesWithoutSaving", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		content := `{"version":1,"entries":[{"id":0,"slug":"a","title":"A","kind":"task","labels":["bug"]}]}`
		path := filepath.Join(dir, "board.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write board.json: %v", err)
		}
		b := boardengine.New(boardengine.Config{Path: dir, Readme: "README.md", DesignPrefix: "design-", Types: testTypes, Labels: testLabels, SkipGit: true})
		got, err := b.View()
		if err != nil {
			t.Fatalf("View: %v", err)
		}
		migrated := boardengine.Task{Handle: "T1", Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"bug"}, Issues: []int{}, DependsOn: []string{}, Status: stringPtr(boardengine.StatusDraft)}
		if entry := viewEntries(got)["a"]; entry.Status != boardengine.StatusDraft || entry.Revision != revisionOf(t, migrated) {
			t.Errorf("View() entry = %+v; want the migrated draft entry and its revision", entry)
		}
		if after, err := os.ReadFile(path); err != nil || string(after) != content {
			t.Errorf("board.json after View = %q, %v; want it unchanged", after, err)
		}
	})
}
