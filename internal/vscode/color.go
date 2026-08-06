// color.go defines the title-bar color palette and the picker that assigns each worktree the first unused non-green color, scanning sibling worktrees' VS Code settings so two open worktrees never share a color.
// Green is reserved for main.

package vscode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ErrUnsupported is returned when vscode launch is attempted on an unsupported platform.
var ErrUnsupported = errors.New("vscode launch unsupported on this platform")

// Color palette (order matters; green is reserved for main).
var palette = []string{
	"#2d7d46",
	"#7d2d6b",
	"#2d4f7d",
	"#7d5c2d",
	"#6b2d2d",
	"#2d6b6b",
	"#4a2d7d",
	"#7d462d",
}

// mainColor is the reserved color for the main worktree.
var mainColor = "#2d7d46"

// PickColor selects an unused non-green color for a child worktree, scanning sibling .vscode/settings.json files for existing assignments.
// Returns the first unused non-green palette color.
// primeName is the main worktree's base name (the caller sources it via fabricengine.PrimeName(l), never resolved here — pulling the entire fabric engine into a colour picker would reintroduce a `git worktree list` subprocess per call);
// the prime-skip step is omitted when primeName is empty.
func PickColor(l *lyxcwd.Location, primeName string) string {
	used := make(map[string]bool)

	entries, err := os.ReadDir(l.HubPath)
	if err != nil {
		// Hub doesn't exist or unreadable; return first non-green
		return palette[1]
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if primeName != "" && entry.Name() == primeName {
			continue
		}

		settingsPath := filepath.Join(
			l.HubPath,
			entry.Name(),
			l.AnchorRel,
			".vscode",
			"settings.json",
		)

		content, err := os.ReadFile(settingsPath)
		if err != nil {
			// Unreadable or missing; skip this sibling
			continue
		}

		var settings map[string]any
		if err := json.Unmarshal(content, &settings); err != nil {
			continue
		}

		if colorCustomizations, ok := settings["workbench.colorCustomizations"].(map[string]any); ok {
			if activeBackground, ok := colorCustomizations["titleBar.activeBackground"].(string); ok {
				used[strings.ToLower(activeBackground)] = true
			}
		}
	}

	for i := 1; i < len(palette); i++ {
		if !used[palette[i]] {
			return palette[i]
		}
	}

	// All non-green colors are used; return first non-green
	return palette[1]
}
