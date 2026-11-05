// render_test.go — unit tests for rendering (render.go).
//
// README and design-doc goldens over the kind-and-label layout: tasks in layers, notes grouped by type with an Other group, done and abandoned entries, dependencies, bodies, and the omitted Done section and empty Notes subsections.
// Also covers the manifest-based cleanup introduced in RenderToDisk: renamed outputs are removed
// across consecutive renders,
// and a missing or corrupt manifest degrades gracefully.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// renderTime is the time every render in these tests reads its ages as of.
var renderTime = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

// seedManifest writes a .board-rendered.json manifest into dir listing names,
// simulating the sidecar that a prior render would have left behind.
func seedManifest(t *testing.T, dir string, names []string) {
	t.Helper()
	data, err := json.Marshal(names)
	if err != nil {
		t.Fatalf("seedManifest: marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".board-rendered.json"), data, 0o644); err != nil {
		t.Fatalf("seedManifest: write: %v", err)
	}
}

// TestRenderToDiskManifestCleanup covers RenderToDisk's outputs and the manifest-based cleanup scenarios:
// a pre-seeded orphan removed with a custom design prefix, renamed outputs removed across consecutive renders, body loss removing a design doc, a removed project dropping its board file, unrelated files left untouched, and graceful degradation for missing/corrupt manifests.
func TestRenderToDiskManifestCleanup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("SeededManifestRemovesOrphanAndWritesOutputs", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{
			{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Body: "proposal A"},
			{ID: 1, Slug: "b", Title: "B", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}, // no body → no design-doc file
		}

		// A stale design doc from a previous render, pre-seeded into the manifest, so the one RenderToDisk call removes it: a first render with no prior manifest removes nothing.
		ghost := filepath.Join(dir, "prop-ghost.md")
		if err := os.WriteFile(ghost, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		seedManifest(t, dir, []string{"prop-ghost.md"})

		if err := boardengine.RenderToDisk(dir, tasks, "", boardengine.Outputs{Readme: "Home.md", DesignPrefix: "prop-"}, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
			t.Errorf("Home.md not written: %v", err)
		}
		if b, err := os.ReadFile(filepath.Join(dir, "prop-a.md")); err != nil || !strings.HasSuffix(string(b), "proposal A") {
			t.Errorf("prop-a.md: got %q, err %v", b, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "prop-b.md")); !os.IsNotExist(err) {
			t.Errorf("prop-b.md should not exist (task has no body)")
		}
		if _, err := os.Stat(ghost); !os.IsNotExist(err) {
			t.Errorf("orphan prop-ghost.md should have been removed")
		}
	})

	t.Run("ReadmeRename", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}}

		// First render produces Home.md and seeds the manifest with it.
		out1 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, "", out1, renderTime); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}

		// Second render uses Index.md; the manifest from the first render lists Home.md,
		// so it is removed because the new output set does not contain it.
		out2 := boardengine.Outputs{Readme: "Index.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, "", out2, renderTime); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "Index.md")); err != nil {
			t.Errorf("Index.md should exist after second render: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "Home.md")); !os.IsNotExist(err) {
			t.Errorf("Home.md should have been removed after rename to Index.md")
		}
	})

	t.Run("ProposalPrefixChange", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Body: "body"}}

		// First render with prefix "proposal-" produces proposal-a.md.
		out1 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, "", out1, renderTime); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render with prefix "task-" produces task-a.md; manifest cleanup
		// removes proposal-a.md because it is no longer in the output set.
		out2 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "task-"}
		if err := boardengine.RenderToDisk(dir, tasks, "", out2, renderTime); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "task-a.md")); err != nil {
			t.Errorf("task-a.md should exist after second render: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); !os.IsNotExist(err) {
			t.Errorf("proposal-a.md should have been removed after prefix change to task-")
		}
	})

	t.Run("BodyLoss", func(t *testing.T) {
		dir := t.TempDir()
		task := boardengine.Task{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Body: "original body"}
		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}

		// First render: task has a body → proposal-a.md is produced and recorded in the manifest.
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render: task loses its body → proposal-a.md is absent from the new
		// output set but present in the manifest, so the manifest cleanup removes it.
		task.Body = ""
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); !os.IsNotExist(err) {
			t.Errorf("proposal-a.md should have been removed after the task lost its body")
		}
	})

	t.Run("ProjectRemoved", func(t *testing.T) {
		dir := t.TempDir()
		project := boardengine.Task{ID: 0, Handle: "P1", Slug: "group", Title: "Group", Kind: boardengine.KindProject}
		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}

		// First render: the project gets its own board file, recorded in the manifest.
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{project}, "", out, renderTime); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "project-group.md")); err != nil {
			t.Fatalf("project-group.md should exist after first render: %v", err)
		}

		// Second render: the project is gone, so the manifest cleanup removes its file.
		if err := boardengine.RenderToDisk(dir, nil, "", out, renderTime); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "project-group.md")); !os.IsNotExist(err) {
			t.Errorf("project-group.md should have been removed after the project left the board")
		}
	})

	t.Run("UnrelatedFileNotRemoved", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}}

		// A hand-added file in the board dir that was never produced by a render.
		readme := filepath.Join(dir, "NOTES.md")
		if err := os.WriteFile(readme, []byte("# Notes"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		// First render seeds the manifest with the rendered files (not NOTES.md).
		if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		// Second render triggers cleanup; NOTES.md was never in the manifest so it is untouched.
		if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(readme); err != nil {
			t.Errorf("NOTES.md should not have been removed (never in manifest): %v", err)
		}
	})

	t.Run("NoManifestSeedsAndRemovesNothing", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}}

		// A file that looks like an orphan under the old glob approach but is absent
		// from the manifest because no manifest exists yet (pre-upgrade state).
		stale := filepath.Join(dir, "proposal-stale.md")
		if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}

		// First render with no prior manifest: nothing is removed (graceful degradation),
		// and the manifest is seeded with the current output set.
		if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk should not fail when no manifest exists: %v", err)
		}
		if _, err := os.Stat(stale); err != nil {
			t.Errorf("stale file should NOT be removed on first render (no prior manifest): %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".board-rendered.json")); err != nil {
			t.Errorf("manifest should have been created after first render: %v", err)
		}
	})

	t.Run("CorruptManifestDoesNotFailWrite", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}}

		// Write a corrupt manifest; RenderToDisk must treat it as absent (no cleanup)
		// and overwrite it with the current render set.
		if err := os.WriteFile(filepath.Join(dir, ".board-rendered.json"), []byte("not valid json {{{"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
			t.Errorf("RenderToDisk should not fail with a corrupt manifest: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
			t.Errorf("Home.md should be written even with corrupt manifest: %v", err)
		}
	})
}

