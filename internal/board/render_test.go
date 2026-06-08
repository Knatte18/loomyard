// render_test.go — unit tests for rendering (render.go).
//
// Home.md / _Sidebar.md / proposal output across task shapes: dependencies,
// status, isolated, deferred, orphans, and title formatting.

package board_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/board"
)

func TestRenderToDiskWritesAndCleansOrphans(t *testing.T) {
	dir := t.TempDir()

	// A stale proposal from a previous render that should be cleaned up.
	ghost := filepath.Join(dir, "proposal-ghost.md")
	if err := os.WriteFile(ghost, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks := []wiki.Task{
		{ID: 0, Slug: "a", Title: "A", Body: "proposal A"},
		{ID: 1, Slug: "b", Title: "B"}, // no body → no proposal file
	}
	if err := wiki.RenderToDisk(dir, tasks); err != nil {
		t.Fatalf("RenderToDisk: %v", err)
	}

	for _, f := range []string{"Home.md", "_Sidebar.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "proposal-a.md")); err != nil || string(b) != "proposal A" {
		t.Errorf("proposal-a.md: got %q, err %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "proposal-b.md")); !os.IsNotExist(err) {
		t.Errorf("proposal-b.md should not exist (task has no body)")
	}
	if _, err := os.Stat(ghost); !os.IsNotExist(err) {
		t.Errorf("orphan proposal-ghost.md should have been removed")
	}
}

func TestRenderEmptyTaskList(t *testing.T) {
	// (a) empty task list → Home.md is exactly "# Tasks\n", Sidebar is "", no proposal files
	result, err := wiki.Render([]wiki.Task{})
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

func TestRenderSingleTaskNoBody(t *testing.T) {
	// (b) single task no body → Home.md has correct heading and slug line, no proposal file
	task := wiki.Task{
		ID:    1,
		Slug:  "test-task",
		Title: "Test Task",
	}
	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Check Home.md has heading and slug line
	home := result["Home.md"]
	if !strings.Contains(home, "## **#001:** Test Task [A]") {
		t.Errorf("Home.md missing expected heading\nGot: %s", home)
	}
	if !strings.Contains(home, "[test-task]") {
		t.Errorf("Home.md missing expected slug line\nGot: %s", home)
	}

	// Check no proposal file
	if _, ok := result["proposal-test-task.md"]; ok {
		t.Errorf("Unexpected proposal file for task without body")
	}
}

func TestRenderSingleTaskWithBody(t *testing.T) {
	// (c) single task with body → proposal-<slug>.md key present with body content
	task := wiki.Task{
		ID:    1,
		Slug:  "test-task",
		Title: "Test Task",
		Body:  "This is the body content",
	}
	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	proposalKey := "proposal-test-task.md"
	if proposalContent, ok := result[proposalKey]; !ok {
		t.Errorf("Missing proposal file: %s", proposalKey)
	} else if proposalContent != "This is the body content" {
		t.Errorf("Proposal content mismatch\nExpected: %q\nGot: %q", "This is the body content", proposalContent)
	}

	// Check Home.md has link to proposal
	home := result["Home.md"]
	if !strings.Contains(home, "[test-task](proposal-test-task.md)") {
		t.Errorf("Home.md missing expected proposal link\nGot: %s", home)
	}
}

func TestRenderTaskStatus(t *testing.T) {
	// (d) task with active status → slug line ends with " [active]"
	activeStatus := "active"
	task := wiki.Task{
		ID:     1,
		Slug:   "test-task",
		Title:  "Test Task",
		Status: &activeStatus,
	}
	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]
	if !strings.Contains(home, "[test-task] [active]") {
		t.Errorf("Home.md missing status suffix\nGot: %s", home)
	}
}

func TestRenderDependencies(t *testing.T) {
	// (e) two tasks A depends on B → bucket headers in correct order (B in Layer A section, A in Layer B section)
	// (i) task with DependsOn → Depends on: #NNN line present
	taskB := wiki.Task{
		ID:    1,
		Slug:  "task-b",
		Title: "Task B",
	}
	taskA := wiki.Task{
		ID:        2,
		Slug:      "task-a",
		Title:     "Task A",
		DependsOn: []string{"task-b"},
	}

	result, err := wiki.Render([]wiki.Task{taskB, taskA})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	// Check bucket order: Layer A before Layer B
	layerAIdx := strings.Index(home, "# Layer A")
	layerBIdx := strings.Index(home, "# Layer B")
	if layerAIdx == -1 || layerBIdx == -1 {
		t.Errorf("Missing layer headers\nGot: %s", home)
	} else if layerAIdx > layerBIdx {
		t.Errorf("Layer A should come before Layer B")
	}

	// Check depends on line
	if !strings.Contains(home, "Depends on: #001") {
		t.Errorf("Home.md missing Depends on line\nGot: %s", home)
	}
}

func TestRenderDoneTask(t *testing.T) {
	// (f) done task → appears under # Done, heading has no layer suffix
	doneStatus := "done"
	task := wiki.Task{
		ID:     1,
		Slug:   "done-task",
		Title:  "Done Task",
		Status: &doneStatus,
	}

	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	if !strings.Contains(home, "# Done") {
		t.Errorf("Home.md missing # Done header\nGot: %s", home)
	}

	// Heading should not have layer suffix
	if !strings.Contains(home, "## **#001:** Done Task\n") {
		t.Errorf("Done task heading should not have layer suffix\nGot: %s", home)
	}
}

func TestRenderIsolatedTask(t *testing.T) {
	// (g) isolated task → appears under letter Z in bucket order after all letter buckets
	taskA := wiki.Task{
		ID:    1,
		Slug:  "task-a",
		Title: "Task A",
	}
	taskZ := wiki.Task{
		ID:       2,
		Slug:     "task-z",
		Title:    "Isolated Task",
		Isolated: true,
	}

	result, err := wiki.Render([]wiki.Task{taskA, taskZ})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	// Check that Layer A comes before Layer Z
	layerAIdx := strings.Index(home, "# Layer A")
	layerZIdx := strings.Index(home, "# Layer Z")
	if layerAIdx == -1 || layerZIdx == -1 {
		t.Errorf("Missing layer headers\nGot: %s", home)
	} else if layerAIdx > layerZIdx {
		t.Errorf("Layer A should come before Layer Z")
	}
}

func TestRenderDeferredTask(t *testing.T) {
	// (h) deferred task → appears under # Someday
	task := wiki.Task{
		ID:       1,
		Slug:     "deferred-task",
		Title:    "Deferred Task",
		Deferred: true,
	}

	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	if !strings.Contains(home, "# Someday") {
		t.Errorf("Home.md missing # Someday header\nGot: %s", home)
	}

	// Heading should not have layer suffix
	if !strings.Contains(home, "## **#001:** Deferred Task\n") {
		t.Errorf("Deferred task heading should not have layer suffix\nGot: %s", home)
	}
}

func TestRenderTaskIDFormatting(t *testing.T) {
	// (j) multiple tasks in same bucket sorted by ID
	tasks := []wiki.Task{
		{ID: 3, Slug: "task-c", Title: "Task C"},
		{ID: 1, Slug: "task-a", Title: "Task A"},
		{ID: 2, Slug: "task-b", Title: "Task B"},
	}

	result, err := wiki.Render(tasks)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	// Check that tasks are in ID order
	taskAIdx := strings.Index(home, "## **#001:** Task A")
	taskBIdx := strings.Index(home, "## **#002:** Task B")
	taskCIdx := strings.Index(home, "## **#003:** Task C")

	if taskAIdx == -1 || taskBIdx == -1 || taskCIdx == -1 {
		t.Errorf("Missing task headings\nGot: %s", home)
	} else if !(taskAIdx < taskBIdx && taskBIdx < taskCIdx) {
		t.Errorf("Tasks should be in ID order")
	}

	// Check ID padding (should be 3 digits)
	if !strings.Contains(home, "#001") || !strings.Contains(home, "#002") || !strings.Contains(home, "#003") {
		t.Errorf("Task IDs should be padded to 3 digits\nGot: %s", home)
	}
}

func TestRenderSidebarBlanks(t *testing.T) {
	// (k) Sidebar has blank line between bucket groups
	taskA := wiki.Task{
		ID:    1,
		Slug:  "task-a",
		Title: "Task A",
	}
	doneStatus := "done"
	taskDone := wiki.Task{
		ID:     2,
		Slug:   "task-done",
		Title:  "Done Task",
		Status: &doneStatus,
	}

	result, err := wiki.Render([]wiki.Task{taskA, taskDone})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	sidebar := result["_Sidebar.md"]

	// Should have a blank line between Layer A and Done sections
	if !strings.Contains(sidebar, "\n\n") {
		t.Errorf("Sidebar should have blank line between bucket groups\nGot: %s", sidebar)
	}
}

func TestRenderBrief(t *testing.T) {
	// Test that brief text is included
	task := wiki.Task{
		ID:    1,
		Slug:  "test-task",
		Title: "Test Task",
		Brief: "This is the brief text",
	}

	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	if !strings.Contains(home, "This is the brief text") {
		t.Errorf("Home.md should contain brief text\nGot: %s", home)
	}
}

func TestRenderMissingDependency(t *testing.T) {
	// Test that missing dependencies are handled
	task := wiki.Task{
		ID:        1,
		Slug:      "task-a",
		Title:     "Task A",
		DependsOn: []string{"missing-task"},
	}

	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	home := result["Home.md"]

	if !strings.Contains(home, "#???: missing-task (missing)") {
		t.Errorf("Home.md should show missing dependency\nGot: %s", home)
	}
}

func TestRenderOrphanDetection(t *testing.T) {
	// (l) orphan detection — render with body, then render again without body → second call's result map has no proposal file
	taskWithBody := wiki.Task{
		ID:    1,
		Slug:  "orphan-task",
		Title: "Orphan Task",
		Body:  "Original body",
	}

	result1, err := wiki.Render([]wiki.Task{taskWithBody})
	if err != nil {
		t.Fatalf("First render failed: %v", err)
	}

	if _, ok := result1["proposal-orphan-task.md"]; !ok {
		t.Errorf("First render should have proposal file")
	}

	// Now render the same task without body
	taskWithoutBody := wiki.Task{
		ID:    1,
		Slug:  "orphan-task",
		Title: "Orphan Task",
		Body:  "",
	}

	result2, err := wiki.Render([]wiki.Task{taskWithoutBody})
	if err != nil {
		t.Fatalf("Second render failed: %v", err)
	}

	if _, ok := result2["proposal-orphan-task.md"]; ok {
		t.Errorf("Second render should not have proposal file for task without body")
	}
}

func TestRenderStatusVariants(t *testing.T) {
	// Test all valid status values
	statuses := []string{"active", "done", "pr-pending", "ready-to-merge", "abandoned"}
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status-%s", status), func(t *testing.T) {
			s := status
			task := wiki.Task{
				ID:     1,
				Slug:   "test-task",
				Title:  "Test Task",
				Status: &s,
			}

			result, err := wiki.Render([]wiki.Task{task})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			home := result["Home.md"]
			expected := fmt.Sprintf("[test-task] [%s]", status)

			// For done status, shouldn't appear in slug line since it's in __done__ bucket
			if status == "done" {
				doneStatus := "done"
				taskDone := wiki.Task{
					ID:     1,
					Slug:   "test-task",
					Title:  "Test Task",
					Status: &doneStatus,
				}
				result, _ := wiki.Render([]wiki.Task{taskDone})
				home := result["Home.md"]
				// Done task should show [done] status
				if !strings.Contains(home, "[test-task] [done]") {
					t.Errorf("Done task should show [done] status in slug line\nGot: %s", home)
				}
			} else if !strings.Contains(home, expected) {
				t.Errorf("Home.md should contain %q\nGot: %s", expected, home)
			}
		})
	}
}

