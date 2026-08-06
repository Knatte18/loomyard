// Package vscode generates VS Code configuration and manages VS Code-specific launch behavior for
// worktrees.
// It is responsible for config generation (settings.json and tasks.json), color-palette selection,
// and launching VS Code.
// The mill values (palette, settings keys, cmd /c code) are baked in — no external Python is read.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitignore"
)

// WriteConfig generates VS Code configuration files in a worktree, only if they don't already exist
// (never clobbering operator edits).
// Returns an error if I/O fails (but not if files already exist).
func WriteConfig(worktreeDir, relpath, slug, color string) error {
	dir := filepath.Join(worktreeDir, relpath)
	vscodePath := filepath.Join(dir, ".vscode")

	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		return err
	}

	settingsPath := filepath.Join(vscodePath, "settings.json")
	if _, err := os.Stat(settingsPath); err == nil {
	} else if os.IsNotExist(err) {
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
		return err
	}

	tasksPath := filepath.Join(vscodePath, "tasks.json")
	if _, err := os.Stat(tasksPath); err == nil {
	} else if os.IsNotExist(err) {
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
		return err
	}

	_, err := gitignore.Ensure(dir, ".vscode/")
	return err
}
