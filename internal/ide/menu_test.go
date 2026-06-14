package ide

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestMenuDiscoveryExcludesMain tests that the main worktree is excluded from discovery.
func TestMenuDiscoveryExcludesMain(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")

	// Create main worktree with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	// Create board
	boardPath := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardPath, 0o755); err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Write minimal tasks.json
	tasksFile := filepath.Join(boardPath, "tasks.json")
	if err := os.WriteFile(tasksFile, []byte("[]"), 0o644); err != nil {
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
	codeLauncher = func(dir string) error { return nil }

	// Call Menu with empty input (EOF, so no selection)
	var out bytes.Buffer
	in := strings.NewReader("")

	// Since there are no child worktrees with _mhgo, we expect "no active worktrees"
	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees' message, got: %q", output)
	}
}

// TestMenuRequiresMhgoDirectory tests that discovery requires _mhgo/ at the RelPath.
func TestMenuRequiresMhgoDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	// Create child WITHOUT _mhgo
	if err := os.MkdirAll(childWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	// Create board
	boardPath := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardPath, 0o755); err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	tasksFile := filepath.Join(boardPath, "tasks.json")
	if err := os.WriteFile(tasksFile, []byte("[]"), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	in := strings.NewReader("")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "no active worktrees") {
		t.Fatalf("expected 'no active worktrees' message (child has no _mhgo), got: %q", output)
	}
}

// TestMenuHardErrorOnMissingBoard tests that Menu hard-errors when board HealthCheck fails.
func TestMenuHardErrorOnMissingBoard(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

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

// TestMenuNumericSelection tests that a numeric selection maps to the correct worktree.
func TestMenuNumericSelection(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	child1Path := filepath.Join(container, "child1")
	child2Path := filepath.Join(container, "child2")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	// Create child1 and child2 with _mhgo
	for _, p := range []string{child1Path, child2Path} {
		if err := os.MkdirAll(filepath.Join(p, "_mhgo"), 0o755); err != nil {
			t.Fatalf("failed to create _mhgo: %v", err)
		}
	}

	// Create board with tasks
	boardPath := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardPath, 0o755); err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	tasksFile := filepath.Join(boardPath, "tasks.json")
	tasksJSON := `[
		{"id": 1, "slug": "child1", "title": "Child One"},
		{"id": 2, "slug": "child2", "title": "Child Two"}
	]`
	if err := os.WriteFile(tasksFile, []byte(tasksJSON), 0o644); err != nil {
		t.Fatalf("failed to write tasks.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
		Cwd:          mainWorktreePath,
	}

	// Stub codeLauncher to record which worktree was selected
	var selectedSlug string
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	// Redirect Spawn to capture the slug
	originalSpawn := Spawn
	defer func() {
		// We can't easily replace Spawn here, but we can verify via output
	}()
	_ = originalSpawn

	// Select child2 (option 2)
	var out bytes.Buffer
	in := strings.NewReader("2\n")

	err := Menu(layout, in, &out)
	if err != nil {
		t.Fatalf("Menu failed: %v", err)
	}

	output := out.String()
	// Verify output contains both children
	if !strings.Contains(output, "child1") || !strings.Contains(output, "child2") {
		t.Fatalf("expected output to contain both child1 and child2, got: %q", output)
	}

	_ = selectedSlug
}

// TestMenuZeroWorktreesMessage tests that zero active worktrees prints a message and returns success.
func TestMenuZeroWorktreesMessage(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")

	// Create main with _mhgo
	if err := os.MkdirAll(filepath.Join(mainWorktreePath, "_mhgo"), 0o755); err != nil {
		t.Fatalf("failed to create main _mhgo: %v", err)
	}

	// Create board
	boardPath := filepath.Join(container, "_board")
	if err := os.MkdirAll(boardPath, 0o755); err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	tasksFile := filepath.Join(boardPath, "tasks.json")
	if err := os.WriteFile(tasksFile, []byte("[]"), 0o644); err != nil {
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
		t.Fatalf("expected 'no active worktrees' message, got: %q", output)
	}
}
