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
	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a child worktree (not a real git worktree, just a directory structure for testing)
	childPath := filepath.Join(container, "child")
	if err := os.Mkdir(childPath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Create _mhgo in child
	if err := os.MkdirAll(filepath.Join(childPath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create child _mhgo: %v", err)
	}

	// Create a minimal board config
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	boardConfigPath := filepath.Join(boardDir, "board.json")
	boardConfig := `{"tasks":[]}`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board config: %v", err)
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

	// Simulate user selecting the first (and only) worktree
	var out bytes.Buffer
	in := strings.NewReader("1\n")

	// This should succeed with child being the only option
	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v; output: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "child") {
		t.Fatalf("expected 'child' in output, got: %q", output)
	}

	// Main should NOT be listed
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "main") && strings.HasPrefix(line, "1)") {
			t.Fatalf("main worktree should not be listed, got: %q", line)
		}
	}
}

// TestMenuRequiresMhgoDir tests that worktrees without _mhgo are excluded.
func TestMenuRequiresMhgoDir(t *testing.T) {
	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a child worktree without _mhgo
	childPath := filepath.Join(container, "child")
	if err := os.Mkdir(childPath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Create a minimal board config
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	boardConfigPath := filepath.Join(boardDir, "board.json")
	boardConfig := `{"tasks":[]}`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board config: %v", err)
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
	// Should print "no active worktrees" because child has no _mhgo
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees' message, got: %q", output)
	}
}

// TestMenuNumericSelection tests that numeric selection invokes Spawn with correct slug.
func TestMenuNumericSelection(t *testing.T) {
	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create child1 with _mhgo
	child1Path := filepath.Join(container, "child1")
	if err := os.Mkdir(child1Path, 0o755); err != nil {
		t.Fatalf("failed to create child1: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(child1Path, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create child1 _mhgo: %v", err)
	}

	// Create child2 with _mhgo
	child2Path := filepath.Join(container, "child2")
	if err := os.Mkdir(child2Path, 0o755); err != nil {
		t.Fatalf("failed to create child2: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(child2Path, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create child2 _mhgo: %v", err)
	}

	// Create a minimal board config with tasks
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	boardConfigPath := filepath.Join(boardDir, "board.json")
	boardConfig := `{"tasks":[{"slug":"child1","title":"Task 1"},{"slug":"child2","title":"Task 2"}]}`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board config: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Stub codeLauncher to track which was opened
	var spawnedSlug string
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		// Extract slug from the path
		spawnedSlug = filepath.Base(filepath.Dir(dir))
		return nil
	}

	// Simulate user selecting item 2 (child2)
	var out bytes.Buffer
	in := strings.NewReader("2\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v; output: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "child1") {
		t.Fatalf("expected 'child1' in output, got: %q", output)
	}
	if !strings.Contains(output, "child2") {
		t.Fatalf("expected 'child2' in output, got: %q", output)
	}

	// Verify the correct worktree was selected
	if spawnedSlug != "child2" {
		t.Fatalf("expected to spawn child2, got: %q", spawnedSlug)
	}
}

// TestMenuZeroWorktreeMessage tests that zero active worktrees prints the correct message.
func TestMenuZeroWorktreeMessage(t *testing.T) {
	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)

	// Create a minimal board config
	boardDir := filepath.Join(mainWorktreePath, "_mhgo", "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("failed to create board dir: %v", err)
	}

	boardConfigPath := filepath.Join(boardDir, "board.json")
	boardConfig := `{"tasks":[]}`
	if err := os.WriteFile(boardConfigPath, []byte(boardConfig), 0o644); err != nil {
		t.Fatalf("failed to write board config: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	var out bytes.Buffer
	// EOF on first read (no worktrees, so menu won't ask for input)
	in := io.Reader(strings.NewReader(""))

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees' message, got: %q", output)
	}
}
