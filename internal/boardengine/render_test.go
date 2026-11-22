// render_test.go unit-tests what RenderToDisk does on disk and the age format the board files share.
// It covers rewriting a file in place, the output paths it accepts and refuses,
// and the manifest-based cleanup of outputs a render no longer produces.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestRenderToDiskRewritesAnExistingFileInPlace pins that a re-render keeps each output file's inode,
// so a file watcher reports the rewrite as a change rather than a creation.
func TestRenderToDiskRewritesAnExistingFileInPlace(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := t.TempDir()
	out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
	task := boardengine.Task{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Body: "first body"}
	if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err != nil {
		t.Fatalf("first RenderToDisk: %v", err)
	}
	designPath := filepath.Join(dir, "proposal-a.md")
	before, err := os.Stat(designPath)
	if err != nil {
		t.Fatalf("stat after first render: %v", err)
	}

	task.Body = "second body"
	if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err != nil {
		t.Fatalf("second RenderToDisk: %v", err)
	}
	after, err := os.Stat(designPath)
	if err != nil {
		t.Fatalf("stat after second render: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("proposal-a.md was replaced by a new file; want the same file rewritten in place")
	}
	if b, err := os.ReadFile(designPath); err != nil || !strings.HasSuffix(string(b), "second body") {
		t.Errorf("proposal-a.md after second render: got %q, err %v; want it to end with %q", b, err, "second body")
	}
}

// TestRenderToDiskOutputPaths pins that an output path under a subdirectory of the board is created and written,
// and that a path escaping the board directory is refused with no file written.
func TestRenderToDiskOutputPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	task := boardengine.Task{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}}
	t.Run("NestedReadmeIsCreatedUnderItsDirectory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		out := boardengine.Outputs{Readme: "views/Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err != nil {
			t.Fatalf("RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "views", "Home.md")); err != nil {
			t.Errorf("views/Home.md not written: %v", err)
		}
	})
	t.Run("ParentEscapingReadmeIsRefused", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		out := boardengine.Outputs{Readme: "../outside.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, "", out, renderTime); err == nil {
			t.Errorf("RenderToDisk with readme %q succeeded; want an error", out.Readme)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside.md")); !os.IsNotExist(err) {
			t.Errorf("outside.md was written outside the board directory: stat err %v", err)
		}
	})
}

// TestRenderToDiskManifestCleanup covers RenderToDisk's outputs and the manifest-based cleanup:
// a seeded orphan removed under a custom design prefix; renamed outputs removed across consecutive renders;
// body loss removing a design doc; a removed project dropping its board file; unrelated files left untouched;
// and graceful degradation for missing or corrupt manifests.
func TestRenderToDiskManifestCleanup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("SeededManifestRemovesOrphanAndWritesOutputs", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{
			{ID: 0, Slug: "a", Title: "A", Kind: boardengine.KindTask, Labels: []string{"enhancement"}, Body: "proposal A"},
			// No body, so no design-doc file.
			{ID: 1, Slug: "b", Title: "B", Kind: boardengine.KindTask, Labels: []string{"enhancement"}},
		}

		// A stale design doc from a previous render, pre-seeded into the manifest, so the one RenderToDisk call removes it.
		// A first render with no prior manifest removes nothing.
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
		manifest, err := os.ReadFile(filepath.Join(dir, ".board-rendered.json"))
		if err != nil {
			t.Fatalf("read the manifest: %v", err)
		}
		for _, name := range []string{boardengine.BoardDataFile, boardengine.BoardPageFile} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("%s not written: %v", name, err)
			}
			if !strings.Contains(string(manifest), `"`+name+`"`) {
				t.Errorf("manifest = %s; want it to list %s", manifest, name)
			}
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

		// A proposal file that no manifest records, as on a board rendered before any manifest existed.
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
