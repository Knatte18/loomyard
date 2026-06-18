// color.go defines the title-bar color palette and the picker that assigns each
// worktree the first unused non-green color, scanning sibling worktrees' VS Code
// settings so two open worktrees never share a color. Green is reserved for main.

package ide

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/paths"
)

// ErrIDEUnsupported is returned when ide launch is attempted on an unsupported platform.
var ErrIDEUnsupported = errors.New("ide launch unsupported on this platform")

// Color palette (order matters; green is reserved for main).
var palette = []string{
	"#2d7d46", // green (reserved for main)
	"#7d2d6b", // purple
	"#2d4f7d", // blue
	"#7d5c2d", // yellow
	"#6b2d2d", // red
	"#2d6b6b", // cyan
	"#4a2d7d", // indigo
	"#7d462d", // orange
}

// mainColor is the reserved color for the main worktree.
var mainColor = "#2d7d46"

// pickColor selects an unused non-green color for a child worktree,
// scanning sibling .vscode/settings.json files for existing color assignments.
//
// Algorithm:
//   - Scan <l.Hub>/<dir>/<l.RelPath>/.vscode/settings.json for each sibling worktree
//   - Collect workbench.colorCustomizations.titleBar.activeBackground (lowercased)
//   - Skip the main worktree and any dir with unreadable settings
//   - Return the first palette color that is not mainColor and not in use
//   - If all non-green colors are used, return the first non-green (palette[1])
//   - If hub/dirs missing, return first non-green
func pickColor(l *paths.Layout) string {
	used := make(map[string]bool)

	// Try to read the hub directory
	entries, err := os.ReadDir(l.Hub)
	if err != nil {
		// Hub doesn't exist or unreadable; return first non-green
		return palette[1]
	}

	primeBase := filepath.Base(l.Prime)

	// Scan each sibling worktree for existing color assignments
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip the main worktree
		if entry.Name() == primeBase {
			continue
		}

		// Build path to .vscode/settings.json
		settingsPath := filepath.Join(
			l.Hub,
			entry.Name(),
			l.RelPath,
			".vscode",
			"settings.json",
		)

		// Try to read and parse settings.json
		content, err := os.ReadFile(settingsPath)
		if err != nil {
			// Unreadable or missing; skip this sibling
			continue
		}

		var settings map[string]any
		if err := json.Unmarshal(content, &settings); err != nil {
			// Invalid JSON; skip this sibling
			continue
		}

		// Extract titleBar.activeBackground color using flat dot-notation key
		if colorCustomizations, ok := settings["workbench.colorCustomizations"].(map[string]any); ok {
			if activeBackground, ok := colorCustomizations["titleBar.activeBackground"].(string); ok {
				used[strings.ToLower(activeBackground)] = true
			}
		}
	}

	// Find first unused non-green color
	for i := 1; i < len(palette); i++ {
		if !used[palette[i]] {
			return palette[i]
		}
	}

	// All non-green colors are used; return first non-green
	return palette[1]
}
