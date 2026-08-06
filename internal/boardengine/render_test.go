// render_test.go — unit tests for rendering (render.go).
//
// README / design-doc output across task shapes: dependencies, status,
// isolated, deferred, orphans, and title formatting. Also covers the
// manifest-based cleanup introduced in RenderToDisk: renamed outputs are removed
// across consecutive renders, and a missing or corrupt manifest degrades gracefully.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// TestRenderToDisk verifies that RenderToDisk writes expected files and removes
// orphaned design-doc files via the manifest. Subtests cover the default prefix and a
// custom prefix. The ghost file is pre-seeded into the manifest so the manifest-based
// cleanup removes it on the single RenderToDisk call (the manifest only removes files
// it previously recorded, so a first render with no prior manifest seeds and removes
// nothing — see TestRenderToDiskManifestCleanup for that scenario).
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
		{ID: 0, Slug: "a", Title: "A", Body: "proposal A"},
		{ID: 1, Slug: "b", Title: "B"}, // no body → no design-doc file
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

			if err := boardengine.RenderToDisk(dir, tasks, nil, tt.out); err != nil {
				t.Fatalf("RenderToDisk: %v", err)
			}

			if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
				t.Errorf("Home.md not written: %v", err)
			}
			if b, err := os.ReadFile(filepath.Join(dir, tt.wantProposal)); err != nil || string(b) != "proposal A" {
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

// TestRenderToDiskManifestCleanup covers the manifest-based cleanup scenarios:
// renamed outputs removed across consecutive renders, body loss removing a design
// doc, unrelated files left untouched, and graceful degradation for missing/corrupt
// manifests.
func TestRenderToDiskManifestCleanup(t *testing.T) {
	t.Run("ReadmeRename", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A"}}

		// First render produces Home.md and seeds the manifest with it.
		out1 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, nil, out1); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}

		// Second render uses Index.md; the manifest from the first render lists Home.md,
		// so it is removed because the new output set does not contain it.
		out2 := boardengine.Outputs{Readme: "Index.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, nil, out2); err != nil {
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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A", Body: "body"}}

		// First render with prefix "proposal-" produces proposal-a.md.
		out1 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, nil, out1); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render with prefix "task-" produces task-a.md; manifest cleanup
		// removes proposal-a.md because it is no longer in the output set.
		out2 := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "task-"}
		if err := boardengine.RenderToDisk(dir, tasks, nil, out2); err != nil {
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
		task := boardengine.Task{ID: 0, Slug: "a", Title: "A", Body: "original body"}
		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}

		// First render: task has a body → proposal-a.md is produced and recorded in the manifest.
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, nil, out); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); err != nil {
			t.Fatalf("proposal-a.md should exist after first render: %v", err)
		}

		// Second render: task loses its body → proposal-a.md is absent from the new
		// output set but present in the manifest, so the manifest cleanup removes it.
		task.Body = ""
		if err := boardengine.RenderToDisk(dir, []boardengine.Task{task}, nil, out); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "proposal-a.md")); !os.IsNotExist(err) {
			t.Errorf("proposal-a.md should have been removed after the task lost its body")
		}
	})

	t.Run("UnrelatedFileNotRemoved", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A"}}

		// A hand-added file in the board dir that was never produced by a render.
		readme := filepath.Join(dir, "NOTES.md")
		if err := os.WriteFile(readme, []byte("# Notes"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		// First render seeds the manifest with the rendered files (not NOTES.md).
		if err := boardengine.RenderToDisk(dir, tasks, nil, out); err != nil {
			t.Fatalf("first RenderToDisk: %v", err)
		}
		// Second render triggers cleanup; NOTES.md was never in the manifest so it is untouched.
		if err := boardengine.RenderToDisk(dir, tasks, nil, out); err != nil {
			t.Fatalf("second RenderToDisk: %v", err)
		}
		if _, err := os.Stat(readme); err != nil {
			t.Errorf("NOTES.md should not have been removed (never in manifest): %v", err)
		}
	})

	t.Run("NoManifestSeedsAndRemovesNothing", func(t *testing.T) {
		dir := t.TempDir()
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A"}}

		// A file that looks like an orphan under the old glob approach but is absent
		// from the manifest because no manifest exists yet (pre-upgrade state).
		stale := filepath.Join(dir, "proposal-stale.md")
		if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}

		// First render with no prior manifest: nothing is removed (graceful degradation),
		// and the manifest is seeded with the current output set.
		if err := boardengine.RenderToDisk(dir, tasks, nil, out); err != nil {
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
		tasks := []boardengine.Task{{ID: 0, Slug: "a", Title: "A"}}

		// Write a corrupt manifest; RenderToDisk must treat it as absent (no cleanup)
		// and overwrite it with the current render set.
		if err := os.WriteFile(filepath.Join(dir, ".board-rendered.json"), []byte("not valid json {{{"), 0o644); err != nil {
			t.Fatal(err)
		}

		out := boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"}
		if err := boardengine.RenderToDisk(dir, tasks, nil, out); err != nil {
			t.Errorf("RenderToDisk should not fail with a corrupt manifest: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "Home.md")); err != nil {
			t.Errorf("Home.md should be written even with corrupt manifest: %v", err)
		}
	})
}

func TestRenderEmptyTaskList(t *testing.T) {
	// (a) empty task list, no notes → Home.md is exactly "# Tasks\n", no design docs.
	result, err := boardengine.Render([]boardengine.Task{}, nil, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expectedHome := "# Tasks\n"
	if result["Home.md"] != expectedHome {
		t.Errorf("Home.md mismatch\nExpected: %q\nGot: %q", expectedHome, result["Home.md"])
	}

	// Check no design-doc files
	for key := range result {
		if strings.HasPrefix(key, "proposal-") {
			t.Errorf("Unexpected design-doc file: %s", key)
		}
	}
}

// TestRenderProposalAndShapesHomepage tests the core boardengine.Render() function for
// various task shapes: dependencies, status variants, isolated tasks, deferred tasks,
// and brief/title formatting. Each case asserts matching expected Home.md substrings.
//
// Folds: TestRenderDependencies, TestRenderSpecialBucketTask, TestRenderIsolatedTask,
// TestRenderTaskIDFormatting, TestRenderBrief, TestRenderMissingDependency, TestRenderLayerBuckets
func TestRenderProposalAndShapesHomepage(t *testing.T) {
	tests := []struct {
		name           string
		tasks          []boardengine.Task
		wantSubstrings []string
		dontWantSubstr []string
	}{
		{
			name: "TestRenderDependencies",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "task-b", Title: "Task B"},
				{ID: 2, Slug: "task-a", Title: "Task A", DependsOn: []string{"task-b"}},
			},
			wantSubstrings: []string{
				"# Layer A",
				"# Layer B",
				"Depends on: #001",
			},
		},
		{
			name: "TestRenderSpecialBucketTask",
			tasks: []boardengine.Task{
				func() boardengine.Task {
					s := "done"
					return boardengine.Task{ID: 1, Slug: "done-task", Title: "Done Task", Status: &s}
				}(),
			},
			wantSubstrings: []string{
				"# Done",
				"## **#001:** Done Task\n",
			},
			dontWantSubstr: []string{"[Done]"},
		},
		{
			name: "TestRenderDeferredTask",
			tasks: []boardengine.Task{
				boardengine.Task{ID: 1, Slug: "deferred-task", Title: "Deferred Task", Deferred: true},
			},
			wantSubstrings: []string{
				"# Someday",
				"## **#001:** Deferred Task\n",
			},
		},
		{
			name: "TestRenderIsolatedTask",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "task-a", Title: "Task A"},
				{ID: 2, Slug: "task-z", Title: "Isolated Task", Isolated: true},
			},
			wantSubstrings: []string{
				"# Layer A",
				"# Layer Z",
			},
		},
		{
			name: "TestRenderTaskIDFormatting",
			tasks: []boardengine.Task{
				{ID: 3, Slug: "task-c", Title: "Task C"},
				{ID: 1, Slug: "task-a", Title: "Task A"},
				{ID: 2, Slug: "task-b", Title: "Task B"},
			},
			wantSubstrings: []string{
				"## **#001:** Task A",
				"## **#002:** Task B",
				"## **#003:** Task C",
			},
		},
		{
			name: "TestRenderBrief",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "test-task", Title: "Test Task", Brief: "This is the brief text"},
			},
			wantSubstrings: []string{
				"This is the brief text",
			},
		},
		{
			name: "TestRenderMissingDependency",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "task-a", Title: "Task A", DependsOn: []string{"missing-task"}},
			},
			wantSubstrings: []string{
				"#???: missing-task (missing)",
			},
		},
		{
			name: "TestRenderLayerBuckets",
			tasks: []boardengine.Task{
				{ID: 1, Slug: "independent-task", Title: "Independent Task"},
				{ID: 2, Slug: "dependent-task", Title: "Dependent Task", DependsOn: []string{"independent-task"}},
			},
			wantSubstrings: []string{
				"# Layer A",
				"# Layer B",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := boardengine.Render(tt.tasks, nil, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			home := result["Home.md"]

			for _, want := range tt.wantSubstrings {
				if !strings.Contains(home, want) {
					t.Errorf("Home.md missing %q\nGot: %s", want, home)
				}
			}

			for _, dontWant := range tt.dontWantSubstr {
				if strings.Contains(home, dontWant) {
					t.Errorf("Home.md should not contain %q\nGot: %s", dontWant, home)
				}
			}
		})
	}
}

