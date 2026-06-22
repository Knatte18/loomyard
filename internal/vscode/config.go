// Package vscode generates VS Code configuration and manages VS Code-specific
// launch behavior for worktrees. It is responsible for config generation (settings.json
// and tasks.json), color-palette selection, and launching VS Code. The mill values
// (palette, settings keys, cmd /c code) are baked in — no external Python is read.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitignore"
)

// WriteConfig generates VS Code configuration files in a worktree,
// only if they don't already exist (never clobbering operator edits).
//
// It writes two files into <worktreeDir>/<relpath>/.vscode/:
//   - settings.json: workbench and window configuration with title-bar colors
//   - tasks.json: one "Start Claude" shell task with runOptions.runOn: "folderOpen"
//
// After writing, it registers .vscode/ in the managed .gitignore via gitignore.Ensure().
//
// Returns an error if I/O fails (but not if files already exist).
func WriteConfig(worktreeDir, relpath, slug, color string) error {
	dir := filepath.Join(worktreeDir, relpath)
	vscodePath := filepath.Join(dir, ".vscode")

	// Ensure .vscode directory exists
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		return err
	}

	// Write settings.json only if absent
	settingsPath := filepath.Join(vscodePath, "settings.json")
	if _, err := os.Stat(settingsPath); err == nil {
		// File exists; skip
	} else if os.IsNotExist(err) {
		// File doesn't exist; write it
		settings := map[string]any{
			"workbench.colorCustomizations": map[string]any{
				"titleBar.activeBackground":   color,
				"titleBar.activeForeground":   "#ffffff",
				"titleBar.inactiveBackground": color,
				"titleBar.inactiveForeground": "#ffffffaa",
			},
			"files.watcherExclude": map[string]any{
				"**/_lyx/**": true,
			},
			"window.title":                                 slug,
			"workbench.startupEditor":                      "none",
			"workbench.secondarySideBar.defaultVisibility": "hidden",
		}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			return err
		}
	} else {
		// Error checking file existence
		return err
	}

	// Write tasks.json only if absent
	tasksPath := filepath.Join(vscodePath, "tasks.json")
	if _, err := os.Stat(tasksPath); err == nil {
		// File exists; skip
	} else if os.IsNotExist(err) {
		// File doesn't exist; write it
		tasks := map[string]any{
			"version": "2.0.0",
			"tasks": []map[string]any{
				{
					"label":   "Start Claude",
					"type":    "shell",
					"command": "claude",
					"runOptions": map[string]any{
						"runOn": "folderOpen",
					},
					"presentation": map[string]any{
						"echo":   true,
						"reveal": "always",
						"panel":  "new",
					},
					"isBackground": false,
				},
			},
		}
		data, err := json.MarshalIndent(tasks, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(tasksPath, data, 0o644); err != nil {
			return err
		}
	} else {
		// Error checking file existence
		return err
	}

	// Register .vscode/ in .gitignore
	_, err := gitignore.Ensure(dir, ".vscode/")
	return err
}
