// Package vscode generates VS Code configuration and manages VS Code-specific launch behavior for
// worktrees.
// It is responsible for config generation (settings.json and tasks.json), color-palette selection, and launching VS Code.
// It also seeds the marked block of key bindings into the user's keybindings.json (see SeedKeybindings):
// the Alt-arrow keys the integrated terminal forwards to reed's tmux bindings, and ctrl+alt+a, which runs the reed attach task.
// The mill values (palette, settings keys, cmd /c code) are baked in — no external Python is read.
//
// Limitation: BuildWorkspace splices the prime's settings verbatim into the hub workspace file at <hub>/<RepoName>.code-workspace, so a relative path value (starting with ./ or ../) resolves against the hub directory, no longer against the prime's own folder.
// RelativeSettingKeys names such keys so the caller can warn; no setting is rewritten.

package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteConfig generates VS Code configuration files in a worktree.
// settings.json is written only if it doesn't already exist, never clobbering operator edits.
// tasks.json is always written, overwriting any existing file, and holds one "reed attach" task, run on demand or by its seeded key binding;
// no task runs on folder open, so opening the folder starts no session and no agent.
// Neither file touches .gitignore; the caller keeps .vscode/ out of git through info/exclude.
// lymPath is the resolved absolute lym path stamped into the task;
// an empty lymPath becomes "lym", so a resolution failure upstream still produces a runnable (PATH-dependent) task file rather than a broken one.
// Returns an error if I/O fails (but not if settings.json already exists).
func WriteConfig(worktreeDir, relpath, slug, color, lymPath string) error {
	if lymPath == "" {
		lymPath = "lym"
	}

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
				"**/_lym/**": true,
			},
			"window.title":                                 slug,
			"workbench.startupEditor":                      "none",
			"workbench.secondarySideBar.defaultVisibility": "hidden",
			// The reed attach terminal is the window's main content, so it opens as an editor tab, not in the panel.
			"terminal.integrated.defaultLocation": "editor",
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

	return writeAttachTask(filepath.Join(vscodePath, "tasks.json"), lymPath)
}

// attachTaskLabel names the reed attach task; the seeded key binding finds the task by this label.
const attachTaskLabel = "reed attach"

// writeAttachTask writes a tasks.json holding the single on-demand "reed attach" task, overwriting any existing file.
// The task carries no runOptions, so VS Code runs it only from "Tasks: Run Task" or its seeded key binding, never on folder open.
func writeAttachTask(tasksPath, lymPath string) error {
	tasks := map[string]any{
		"version": "2.0.0",
		"tasks": []map[string]any{
			{
				"label":   attachTaskLabel,
				"type":    "shell",
				"command": lymPath,
				"args":    []string{"reed", "attach"},
				"presentation": map[string]any{
					"reveal": "always",
					"panel":  "new",
					"echo":   true,
					"focus":  true,
				},
			},
		},
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tasksPath, data, 0o644)
}