// TestRenderStatusVariants tests all valid status values; asserts the appropriate
// status suffix appears in the slug line.
//
// Folded: directly tested (no original separate func)
func TestRenderStatusVariants(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		wantText string
	}{
		{"status-active", "active", "[test-task] [active]"},
		{"status-pr-pending", "pr-pending", "[test-task] [pr-pending]"},
		{"status-ready-to-merge", "ready-to-merge", "[test-task] [ready-to-merge]"},
		{"status-abandoned", "abandoned", "[test-task] [abandoned]"},
		{"status-done", "done", "[test-task] [done]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.status
			task := boardengine.Task{
				ID:     1,
				Slug:   "test-task",
				Title:  "Test Task",
				Status: &s,
			}

			result, err := boardengine.Render([]boardengine.Task{task}, nil, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			home := result["Home.md"]
			if !strings.Contains(home, tt.wantText) {
				t.Errorf("Home.md should contain %q\nGot: %s", tt.wantText, home)
			}
		})
	}
}

// TestRenderSingleTask tests single-task rendering with and without body,
// verifying design-doc file creation and Home.md content.
//
// Folds: TestRenderSingleTaskNoBody, TestRenderSingleTaskWithBody, TestRenderOrphanDetection
func TestRenderSingleTask(t *testing.T) {
	tests := []struct {
		name            string
		task            boardengine.Task
		wantProposalKey string
		wantProposal    bool // true if design-doc file should exist
		wantHome        []string
		dontWantHome    []string
	}{
		{
			name:            "TestRenderSingleTaskNoBody",
			task:            boardengine.Task{ID: 1, Slug: "test-task", Title: "Test Task"},
			wantProposalKey: "proposal-test-task.md",
			wantProposal:    false,
			wantHome: []string{
				"## **#001:** Test Task [A]",
				"[test-task]",
			},
		},
		{
			name:            "TestRenderSingleTaskWithBody",
			task:            boardengine.Task{ID: 1, Slug: "test-task", Title: "Test Task", Body: "This is the body content"},
			wantProposalKey: "proposal-test-task.md",
			wantProposal:    true,
			wantHome: []string{
				"[test-task](proposal-test-task.md)",
			},
		},
		{
			name:            "TestRenderOrphanDetection",
			task:            boardengine.Task{ID: 1, Slug: "orphan-task", Title: "Orphan Task", Body: "Original body"},
			wantProposalKey: "proposal-orphan-task.md",
			wantProposal:    true,
			wantHome: []string{
				"## **#001:** Orphan Task [A]",
				"[orphan-task]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := boardengine.Render([]boardengine.Task{tt.task}, nil, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			home := result["Home.md"]

			for _, want := range tt.wantHome {
				if !strings.Contains(home, want) {
					t.Errorf("Home.md missing %q\nGot: %s", want, home)
				}
			}

			for _, dontWant := range tt.dontWantHome {
				if strings.Contains(home, dontWant) {
					t.Errorf("Home.md should not contain %q\nGot: %s", dontWant, home)
				}
			}

			if tt.wantProposal {
				if proposalContent, ok := result[tt.wantProposalKey]; !ok {
					t.Errorf("Missing design-doc file: %s", tt.wantProposalKey)
				} else if proposalContent != tt.task.Body {
					t.Errorf("Design-doc content mismatch\nExpected: %q\nGot: %q", tt.task.Body, proposalContent)
				}
			} else {
				if _, ok := result[tt.wantProposalKey]; ok {
					t.Errorf("Unexpected design-doc file: %s", tt.wantProposalKey)
				}
			}
		})
	}
}

