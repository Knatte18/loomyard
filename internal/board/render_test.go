// render_test.go — unit tests for rendering (render.go).
//
// Home.md / _Sidebar.md / proposal output across task shapes: dependencies,
// status, isolated, deferred, orphans, and title formatting.

package board_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// TestRenderToDisk verifies that RenderToDisk writes expected files and removes
// orphaned proposal files. Subtests cover the default prefix and a custom prefix.
//
// Folds: TestRenderToDiskWritesAndCleansOrphans, TestRenderToDiskWithCustomProposalPrefix
func TestRenderToDisk(t *testing.T) {
	tests := []struct {
		name         string
		out          board.Outputs
		ghostFile    string // stale proposal filename to pre-create
		wantProposal string // expected proposal file after render
	}{
		{
			name:         "TestRenderToDiskWritesAndCleansOrphans",
			out:          board.DefaultOutputs(),
			ghostFile:    "proposal-ghost.md",
			wantProposal: "proposal-a.md",
		},
		{
			name:         "TestRenderToDiskWithCustomProposalPrefix",
			out:          board.Outputs{Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "prop-"},
			ghostFile:    "prop-ghost.md",
			wantProposal: "prop-a.md",
		},
	}

	tasks := []board.Task{
		{ID: 0, Slug: "a", Title: "A", Body: "proposal A"},
		{ID: 1, Slug: "b", Title: "B"}, // no body → no proposal file
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// A stale proposal from a previous render that should be cleaned up.
			ghost := filepath.Join(dir, tt.ghostFile)
			if err := os.WriteFile(ghost, []byte("old"), 0o644); err != nil {
				t.Fatal(err)
			}

			if err := board.RenderToDisk(dir, tasks, tt.out); err != nil {
				t.Fatalf("RenderToDisk: %v", err)
			}

			for _, f := range []string{"Home.md", "_Sidebar.md"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Errorf("%s not written: %v", f, err)
				}
			}
			if b, err := os.ReadFile(filepath.Join(dir, tt.wantProposal)); err != nil || string(b) != "proposal A" {
				t.Errorf("%s: got %q, err %v", tt.wantProposal, b, err)
			}
			noBodyProposal := filepath.Join(dir, tt.out.ProposalPrefix+"b.md")
			if _, err := os.Stat(noBodyProposal); !os.IsNotExist(err) {
				t.Errorf("%sb.md should not exist (task has no body)", tt.out.ProposalPrefix)
			}
			if _, err := os.Stat(ghost); !os.IsNotExist(err) {
				t.Errorf("orphan %s should have been removed", tt.ghostFile)
			}
		})
	}
}

func TestRenderEmptyTaskList(t *testing.T) {
	// (a) empty task list → Home.md is exactly "# Tasks\n", Sidebar is "", no proposal files
	result, err := board.Render([]board.Task{}, board.DefaultOutputs())
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expectedHome := "# Tasks\n"
	if result["Home.md"] != expectedHome {
		t.Errorf("Home.md mismatch\nExpected: %q\nGot: %q", expectedHome, result["Home.md"])
	}

	expectedSidebar := ""
	if result["_Sidebar.md"] != expectedSidebar {
		t.Errorf("Sidebar mismatch\nExpected: %q\nGot: %q", expectedSidebar, result["_Sidebar.md"])
	}

	// Check no proposal files
	for key := range result {
		if strings.HasPrefix(key, "proposal-") {
			t.Errorf("Unexpected proposal file: %s", key)
		}
	}
}

