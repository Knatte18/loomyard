//go:build integration

// spawn_driven_hub_integration_test.go drives SpawnDriven against real hubs from hubforge.NewHub and hubforge.AddPair:
// the attach-only chain, the shared info/exclude, and the tracked-tasks.json skip.
// Serial by design, because every test swaps the package-level CodeLauncher.

package ideengine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

const drivenSlug = "some-task"

// gitOut runs git in dir and returns its trimmed stdout, failing the test on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// sharedExcludePath resolves the shared info/exclude of the task worktree's repo.
func sharedExcludePath(t *testing.T, worktreeDir string) string {
	t.Helper()
	p := gitOut(t, worktreeDir, "rev-parse", "--git-path", "info/exclude")
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

// TestSpawnDrivenWritesAttachOnlyAndExcludes covers the launch argument, the single attach task, a clean tree and the anchored exclude at both anchors.
func TestSpawnDrivenWritesAttachOnlyAndExcludes(t *testing.T) {
	for _, anchor := range []string{".", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			h := hubforge.NewHub(t, anchor)
			l := h.Location
			launched := recordLauncher(t)
			hubforge.AddPair(t, h, drivenSlug)
			worktreeDir := fabricengine.WorktreePath(l, drivenSlug)
			excludePath := sharedExcludePath(t, worktreeDir)
			custom := "# custom-line-kept\n"
			existing, _ := os.ReadFile(excludePath)
			if err := os.WriteFile(excludePath, append(existing, []byte(custom)...), 0o644); err != nil {
				t.Fatalf("write custom exclude line: %v", err)
			}

			if err := SpawnDriven(l, drivenSlug); err != nil {
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
			tasks := readTasks(t, want)
			if len(tasks) != 1 || tasks[0]["label"] != "reed attach" {
				t.Errorf("tasks = %v, want the single attach task", tasks)
			}
			if status := gitOut(t, worktreeDir, "status", "--porcelain"); status != "" {
				t.Errorf("git status --porcelain not empty:\n%s", status)
			}
			ignored := filepath.ToSlash(filepath.Join(l.AnchorRel, ".vscode")) + "/"
			if l.AnchorRel == "." {
				ignored = ".vscode/"
			}
			if out := gitOut(t, worktreeDir, "check-ignore", ignored); out == "" {
				t.Errorf("check-ignore reported %s not ignored", ignored)
			}
			got, err := os.ReadFile(excludePath)
			if err != nil {
				t.Fatalf("read exclude: %v", err)
			}
			if !strings.Contains(string(got), custom) {
				t.Errorf("custom exclude line lost:\n%s", got)
			}
		})
	}
}

// TestSpawnDrivenOverwritesTasksKeepsSettings asserts an untracked interactive tasks.json is replaced and settings.json is untouched.
func TestSpawnDrivenOverwritesTasksKeepsSettings(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	l := h.Location
	recordLauncher(t)
	hubforge.AddPair(t, h, drivenSlug)
	vscodeDir := filepath.Join(fabricengine.WorktreePath(l, drivenSlug), l.AnchorRel, ".vscode")
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

	if err := SpawnDriven(l, drivenSlug); err != nil {
		t.Fatalf("SpawnDriven: %v", err)
	}

	if tasks := readTasks(t, filepath.Dir(vscodeDir)); len(tasks) != 1 {
		t.Errorf("tasks = %v, want the single attach task", tasks)
	}
	got, err := os.ReadFile(filepath.Join(vscodeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(got) != settings {
		t.Errorf("settings.json changed:\n%s", got)
	}
}

// TestSpawnDrivenSkipsTrackedVSCode asserts a committed tasks.json is left alone, the exclude is unchanged, and the launch still happens with one warning.
func TestSpawnDrivenSkipsTrackedVSCode(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, drivenSlug)
	worktreeDir := fabricengine.WorktreePath(l, drivenSlug)
	anchorDir := filepath.Join(worktreeDir, l.AnchorRel)
	tasksPath := filepath.Join(anchorDir, ".vscode", "tasks.json")
	if err := os.MkdirAll(filepath.Dir(tasksPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	committed := []byte(`{"version":"2.0.0","tasks":[]}`)
	if err := os.WriteFile(tasksPath, committed, 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}
	gitOut(t, worktreeDir, "add", "-f", ".vscode/tasks.json")
	gitOut(t, worktreeDir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "track vscode")
	excludePath := sharedExcludePath(t, worktreeDir)
	excludeBefore, _ := os.ReadFile(excludePath)

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if err := SpawnDriven(l, drivenSlug); err != nil {
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
		if strings.Contains(line, "level=WARN") && strings.Contains(line, drivenSlug) {
			warns++
		}
	}
	if warns != 1 {
		t.Errorf("warning lines naming the slug = %d, want 1:\n%s", warns, logBuf.String())
	}
}