// readmeTypes is the type-label order the README goldens render Notes sections in.
var readmeTypes = []string{"bug", "enhancement", "research"}

// readmeFixture holds tasks and notes with labels, two entries with dates and the rest without, plus a done task that another task depends on, an abandoned note, a body, an isolated task, a two-layer chain, a title with a pipe, a two-line brief, a research note, a note whose type label is not in readmeTypes, a high-priority note, a low-priority note listed before a normal one of the same type, a profile with a pipe on a task with no recipe, a recipe with a profile, and run settings on three tasks: two keys in one module, a nested key with a pipe in its value beside a list value, and one key with no brief.
func readmeFixture() []boardengine.Task {
	task, note := boardengine.KindTask, boardengine.KindNote
	return []boardengine.Task{
		{ID: 1, Handle: "T1", Slug: "base", Title: "Base work", Kind: task, Labels: []string{"enhancement"}, Status: stringPtr(boardengine.StatusDraft), Brief: "The foundation.", Created: renderTime.AddDate(0, 0, -40), Revised: renderTime.AddDate(0, 0, -3), RunSettings: map[string]any{"loom": map[string]any{"review_max_bounces": 2.0, "review_hard_max_bounces": 3.0}}},
		{ID: 2, Handle: "T2", Slug: "top", Title: "Top | work", Kind: task, Labels: []string{"bug", "area"}, Status: stringPtr("running"), Profile: "a|b", Brief: "Builds on base.\nSecond brief line.", Body: "Design.\nSecond line.", DependsOn: []string{"base", "shipped"}, RunSettings: map[string]any{"darn": map[string]any{"models": map[string]any{"writer": "opus|fast"}, "skip": []any{"a&b", "<c>"}, "note": "use `x`"}}},
		{ID: 6, Handle: "T3", Slug: "alone", Title: "Alone work", Kind: task, Labels: []string{"enhancement"}, Isolated: true, Recipe: "darn", Profile: "docs", RunSettings: map[string]any{"darn": map[string]any{"review_max_bounces": 1.0}}},
		{ID: 3, Handle: "E2", Slug: "idea", Title: "An idea", Kind: note, Labels: []string{"enhancement", "undecided"}, Priority: boardengine.PriorityLow},
		{ID: 8, Handle: "E1", Slug: "plain", Title: "Plain idea", Kind: note, Labels: []string{"enhancement"}},
		{ID: 9, Handle: "R1", Slug: "measure", Title: "A measurement", Kind: note, Labels: []string{"research"}},
		{ID: 4, Handle: "B1", Slug: "dropped", Title: "Dropped idea", Kind: note, Labels: []string{"bug"}, Status: stringPtr("abandoned"), Brief: "No longer wanted.", Priority: boardengine.PriorityHigh},
		{ID: 7, Handle: "X1", Slug: "stray", Title: "Stray note", Kind: note, Labels: []string{"retired"}},
		{ID: 5, Handle: "T5", Slug: "shipped", Title: "Shipped work", Kind: task, Labels: []string{"enhancement"}, Status: stringPtr("done"), Created: renderTime.AddDate(-1, 0, -10), Revised: renderTime.AddDate(0, -5, 0)},
	}
}