func TestRenderExtendedTitle(t *testing.T) {
	// Test that sidebar uses ExtendedTitle correctly
	task := wiki.Task{
		ID:    1,
		Slug:  "test-task",
		Title: "Test Task",
	}

	result, err := wiki.Render([]wiki.Task{task})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	sidebar := result["_Sidebar.md"]

	// Extended title should have layer suffix for non-done/deferred tasks
	if !strings.Contains(sidebar, "- Test Task [A]") {
		t.Errorf("Sidebar should use extended title with layer\nGot: %s", sidebar)
	}
}

func TestRenderLayerBuckets(t *testing.T) {
	// Test layer bucket ordering
	tests := []struct {
		name     string
		task     wiki.Task
		expected string
	}{
		{
			name:     "Letter bucket A",
			task:     wiki.Task{ID: 1, Slug: "a", Title: "Task A"},
			expected: "# Layer A",
		},
		{
			name: "Isolated bucket Z",
			task: wiki.Task{ID: 1, Slug: "z", Title: "Isolated", Isolated: true},
			expected: "# Layer Z",
		},
		{
			name: "Deferred bucket",
			task: wiki.Task{ID: 1, Slug: "d", Title: "Deferred", Deferred: true},
			expected: "# Someday",
		},
		{
			name: "Done bucket",
			task: func() wiki.Task {
				s := "done"
				return wiki.Task{ID: 1, Slug: "done", Title: "Done", Status: &s}
			}(),
			expected: "# Done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := wiki.Render([]wiki.Task{tt.task})
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			home := result["Home.md"]
			if !strings.Contains(home, tt.expected) {
				t.Errorf("Home.md missing %q\nGot: %s", tt.expected, home)
			}
		})
	}
}
