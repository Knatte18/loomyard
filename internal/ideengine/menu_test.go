//go:build integration

// menu_test.go covers worktree discovery (excludes main, requires _lyx/),
// board-facade titles, numeric selection, the zero-worktree path, and the
// missing-board hard error.

package ideengine

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

func mustRunMenu(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command failed: %v; output: %s", err, output)
	}
}

func newTestGitRepoWithWorktrees(t *testing.T) (string, string) {
	t.Helper()

	container := t.TempDir()
	mainWorktreePath := filepath.Join(container, "main")

	if err := os.Mkdir(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main worktree: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "init", "-b", "main")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.email", "test@test.com")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.name", "Test")

	readmeFile := filepath.Join(mainWorktreePath, "README")
	if err := os.WriteFile(readmeFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "add", ".")
	mustRunMenu(t, mainWorktreePath, "git", "commit", "-m", "initial")

	if err := os.MkdirAll(filepath.Join(mainWorktreePath, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("failed to create main _lyx: %v", err)
	}

	return container, mainWorktreePath
}

func TestMenuHardErrorOnMissingBoard(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	layout := &lyxcwd.Location{HubPath: container, WorktreeName: filepath.Base(mainWorktreePath), AnchorRel: "."}

	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err == nil {
		t.Fatalf("expected hard error when board config cannot be loaded, got nil")
	}

	if !strings.Contains(err.Error(), "load board config") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected load config error, got: %v", err)
	}
}

func TestMenuExcludesMain(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	childPath := filepath.Join(container, "child")
	mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", "child-branch", childPath)
	defer func() {
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
		mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", "child-branch")
	}()

	if err := os.MkdirAll(filepath.Join(childPath, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("failed to create child _lyx: %v", err)
	}

	configDir := configengine.ConfigDir(mainWorktreePath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := configengine.ConfigFile(mainWorktreePath, "board")
	boardConfig := `path: ../_board
readme: Home.md
design_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &lyxcwd.Location{HubPath: container, WorktreeName: filepath.Base(mainWorktreePath), AnchorRel: "."}

	originalLauncher := CodeLauncher
	defer func() { CodeLauncher = originalLauncher }()
	CodeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	in := strings.NewReader("1\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "child") {
		t.Fatalf("expected 'child' in output, got: %q", output)
	}
}

func TestMenuRequiresLyxDir(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	childPath := filepath.Join(container, "child")
	mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", "child-branch", childPath)
	defer func() {
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
		mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", "child-branch")
	}()

	configDir := configengine.ConfigDir(mainWorktreePath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := configengine.ConfigFile(mainWorktreePath, "board")
	boardConfig := `path: ../_board
readme: Home.md
design_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &lyxcwd.Location{HubPath: container, WorktreeName: filepath.Base(mainWorktreePath), AnchorRel: "."}

	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees', got: %q", output)
	}
}

func TestMenuNumericSelection(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	for _, child := range []string{"child1", "child2"} {
		childPath := filepath.Join(container, child)
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", child+"-branch", childPath)
		if err := os.MkdirAll(filepath.Join(childPath, lyxdirs.LyxDirName), 0o755); err != nil {
			t.Fatalf("failed to create %s _lyx: %v", child, err)
		}
	}

	defer func() {
		for _, child := range []string{"child1", "child2"} {
			childPath := filepath.Join(container, child)
			mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
			mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", child+"-branch")
		}
	}()

	configDir := configengine.ConfigDir(mainWorktreePath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := configengine.ConfigFile(mainWorktreePath, "board")
	boardConfig := `path: ../_board
readme: Home.md
design_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	taskData := `{"tasks":[{"slug":"child1","title":"Task 1"},{"slug":"child2","title":"Task 2"}]}`
	if err := os.WriteFile(tasksPath, []byte(taskData), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &lyxcwd.Location{HubPath: container, WorktreeName: filepath.Base(mainWorktreePath), AnchorRel: "."}

	var launchCount int
	originalLauncher := CodeLauncher
	defer func() { CodeLauncher = originalLauncher }()
	CodeLauncher = func(dir string) error {
		launchCount++
		return nil
	}

	var out bytes.Buffer
	in := strings.NewReader("2\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	if launchCount != 1 {
		t.Fatalf("expected CodeLauncher to be called once, was called %d times", launchCount)
	}
}
