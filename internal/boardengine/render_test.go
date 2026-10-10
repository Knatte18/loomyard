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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

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
// a pre-seeded orphan removed with a custom design prefix, renamed outputs removed across consecutive renders, body loss removing a design doc, unrelated files left untouched, and graceful degradation for missing/corrupt manifests.
func TestRenderToDiskManifestCleanup(t *testing.T) {
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

		if err := boardengine.RenderToDisk(dir, tasks, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "prop-"}); err != nil {
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
		if err := boardengine.RenderToDisk(dir, tasks, out1); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}

		// Second render uses Index.md; the manifest from the first render lists Home.md,
		// so it is removed because the new output set does not contain it.
		out2 := boardengine.Outputs{Readme: "Index.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, out2); err != nil {
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
		if err := boardengine.RenderToDisk(dir, tasks, out1); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render with prefix "task-" produces task-a.md; manifest cleanup
		// removes proposal-a.md because it is no longer in the output set.
		out2 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "task-"}
		if err := boardengine.RenderToDisk(dir, tasks, out2); err != nil {
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
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, out); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render: task loses its body → proposal-a.md is absent from the new
		// output set but present in the manifest, so the manifest cleanup removes it.
		task.Body = ""
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, out); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); !os.IsNotExist(err) {
			t.Errorf("proposal-a.md should have been removed after the task lost its body")
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
		if err := boardengine.RenderToDisk(dir, tasks, out); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		// Second render triggers cleanup; NOTES.md was never in the manifest so it is untouched.
		if err := boardengine.RenderToDisk(dir, tasks, out); err != nil {
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
		if err := boardengine.RenderToDisk(dir, tasks, out); err != nil {
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
		if err := boardengine.RenderToDisk(dir, tasks, out); err != nil {
			t.Errorf("RenderToDisk should not fail with a corrupt manifest: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
			t.Errorf("Home.md should be written even with corrupt manifest: %v", err)
		}
	})
}

// readmeTypes is the type-label order the README goldens render Notes sections in.
var readmeTypes = []string{"bug", "enhancement"}

// readmeFixture holds tasks and notes with labels, plus a done task that another task depends on, an abandoned note, a body, an isolated task, a two-layer chain, a title with a pipe, a two-line brief, a note whose type label is not in readmeTypes, a high-priority task, and a low-priority note listed before a normal one of the same type.
func readmeFixture() []boardengine.Task {
	task, note := boardengine.KindTask, boardengine.KindNote
	return []boardengine.Task{
		{ID: 1, Slug: "base", Title: "Base work", Kind: task, Labels: []string{"enhancement"}, Brief: "The foundation."},
		{ID: 2, Slug: "top", Title: "Top | work", Kind: task, Labels: []string{"bug", "area"}, Status: stringPtr("running"), Brief: "Builds on base.\nSecond brief line.", Body: "Design.\nSecond line.", DependsOn: []string{"base", "shipped"}},
		{ID: 6, Slug: "alone", Title: "Alone work", Kind: task, Labels: []string{"enhancement"}, Isolated: true, Priority: boardengine.PriorityHigh},
		{ID: 3, Slug: "idea", Title: "An idea", Kind: note, Labels: []string{"enhancement", "undecided"}, Priority: boardengine.PriorityLow},
		{ID: 8, Slug: "plain", Title: "Plain idea", Kind: note, Labels: []string{"enhancement"}},
		{ID: 4, Slug: "dropped", Title: "Dropped idea", Kind: note, Labels: []string{"bug"}, Status: stringPtr("abandoned"), Brief: "No longer wanted."},
		{ID: 7, Slug: "stray", Title: "Stray note", Kind: note, Labels: []string{"retired"}},
		{ID: 5, Slug: "shipped", Title: "Shipped work", Kind: task, Labels: []string{"enhancement"}, Status: stringPtr("done")},
	}
}

// TestRenderReadmeGolden pins the README tables for a fixture with tasks and notes, notes grouped by type in Outputs.Types order and an Other group, a done entry, an abandoned note, a slug linked to its design doc, only non-type labels, an After cell that leaves out a done dependency, an isolated task, and a Ready task with one in Layer A after it.
// The same row pins that each open group splits into one table per priority present, under a `####` priority heading, high, normal, low, with no heading for an absent priority and the rows numbered on across the group's tables, that a low note follows a normal note with a higher ID, and that a pipe in a title is escaped and a line break in a brief becomes a space, so each row stays one table row.
// A second row pins that Ready renders _None._ when empty, and that the Done section and every empty Notes subsection are omitted when no entry is done and no note exists for them.
// Neither row has a run status, the first only the hand-set word "running", so neither renders a Running subsection.
// A third row pins that a task with a run status renders under Running, before Ready and in no layer, its At cell dropping the state `running` and keeping any other.
// The same row pins that a task waiting on a running one lands in Layer A, not Ready, still naming it under After, and that a task after that one lands in Layer B.
func TestRenderReadmeGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		tasks []boardengine.Task
		want  string
	}{
		{
			name:  "tasks, notes, done and abandoned entries",
			tasks: readmeFixture(),
			want: "# Board\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## Tasks\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"### Ready\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Base work**<br>• The foundation. | `base` |  |  |\n" +
				"\n" +
				"### Layer A\n" +
				"\n" +
				"Waits only on Running or Ready entries.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Top \\| work**<br>• Builds on base. Second brief line. | [`top`](design-top.md) | `base` | area |\n" +
				"\n" +
				"### Independent\n" +
				"\n" +
				"Depends on nothing and nothing depends on it, by design.\n" +
				"\n" +
				"#### High priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Alone work** | `alone` |  |  |\n" +
				"\n" +
				"## Notes\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"### Bugs\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Note | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 1 | **Dropped idea**<br>• No longer wanted. | `dropped` |  |\n" +
				"\n" +
				"### Enhancements\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Note | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 1 | **Plain idea** | `plain` |  |\n" +
				"\n" +
				"#### Low priority\n" +
				"\n" +
				"| # | Note | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 2 | **An idea** | `idea` | undecided |\n" +
				"\n" +
				"### Other\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Note | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 1 | **Stray note** | `stray` | retired |\n" +
				"\n" +
				"## Done\n" +
				"\n" +
				"Finished, awaiting `lyx board prune`.\n" +
				"\n" +
				"| # | Entry | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 1 | **Shipped work** | `shipped` |  |\n",
		},
		{
			name:  "no done section and no empty notes subsections",
			tasks: []boardengine.Task{{ID: 1, Slug: "a", Title: "A", Kind: boardengine.KindNote, Labels: []string{"enhancement"}}},
			want: "# Board\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## Tasks\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"### Ready\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"_None._\n" +
				"\n" +
				"## Notes\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n" +
				"\n" +
				"### Enhancements\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Note | Slug | Labels |\n" +
				"| --- | --- | --- | --- |\n" +
				"| 1 | **A** | `a` |  |\n",
		},
		{
			name: "running task under Running before Ready and in no layer",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "base", Title: "Base work", Kind: boardengine.KindTask, Labels: []string{"enhancement"}},
				{ID: 2, Slug: "held", Title: "Held work", Kind: boardengine.KindTask, Labels: []string{"bug", "area"}, Status: stringPtr(boardengine.RunStatus("running", "Webster")), DependsOn: []string{"base"}},
				{ID: 3, Slug: "next", Title: "Next work", Kind: boardengine.KindTask, DependsOn: []string{"held"}},
				{ID: 4, Slug: "last", Title: "Last work", Kind: boardengine.KindTask, DependsOn: []string{"next"}},
				{ID: 5, Slug: "halted", Title: "Halted work", Kind: boardengine.KindTask, Labels: []string{"bug"}, Status: stringPtr(boardengine.RunStatus("paused", "Plan-Write"))},
			},
			want: "# Board\n" +
				"\n" +
				"Tasks are grouped by dependency layer and notes by type.\n" +
				"An entry waits only on the open entries it names under After, so the entries in one layer can run in parallel.\n" +
				"\n" +
				"## Tasks\n" +
				"\n" +
				"Concrete and claimable; only a task can run.\n" +
				"\n" +
				"### Running\n" +
				"\n" +
				"Held by a run; its scope is locked until the run ends.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | At | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Held work** | `held` | Webster | area |\n" +
				"| 2 | **Halted work** | `halted` | paused · Plan-Write |  |\n" +
				"\n" +
				"### Ready\n" +
				"\n" +
				"Waits on nothing open; can start now.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Base work** | `base` |  |  |\n" +
				"\n" +
				"### Layer A\n" +
				"\n" +
				"Waits only on Running or Ready entries.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Next work** | `next` | `held` |  |\n" +
				"\n" +
				"### Layer B\n" +
				"\n" +
				"Starts when every entry it names under After is done.\n" +
				"\n" +
				"#### Normal priority\n" +
				"\n" +
				"| # | Task | Slug | After | Labels |\n" +
				"| --- | --- | --- | --- | --- |\n" +
				"| 1 | **Last work** | `last` | `next` |  |\n" +
				"\n" +
				"## Notes\n" +
				"\n" +
				"Not tasks: ideas and observations, merged into a task when one is promoted.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := boardengine.Render(tt.tasks, boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			got := result["README.md"]
			if got != tt.want {
				t.Errorf("README mismatch\nwant:\n%s\ngot:\n%s", tt.want, got)
			}
			for _, retired := range []string{"Next Up", "tier", "Tier", "Before"} {
				if strings.Contains(got, retired) {
					t.Errorf("README still mentions %q", retired)
				}
			}
		})
	}
}

// TestRenderDesignDocGoldens pins the design-doc header, the Depends on line with one linked and one named dependency, and a multi-line body appearing byte-identical.
func TestRenderDesignDocGoldens(t *testing.T) {
	task, note := boardengine.KindTask, boardengine.KindNote
	tasks := []boardengine.Task{
		{ID: 1, Slug: "linked", Title: "Linked", Kind: task, Labels: []string{"enhancement"}, Body: "linked body"},
		{ID: 2, Slug: "bare", Title: "Bare", Kind: task, Labels: []string{"bug"}},
		{ID: 3, Slug: "main", Title: "Main", Kind: task, Labels: []string{"bug", "area"}, Status: stringPtr("active"),
			DependsOn: []string{"linked", "bare"}, Body: "line one\n\n  indented\nline three"},
		{ID: 4, Slug: "plain", Title: "Plain", Kind: note, Labels: []string{"enhancement"}, Body: "just a body"},
	}
	result, err := boardengine.Render(tasks, boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-", Types: readmeTypes})
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
		result, err := boardengine.Render([]boardengine.Task{task}, out)
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
		result, err := boardengine.Render([]boardengine.Task{task}, out)
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