// TestRenderReadmeGolden pins the README tables for a fixture with tasks and notes, notes grouped by type with research first, headed Research, then the other types in Outputs.Types order and an Other group, a done entry, an abandoned note, a slug linked to its design doc, only non-type labels, an After cell naming the open dependency by handle and leaving out a done one, an isolated task, and a Ready task with one in Layer A after it.
// The same row pins that every row opens with the entry's handle and that each group is one table sorted by handle, a note with a lower handle and a higher ID first.
// It pins that among the root's tables only the Notes tables carry the priority column, headed Prio, H for high, L for low and blank for normal.
// It pins that a pipe in a title is escaped and a line break in a brief becomes a space, so each row stays one table row.
// The same row pins that each run setting stacks as a code-span bullet in the entry cell after any brief, sorted, a nested key as its dotted path, a non-string value as JSON and a pipe escaped, adding no column.
// Every Tasks table pins a Recipe column after the slug, the fixture's default recipe for an empty recipe and the stored name otherwise, followed by ` · ` and the profile, pipe escaped, when the task names one, and the Notes and Done tables pin its absence.
// Every README pins the root's own sections under the root project's heading, P0 linked to project-root.md and counting its open entries, then its facts line naming only the prime owner, one heading level below a project file's.
// Every README pins the render time at its top and the prime owner under it, the machine in a code span and none recorded when the board records none;
// every table pins the one column before Labels holding the age above the time since revision, as of that time and blank for unknown dates.
// A second row pins that Ready renders _None._ when empty, and that the Done section and every empty Notes subsection are omitted when no entry is done and no note exists for them.
// Neither row has a run status, the first only the hand-set word "running", so neither renders a Running subsection.
// A third row pins that a task with a run status renders under Running, before Ready and in no layer, ordered by handle over ID, its At cell dropping the state `running` and keeping any other, and its Owner cell holding the prime owner for a task outside every beam.
// The same row pins that a task waiting on a running one lands in Layer A, not Ready, still naming its handle under After, and that a task after that one lands in Layer B.
// A fourth row pins that README.md renders every open project under no other at heading level two, by handle, and nests sub-projects one heading level per depth, in their parent's order and then by handle, with an open project under a done one counted as top-level, each heading linking the project and counting its open members, then its facts, the slug there linked to its design file only for a project with a body, its brief, its sub-projects and its own Tasks, Notes and Done split as the root's.
// The same row pins that the root's section comes last, at the top-level projects' level, holding only the entries under no project, its projects not counted as its members, a done project a linked Done row there, that a project with sub-projects holds its own entries under an Own entries heading after them and one without holds them directly, and that every project, open or done, renders to project-<slug>.md and the root to project-root.md.
// A project file opens with its handle and title, a link up to its parent's file, the brief, and for a beam only, beam and its recorded owner, renders its nested projects as sections in its order and then by handle, and holds its own entries under Own entries after them.
// The root's file opens the same way with a link up to README.md and its facts line, and holds only its own entries.
// A task there waiting on a running member and a root task stands in Layer A, its After cell one handle per line, and the running member shows the beam's recorded owner.
// A dependency naming no board entry shows in the After cell as a code span of its slug, and a draft task in a project file stands in its own Draft subsection after Ready.
// A fifth row pins that a beam with a recorded owner on a board with no prime owner shows that owner in its own file.
// A sixth row pins that a chain of nested projects stops deepening its headings at level six.
// A seventh row pins that a task depending on an open project shows the project's handle under After, and one depending on a plain project whose every landing member is done leaves After empty and stands in Ready.
func TestRenderReadmeGolden(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		tasks      []boardengine.Task
		primeOwner string
		want       string
		// files holds the expected content of other board files by name.
		files map[string]string
		// projectFiles, when set, is every project file the render returns, sorted.
		projectFiles []string
	}{
		{
			name:       "tasks, notes, done and abandoned entries",
			tasks:      readmeFixture(),
			primeOwner: "neve",
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: `neve`.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				rootProjectHeading(2, 8, "`neve`") +
				"### Tasks (3)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Ready (0)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"_None._\n" +
				"\n" +
				"#### Layer A (1)\n" +
				"\n" +
				"Waits only on Running, Ready or Draft entries.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T2 | **Top \\| work**<br>• Builds on base. Second brief line.<br>• `darn: models.writer opus\\|fast`<br>• `` darn: note use `x` ``<br>• `darn: skip [\"a&b\",\"<c>\"]` | [`top`](design-top.md) | tapestry · a\\|b | T1 |  | area |\n" +
				"\n" +
				"#### Independent (1)\n" +
				"\n" +
				"Depends on nothing and nothing depends on it, by design.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T3 | **Alone work**<br>• `darn: review_max_bounces 1` | `alone` | darn · docs |  |  |  |\n" +
				"\n" +
				"#### Draft (1)\n" +
				"\n" +
				"Not released; nothing starts it until its status is cleared.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T1 | `draft` **Base work**<br>• The foundation.<br>• `loom: review_hard_max_bounces 3`<br>• `loom: review_max_bounces 2` | `base` | tapestry |  | 5w<br>3d |  |\n" +
				"\n" +
				"### Notes (5)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"#### Research (1)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| R1 |  | **A measurement** | `measure` |  |  |\n" +
				"\n" +
				"#### Bugs (1)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| B1 | H | **Dropped idea**<br>• No longer wanted. | `dropped` |  |  |\n" +
				"\n" +
				"#### Enhancements (2)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| E1 |  | **Plain idea** | `plain` |  |  |\n" +
				"| E2 | L | **An idea** | `idea` |  | undecided |\n" +
				"\n" +
				"#### Other (1)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| X1 |  | **Stray note** | `stray` |  | retired |\n" +
				"\n" +
				"### Done (1)\n" +
				"\n" +
				"Finished, awaiting `lym board prune --apply`.\n" +
				"\n" +
				"| # | Entry | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| T5 | **Shipped work** | `shipped` | 1y<br>5mo |  |\n",
		},
		{
			name:  "no done section and no empty notes subsections",
			tasks: []boardengine.Task{{ID: 1, Handle: "E1", Slug: "a", Title: "A", Kind: boardengine.KindNote, Labels: []string{"enhancement"}}},
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: none recorded.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				rootProjectHeading(2, 1, "none recorded") +
				"### Tasks (0)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Ready (0)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"_None._\n" +
				"\n" +
				"### Notes (1)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"#### Enhancements (1)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| E1 |  | **A** | `a` |  |  |\n",
		},
		{
			name: "running task under Running before Ready and in no layer",
			tasks: []boardengine.Task{
				{ID: 1, Handle: "T1", Slug: "base", Title: "Base work", Kind: boardengine.KindTask, Labels: []string{"enhancement"}},
				{ID: 2, Handle: "T5", Slug: "held", Title: "Held work", Kind: boardengine.KindTask, Labels: []string{"bug", "area"}, Status: stringPtr(boardengine.RunStatus("running", "Webster")), DependsOn: []string{"base"}},
				{ID: 3, Handle: "T3", Slug: "next", Title: "Next work", Kind: boardengine.KindTask, DependsOn: []string{"held"}},
				{ID: 4, Handle: "T4", Slug: "last", Title: "Last work", Kind: boardengine.KindTask, DependsOn: []string{"next"}},
				{ID: 5, Handle: "T2", Slug: "halted", Title: "Halted work", Kind: boardengine.KindTask, Labels: []string{"bug"}, Status: stringPtr(boardengine.RunStatus("paused", "Plan-Write")), Recipe: "darn"},
			},
			primeOwner: "neve",
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: `neve`.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				rootProjectHeading(2, 5, "`neve`") +
				"### Tasks (5)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Running (2)\n" +
				"\n" +
				"Held by a run; its scope is locked until the run ends.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | At | Owner | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T2 | **Halted work** | `halted` | darn | paused · Plan-Write | neve |  |  |\n" +
				"| T5 | **Held work** | `held` | tapestry | Webster | neve |  | area |\n" +
				"\n" +
				"#### Ready (1)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T1 | **Base work** | `base` | tapestry |  |  |  |\n" +
				"\n" +
				"#### Layer A (1)\n" +
				"\n" +
				"Waits only on Running, Ready or Draft entries.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T3 | **Next work** | `next` | tapestry | T5 |  |  |\n" +
				"\n" +
				"#### Layer B (1)\n" +
				"\n" +
				"Starts when every entry it names under After is done.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T4 | **Last work** | `last` | tapestry | T3 |  |  |\n" +
				"\n" +
				"### Notes (0)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
		},
		{
			name: "each project renders to its own file under its parent",
			tasks: []boardengine.Task{
				{ID: 1, Handle: "P1", Slug: "ship", Title: "Ship it", Kind: boardengine.KindProject, Beam: true, Brief: "Ships the thing.", OwnerMachine: "box", Order: []string{"later"}, Created: renderTime.AddDate(0, 0, -3), Body: "Ship design."},
				{ID: 2, Handle: "P2", Slug: "tidy", Title: "Tidy up", Kind: boardengine.KindProject, Brief: "Loose ends."},
				{ID: 3, Handle: "P3", Slug: "old", Title: "Old project", Kind: boardengine.KindProject, Status: stringPtr("done")},
				{ID: 10, Handle: "P4", Slug: "inner", Title: "Inner", Kind: boardengine.KindProject, Project: "ship", Brief: "Nested work."},
				{ID: 11, Handle: "P5", Slug: "later", Title: "Later", Kind: boardengine.KindProject, Project: "ship"},
				{ID: 4, Handle: "T1", Slug: "first", Title: "First step", Kind: boardengine.KindTask, Project: "ship", Priority: boardengine.PriorityLow},
				{ID: 5, Handle: "T2", Slug: "second", Title: "Second step", Kind: boardengine.KindTask, Project: "ship", Status: stringPtr(boardengine.RunStatus("running", "Webster")), Priority: boardengine.PriorityHigh, Body: "Second design."},
				{ID: 6, Handle: "T3", Slug: "third", Title: "Third step", Kind: boardengine.KindTask, Project: "ship", DependsOn: []string{"second", "base", "ghost", "shipped"}},
				{ID: 7, Handle: "E1", Slug: "thought", Title: "A thought", Kind: boardengine.KindNote, Project: "ship"},
				{ID: 8, Handle: "T4", Slug: "shipped", Title: "Shipped step", Kind: boardengine.KindTask, Project: "ship", Status: stringPtr("done")},
				{ID: 9, Handle: "T5", Slug: "chore", Title: "Chore", Kind: boardengine.KindTask, Project: "tidy", Status: stringPtr(boardengine.StatusDraft)},
				{ID: 12, Handle: "T7", Slug: "base", Title: "Base", Kind: boardengine.KindTask},
				{ID: 13, Handle: "P6", Slug: "orphan", Title: "Orphan", Kind: boardengine.KindProject, Project: "old", Brief: "Outlived its parent."},
			},
			primeOwner: "neve",
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: `neve`.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## [P1 Ship it](project-ship.md) (6)\n" +
				"\n" +
				"[`ship`](design-ship.md) · Age 3d · Owner `box`\n" +
				"\n" +
				"Ships the thing.\n" +
				"\n" +
				"### [P5 Later](project-later.md) (0)\n" +
				"\n" +
				"`later` · Owner `box`\n" +
				"\n" +
				emptyProjectEntries(4) +
				"\n" +
				"### [P4 Inner](project-inner.md) (0)\n" +
				"\n" +
				"`inner` · Owner `box`\n" +
				"\n" +
				"Nested work.\n" +
				"\n" +
				emptyProjectEntries(4) +
				"\n" +
				ownEntriesHeading(3) +
				"#### Tasks (3)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"##### Running (1)\n" +
				"\n" +
				"Held by a run; its scope is locked until the run ends.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | At | Owner | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T2 | **Second step** | [`second`](design-second.md) | tapestry | Webster | box |  |  |\n" +
				"\n" +
				"##### Ready (1)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T1 | **First step** | `first` | tapestry |  |  |  |\n" +
				"\n" +
				"##### Layer A (1)\n" +
				"\n" +
				"Waits only on Running, Ready or Draft entries.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T3 | **Third step** | `third` | tapestry | T2<br>T7<br>`ghost` |  |  |\n" +
				"\n" +
				"#### Notes (1)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"##### Other (1)\n" +
				"\n" +
				"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- |\n" +
				"| E1 |  | **A thought** | `thought` |  |  |\n" +
				"\n" +
				"#### Done (1)\n" +
				"\n" +
				"Finished, awaiting `lym board prune --apply`.\n" +
				"\n" +
				"| # | Entry | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| T4 | **Shipped step** | `shipped` |  |  |\n" +
				"\n" +
				"## [P2 Tidy up](project-tidy.md) (1)\n" +
				"\n" +
				"`tidy` · Owner `neve`\n" +
				"\n" +
				"Loose ends.\n" +
				"\n" +
				"### Tasks (1)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Ready (0)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"_None._\n" +
				"\n" +
				"#### Draft (1)\n" +
				"\n" +
				"Not released; nothing starts it until its status is cleared.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T5 | `draft` **Chore** | `chore` | tapestry |  |  |  |\n" +
				"\n" +
				"### Notes (0)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"## [P6 Orphan](project-orphan.md) (0)\n" +
				"\n" +
				"`orphan` · Owner `neve`\n" +
				"\n" +
				"Outlived its parent.\n" +
				"\n" +
				emptyProjectEntries(3) +
				"\n" +
				rootProjectHeading(2, 1, "`neve`") +
				rootEntriesWithProjects(3),
			files: map[string]string{
				"project-root.md": "# P0 Root: entries under no project\n" +
					"\n" +
					"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
					"Under: [Board](README.md).\n" +
					"\n" +
					"Owner `neve`\n" +
					"\n" +
					"Tasks are grouped by dependency layer and notes by type.\n" +
					"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
					"\n" +
					rootEntriesWithProjects(2),
				"project-ship.md": "# P1 Ship it\n" +
					"\n" +
					"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
					"Under: [Board](README.md).\n" +
					"\n" +
					"[`ship`](design-ship.md) · Age 3d · Owner `box`\n" +
					"\n" +
					"Ships the thing.\n" +
					"\n" +
					"beam\n" +
					"\n" +
					"Ship design.\n" +
					"\n" +
					"Tasks are grouped by dependency layer and notes by type.\n" +
					"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
					"\n" +
					"## [P5 Later](project-later.md) (0)\n" +
					"\n" +
					"`later` · Owner `box`\n" +
					"\n" +
					emptyProjectEntries(3) +
					"\n" +
					"## [P4 Inner](project-inner.md) (0)\n" +
					"\n" +
					"`inner` · Owner `box`\n" +
					"\n" +
					"Nested work.\n" +
					"\n" +
					emptyProjectEntries(3) +
					"\n" +
					ownEntriesHeading(2) +
					"### Tasks (3)\n" +
					"\n" +
					"Concrete and claimable; only a task can run.\n" +
					"\n" +
					"#### Running (1)\n" +
					"\n" +
					"Held by a run; its scope is locked until the run ends.\n" +
					"\n" +
					"| # | Task | Slug | Recipe | At | Owner | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- | --- | --- | --- |\n" +
					"| T2 | **Second step** | [`second`](design-second.md) | tapestry | Webster | box |  |  |\n" +
					"\n" +
					"#### Ready (1)\n" +
					"\n" +
					"Waits on nothing open; can start now.\n" +
					"\n" +
					"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- | --- | --- |\n" +
					"| T1 | **First step** | `first` | tapestry |  |  |  |\n" +
					"\n" +
					"#### Layer A (1)\n" +
					"\n" +
					"Waits only on Running, Ready or Draft entries.\n" +
					"\n" +
					"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- | --- | --- |\n" +
					"| T3 | **Third step** | `third` | tapestry | T2<br>T7<br>`ghost` |  |  |\n" +
					"\n" +
					"### Notes (1)\n" +
					"\n" +
					"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
					"\n" +
					"#### Other (1)\n" +
					"\n" +
					"| # | Prio | Note | Slug | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- | --- |\n" +
					"| E1 |  | **A thought** | `thought` |  |  |\n" +
					"\n" +
					"### Done (1)\n" +
					"\n" +
					"Finished, awaiting `lym board prune --apply`.\n" +
					"\n" +
					"| # | Entry | Slug | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- |\n" +
					"| T4 | **Shipped step** | `shipped` |  |  |\n",
				"project-inner.md": "# P4 Inner\n" +
					"\n" +
					"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
					"Under: [P1 Ship it](project-ship.md).\n" +
					"\n" +
					"`inner` · Owner `box`\n" +
					"\n" +
					"Nested work.\n" +
					"\n" +
					"Tasks are grouped by dependency layer and notes by type.\n" +
					"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
					"\n" +
					"## Tasks (0)\n" +
					"\n" +
					"Concrete and claimable; only a task can run.\n" +
					"\n" +
					"### Ready (0)\n" +
					"\n" +
					"Waits on nothing open; can start now.\n" +
					"\n" +
					"_None._\n" +
					"\n" +
					"## Notes (0)\n" +
					"\n" +
					"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
				"project-tidy.md": "# P2 Tidy up\n" +
					"\n" +
					"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
					"Under: [Board](README.md).\n" +
					"\n" +
					"`tidy` · Owner `neve`\n" +
					"\n" +
					"Loose ends.\n" +
					"\n" +
					"Tasks are grouped by dependency layer and notes by type.\n" +
					"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
					"\n" +
					"## Tasks (1)\n" +
					"\n" +
					"Concrete and claimable; only a task can run.\n" +
					"\n" +
					"### Ready (0)\n" +
					"\n" +
					"Waits on nothing open; can start now.\n" +
					"\n" +
					"_None._\n" +
					"\n" +
					"### Draft (1)\n" +
					"\n" +
					"Not released; nothing starts it until its status is cleared.\n" +
					"\n" +
					"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
					"| --- | --- | --- | --- | --- | --- | --- |\n" +
					"| T5 | `draft` **Chore** | `chore` | tapestry |  |  |  |\n" +
					"\n" +
					"## Notes (0)\n" +
					"\n" +
					"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
			},
			projectFiles: []string{"project-inner.md", "project-later.md", "project-old.md", "project-orphan.md", "project-root.md", "project-ship.md", "project-tidy.md"},
		},
		{
			name: "on a board with no prime owner a beam shows its recorded owner and a plain project none",
			tasks: []boardengine.Task{
				{ID: 1, Handle: "P1", Slug: "lone", Title: "Lone beam", Kind: boardengine.KindProject, Beam: true, OwnerMachine: "box"},
				{ID: 2, Handle: "P2", Slug: "loose", Title: "Loose", Kind: boardengine.KindProject},
			},
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: none recorded.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## [P1 Lone beam](project-lone.md) (0)\n" +
				"\n" +
				"`lone` · Owner `box`\n" +
				"\n" +
				emptyProjectEntries(3) +
				"\n" +
				"## [P2 Loose](project-loose.md) (0)\n" +
				"\n" +
				"`loose` · Owner none recorded\n" +
				"\n" +
				emptyProjectEntries(3) +
				"\n" +
				rootProjectHeading(2, 0, "none recorded") +
				emptyProjectEntries(3),
			files: map[string]string{
				"project-lone.md": "# P1 Lone beam\n" +
					"\n" +
					"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
					"Under: [Board](README.md).\n" +
					"\n" +
					"`lone` · Owner `box`\n" +
					"\n" +
					"beam\n" +
					"\n" +
					"Tasks are grouped by dependency layer and notes by type.\n" +
					"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
					"\n" +
					"## Tasks (0)\n" +
					"\n" +
					"Concrete and claimable; only a task can run.\n" +
					"\n" +
					"### Ready (0)\n" +
					"\n" +
					"Waits on nothing open; can start now.\n" +
					"\n" +
					"_None._\n" +
					"\n" +
					"## Notes (0)\n" +
					"\n" +
					"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
			},
		},
		{
			name: "nested projects stop deepening their headings at level six",
			tasks: []boardengine.Task{
				{ID: 1, Handle: "P1", Slug: "one", Title: "One", Kind: boardengine.KindProject},
				{ID: 2, Handle: "P2", Slug: "two", Title: "Two", Kind: boardengine.KindProject, Project: "one"},
				{ID: 3, Handle: "P3", Slug: "three", Title: "Three", Kind: boardengine.KindProject, Project: "two"},
				{ID: 4, Handle: "P4", Slug: "four", Title: "Four", Kind: boardengine.KindProject, Project: "three"},
				{ID: 5, Handle: "P5", Slug: "five", Title: "Five", Kind: boardengine.KindProject, Project: "four"},
			},
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: none recorded.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## [P1 One](project-one.md) (1)\n" +
				"\n" +
				"`one` · Owner none recorded\n" +
				"\n" +
				"### [P2 Two](project-two.md) (1)\n" +
				"\n" +
				"`two` · Owner none recorded\n" +
				"\n" +
				"#### [P3 Three](project-three.md) (1)\n" +
				"\n" +
				"`three` · Owner none recorded\n" +
				"\n" +
				"##### [P4 Four](project-four.md) (1)\n" +
				"\n" +
				"`four` · Owner none recorded\n" +
				"\n" +
				"###### [P5 Five](project-five.md) (0)\n" +
				"\n" +
				"`five` · Owner none recorded\n" +
				"\n" +
				emptyEntriesAtLevelSix +
				"\n" +
				ownEntriesHeading(6) +
				emptyEntriesAtLevelSix +
				"\n" +
				ownEntriesHeading(5) +
				emptyEntriesAtLevelSix +
				"\n" +
				ownEntriesHeading(4) +
				emptyProjectEntries(5) +
				"\n" +
				ownEntriesHeading(3) +
				emptyProjectEntries(4) +
				"\n" +
				rootProjectHeading(2, 0, "none recorded") +
				emptyProjectEntries(3),
		},
		{
			name: "a project dependency shows by handle while the project is open and drops once every landing member is done",
			tasks: []boardengine.Task{
				{ID: 1, Handle: "P1", Slug: "open", Title: "Open", Kind: boardengine.KindProject},
				{ID: 2, Handle: "P2", Slug: "finished", Title: "Finished", Kind: boardengine.KindProject},
				{ID: 3, Handle: "T1", Slug: "member", Title: "Member", Kind: boardengine.KindTask, Project: "finished", Status: stringPtr("done")},
				{ID: 4, Handle: "T2", Slug: "after-open", Title: "After open", Kind: boardengine.KindTask, DependsOn: []string{"open"}},
				{ID: 5, Handle: "T3", Slug: "after-finished", Title: "After finished", Kind: boardengine.KindTask, DependsOn: []string{"finished"}},
			},
			want: "# Board\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Prime owner: none recorded.\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## [P1 Open](project-open.md) (0)\n" +
				"\n" +
				"`open` · Owner none recorded\n" +
				"\n" +
				emptyProjectEntries(3) +
				"\n" +
				"## [P2 Finished](project-finished.md) (0)\n" +
				"\n" +
				"`finished` · Owner none recorded\n" +
				"\n" +
				"### Tasks (0)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Ready (0)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"_None._\n" +
				"\n" +
				"### Notes (0)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"### Done (1)\n" +
				"\n" +
				"Finished, awaiting `lym board prune --apply`.\n" +
				"\n" +
				"| # | Entry | Slug | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| T1 | **Member** | `member` |  |  |\n" +
				"\n" +
				rootProjectHeading(2, 2, "none recorded") +
				"### Tasks (2)\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"#### Ready (1)\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T3 | **After finished** | `after-finished` | tapestry |  |  |  |\n" +
				"\n" +
				"#### Layer A (1)\n" +
				"\n" +
				"Waits only on Running, Ready or Draft entries.\n" +
				"\n" +
				"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
				"| --- | --- | --- | --- | --- | --- | --- |\n" +
				"| T2 | **After open** | `after-open` | tapestry | P1 |  |  |\n" +
				"\n" +
				"### Notes (0)\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := boardengine.Render(tt.tasks, tt.primeOwner, boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, DefaultRecipe: "tapestry"}, renderTime)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			got := result["README.md"]
			if got != tt.want {
				t.Errorf("README mismatch\nwant:\n%s\ngot:\n%s", tt.want, got)
			}
			for _, retired := range []string{"Next Up", "tier", "Tier", "Before", "Standing", "Groups of entries"} {
				if strings.Contains(got, retired) {
					t.Errorf("README still mentions %q", retired)
				}
			}
			for file, want := range tt.files {
				if got := result[file]; got != want {
					t.Errorf("%s mismatch\nwant:\n%s\ngot:\n%s", file, want, got)
				}
			}
			if tt.projectFiles != nil {
				var projectFiles []string
				for file := range result {
					if strings.HasPrefix(file, "project-") {
						projectFiles = append(projectFiles, file)
					}
				}
				slices.Sort(projectFiles)
				if !slices.Equal(projectFiles, tt.projectFiles) {
					t.Errorf("Render() project files = %v; want %v", projectFiles, tt.projectFiles)
				}
			}
		})
	}
}

