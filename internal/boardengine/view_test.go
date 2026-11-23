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

// viewFixture returns a board of three open projects and two done ones, one nested in the other.
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
		{ID: 17, Handle: "T11", Slug: "glanded", Title: "Gamma landed", Kind: task, Labels: []string{"bug"}, Project: "gamma", Status: stringPtr(boardengine.StatusDone)},
		{ID: 18, Handle: "P5", Slug: "epsilon", Title: "Epsilon", Kind: project, Labels: []string{"bug"}, Project: "gamma", Status: stringPtr(boardengine.StatusDone)},
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

// placement is where board.html shows one entry: its project, its section and the handles it waits on.
type placement struct {
	project string
	section string
	waitsOn []string
}

// TestRenderBoardData pins board-data.js, the view projection Render writes beside the README for board.html.
// It covers every entry's placement and the project tree.
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

	t.Run("EntriesStandInTheirProjectAndSection", func(t *testing.T) {
		t.Parallel()
		want := map[string]placement{
			"T1":  {project: "P0", section: "Ready"},
			"T2":  {project: "P0", section: "Layer A", waitsOn: []string{"T1"}},
			"T3":  {project: "P0", section: "Layer B", waitsOn: []string{"T2", "ghost"}},
			"T4":  {project: "P0", section: "Independent"},
			"T5":  {project: "P0", section: "Running"},
			"T6":  {project: "P0", section: "Draft"},
			"T7":  {project: "P0", section: "Done"},
			"T8":  {project: "P0", section: "Ready"},
			"T9":  {project: "P1", section: "Layer A", waitsOn: []string{"T1"}},
			"T10": {project: "P3", section: "Ready"},
			"T11": {project: "P3", section: "Done"},
			"B1":  {project: "P0", section: "Bugs"},
			"E1":  {project: "P0", section: "Enhancements"},
		}
		if len(view.Entries) != len(want) {
			t.Errorf("entries = %d; want every task and note, %d", len(view.Entries), len(want))
		}
		for _, entry := range view.Entries {
			w, found := want[entry.Handle]
			if !found {
				t.Errorf("entries hold %s; want only the fixture's tasks and notes", entry.Handle)
				continue
			}
			if entry.Project != w.project || entry.Section != w.section || !slices.Equal(entry.WaitsOn, w.waitsOn) {
				t.Errorf("%s = project %q, section %q, waits_on %v; want %q, %q, %v",
					entry.Handle, entry.Project, entry.Section, entry.WaitsOn, w.project, w.section, w.waitsOn)
			}
		}
	})

	t.Run("ProjectsListTheRootThenEachOpenThenEachDoneProjectWithItsParent", func(t *testing.T) {
		t.Parallel()
		want := []boardengine.ViewProject{
			{Handle: "P0", Slug: "root", Title: "Root: entries under no project", Labels: []string{}},
			{Handle: "P1", Slug: "alpha", Title: "Alpha", Brief: "Alpha brief.", Labels: []string{"enhancement"}, Parent: "P0", DesignFile: "design-alpha.md"},
			{Handle: "P2", Slug: "beta", Title: "Beta", Labels: []string{"bug"}, Parent: "P1"},
			// The README nests an open project under a done one beneath the root.
			{Handle: "P4", Slug: "delta", Title: "Delta", Labels: []string{"bug"}, Parent: "P0"},
			// A done project stays listed, so board.html files its members, done ones included, under it and not under the root.
			{Handle: "P3", Slug: "gamma", Title: "Gamma", Labels: []string{"bug"}, Parent: "P0", Done: true},
			// A done project inside a done project stands under it.
			{Handle: "P5", Slug: "epsilon", Title: "Epsilon", Labels: []string{"bug"}, Parent: "P3", Done: true},
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
		want := []string{"Running", "Ready", "Layer A", "Layer B", "Independent", "Draft", "Bugs", "Enhancements", "Done"}
		if !slices.Equal(view.Sections, want) {
			t.Errorf("sections = %v; want %v", view.Sections, want)
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
		given := []boardengine.Task{discussed}
		if err := boardengine.RenderToDisk(dir, given, viewPrimeOwner, viewOutputs, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		if !given[0].Discussed {
			t.Errorf("given[0].Discussed = false after RenderToDisk; want the caller's entry left flagged")
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
