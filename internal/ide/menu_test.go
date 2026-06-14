package ide

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
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

	// Create main's _mhgo directory
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	return container, mainWorktreePath
}

// TestMenuHardErrorOnMissingBoard tests that Menu hard-errors when board HealthCheck fails.
func TestMenuHardErrorOnMissingBoard(t *testing.T) {
	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Call Menu without a board directory
	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err == nil {
		t.Fatalf("expected hard error when board is missing, got nil")
	}

	if !strings.Contains(err.Error(), "health check") {
		t.Fatalf("expected health check error, got: %v", err)
	}
}

// TestMenuExcludesMain tests that the main worktree is excluded from discovery.
func TestMenuExcludesMain(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a child worktree with _mhgo
	childPath := filepath.Join(container, "child")
	if err := os.Mkdir(childPath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(childPath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create child _mhgo: %v", err)
	}

	// Create board directory with tasks.json
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		return nil
	}

	// Simulate user selecting first worktree
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

// TestMenuRequiresMhgoDir tests that worktrees without _mhgo are excluded.
func TestMenuRequiresMhgoDir(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a child worktree WITHOUT _mhgo (should be excluded)
	childPath := filepath.Join(container, "child")
	if err := os.Mkdir(childPath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Create board directory with tasks.json
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
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

	// Create child1 and child2 with _mhgo
	for _, child := range []string{"child1", "child2"} {
		childPath := filepath.Join(container, child)
		if err := os.Mkdir(childPath, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", child, err)
		}
		if err := os.MkdirAll(filepath.Join(childPath, "_mhgo"), 0o755); err != nil {
			t.Fatalf("failed to create %s _mhgo: %v", child, err)
		}
	}

	// Create board directory with tasks.json
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	taskData := `{"tasks":[{"slug":"child1","title":"Task 1"},{"slug":"child2","title":"Task 2"}]}`
	if err := os.WriteFile(tasksPath, []byte(taskData), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Stub codeLauncher to track spawned slug
	var spawnedSlug string
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		spawnedSlug = filepath.Base(filepath.Dir(dir))
		return nil
	}

	// Simulate user selecting item 2 (child2)
	var out bytes.Buffer
	in := strings.NewReader("2\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	if spawnedSlug != "child2" {
		t.Fatalf("expected to spawn child2, got: %q", spawnedSlug)
	}
}

// TestMenuZeroWorktreeMessage tests that zero active worktrees prints the correct message.
func TestMenuZeroWorktreeMessage(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create board directory with tasks.json (no child worktrees)
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	tasksPath := filepath.Join(boardDir, "tasks.json")
	if err := os.WriteFile(tasksPath, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	var out bytes.Buffer
	in := io.Reader(strings.NewReader(""))

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees', got: %q", output)
	}
}
