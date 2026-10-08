//go:build integration

// spawn_clean_integration_test.go holds the interactive-chain Spawn checks that run against task pairs and the prime of one hubforge hub:
// a clean tree through info/exclude, the overwrite of an untracked tasks.json and the tracked-tasks.json skip.
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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// readTasks decodes the tasks.json under anchorDir.
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

// commitFile writes content to rel under dir and commits it, force-adding because the path may be ignored.
func commitFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	gitkit.Git(t, dir, "add", "-f", "--", rel)
	gitkit.Git(t, dir, "commit", "-m", "add "+rel)
}

// assertInteractiveChain fails the test unless anchorDir's tasks.json is the interactive chain with absolute commands and --unless-name orch on the add row.
func assertInteractiveChain(t *testing.T, anchorDir string) {
	t.Helper()
	var add map[string]any
	labels := map[string]bool{}
	for _, task := range readTasks(t, anchorDir) {
		labels[task["label"].(string)] = true
		if command, ok := task["command"].(string); ok && !filepath.IsAbs(command) {
			t.Errorf("task %v command = %q; want an absolute path", task["label"], command)
		}
		if task["label"] == "reed add claude" {
			add = task
		}
	}
	for _, want := range []string{"reed up", "reed add claude", "reed attach", "Start Claude"} {
		if !labels[want] {
			t.Errorf("missing task %q in %v", want, labels)
		}
	}
	if add == nil {
		t.Fatalf("no reed add claude task")
	}
	var args []string
	for _, a := range add["args"].([]any) {
		args = append(args, a.(string))
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--if-absent", "--unless-name orch", "--name claude", "--focus"} {
		if !strings.Contains(joined, want) {
			t.Errorf("reed add claude args = %v; missing %q", args, want)
		}
	}
}

// checkTaskPairStaysClean asserts a repo with a tracked .gitignore stays clean after Spawn, with .vscode/ ignored through info/exclude or already ignored by the repo.
func checkTaskPairStaysClean(t *testing.T, h *hubforge.Hub, slug, gitignore string, wantExclude bool) {
	l := h.Location
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	worktreeDir := fabricengine.WorktreePath(l, slug)
	commitFile(t, worktreeDir, ".gitignore", gitignore)
	excludePath := sharedExcludePath(t, worktreeDir)
	excludeBefore, _ := os.ReadFile(excludePath)

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	anchorDir := filepath.Join(worktreeDir, l.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != anchorDir {
		t.Fatalf("CodeLauncher calls = %v, want [%s]", *launched, anchorDir)
	}
	if status := gitkit.Git(t, worktreeDir, "status", "--porcelain"); status != "" {
		t.Errorf("git status --porcelain not empty:\n%s", status)
	}
	if out := gitkit.Git(t, worktreeDir, "check-ignore", ".vscode/"); out == "" {
		t.Errorf("check-ignore reported .vscode/ not ignored")
	}
	excludeAfter, _ := os.ReadFile(excludePath)
	if got := strings.Contains(string(excludeAfter), "/.vscode/"); got != wantExclude {
		t.Errorf("info/exclude carries the .vscode line = %v, want %v:\n%s", got, wantExclude, excludeAfter)
	}
	if !wantExclude && !bytes.Equal(excludeBefore, excludeAfter) {
		t.Errorf("info/exclude changed:\n%s", excludeAfter)
	}
	assertInteractiveChain(t, anchorDir)
}

// checkOverwritesTasksKeepsSettings asserts an untracked tasks.json holding an old chain is replaced and an existing settings.json is untouched.
func checkOverwritesTasksKeepsSettings(t *testing.T, h *hubforge.Hub, slug string) {
	l := h.Location
	recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	vscodeDir := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldChain := `{"version":"2.0.0","tasks":[{"label":"old chain"}]}`
	settings := "{\n  // keep me\n  \"editor.tabSize\": 2\n}"
	if err := os.WriteFile(filepath.Join(vscodeDir, "tasks.json"), []byte(oldChain), 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	assertInteractiveChain(t, filepath.Dir(vscodeDir))
	got, err := os.ReadFile(filepath.Join(vscodeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if string(got) != settings {
		t.Errorf("settings.json changed:\n%s", got)
	}
}

// checkSkipsTrackedVSCode asserts a committed tasks.json is left alone, no settings.json or exclude line is written, and the launch still happens, on a task pair and on the prime.
// It commits to the prime when onPrime is set, so no later step may rely on a clean prime.
func checkSkipsTrackedVSCode(t *testing.T, h *hubforge.Hub, slug string, onPrime bool) {
	l := h.Location
	launched := recordLauncher(t)
	prime := primeOf(t, h)
	if onPrime {
		resetPrimeEditorState(t, h)
		slug = prime
	} else {
		hubforge.AddPair(t, h, slug)
	}
	worktreeDir := fabricengine.WorktreePath(l, slug)
	anchorDir := filepath.Join(worktreeDir, l.AnchorRel)
	committed := `{"version":"2.0.0","tasks":[]}`
	commitFile(t, worktreeDir, filepath.ToSlash(filepath.Join(l.AnchorRel, ".vscode", "tasks.json")), committed)
	excludePath := sharedExcludePath(t, worktreeDir)
	excludeBefore, _ := os.ReadFile(excludePath)
	logBuf := logcapture.Capture(t)

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(anchorDir, ".vscode", "tasks.json"))
	if string(got) != committed {
		t.Errorf("tracked tasks.json changed:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(anchorDir, ".vscode", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json written despite the tracked tasks.json (stat err = %v)", err)
	}
	excludeAfter, _ := os.ReadFile(excludePath)
	if !bytes.Equal(excludeBefore, excludeAfter) {
		t.Errorf("info/exclude changed:\n%s", excludeAfter)
	}
	if !strings.Contains(logBuf.String(), "tracked .vscode/tasks.json") {
		t.Errorf("no tracked-skip warning logged:\n%s", logBuf.String())
	}
	want := anchorDir
	if onPrime {
		want = fabricengine.HubWorkspacePath(l, prime)
		if _, err := os.Stat(want); err != nil {
			t.Errorf("hub workspace file not written: %v", err)
		}
	}
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Errorf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
}

// checkPrimeNameFailureOpensBareFolder asserts a Location whose worktree is not a git checkout logs the resolve-prime-name warning and launches the pair's bare folder.
func checkPrimeNameFailureOpensBareFolder(t *testing.T, h *hubforge.Hub, slug string) {
	launched := recordLauncher(t)
	hubforge.AddPair(t, h, slug)
	notGit := "not-a-checkout"
	notGitDir := filepath.Join(h.Path, notGit)
	if err := os.MkdirAll(notGitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(notGitDir) })
	l := &lyxcwd.Location{RepoName: h.Location.RepoName, HubPath: h.Path, WorktreeName: notGit, AnchorRel: h.Location.AnchorRel}
	logBuf := logcapture.Capture(t)

	if _, err := Spawn(l, slug); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !strings.Contains(logBuf.String(), "resolve prime name") {
		t.Errorf("no resolve prime name warning logged:\n%s", logBuf.String())
	}
	want := filepath.Join(fabricengine.WorktreePath(h.Location, slug), h.Location.AnchorRel)
	if len(*launched) != 1 || (*launched)[0] != want {
		t.Errorf("CodeLauncher calls = %v, want [%s]", *launched, want)
	}
}