// TestRenderProposalAndShapesHomepage tests the core board.Render() function for
// various task shapes: dependencies, status variants, isolated tasks, deferred tasks,
// and brief/title formatting. Each case asserts matching expected Home.md substrings.
//
// Folds: TestRenderDependencies, TestRenderSpecialBucketTask, TestRenderIsolatedTask,
// TestRenderTaskIDFormatting, TestRenderBrief, TestRenderMissingDependency, TestRenderLayerBuckets
func TestRenderProposalAndShapesHomepage(t *testing.T) {
	tests := []struct {
		name           string
		tasks          []board.Task
		wantSubstrings []string
		dontWantSubstr []string
	}{
		{
			name: "TestRenderDependencies",
			tasks: []board.Task{
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
			tasks: []board.Task{
				func() board.Task {
					s := "done"
					return board.Task{ID: 1, Slug: "done-task", Title: "Done Task", Status: &s}
				}(),
			},
			wantSubstrings: []string{
				"# Done",
				"## **#001:** Done Task\n",
			},
			dontWantSubstr: []string{"[Done]"},
		},
		{
			name: "TestRenderIsolatedTask",
			tasks: []board.Task{
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
			tasks: []board.Task{
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
			tasks: []board.Task{
				{ID: 1, Slug: "test-task", Title: "Test Task", Brief: "This is the brief text"},
			},
			wantSubstrings: []string{
				"This is the brief text",
			},
		},
		{
			name: "TestRenderMissingDependency",
			tasks: []board.Task{
				{ID: 1, Slug: "task-a", Title: "Task A", DependsOn: []string{"missing-task"}},
			},
			wantSubstrings: []string{
				"#???: missing-task (missing)",
			},
		},
		{
			name: "TestRenderLayerBuckets",
			tasks: []board.Task{
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
			result, err := board.Render(tt.tasks, board.DefaultOutputs())
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
			task := board.Task{
				ID:     1,
				Slug:   "test-task",
				Title:  "Test Task",
				Status: &s,
			}

			result, err := board.Render([]board.Task{task}, board.DefaultOutputs())
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
// verifying proposal file creation and Home.md content.
//
// Folds: TestRenderSingleTaskNoBody, TestRenderSingleTaskWithBody, TestRenderOrphanDetection
func TestRenderSingleTask(t *testing.T) {
	tests := []struct {
		name            string
		task            board.Task
		wantProposalKey string
		wantProposal    bool // true if proposal file should exist
		wantHome        []string
		dontWantHome    []string
	}{
		{
			name:            "TestRenderSingleTaskNoBody",
			task:            board.Task{ID: 1, Slug: "test-task", Title: "Test Task"},
			wantProposalKey: "proposal-test-task.md",
			wantProposal:    false,
			wantHome: []string{
				"## **#001:** Test Task [A]",
				"[test-task]",
			},
		},
		{
			name:            "TestRenderSingleTaskWithBody",
			task:            board.Task{ID: 1, Slug: "test-task", Title: "Test Task", Body: "This is the body content"},
			wantProposalKey: "proposal-test-task.md",
			wantProposal:    true,
			wantHome: []string{
				"[test-task](proposal-test-task.md)",
			},
		},
		{
			name:            "TestRenderOrphanDetection",
			task:            board.Task{ID: 1, Slug: "orphan-task", Title: "Orphan Task", Body: "Original body"},
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
			result, err := board.Render([]board.Task{tt.task}, board.DefaultOutputs())
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
					t.Errorf("Missing proposal file: %s", tt.wantProposalKey)
				} else if proposalContent != tt.task.Body {
					t.Errorf("Proposal content mismatch\nExpected: %q\nGot: %q", tt.task.Body, proposalContent)
				}
			} else {
				if _, ok := result[tt.wantProposalKey]; ok {
					t.Errorf("Unexpected proposal file: %s", tt.wantProposalKey)
				}
			}
		})
	}
}

// TestRenderSidebarExtendedTitle tests that the sidebar correctly renders task titles
// with extended formatting (layer suffix for regular tasks) and verifies blank lines
// between bucket groups.
//
// Folds: TestRenderExtendedTitle, TestRenderSidebarBlanks
func TestRenderSidebarExtendedTitle(t *testing.T) {
	tests := []struct {
		name          string
		task          board.Task
		wantSidebar   string
		wantBlankLine bool
	}{
		{
			name:        "TestRenderExtendedTitle",
			task:        board.Task{ID: 1, Slug: "test-task", Title: "Test Task"},
			wantSidebar: "- Test Task [A]",
		},
		{
			name: "TestRenderSidebarBlanks",
			task: board.Task{ID: 1, Slug: "task-a", Title: "Task A"},
			// This case will be tested with a multi-task setup
		},
	}

	// Test single task extended title
	t.Run(tests[0].name, func(t *testing.T) {
		result, err := board.Render([]board.Task{tests[0].task}, board.DefaultOutputs())
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		sidebar := result["_Sidebar.md"]
		if !strings.Contains(sidebar, tests[0].wantSidebar) {
			t.Errorf("Sidebar should use extended title\nExpected: %s\nGot: %s", tests[0].wantSidebar, sidebar)
		}
	})

	// Test blank lines between bucket groups
	t.Run("TestRenderSidebarBlanks", func(t *testing.T) {
		taskA := board.Task{ID: 1, Slug: "task-a", Title: "Task A"}
		doneStatus := "done"
		taskDone := board.Task{ID: 2, Slug: "task-done", Title: "Done Task", Status: &doneStatus}

		result, err := board.Render([]board.Task{taskA, taskDone}, board.DefaultOutputs())
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		sidebar := result["_Sidebar.md"]
		// Should have a blank line between Layer A and Done sections
		if !strings.Contains(sidebar, "\n\n") {
			t.Errorf("Sidebar should have blank line between bucket groups\nGot: %s", sidebar)
		}
	})
}

// TestRenderCustomOutputs verifies that Render respects configurable Outputs fields,
// covering both a custom Home filename and a custom proposal prefix.
//
// Folds: TestRenderConfigurableHomeFilename, TestRenderConfigurableProposalPrefix
func TestRenderCustomOutputs(t *testing.T) {
	t.Run("TestRenderConfigurableHomeFilename", func(t *testing.T) {
		// Test that Render uses configured Home filename instead of "Home.md"
		task := board.Task{
			ID:    1,
			Slug:  "test-task",
			Title: "Test Task",
		}
		out := board.Outputs{
			Home:           "README.md",
			Sidebar:        "_Sidebar.md",
			ProposalPrefix: "proposal-",
		}
		result, err := board.Render([]board.Task{task}, out)
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
		// Test that Render uses configured proposal prefix
		task := board.Task{
			ID:    1,
			Slug:  "test-task",
			Title: "Test Task",
			Body:  "Proposal body",
		}
		out := board.Outputs{
			Home:           "Home.md",
			Sidebar:        "_Sidebar.md",
			ProposalPrefix: "prop-",
		}
		result, err := board.Render([]board.Task{task}, out)
		if err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		// Check proposal file uses custom prefix
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

		// Check links in Sidebar use custom prefix
		sidebar := result["_Sidebar.md"]
		if !strings.Contains(sidebar, "[Test Task [A]](prop-test-task.md)") {
			t.Errorf("Sidebar should use custom prefix in links\nGot: %s", sidebar)
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