// TestRenderCustomOutputs verifies that Render respects configurable Outputs fields,
// covering both a custom Readme filename and a custom design prefix.
//
// Folds: TestRenderConfigurableHomeFilename, TestRenderConfigurableProposalPrefix
func TestRenderCustomOutputs(t *testing.T) {
	t.Run("TestRenderConfigurableHomeFilename", func(t *testing.T) {
		// Test that Render uses configured Readme filename instead of "Home.md"
		task := boardengine.Task{
			ID:    1,
			Slug:  "test-task",
			Title: "Test Task",
		}
		out := boardengine.Outputs{
			Readme:       "README.md",
			DesignPrefix: "proposal-",
		}
		result, err := boardengine.Render([]boardengine.Task{task}, nil, out)
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
			Body:  "Proposal body",
		}
		out := boardengine.Outputs{
			Readme:       "Home.md",
			DesignPrefix: "prop-",
		}
		result, err := boardengine.Render([]boardengine.Task{task}, nil, out)
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
		if !strings.Contains(home, "[test-task](prop-test-task.md)") {
			t.Errorf("Home.md should use custom prefix in links\nGot: %s", home)
		}
	})
}

// TestRenderManifestSection covers the README's "# Manifest" section: a note with a
// non-empty body gets a design-doc link, a note depending on another note gets a
// resolved "Depends on:" line, and the Manifest heading is positioned after the
// Tasks section's content. Also verifies renderDesigns's union-of-tasks-and-notes
// behavior: a note with a body produces a design-<slug>.md entry in the result map,
// same as a task would.
func TestRenderManifestSection(t *testing.T) {
	tasks := []boardengine.Task{
		{ID: 1, Slug: "task-a", Title: "Task A"},
	}
	notes := []boardengine.Task{
		{ID: 2, Slug: "note-body", Title: "Note With Body", Body: "note body content"},
		{ID: 3, Slug: "note-dep", Title: "Note With Dep", DependsOn: []string{"note-body"}},
		{ID: 4, Slug: "note-plain", Title: "Plain Note"},
	}

	result, err := boardengine.Render(tasks, notes, boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-"})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	readme := result["Home.md"]

	tasksIdx := strings.Index(readme, "# Tasks")
	manifestIdx := strings.Index(readme, "# Manifest")
	if tasksIdx == -1 {
		t.Fatalf("Home.md missing \"# Tasks\" heading\nGot: %s", readme)
	}
	if manifestIdx == -1 {
		t.Fatalf("Home.md missing \"# Manifest\" heading\nGot: %s", readme)
	}
	if manifestIdx < tasksIdx {
		t.Errorf("\"# Manifest\" heading should come after \"# Tasks\" content\nGot: %s", readme)
	}

	wantSubstrings := []string{
		"## **#002:** Note With Body",
		"[note-body](proposal-note-body.md)",
		"## **#003:** Note With Dep",
		"Depends on: #002",
		"## **#004:** Plain Note",
		"[note-plain]",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("Home.md missing %q\nGot: %s", want, readme)
		}
	}

	if _, ok := result["proposal-note-body.md"]; !ok {
		t.Errorf("Result should have proposal-note-body.md key (renderDesigns union of tasks+notes), got keys: %v", getKeys(result))
	}
}

// getKeys extracts all keys from a string map, used for error messages.
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
