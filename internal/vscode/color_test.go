// color_test.go covers the palette picker, including scanning sibling worktrees'
// VS Code settings for colors already in use.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestPickColor covers the palette picker, ensuring it selects unused non-green colors
// and respects the RelPath directory structure when scanning sibling worktrees.
func TestPickColor(t *testing.T) {
	tests := []struct {
		name      string
		RelPath   string
		setupFunc func(tmpDir, mainPath string)
		wantColor string
		wantNot   string // color(s) that should NOT be returned
	}{
		{
			name:    "TestPickColorNeverReturnsGreen",
			RelPath: ".",
			setupFunc: func(tmpDir, mainPath string) {
				// Just create main and child; no colors in use
				childPath := filepath.Join(tmpDir, "child")
				if err := os.MkdirAll(childPath, 0o755); err != nil {
					t.Fatalf("failed to create child: %v", err)
				}
			},
			wantColor: "",
			wantNot:   mainColor, // should not be green
		},
		{
			name:    "TestPickColorFirstUnusedNonGreen",
			RelPath: ".",
			setupFunc: func(tmpDir, mainPath string) {
				// Create child1 with purple in use
				child1Path := filepath.Join(tmpDir, "child1")
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
			},
			wantColor: "#2d4f7d", // blue (first unused non-green)
			wantNot:   "",
		},
		{
			name:    "TestPickColorWrapAroundAllUsed",
			RelPath: ".vscode",
			setupFunc: func(tmpDir, mainPath string) {
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
			},
			wantColor: palette[1], // wrap around to first non-green
			wantNot:   "",
		},
		{
			name:    "TestPickColorIgnoresUnreadable",
			RelPath: ".vscode",
			setupFunc: func(tmpDir, mainPath string) {
				// Create child with missing settings.json
				childPath := filepath.Join(tmpDir, "child")
				childVscode := filepath.Join(childPath, ".vscode")
				if err := os.MkdirAll(childVscode, 0o755); err != nil {
					t.Fatalf("failed to create .vscode: %v", err)
				}
			},
			wantColor: palette[1], // first non-green since nothing in use
			wantNot:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			mainPath := filepath.Join(tmpDir, "main")

			if err := os.MkdirAll(mainPath, 0o755); err != nil {
				t.Fatalf("failed to create main: %v", err)
			}

			tt.setupFunc(tmpDir, mainPath)

			layout := &paths.Layout{
				Hub:     tmpDir,
				Prime:   mainPath,
				RelPath: tt.RelPath,
			}

			color := PickColor(layout)

			// Check wantNot constraint
			if tt.wantNot != "" && color == tt.wantNot {
				t.Errorf("pickColor() = %s; should not return %s", color, tt.wantNot)
			}

			// Check wantColor if specified
			if tt.wantColor != "" && color != tt.wantColor {
				t.Errorf("pickColor() = %s; want %s", color, tt.wantColor)
			}
		})
	}
}
