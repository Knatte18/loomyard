package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestSpawnGeneratesConfig tests that Spawn generates .vscode/ config.
func TestSpawnGeneratesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")

	// Create main and child worktree directories
	if err := os.MkdirAll(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}
	if err := os.MkdirAll(childWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
	}

	// Stub codeLauncher to record its argument without launching real VS Code
	launchArgs := []string{}
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		launchArgs = append(launchArgs, dir)
		return nil
	}

	// Call Spawn
	err := Spawn(layout, "child")
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Verify .vscode/settings.json was created
	settingsPath := filepath.Join(childWorktreePath, ".", ".vscode", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// Verify .vscode/tasks.json was created
	tasksPath := filepath.Join(childWorktreePath, ".", ".vscode", "tasks.json")
	if _, err := os.Stat(tasksPath); err != nil {
		t.Fatalf("tasks.json not created: %v", err)
	}

	// Verify codeLauncher was called with correct path
	expectedDir := filepath.Join(childWorktreePath, ".")
	if len(launchArgs) != 1 || launchArgs[0] != expectedDir {
		t.Fatalf("expected codeLauncher called with %q, got %v", expectedDir, launchArgs)
	}
}

// TestSpawnDoesNotClobber tests that a second Spawn does not clobber existing .vscode/ files.
func TestSpawnDoesNotClobber(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")

	// Create directories
	if err := os.MkdirAll(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}
	if err := os.MkdirAll(childWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
	}

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	// First Spawn
	if err := Spawn(layout, "child"); err != nil {
		t.Fatalf("first Spawn failed: %v", err)
	}

	// Read the original settings.json
	settingsPath := filepath.Join(childWorktreePath, ".", ".vscode", "settings.json")
	originalSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json after first Spawn: %v", err)
	}

	// Second Spawn (should not clobber)
	if err := Spawn(layout, "child"); err != nil {
		t.Fatalf("second Spawn failed: %v", err)
	}

	// Verify settings.json was not modified
	newSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json after second Spawn: %v", err)
	}

	if string(originalSettings) != string(newSettings) {
		t.Fatalf("settings.json was clobbered by second Spawn")
	}
}

// TestSpawnCallsCodeLauncher tests that Spawn invokes the launcher with correct path.
func TestSpawnCallsCodeLauncher(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")
	relpath := "subdir"

	// Create directories
	for _, p := range []string{mainWorktreePath, childWorktreePath} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      relpath,
	}

	// Stub codeLauncher to record its argument
	var launchedDir string
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error {
		launchedDir = dir
		return nil
	}

	// Call Spawn
	if err := Spawn(layout, "child"); err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Verify codeLauncher was called with worktreeDir/relpath
	expectedDir := filepath.Join(childWorktreePath, relpath)
	if launchedDir != expectedDir {
		t.Fatalf("expected codeLauncher called with %q, got %q", expectedDir, launchedDir)
	}
}

// TestSpawnColorSelection tests that Spawn picks a color via pickColor.
func TestSpawnColorSelection(t *testing.T) {
	tmpDir := t.TempDir()
	container := tmpDir
	mainWorktreePath := filepath.Join(container, "main")
	childWorktreePath := filepath.Join(container, "child")

	// Create directories
	if err := os.MkdirAll(mainWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}
	if err := os.MkdirAll(childWorktreePath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	layout := &paths.Layout{
		Container:    container,
		MainWorktree: mainWorktreePath,
		RelPath:      ".",
	}

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	// Call Spawn
	if err := Spawn(layout, "child"); err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Read settings.json and verify it has a color set
	settingsPath := filepath.Join(childWorktreePath, ".", ".vscode", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Verify color customization is present
	if _, ok := settings["workbench.colorCustomizations"]; !ok {
		t.Fatalf("missing workbench.colorCustomizations in settings.json")
	}
}
