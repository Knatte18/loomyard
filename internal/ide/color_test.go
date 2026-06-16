// color_test.go covers the palette picker, including scanning sibling worktrees'
// VS Code settings for colors already in use.

package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestPickColorNeverReturnsGreen tests that green is never returned for a child worktree.
func TestPickColorNeverReturnsGreen(t *testing.T) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main")
	childPath := filepath.Join(tmpDir, "child")

	// Create main and child worktree directories
	if err := os.MkdirAll(mainPath, 0o755); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}
	if err := os.MkdirAll(childPath, 0o755); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	layout := &paths.Layout{
		Container:    tmpDir,
		MainWorktree: mainPath,
		RelPath:      ".",
	}

	color := pickColor(layout)
	if color == mainColor {
		t.Fatalf("pickColor returned green (#2d7d46) for a child worktree; got %s", color)
	}
}

// TestPickColorFirstUnusedNonGreen tests that the first unused non-green color is chosen.
func TestPickColorFirstUnusedNonGreen(t *testing.T) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main")
	child1Path := filepath.Join(tmpDir, "child1")

	// Create directories
	for _, p := range []string{mainPath, child1Path} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	// Write a settings.json in child1 with purple color
	// The settings path must be <container>/<dir>/<relpath>/.vscode/settings.json
	// So for relpath=".", it's <container>/child1/.vscode/settings.json
	child1Settings := filepath.Join(child1Path, ".vscode")
	if err := os.MkdirAll(child1Settings, 0o755); err != nil {
		t.Fatalf("failed to create .vscode: %v", err)
	}
	settingsFile := filepath.Join(child1Settings, "settings.json")
	settings := map[string]any{
		"workbench.colorCustomizations": map[string]any{
			"titleBar.activeBackground": "#7d2d6b", // purple
		},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(settingsFile, data, 0o644); err != nil {
		t.Fatalf("failed to write settings.json: %v", err)
	}

	layout := &paths.Layout{
		Container:    tmpDir,
		MainWorktree: mainPath,
		RelPath:      ".",
	}

	color := pickColor(layout)
	// Should NOT return purple since it's in use, should return next non-green
	if color == "#7d2d6b" {
		t.Fatalf("pickColor returned purple which is in use; got %s", color)
	}
	if color == mainColor {
		t.Fatalf("pickColor returned green; got %s", color)
	}
	// We expect blue (#2d4f7d) since purple is taken
	if color != "#2d4f7d" {
		t.Fatalf("expected blue #2d4f7d (first unused non-green), got %s", color)
	}
}

// TestPickColorWrapAroundAllUsed tests wrap-around returns first non-green when all are used.
func TestPickColorWrapAroundAllUsed(t *testing.T) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main")

	if err := os.MkdirAll(mainPath, 0o755); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}

	// Create children with all non-green colors in use
	for i, color := range palette[1:] {
		childPath := filepath.Join(tmpDir, "child"+string(rune('0'+i)))
		childSettings := filepath.Join(childPath, ".vscode")

		if err := os.MkdirAll(childSettings, 0o755); err != nil {
			t.Fatalf("failed to create .vscode: %v", err)
		}

		settings := map[string]any{
			"workbench.colorCustomizations": map[string]any{
				"titleBar.activeBackground": color,
			},
		}
		data, _ := json.Marshal(settings)
		settingsFile := filepath.Join(childSettings, "settings.json")
		if err := os.WriteFile(settingsFile, data, 0o644); err != nil {
			t.Fatalf("failed to write settings.json: %v", err)
		}
	}

	layout := &paths.Layout{
		Container:    tmpDir,
		MainWorktree: mainPath,
		RelPath:      ".vscode",
	}

	color := pickColor(layout)
	// Should wrap around to first non-green (palette[1])
	if color != palette[1] {
		t.Fatalf("expected wrap-around to first non-green %s, got %s", palette[1], color)
	}
}

// TestPickColorIgnoresUnreadable tests that unreadable/missing settings are ignored.
func TestPickColorIgnoresUnreadable(t *testing.T) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main")
	childPath := filepath.Join(tmpDir, "child")

	// Create directories
	for _, p := range []string{mainPath, childPath} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	// Create a child with missing .vscode/settings.json
	childVscode := filepath.Join(childPath, ".vscode")
	if err := os.MkdirAll(childVscode, 0o755); err != nil {
		t.Fatalf("failed to create .vscode: %v", err)
	}

	layout := &paths.Layout{
		Container:    tmpDir,
		MainWorktree: mainPath,
		RelPath:      ".vscode",
	}

	color := pickColor(layout)
	// Should return first non-green (purple) since nothing is in use
	if color != palette[1] {
		t.Fatalf("expected first non-green %s, got %s", palette[1], color)
	}
}
