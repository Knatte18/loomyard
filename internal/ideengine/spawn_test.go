// spawn_test.go covers the end-to-end spawn flow with a stubbed code launcher.

package ideengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestSpawn covers end-to-end spawn flow: config generation, launcher invocation,
// and no-clobber behavior. The shared CodeLauncher global keeps this test serial.
func TestSpawn(t *testing.T) {
	tests := []struct {
		name         string
		relpath      string
		checkClobber bool // if true, run a second Spawn to verify no-clobber
	}{
		{
			name:         "TestSpawnGeneratesConfig",
			relpath:      ".",
			checkClobber: false,
		},
		{
			name:         "TestSpawnCallsCodeLauncher",
			relpath:      "subdir",
			checkClobber: false,
		},
		{
			name:         "TestSpawnDoesNotClobber",
			relpath:      ".",
			checkClobber: true,
		},
	}

	// Stub CodeLauncher once for all subtests
	originalLauncher := CodeLauncher
	defer func() { CodeLauncher = originalLauncher }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			container := tmpDir
			mainWorktreePath := filepath.Join(container, "main")
			childWorktreePath := filepath.Join(container, "child")

			// Create directories
			for _, p := range []string{mainWorktreePath, childWorktreePath} {
				if err := os.MkdirAll(p, 0o755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
			}

			layout := &paths.Layout{
				Hub:     container,
				Prime:   mainWorktreePath,
				RelPath: tt.relpath,
			}

			// Stub CodeLauncher to record its argument
			var launchedDir string
			CodeLauncher = func(dir string) error {
				launchedDir = dir
				return nil
			}

			// Call Spawn
			err := Spawn(layout, "child")
			if err != nil {
				t.Fatalf("Spawn failed: %v", err)
			}

			// Verify .vscode/settings.json was created
			settingsPath := filepath.Join(childWorktreePath, tt.relpath, ".vscode", "settings.json")
			if _, err := os.Stat(settingsPath); err != nil {
				t.Fatalf("settings.json not created: %v", err)
			}

			// Verify .vscode/tasks.json was created
			tasksPath := filepath.Join(childWorktreePath, tt.relpath, ".vscode", "tasks.json")
			if _, err := os.Stat(tasksPath); err != nil {
				t.Fatalf("tasks.json not created: %v", err)
			}

			// Verify CodeLauncher was called with correct path (worktreeDir/relpath)
			expectedDir := filepath.Join(childWorktreePath, tt.relpath)
			if launchedDir != expectedDir {
				t.Errorf("CodeLauncher called with %q; want %q", launchedDir, expectedDir)
			}

			// For TestSpawnDoesNotClobber, run a second Spawn and verify no modification
			if tt.checkClobber {
				originalSettings, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatalf("failed to read settings.json after first Spawn: %v", err)
				}

				// Second Spawn
				if err := Spawn(layout, "child"); err != nil {
					t.Fatalf("second Spawn failed: %v", err)
				}

				newSettings, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatalf("failed to read settings.json after second Spawn: %v", err)
				}

				if string(originalSettings) != string(newSettings) {
					t.Errorf("settings.json was modified by second Spawn")
				}
			}
		})
	}
}
