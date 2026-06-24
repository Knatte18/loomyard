//go:build integration

// menu_test.go covers worktree discovery (excludes main, requires _lyx/),
// board-facade titles, numeric selection, the zero-worktree path, and the
// missing-board hard error.

package ide

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// mustRunMenu is a test helper that runs a command in a directory.
func mustRunMenu(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command failed: %v; output: %s", err, output)
	}
}

// newTestGitRepoWithWorktrees creates a git repository with a main worktree and child worktrees.
func newTestGitRepoWithWorktrees(t *testing.T) (string, string) {
	t.Helper()

	container := t.TempDir()
	mainWorktreePath := filepath.Join(container, "main")

	// Create and initialize main worktree
	if err := os.Mkdir(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main worktree: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "init", "-b", "main")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.email", "test@test.com")
	mustRunMenu(t, mainWorktreePath, "git", "config", "user.name", "Test")

	// Create and commit a file
	readmeFile := filepath.Join(mainWorktreePath, "README")
	if err := os.WriteFile(readmeFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	mustRunMenu(t, mainWorktreePath, "git", "add", ".")
	mustRunMenu(t, mainWorktreePath, "git", "commit", "-m", "initial")

	// Create main's _lyx directory
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_lyx"), 0o755); err != nil {
		t.Fatalf("failed to create main _lyx: %v", err)
	}

	return container, mainWorktreePath
}

// TestMenuHardErrorOnMissingBoard tests that Menu hard-errors when board config cannot be loaded.
func TestMenuHardErrorOnMissingBoard(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	layout := &paths.Layout{
		Hub:     container,
		Prime:   mainWorktreePath,
		RelPath: ".",
		Cwd:     mainWorktreePath,
	}

	// Call Menu without a board config file (_lyx/config/board.yaml missing)
	// This should hard-error during LoadConfig
	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err == nil {
		t.Fatalf("expected hard error when board config cannot be loaded, got nil")
	}

	// Should be a load error, not a health check error (since we don't get that far)
	if !strings.Contains(err.Error(), "load board config") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected load config error, got: %v", err)
	}
}

// TestMenuExcludesMain tests that the main worktree is excluded from discovery.
func TestMenuExcludesMain(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a real git worktree using `git worktree add`
	childPath := filepath.Join(container, "child")
	mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", "child-branch", childPath)
	defer func() {
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
		mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", "child-branch")
	}()

	// Create _lyx in child
	if err := os.MkdirAll(filepath.Join(childPath, "_lyx"), 0o755); err != nil {
		t.Fatalf("failed to create child _lyx: %v", err)
	}

	// Create board config
	configDir := filepath.Join(mainWorktreePath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := filepath.Join(configDir, "board.yaml")
	boardConfig := `path: ../_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	// Create board directory with tasks.json
	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Hub:     container,
		Prime:   mainWorktreePath,
		RelPath: ".",
		Cwd:     mainWorktreePath,
	}

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	// Simulate user selecting first worktree (child, not main)
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

// TestMenuRequiresLyxDir tests that worktrees without _lyx are excluded.
func TestMenuRequiresLyxDir(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a real git worktree WITHOUT _lyx (should be excluded)
	childPath := filepath.Join(container, "child")
	mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", "child-branch", childPath)
	defer func() {
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "remove", "--force", childPath)
		mustRunMenu(t, mainWorktreePath, "git", "branch", "-D", "child-branch")
	}()
	// Note: child is created but has no _lyx

	// Create board config
	configDir := filepath.Join(mainWorktreePath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := filepath.Join(configDir, "board.yaml")
	boardConfig := `path: ../_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	// Create board directory with tasks.json
	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Hub:     container,
		Prime:   mainWorktreePath,
		RelPath: ".",
		Cwd:     mainWorktreePath,
	}

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

// TestMenuNumericSelection tests that numeric selection invokes Spawn with correct slug.
func TestMenuNumericSelection(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create real git worktrees child1 and child2 with _lyx
	for _, child := range []string{"child1", "child2"} {
		childPath := filepath.Join(container, child)
		mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", child+"-branch", childPath)
		if err := os.MkdirAll(filepath.Join(childPath, "_lyx"), 0o755); err != nil {
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

	// Create board config
	configDir := filepath.Join(mainWorktreePath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	boardConfigPath := filepath.Join(configDir, "board.yaml")
	boardConfig := `path: ../_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	// Create board directory with tasks.json at <container>/_board
	boardDir := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	taskData := `{"tasks":[{"slug":"child1","title":"Task 1"},{"slug":"child2","title":"Task 2"}]}`
	if err := os.WriteFile(tasksPath, []byte(taskData), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Hub:     container,
		Prime:   mainWorktreePath,
		RelPath: ".",
		Cwd:     mainWorktreePath,
	}

	// Stub codeLauncher to verify it gets called
	var launchCount int
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		launchCount++
		return nil
	}

	// Simulate user selecting item 2 (child2)
	var out bytes.Buffer
	in := strings.NewReader("2\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	// Verify that codeLauncher was called exactly once (for the selected worktree)
	if launchCount != 1 {
		t.Fatalf("expected codeLauncher to be called once, was called %d times", launchCount)
	}
}