// TestRenderRecipeCellTakesTheDefaultRecipe pins that a task with no recipe shows Outputs.DefaultRecipe in its Recipe cell and a task naming a recipe shows its own.
func TestRenderRecipeCellTakesTheDefaultRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tasks := []boardengine.Task{
		{ID: 1, Handle: "T1", Slug: "plain", Title: "Plain", Kind: boardengine.KindTask, Labels: []string{"bug"}},
		{ID: 2, Handle: "T2", Slug: "named", Title: "Named", Kind: boardengine.KindTask, Labels: []string{"bug"}, Recipe: "tapestry"},
	}
	result, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, DefaultRecipe: "darn"}, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := result["README.md"]
	for _, want := range []string{"| T1 | **Plain** | `plain` | darn |", "| T2 | **Named** | `named` | tapestry |"} {
		if !strings.Contains(got, want) {
			t.Errorf("README lacks row %q:\n%s", want, got)
		}
	}
}

// TestRenderProjectFileShowsTheBodyUnderTheBrief pins that a project file renders the project's body between its brief and the intro.
// Its ATX headings shift down together until the shallowest is level two, and lines in fenced or indented code stay as they are.
// A project with no body, or a blank one, renders no body.
func TestRenderProjectFileShowsTheBodyUnderTheBrief(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const linkedFacts = "[`plan`](design-plan.md) · Owner none recorded"
	tests := []struct {
		name      string
		body      string
		wantFacts string
		wantBody  string
	}{
		{"NoBody", "", "`plan` · Owner none recorded", ""},
		{"BlankBody", "  \n", linkedFacts, ""},
		{"PlainBody", "Goal text.\n\nMore text.\n", linkedFacts, "Goal text.\n\nMore text.\n\n"},
		{
			"TopLevelHeadingsShiftDown",
			"# Goal\nText.\n## Detail\n###### Deep\n",
			linkedFacts,
			"## Goal\nText.\n### Detail\n###### Deep\n\n",
		},
		{"SecondLevelHeadingsStay", "## Seen\n\nIt broke.\n### Why\n", linkedFacts, "## Seen\n\nIt broke.\n### Why\n\n"},
		{
			"CodeLinesStay",
			"# Goal\n```go\n# not a heading\n```go\n# still code\n```\n~~~\n# tilde code\n~~~\n    # indented code\n#hashtag\n",
			linkedFacts,
			"## Goal\n```go\n# not a heading\n```go\n# still code\n```\n~~~\n# tilde code\n~~~\n    # indented code\n#hashtag\n\n",
		},
		{"FencedHeadingsSetNoShift", "## Seen\n```\n# comment\n```\n", linkedFacts, "## Seen\n```\n# comment\n```\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tasks := []boardengine.Task{{ID: 1, Handle: "P1", Slug: "plan", Title: "Plan", Kind: boardengine.KindProject, Brief: "Brief.", Body: tt.body}}
			result, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, DefaultRecipe: "darn"}, renderTime)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			want := "# P1 Plan\n" +
				"\n" +
				"Rendered 2026-10-12 09:30 UTC; every age below is as of then.\n" +
				"Under: [Board](README.md).\n" +
				"\n" +
				tt.wantFacts + "\n" +
				"\n" +
				"Brief.\n" +
				"\n" +
				tt.wantBody +
				"Tasks are grouped by dependency layer and notes by type.\n"
			if got := result["project-plan.md"]; !strings.HasPrefix(got, want) {
				t.Errorf("project-plan.md opens:\n%s\nwant it to open:\n%s", got, want)
			}
		})
	}
}

