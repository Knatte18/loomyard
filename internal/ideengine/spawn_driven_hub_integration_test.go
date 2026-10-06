//go:build integration

// spawn_driven_hub_integration_test.go holds the SpawnDriven checks that run against task pairs of one hubforge hub:
// the attach-only chain, the shared info/exclude, and the tracked-tasks.json skip.
// Each check is a step of a scenario in spawn_scenario_integration_test.go, which names why they run serially.

package ideengine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// sharedExcludePath resolves the shared info/exclude of the task worktree's repo.
func sharedExcludePath(t *testing.T, worktreeDir string) string {
	t.Helper()
	p := gitkit.Git(t, worktreeDir, "rev-parse", "--git-path", "info/exclude")
	if !filepath.IsAbs(p) {
		p = filepath.Join(worktreeDir, p)
	}
	return p
}

// readTasks decodes the attach-only tasks.json under anchorDir.
func readTasks(t *testing.T, anchorDir string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(anchorDir, ".vscode", "tasks.json"))
	if err != nil {
		t.Fatalf("read tasks.json: %v", err)
	}
	var tasks struct {
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("tasks.json is not valid JSON: %v\n%s", err, data)
	}
	return tasks.Tasks
}

// assertSingleAttachTask fails the test unless anchorDir's tasks.json holds exactly one task running `reed attach` on folderOpen.
func assertSingleAttachTask(t *testing.T, anchorDir string) {
	t.Helper()
	tasks := readTasks(t, anchorDir)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %v, want exactly one", tasks)
	}
	task := tasks[0]
	args, _ := task["args"].([]any)
	runOptions, _ := task["runOptions"].(map[string]any)
	if task["label"] != "reed attach" || len(args) != 2 || args[0] != "reed" || args[1] != "attach" || runOptions["runOn"] != "folderOpen" {
		t.Errorf("task = %v, want label and args reed attach, runOn folderOpen", task)
	}
}

// checkDrivenWritesAttachOnlyAndExcludes covers the launch argument, the single attach task, a clean tree and the anchored exclude at the hub's anchor.
func checkDrivenWritesAttachOnlyAndExcludes(t *testing.T, h *hubforge.Hub, slug string) {
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	worktreeDir := fabricengine.WorktreePath(l, slug)
	excludePath := sharedExcludePath(t, worktreeDir)
	custom := "# custom-line-kept\n"
	existing, _ := os.ReadFile(excludePath)
	if err := os.WriteFile(excludePath, append(existing, []byte(custom)...), 0o644); err != nil {
		t.Fatalf("write custom exclude line: %v", err)
	}

	if err := SpawnDriven(l, slug); err != nil {
		t.Fatalf("SpawnDriven: %v", err)
	}

	want := filepath.Join(worktreeDir, l.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Fatalf("CodeLauncher calls = %v, want exactly [%s]", *launched, want)
	}
	resolved, err := lyxcwd.ResolveWorktree(worktreeDir)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	if resolved.AnchorPath() != want {
		t.Errorf("launched %s, ResolveWorktree AnchorPath = %s", want, resolved.AnchorPath())
	}
	assertSingleAttachTask(t, want)
	if status := gitkit.Git(t, worktreeDir, "status", "--porcelain"); status != "" {
		t.Errorf("git status --porcelain not empty:\n%s", status)
	}
	ignored := filepath.ToSlash(filepath.Join(l.AnchorRel, ".vscode")) + "/"
	if l.AnchorRel == "." {
		ignored = ".vscode/"
	}
	if out := gitkit.Git(t, worktreeDir, "check-ignore", ignored); out == "" {
		t.Errorf("check-ignore reported %s not ignored", ignored)
	}
	got, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if !strings.Contains(string(got), custom) {
		t.Errorf("custom exclude line lost:\n%s", got)
	}
}

// checkDrivenOverwritesTasksKeepsSettings asserts an untracked interactive tasks.json is replaced and settings.json is untouched.
func checkDrivenOverwritesTasksKeepsSettings(t *testing.T, h *hubforge.Hub, slug string) {
	l := h.Location
	recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	vscodeDir := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	interactive := `{"version":"2.0.0","tasks":[{"label":"reed up"},{"label":"reed add claude"},{"label":"reed attach"}]}`
	settings := "{\n  // keep me\n  \"editor.tabSize\": 2\n}"
	if err := os.WriteFile(filepath.Join(vscodeDir, "tasks.json"), []byte(interactive), 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	if err := SpawnDriven(l, slug); err != nil {
		t.Fatalf("SpawnDriven: %v", err)
	}

	assertSingleAttachTask(t, filepath.Dir(vscodeDir))
	got, err := os.ReadFile(filepath.Join(vscodeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(got) != settings {
		t.Errorf("settings.json changed:\n%s", got)
	}
}

// checkDrivenSkipsTrackedVSCode asserts a committed tasks.json is left alone, the exclude is unchanged, and the launch still happens with one warning.
func checkDrivenSkipsTrackedVSCode(t *testing.T, h *hubforge.Hub, slug string) {
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	worktreeDir := fabricengine.WorktreePath(l, slug)
	anchorDir := filepath.Join(worktreeDir, l.AnchorRel)
	tasksPath := filepath.Join(anchorDir, ".vscode", "tasks.json")
	if err := os.MkdirAll(filepath.Dir(tasksPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	committed := []byte(`{"version":"2.0.0","tasks":[]}`)
	if err := os.WriteFile(tasksPath, committed, 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}
	gitkit.Git(t, worktreeDir, "add", "-f", ".vscode/tasks.json")
	gitkit.Git(t, worktreeDir, "commit", "-m", "track vscode")
	excludePath := sharedExcludePath(t, worktreeDir)
	excludeBefore, _ := os.ReadFile(excludePath)

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if err := SpawnDriven(l, slug); err != nil {
		t.Fatalf("SpawnDriven: %v", err)
	}

	got, _ := os.ReadFile(tasksPath)
	if !bytes.Equal(got, committed) {
		t.Errorf("tracked tasks.json changed:\n%s", got)
	}
	excludeAfter, _ := os.ReadFile(excludePath)
	if !bytes.Equal(excludeBefore, excludeAfter) {
		t.Errorf("info/exclude changed:\n%s", excludeAfter)
	}
	if len(*launched) != 1 || (*launched)[0] != anchorDir {
		t.Errorf("CodeLauncher calls = %v, want [%s]", *launched, anchorDir)
	}
	warns := 0
	for _, line := range strings.Split(logBuf.String(), "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, slug) {
			warns++
		}
	}
	if warns != 1 {
		t.Errorf("warning lines naming the slug = %d, want 1:\n%s", warns, logBuf.String())
	}
}
