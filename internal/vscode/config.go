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
// lyxPath and claudePath are the resolved absolute binary paths stamped into the generated
// folderOpen launch chain; WriteConfig owns the bare-name fallback for either one: an empty
// lyxPath becomes "lyx" and an empty claudePath becomes "claude", so a resolution failure upstream
// still produces a runnable (PATH-dependent) task file rather than a broken one.
// Returns an error if I/O fails (but not if files already exist).
func WriteConfig(worktreeDir, relpath, slug, color, lyxPath, claudePath string) error {
	if lyxPath == "" {
		lyxPath = "lyx"
	}
	if claudePath == "" {
		claudePath = "claude"
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
		// dependsOrder: "sequence" is relied on for ordering alone. VS Code's runner has
		// historically run the next dependent task regardless of the previous one's exit
		// code, and the chain is safe either way: AddStrand pre-flights requireSessionLocked
		// and attach pre-flights Status, so a failed "reed up" ends with no strand, no pane,
		// and no bare claude. No compensating guard of the runner's behaviour is added here.
		tasks := map[string]any{
			"version": "2.0.0",
			"tasks": []map[string]any{
				{
					"label":   "reed up",
					"type":    "shell",
					"command": lyxPath,
					"args":    []string{"reed", "up"},
					"presentation": map[string]any{
						"reveal": "silent",
						"panel":  "shared",
						"echo":   true,
						"focus":  false,
					},
				},
				{
					"label":   "reed add claude",
					"type":    "shell",
					"command": lyxPath,
					"args": []string{
						"reed", "add",
						"--if-absent",
						"--cmd", claudePath,
						"--name", "claude",
						"--focus",
					},
					"presentation": map[string]any{
						"reveal": "silent",
						"panel":  "shared",
						"echo":   true,
						"focus":  false,
					},
				},
				{
					"label":   "reed attach",
					"type":    "shell",
					"command": lyxPath,
					"args":    []string{"reed", "attach"},
					"presentation": map[string]any{
						"reveal": "always",
						"panel":  "new",
						"echo":   true,
						"focus":  true,
					},
				},
				{
					"label":        "Start Claude",
					"dependsOn":    []string{"reed up", "reed add claude", "reed attach"},
					"dependsOrder": "sequence",
					"runOptions": map[string]any{
						"runOn": "folderOpen",
					},
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
