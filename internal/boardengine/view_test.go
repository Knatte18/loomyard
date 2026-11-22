// view_test.go unit-tests the board's view projection in view.go.
// It covers board-data.js as Render and RenderToDisk write it, the entry and board revisions and Board.View.

package boardengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewOutputs is the configuration the projection tests render with.
var viewOutputs = boardengine.Outputs{
	Readme:        "README.md",
	DesignPrefix:  "design-",
	DefaultRecipe: "darn",
	Types:         []string{"bug", "enhancement"},
	Labels:        []string{"area", "docs"},
}

// viewPrimeOwner is the prime owner the projection tests render with.
const viewPrimeOwner = "neve"

// scriptBody holds a closing script tag and an HTML tag, which board-data.js must carry only escaped.
const scriptBody = "Closes </script> and <b>bold</b> & more"

// viewFixture returns a board of three projects and a done one.
// It holds tasks in every layer, notes of two types, an entry with a body and a discussed entry.
// The root holds one entry of every section the README can show, so its section list is the whole README order.
func viewFixture() []boardengine.Task {
	task, note, project := boardengine.KindTask, boardengine.KindNote, boardengine.KindProject
	return []boardengine.Task{
		{ID: 1, Handle: "P1", Slug: "alpha", Title: "Alpha", Kind: project, Labels: []string{"enhancement"}, Brief: "Alpha brief.", Body: "Alpha design."},
		{ID: 2, Handle: "P2", Slug: "beta", Title: "Beta", Kind: project, Labels: []string{"bug"}, Project: "alpha"},
		{ID: 3, Handle: "P3", Slug: "gamma", Title: "Gamma", Kind: project, Labels: []string{"bug"}, Status: stringPtr(boardengine.StatusDone)},
		{ID: 4, Handle: "P4", Slug: "delta", Title: "Delta", Kind: project, Labels: []string{"bug"}, Project: "gamma"},
		{ID: 5, Handle: "T1", Slug: "base", Title: "Base", Kind: task, Labels: []string{"enhancement", "area"}, Body: scriptBody, Recipe: "tapestry",
			Created: renderTime.AddDate(0, 0, -9), Revised: renderTime.AddDate(0, 0, -2)},
		{ID: 6, Handle: "T2", Slug: "next", Title: "Next", Kind: task, Labels: []string{"bug"}, DependsOn: []string{"base"}},
		{ID: 7, Handle: "T3", Slug: "later", Title: "Later", Kind: task, Labels: []string{"bug"}, DependsOn: []string{"next", "ghost"}},
		{ID: 8, Handle: "T4", Slug: "solo", Title: "Solo", Kind: task, Labels: []string{"bug"}, Isolated: true},
		{ID: 9, Handle: "T5", Slug: "wip", Title: "Wip", Kind: task, Labels: []string{"bug"}, Status: stringPtr(boardengine.RunStatus("running", "loom"))},
		{ID: 10, Handle: "T6", Slug: "drafty", Title: "Drafty", Kind: task, Labels: []string{"enhancement"}, Status: stringPtr(boardengine.StatusDraft)},
		{ID: 11, Handle: "T7", Slug: "finished", Title: "Finished", Kind: task, Labels: []string{"bug"}, Status: stringPtr(boardengine.StatusDone)},
		{ID: 12, Handle: "T8", Slug: "decided", Title: "Decided", Kind: task, Labels: []string{"enhancement"}, Body: "Decided design.", Discussed: true},
		{ID: 13, Handle: "T9", Slug: "alphatask", Title: "Alpha task", Kind: task, Labels: []string{"bug"}, Project: "alpha", DependsOn: []string{"base"}},
		{ID: 14, Handle: "T10", Slug: "gtask", Title: "Gamma task", Kind: task, Labels: []string{"bug"}, Project: "gamma"},
		{ID: 15, Handle: "B1", Slug: "bugnote", Title: "Bug note", Kind: note, Labels: []string{"bug", "area"}, Brief: "A bug.", Priority: boardengine.PriorityHigh},
		{ID: 16, Handle: "E1", Slug: "enhnote", Title: "Enhancement note", Kind: note, Labels: []string{"enhancement"}},
	}
}