// TestRenderSlugCellLinksTheDecisionRecord pins that a discussed entry's Slug cell stacks a Sketch link to its decision record under the slug, in an open and a Done table of every board file listing it, and that an entry not flagged discussed renders the slug alone.
func TestRenderSlugCellLinksTheDecisionRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tasks := []boardengine.Task{
		{ID: 1, Handle: "T1", Slug: "sketched", Title: "Sketched", Kind: boardengine.KindTask, Labels: []string{"bug"}, Body: "body", Discussed: true},
		{ID: 2, Handle: "T2", Slug: "plain", Title: "Plain", Kind: boardengine.KindTask, Labels: []string{"bug"}, Body: "body"},
		{ID: 3, Handle: "T3", Slug: "landed", Title: "Landed", Kind: boardengine.KindTask, Labels: []string{"bug"}, Status: stringPtr("done"), Discussed: true},
	}
	result, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, DefaultRecipe: "darn"}, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	wantRows := []string{
		"| T1 | **Sketched** | [`sketched`](design-sketched.md)<br>[Sketch](decision-sketched.md) | darn |",
		"| T2 | **Plain** | [`plain`](design-plain.md) | darn |",
		"| T3 | **Landed** | `landed`<br>[Sketch](decision-landed.md) |",
	}
	for _, file := range []string{"README.md", "project-root.md"} {
		for _, want := range wantRows {
			if !strings.Contains(result[file], want) {
				t.Errorf("%s lacks row %q:\n%s", file, want, result[file])
			}
		}
	}
}

