// render_test.go — unit tests for rendering (render.go).
//
// README and design-doc goldens over the tier-section layout: an entry per tier, an empty tier, done and abandoned entries, dependencies, bodies, and the omitted Done section.
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

// TestRenderToDisk verifies that RenderToDisk writes expected files and removes orphaned design-doc
// files via the manifest.
// Subtests cover the default prefix and a custom prefix.
// The ghost file is pre-seeded into the manifest so the manifest-based cleanup removes it on the
// single RenderToDisk call (the manifest only removes files it previously recorded, so a first
// render with no prior manifest seeds and removes nothing — see TestRenderToDiskManifestCleanup for
// that scenario).
//
// Folds: TestRenderToDiskWritesAndCleansOrphans, TestRenderToDiskWithCustomProposalPrefix
func TestRenderToDisk(t *testing.T) {
	tests := []struct {
		name         string
		out          boardengine.Outputs
		ghostFile    string // stale design-doc filename to pre-create and pre-seed in manifest
		wantProposal string // expected design-doc file after render
	}{
		{
			name:         "TestRenderToDiskWritesAndCleansOrphans",
			out:          boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"},
			ghostFile:    "proposal-ghost.md",
			wantProposal: "proposal-a.md",
		},
		{
			name:         "TestRenderToDiskWithCustomProposalPrefix",
			out:          boardengine.Outputs{Readme: "Home.md", DesignPrefix: "prop-"},
			ghostFile:    "prop-ghost.md",
			wantProposal: "prop-a.md",
		},
	}

	tasks := []boardengine.Task{
		{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature", Body: "proposal A"},
		{ID: 1, Slug: "b", Title: "B", Tier: 1, Type: "feature"}, // no body → no design-doc file
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// A stale design doc from a previous render that should be cleaned up.
			ghost := filepath.Join(dir, tt.ghostFile)
			if err := os.WriteFile(ghost, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}

			// Pre-seed the manifest to simulate a prior render that produced the ghost
			// file; the manifest-based cleanup removes it in the next RenderToDisk call.
			seedManifest(t, dir, []string{tt.ghostFile})

			if err := boardengine.RenderToDisk(dir, tasks, tt.out); err != nil {
				t.Fatalf("RenderToDisk: %v", err)
			}

			if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
				t.Errorf("Home.md not written: %v", err)
			}
			if b, err := os.ReadFile(filepath.Join(dir, tt.wantProposal)); err != nil || !strings.HasSuffix(string(b), "proposal A") {
				t.Errorf("%s: got %q, err %v", tt.wantProposal, b, err)
			}
			noBodyProposal := filepath.Join(dir, tt.out.DesignPrefix+"b.md")
			if _, err := os.Stat(noBodyProposal); !os.IsNotExist(err) {
				t.Errorf("%sb.md should not exist (task has no body)", tt.out.DesignPrefix)
			}
			if _, err := os.Stat(ghost); !os.IsNotExist(err) {
				t.Errorf("orphan %s should have been removed", tt.ghostFile)
			}
		})
	}
}

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