// decodeBoardData strips the assignment board-data.js wraps its JSON in and decodes the projection.
func decodeBoardData(t *testing.T, script string) boardengine.BoardView {
	t.Helper()
	const prefix, suffix = "window.BOARD = ", ";\n"
	if !strings.HasPrefix(script, prefix) || !strings.HasSuffix(script, suffix) {
		t.Fatalf("board-data.js = %q...; want it to start with %q and end with %q", script[:min(len(script), 40)], prefix, suffix)
	}
	var view boardengine.BoardView
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(script, prefix), suffix)), &view); err != nil {
		t.Fatalf("decode board-data.js: %v", err)
	}
	return view
}

// readmePlacement is where the README shows one entry: its project, its section and the After cell's references.
type readmePlacement struct {
	project string
	section string
	waitsOn []string
}

var (
	projectHeadingPattern = regexp.MustCompile(`^(#+) \[(P\d+) `)
	ownEntriesPattern     = regexp.MustCompile(`^(#+) Own entries$`)
	sectionHeadingPattern = regexp.MustCompile(`^#+ (.+) \(\d+\)$`)
	entryRowPattern       = regexp.MustCompile(`^\| ([A-Z]\d+) \|`)
)

// tableCells splits one markdown table row into its trimmed cells.
func tableCells(row string) []string {
	cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(row, "| "), " |"), " | ")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// readmePlacements reads, from the README alone, where each entry row stands.
// A placement is the entry's project, its section and its After cell.
func readmePlacements(readme string) map[string]readmePlacement {
	type frame struct {
		level  int
		handle string
	}
	var stack []frame
	popTo := func(level int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
	}
	placements := map[string]readmePlacement{}
	section, afterColumn := "", -1
	for _, line := range strings.Split(readme, "\n") {
		if m := projectHeadingPattern.FindStringSubmatch(line); m != nil {
			popTo(len(m[1]))
			stack = append(stack, frame{len(m[1]), m[2]})
		} else if m := ownEntriesPattern.FindStringSubmatch(line); m != nil {
			popTo(len(m[1]))
		} else if m := sectionHeadingPattern.FindStringSubmatch(line); m != nil {
			section = m[1]
		} else if strings.HasPrefix(line, "| # |") {
			afterColumn = slices.Index(tableCells(line), "After")
		} else if m := entryRowPattern.FindStringSubmatch(line); m != nil {
			placement := readmePlacement{project: stack[len(stack)-1].handle, section: section}
			if afterColumn >= 0 {
				for _, ref := range strings.Split(tableCells(line)[afterColumn], "<br>") {
					if ref = strings.Trim(ref, "`"); ref != "" {
						placement.waitsOn = append(placement.waitsOn, ref)
					}
				}
			}
			placements[m[1]] = placement
		}
	}
	return placements
}