// TestRenderDiscussedSection pins how a task a Discussion-only run finished renders: under a Discussed subsection before Draft, its title led by a `discussed` code span.
func TestRenderDiscussedSection(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tasks := []boardengine.Task{
		{ID: 1, Handle: "T1", Slug: "sketched", Title: "Sketched", Kind: boardengine.KindTask, Labels: []string{"bug"}, Status: stringPtr(boardengine.StatusDiscussed), Discussed: true},
		{ID: 2, Handle: "T2", Slug: "unreleased", Title: "Unreleased", Kind: boardengine.KindTask, Labels: []string{"bug"}, Status: stringPtr(boardengine.StatusDraft)},
	}
	result, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes, DefaultRecipe: "darn"}, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	readme := result["README.md"]
	discussed := strings.Index(readme, " Discussed (1)\n\nDiscussion approved and its decision record stored; nothing starts it until its status is cleared.\n")
	draft := strings.Index(readme, " Draft (1)\n")
	if discussed < 0 || draft < 0 || discussed > draft {
		t.Errorf("README Discussed section at %d, Draft at %d; want both, Discussed first:\n%s", discussed, draft, readme)
	}
	if want := "| T1 | `discussed` **Sketched** |"; !strings.Contains(readme, want) {
		t.Errorf("README lacks row opening %q:\n%s", want, readme)
	}
	if discussed >= 0 && draft > discussed {
		for _, column := range []string{"Recipe", "After"} {
			if !strings.Contains(readme[discussed:draft], column) {
				t.Errorf("README Discussed section lacks the %s column:\n%s", column, readme[discussed:draft])
			}
		}
	}
}