// TestRenderToDiskManifestCleanup covers the manifest-based cleanup scenarios: renamed outputs
// removed across consecutive renders, body loss removing a design doc, unrelated files left
// untouched, and graceful degradation for missing/corrupt manifests.
func TestRenderToDiskManifestCleanup(t *testing.T) {
	t.Run("ReadmeRename", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature"}}

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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature", Body: "body"}}

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
		task := boardengine.Task{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature", Body: "original body"}
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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature"}}

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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature"}}

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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Tier: 1, Type: "feature"}}

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

// readmeFixture holds an entry in tiers 1 and 3, leaving tier 2 empty, plus a done entry, an abandoned entry, a body, a dependency and a two-layer chain inside tier 1.
func readmeFixture() []boardengine.Task {
	return []boardengine.Task{
		{ID: 1, Slug: "base", Title: "Base work", Tier: 1, Type: "feature", Brief: "The foundation."},
		{ID: 2, Slug: "top", Title: "Top work", Tier: 1, Type: "bug", Brief: "Builds on base.", Body: "Design.\nSecond line.", DependsOn: []string{"base"}},
		{ID: 3, Slug: "idea", Title: "An idea", Tier: 3, Type: "design"},
		{ID: 4, Slug: "dropped", Title: "Dropped idea", Tier: 3, Type: "chore", Status: stringPtr("abandoned"), Brief: "No longer wanted."},
		{ID: 5, Slug: "shipped", Title: "Shipped work", Tier: 1, Type: "feature", Status: stringPtr("done")},
	}
}

// TestRenderReadmeGolden pins the README for a fixture with an entry per tier, an empty tier 2, a done entry, an abandoned tier-3 entry, a body, a dependency, and a two-layer chain in one section.
func TestRenderReadmeGolden(t *testing.T) {
	result, err := boardengine.Render(readmeFixture(), boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := "# Board\n" +
		"\n" +
		"Entries grouped by tier, in dependency order within each tier.\n" +
		"\n" +
		"## Planned\n" +
		"\n" +
		"Concretized and claimable.\n" +
		"\n" +
		"1. **Base work** — `base` · feature\n" +
		"   The foundation.\n" +
		"1. **Top work** — `top` · bug\n" +
		"   Builds on base. [design](design-top.md)\n" +
		"   After `base`.\n" +
		"\n" +
		"## Next Up\n" +
		"\n" +
		"Planned next, but not yet concretized.\n" +
		"\n" +
		"## Someday\n" +
		"\n" +
		"Loose ideas.\n" +
		"\n" +
		"1. **An idea** — `idea` · design\n" +
		"1. **Dropped idea** — `dropped` · chore · abandoned\n" +
		"   No longer wanted.\n" +
		"\n" +
		"## Done\n" +
		"\n" +
		"Finished, awaiting `lyx board prune`.\n" +
		"\n" +
		"1. **Shipped work** — `shipped` · feature · done\n"
	if got := result["README.md"]; got != want {
		t.Errorf("README mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// TestRenderReadmeNoDoneSection asserts the Done section is omitted when no entry is done.
func TestRenderReadmeNoDoneSection(t *testing.T) {
	tasks := []boardengine.Task{{ID: 1, Slug: "a", Title: "A", Tier: 2, Type: "chore"}}
	result, err := boardengine.Render(tasks, boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := "# Board\n" +
		"\n" +
		"Entries grouped by tier, in dependency order within each tier.\n" +
		"\n" +
		"## Planned\n" +
		"\n" +
		"Concretized and claimable.\n" +
		"\n" +
		"## Next Up\n" +
		"\n" +
		"Planned next, but not yet concretized.\n" +
		"\n" +
		"1. **A** — `a` · chore\n" +
		"\n" +
		"## Someday\n" +
		"\n" +
		"Loose ideas.\n"
	got := result["README.md"]
	if got != want {
		t.Errorf("README mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
	if strings.Contains(got, "## Done") {
		t.Errorf("README should omit ## Done when no entry is done:\n%s", got)
	}
}

// TestRenderDesignDocGoldens pins the design-doc header, the Depends on line with one linked and one named dependency, and a multi-line body appearing byte-identical.
func TestRenderDesignDocGoldens(t *testing.T) {
	tasks := []boardengine.Task{
		{ID: 1, Slug: "linked", Title: "Linked", Tier: 1, Type: "feature", Body: "linked body"},
		{ID: 2, Slug: "bare", Title: "Bare", Tier: 2, Type: "chore"},
		{ID: 3, Slug: "main", Title: "Main", Tier: 2, Type: "bug", Status: stringPtr("active"),
			DependsOn: []string{"linked", "bare"}, Body: "line one\n\n  indented\nline three"},
		{ID: 4, Slug: "plain", Title: "Plain", Tier: 3, Type: "design", Body: "just a body"},
	}
	result, err := boardengine.Render(tasks, boardengine.Outputs{Readme: "README.md", DesignPrefix: "design-"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	tests := []struct {
		file string
		want string
	}{
		{
			file: "design-main.md",
			want: "# Main\n\n`main` · Next Up · bug · active\n\n" +
				"Depends on: [`linked`](design-linked.md), `bare`\n\n" +
				"line one\n\n  indented\nline three",
		},
		{
			file: "design-plain.md",
			want: "# Plain\n\n`plain` · Someday · design\n\njust a body",
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

// TestRenderCustomOutputs verifies that Render respects configurable Outputs fields, covering both
// a custom Readme filename and a custom design prefix.
//
// Folds: TestRenderConfigurableHomeFilename, TestRenderConfigurableProposalPrefix
func TestRenderCustomOutputs(t *testing.T) {
	t.Run("TestRenderConfigurableHomeFilename", func(t *testing.T) {
		// Test that Render uses configured Readme filename instead of "Home.md"
		task := boardengine.Task{
			ID:    1,
			Slug:  "test-task",
			Title: "Test Task",
			Tier:  1,
			Type:  "feature",
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
			ID:    1,
			Slug:  "test-task",
			Title: "Test Task",
			Tier:  1,
			Type:  "feature",
			Body:  "Proposal body",
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
		if !strings.Contains(home, "[design](prop-test-task.md)") {
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