// rootSectionNames lists the section headings of the root's README block, which holds a section of every kind.
func rootSectionNames(t *testing.T, readme string) []string {
	t.Helper()
	_, rootBlock, found := strings.Cut(readme, "](project-root.md)")
	if !found {
		t.Fatalf("README has no root section")
	}
	var names []string
	for _, line := range strings.Split(rootBlock, "\n") {
		if m := sectionHeadingPattern.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	return names
}

// TestRenderBoardData pins board-data.js, the view projection Render writes beside the README.
// It covers every entry's placement as the README shows it and the project tree.
// It also covers the sections, the escaping and the revisions.
// It also covers the render-to-disk copy that clears a missing decision record, BoardRevision and Board.View.
func TestRenderBoardData(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tasks := viewFixture()
	files, err := boardengine.Render(tasks, viewPrimeOwner, viewOutputs, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	script := files[boardengine.BoardDataFile]
	view := decodeBoardData(t, script)
	entries := map[string]boardengine.ViewEntry{}
	for _, entry := range view.Entries {
		entries[entry.Slug] = entry
	}
	stored := map[string]boardengine.Task{}
	for _, task := range tasks {
		stored[task.Slug] = task
	}

	t.Run("EntriesStandWhereTheReadmeShowsThem", func(t *testing.T) {
		t.Parallel()
		placements := readmePlacements(files["README.md"])
		if len(view.Entries) != len(tasks)-4 {
			t.Errorf("entries = %d; want every task and note, %d", len(view.Entries), len(tasks)-4)
		}
		for _, entry := range view.Entries {
			if entry.Slug == "gtask" {
				// A done project has no README section, so its task has no row to compare.
				if entry.Project != "P3" {
					t.Errorf("gtask project = %q; want the done project's handle P3", entry.Project)
				}
				continue
			}
			want, shown := placements[entry.Handle]
			if !shown {
				t.Errorf("%s has no row in the README", entry.Handle)
				continue
			}
			if entry.Project != want.project || entry.Section != want.section || !slices.Equal(entry.WaitsOn, want.waitsOn) {
				t.Errorf("%s = project %q, section %q, waits_on %v; want the README's %q, %q, %v",
					entry.Handle, entry.Project, entry.Section, entry.WaitsOn, want.project, want.section, want.waitsOn)
			}
		}
	})

	t.Run("ProjectsListTheRootThenEachOpenProjectWithItsParent", func(t *testing.T) {
		t.Parallel()
		want := []boardengine.ViewProject{
			{Handle: "P0", Slug: "root", Title: "Root: entries under no project", Labels: []string{}},
			{Handle: "P1", Slug: "alpha", Title: "Alpha", Brief: "Alpha brief.", Labels: []string{"enhancement"}, Parent: "P0", DesignFile: "design-alpha.md"},
			{Handle: "P2", Slug: "beta", Title: "Beta", Labels: []string{"bug"}, Parent: "P1"},
			// The README nests an open project under a done one beneath the root.
			{Handle: "P4", Slug: "delta", Title: "Delta", Labels: []string{"bug"}, Parent: "P0"},
		}
		if !reflect.DeepEqual(view.Projects, want) {
			t.Errorf("projects = %+v; want %+v", view.Projects, want)
		}
	})

	t.Run("HeaderAndSections", func(t *testing.T) {
		t.Parallel()
		if want := renderTime.UTC().Format(time.RFC3339); view.Rendered != want {
			t.Errorf("rendered = %q; want %q", view.Rendered, want)
		}
		if view.PrimeOwner != viewPrimeOwner {
			t.Errorf("prime_owner = %q; want %q", view.PrimeOwner, viewPrimeOwner)
		}
		if !reflect.DeepEqual(view.Types, viewOutputs.Types) || !reflect.DeepEqual(view.Areas, viewOutputs.Labels) {
			t.Errorf("types, areas = %v, %v; want %v, %v", view.Types, view.Areas, viewOutputs.Types, viewOutputs.Labels)
		}
		if want := rootSectionNames(t, files["README.md"]); !slices.Equal(view.Sections, want) {
			t.Errorf("sections = %v; want the README's %v", view.Sections, want)
		}
	})

	t.Run("EntriesCarryTheirStoredValues", func(t *testing.T) {
		t.Parallel()
		base := entries["base"]
		if base.DesignFile != "design-base.md" || entries["next"].DesignFile != "" {
			t.Errorf("design files = %q, %q; want design-base.md for the entry with a body and none for the other", base.DesignFile, entries["next"].DesignFile)
		}
		if base.Recipe != "tapestry" || entries["next"].Recipe != "darn" || entries["bugnote"].Recipe != "" {
			t.Errorf("recipes = %q, %q, %q; want its own, the default and none for a note", base.Recipe, entries["next"].Recipe, entries["bugnote"].Recipe)
		}
		if base.Owner != viewPrimeOwner || base.Created != "2026-10-03T09:30:00Z" || base.Revised != "2026-10-10T09:30:00Z" {
			t.Errorf("base owner, created, revised = %q, %q, %q; want the prime owner and the stored dates", base.Owner, base.Created, base.Revised)
		}
		if got := entries["wip"].Status; got != boardengine.RunStatus("running", "loom") {
			t.Errorf("wip status = %q; want its run status", got)
		}
		if got := entries["decided"]; got.DecisionFile != "decision-decided.md" || got.DesignFile != "design-decided.md" || base.DecisionFile != "" {
			t.Errorf("decided files = %q, %q, base decision file %q; want both files named and base's empty", got.DesignFile, got.DecisionFile, base.DecisionFile)
		}
		if bug := entries["bugnote"]; !slices.Equal(bug.Types, []string{"bug"}) || !slices.Equal(bug.Areas, []string{"area"}) {
			t.Errorf("bugnote types, areas = %v, %v; want [bug], [area]", bug.Types, bug.Areas)
		}
		for _, entry := range view.Entries {
			if slices.ContainsFunc(entry.Areas, func(label string) bool { return slices.Contains(view.Types, label) }) {
				t.Errorf("%s areas = %v; want no type label among them", entry.Handle, entry.Areas)
			}
			encoded, err := json.Marshal(stored[entry.Slug])
			if err != nil {
				t.Fatalf("encode stored %s: %v", entry.Handle, err)
			}
			sum := sha256.Sum256(encoded)
			if want := hex.EncodeToString(sum[:]); entry.Revision != want {
				t.Errorf("%s revision = %q; want the stored entry's %q", entry.Handle, entry.Revision, want)
			}
		}
	})

	t.Run("ABodyCannotEndTheScript", func(t *testing.T) {
		t.Parallel()
		if strings.Contains(script, "</script>") || strings.Contains(script, "<b>") {
			t.Errorf("board-data.js holds a literal </script> or <b>; want them escaped")
		}
		if got := entries["base"].Body; got != scriptBody {
			t.Errorf("base body = %q; want it to round-trip as %q", got, scriptBody)
		}
	})

	t.Run("RenderToDiskClearsAMissingDecisionRecordButKeepsTheStoredRevision", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		discussed := stored["decided"]
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{discussed}, viewPrimeOwner, viewOutputs, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		script, err := os.ReadFile(filepath.Join(dir, boardengine.BoardDataFile))
		if err != nil {
			t.Fatalf("read board-data.js: %v", err)
		}
		written := decodeBoardData(t, string(script))
		if len(written.Entries) != 1 || written.Entries[0].DecisionFile != "" {
			t.Fatalf("entries = %+v; want the one entry naming no decision file", written.Entries)
		}
		if want := boardengine.EntryRevision(discussed); written.Entries[0].Revision != want {
			t.Errorf("revision = %q; want the stored, still-discussed entry's %q", written.Entries[0].Revision, want)
		}
	})

	t.Run("BoardRevisionIsTheHashOfBoardJSON", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got, err := boardengine.BoardRevision(dir); err != nil || got != "" {
			t.Errorf("BoardRevision(absent) = %q, %v; want empty and no error", got, err)
		}
		content := []byte(`{"version":1,"entries":[]}`)
		if err := os.WriteFile(filepath.Join(dir, "board.json"), content, 0o644); err != nil {
			t.Fatalf("seed board.json: %v", err)
		}
		sum := sha256.Sum256(content)
		if got, err := boardengine.BoardRevision(dir); err != nil || got != hex.EncodeToString(sum[:]) {
			t.Errorf("BoardRevision() = %q, %v; want %q", got, err, hex.EncodeToString(sum[:]))
		}
	})
}