// emptyProjectEntries is the rendered own entries of a project holding none, its Tasks and Notes at heading level level and Ready one level down.
func emptyProjectEntries(level int) string {
	return strings.Repeat("#", level) + " Tasks (0)\n" +
		"\n" +
		"Concrete and claimable; only a task can run.\n" +
		"\n" +
		strings.Repeat("#", level+1) + " Ready (0)\n" +
		"\n" +
		"Waits on nothing open; can start now.\n" +
		"\n" +
		"_None._\n" +
		"\n" +
		strings.Repeat("#", level) + " Notes (0)\n" +
		"\n" +
		"Not tasks: ideas and observations, merged into a task when one is promoted.\n"
}

// emptyEntriesAtLevelSix is the rendered own entries of a project holding none, nested so deep that its Tasks, Ready and Notes headings all stop at level six.
const emptyEntriesAtLevelSix = "###### Tasks (0)\n" +
	"\n" +
	"Concrete and claimable; only a task can run.\n" +
	"\n" +
	"###### Ready (0)\n" +
	"\n" +
	"Waits on nothing open; can start now.\n" +
	"\n" +
	"_None._\n" +
	"\n" +
	"###### Notes (0)\n" +
	"\n" +
	"Not tasks: ideas and observations, merged into a task when one is promoted.\n"

// rootProjectHeading is the root project's heading at level level counting members open entries, then its facts line naming owner.
func rootProjectHeading(level, members int, owner string) string {
	return strings.Repeat("#", level) + " [P0 Root: entries under no project](project-root.md) (" + strconv.Itoa(members) + ")\n" +
		"\n" +
		"Owner " + owner + "\n" +
		"\n"
}

// rootEntriesWithProjects is the rendered own entries of the root in the fixture with projects, its Tasks, Notes and Done at heading level level: one Ready task and one done project.
func rootEntriesWithProjects(level int) string {
	section, subsection := strings.Repeat("#", level), strings.Repeat("#", level+1)
	return section + " Tasks (1)\n" +
		"\n" +
		"Concrete and claimable; only a task can run.\n" +
		"\n" +
		subsection + " Ready (1)\n" +
		"\n" +
		"Waits on nothing open; can start now.\n" +
		"\n" +
		"| # | Task | Slug | Recipe | After | Age<br>Revised | Labels |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| T7 | **Base** | `base` | tapestry |  |  |  |\n" +
		"\n" +
		section + " Notes (0)\n" +
		"\n" +
		"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
		"\n" +
		section + " Done (1)\n" +
		"\n" +
		"Finished, awaiting `lym board prune --apply`.\n" +
		"\n" +
		"| # | Entry | Slug | Age<br>Revised | Labels |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| P3 | **[Old project](project-old.md)** | `old` |  |  |\n"
}

// ownEntriesHeading is the heading at level level and the meaning line over a project's own entries when it has open sub-projects.
func ownEntriesHeading(level int) string {
	return strings.Repeat("#", level) + " Own entries\n" +
		"\n" +
		"The entries directly under this project and under none of its sub-projects.\n" +
		"\n"
}

// TestRenderDesignDocGoldens pins the design-doc header, the Depends on line with one linked and one named dependency, and a multi-line body appearing byte-identical.
func TestRenderDesignDocGoldens(t *testing.T) {
	tiergate.Require(t, 0)
	task, note := boardengine.KindTask, boardengine.KindNote
	tasks := []boardengine.Task{
		{ID: 1, Slug: "linked", Title: "Linked", Kind: task, Labels: []string{"enhancement"}, Body: "linked body"},
		{ID: 2, Slug: "bare", Title: "Bare", Kind: task, Labels: []string{"bug"}},
		{ID: 3, Slug: "main", Title: "Main", Kind: task, Labels: []string{"bug", "area"}, Status: stringPtr("active"),
			DependsOn: []string{"linked", "bare"}, Body: "line one\n\n  indented\nline three"},
		{ID: 4, Slug: "plain", Title: "Plain", Kind: note, Labels: []string{"enhancement"}, Body: "just a body"},
	}
	result, err := boardengine.Render(tasks, "", boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes}, renderTime)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	tests := []struct {
		file string
		want string
	}{
		{
			file: "design-main.md",
			want: "# Main\n\n`main` · Task · bug, area · active\n\n" +
				"Depends on: [`linked`](design-linked.md), `bare`\n\n" +
				"line one\n\n  indented\nline three",
		},
		{
			file: "design-plain.md",
			want: "# Plain\n\n`plain` · Note · enhancement\n\njust a body",
		},
	}
	for _, tt := range tests {
		if got := result[tt.file]; got != tt.want {
			t.Errorf("%s mismatch\nwant: %q\ngot:  %q", tt.file, tt.want, got)
		}
	}
	if _, ok := result["design-bare.md"]; ok {
		t.Errorf("design-bare.md should not exist (no body)")
	}
}

// TestRenderCustomOutputs verifies that Render respects configurable Outputs fields, covering both a custom Readme filename and a custom design prefix.
//
// Folds: TestRenderConfigurableHomeFilename, TestRenderConfigurableProposalPrefix
//
//testtiming:keep pins the result keys and design-doc links for a custom readme name and design prefix, which its covering test does not assert
func TestRenderCustomOutputs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("TestRenderConfigurableHomeFilename", func(t *testing.T) {
		// Test that Render uses configured Readme filename instead of "Home.md"
		task := boardengine.Task{
			ID:     1,
			Slug:   "test-task",
			Title:  "Test Task",
			Kind:   boardengine.KindTask,
			Labels: []string{"enhancement"},
		}
		out := boardengine.Outputs{
			Readme:       "README.md",
			DesignPrefix: "proposal-",
		}
		result, err := boardengine.Render([]boardengine.Task{task}, "", out, renderTime)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		if _, ok := result["README.md"]; !ok {
			t.Errorf("Result should have README.md key, got keys: %v", getKeys(result))
		}
		if _, ok := result["Home.md"]; ok {
			t.Errorf("Result should not have Home.md key when configured differently")
		}
	})

	t.Run("TestRenderConfigurableProposalPrefix", func(t *testing.T) {
		// Test that Render uses configured design prefix
		task := boardengine.Task{
			ID:     1,
			Slug:   "test-task",
			Title:  "Test Task",
			Kind:   boardengine.KindTask,
			Labels: []string{"enhancement"},
			Body:   "Proposal body",
		}
		out := boardengine.Outputs{
			Readme:       "Home.md",
			DesignPrefix: "prop-",
		}
		result, err := boardengine.Render([]boardengine.Task{task}, "", out, renderTime)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		// Check design-doc file uses custom prefix
		if _, ok := result["prop-test-task.md"]; !ok {
			t.Errorf("Result should have prop-test-task.md key, got keys: %v", getKeys(result))
		}
		if _, ok := result["proposal-test-task.md"]; ok {
			t.Errorf("Result should not have proposal-test-task.md with custom prefix")
		}

		// Check links in Home.md use custom prefix
		home := result["Home.md"]
		if !strings.Contains(home, "[`test-task`](prop-test-task.md)") {
			t.Errorf("Home.md should use custom prefix in links\nGot: %s", home)
		}
	})
}

// getKeys extracts all keys from a string map, used for error messages.
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestFormatAge pins the unit boundaries, the unknown-date blank and the clamp of a future date.
func TestFormatAge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const day = 24 * time.Hour
	tests := []struct {
		name  string
		since time.Time
		want  string
	}{
		{"zero age", renderTime, "0m"},
		{"last minute unit", renderTime.Add(-45 * time.Minute), "45m"},
		{"first hour unit", renderTime.Add(-time.Hour), "1h"},
		{"hours", renderTime.Add(-5 * time.Hour), "5h"},
		{"first day unit", renderTime.Add(-day), "1d"},
		{"last day unit", renderTime.Add(-14*day + time.Second), "13d"},
		{"first week unit", renderTime.Add(-14 * day), "2w"},
		{"last week unit", renderTime.Add(-91*day + time.Second), "12w"},
		{"first month unit", renderTime.Add(-91 * day), "3mo"},
		{"last month unit", renderTime.Add(-365*day + time.Second), "12mo"},
		{"first year unit", renderTime.Add(-365 * day), "1y"},
		{"years", renderTime.Add(-2 * 365 * day), "2y"},
		{"unknown date", time.Time{}, ""},
		{"future date", renderTime.Add(time.Hour), "0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := boardengine.FormatAge(tt.since, renderTime); got != tt.want {
				t.Errorf("FormatAge(%v) = %q, want %q", tt.since, got, tt.want)
			}
		})
	}
}
