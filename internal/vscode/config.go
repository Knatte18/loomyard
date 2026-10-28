// Package vscode generates VS Code configuration and manages VS Code-specific launch behavior for
// worktrees.
// It is responsible for config generation (settings.json and tasks.json), color-palette selection, and launching VS Code.
//
// A window is coloured in the two areas that keep their colour in an unfocused window:
// the title bar takes the project's colour, the prime's green or its beam's colour, and the activity bar the worktree's own colour.
// A task's activity-bar colour is any palette colour but its project's, avoiding its siblings', the worktrees whose title bar holds the same project colour, while one is free.
// Text on either bar is black or white by the background's luminance.
// No status-bar, border or tab colour is written: the status bar loses its background in an unfocused window, the active border is not drawn, and tab colours follow the selected tab.
// lym owns those keys and window.title in a worktree's settings.json and rewrites them at every seed; every other key is the operator's.
// It also seeds the marked block of key bindings into the user's keybindings.json (see SeedKeybindings):
// the Alt-arrow keys the integrated terminal forwards to reed's tmux bindings, and ctrl+alt+a, which runs the reed attach task.
// The mill values (palette, settings keys, cmd /c code) are baked in — no external Python is read.
//
// Limitation: BuildWorkspace splices the prime's settings verbatim into the hub workspace file at <hub>/<RepoName>.code-workspace, so a relative path value (starting with ./ or ../) resolves against the hub directory, no longer against the prime's own folder.
// RelativeSettingKeys names such keys so the caller can warn; no setting is rewritten.

package vscode

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Settings keys lym owns in a worktree's settings.json.
const (
	colorCustomizationsKey        = "workbench.colorCustomizations"
	windowTitleKey                = "window.title"
	titleBarActiveBackground      = "titleBar.activeBackground"
	titleBarInactiveBackground    = "titleBar.inactiveBackground"
	titleBarActiveForeground      = "titleBar.activeForeground"
	titleBarInactiveForeground    = "titleBar.inactiveForeground"
	activityBarBackground         = "activityBar.background"
	activityBarForeground         = "activityBar.foreground"
	activityBarInactiveForeground = "activityBar.inactiveForeground"
	inactiveForegroundAlphaHex    = "aa"
)

// Look is how a worktree's VS Code window shows itself: its window title, the project colour its title bar takes and the worktree's own colour its activity bar takes, each a #rrggbb colour.
type Look struct {
	Title         string
	ProjectColor  string
	WorktreeColor string
}

// WriteConfig writes a worktree's VS Code configuration under <worktreeDir>/<relpath>/.vscode.
// A missing settings.json is created with look and lym's defaults.
// In an existing one only the keys lym owns are set from look: window.title and the title-bar and activity-bar keys of workbench.colorCustomizations;
// every other key is kept, though comments are not, and a file that does not decode as JSONC is left untouched with a warning.
// tasks.json is always written, overwriting any existing file, and holds one "reed attach" task, run on demand or by its seeded key binding;
// no task runs on folder open, so opening the folder starts no session and no agent.
// Neither file touches .gitignore; the caller keeps .vscode/ out of git through info/exclude.
// lymPath is the resolved absolute lym path stamped into the task;
// an empty lymPath becomes "lym", so a resolution failure upstream still produces a runnable (PATH-dependent) task file rather than a broken one.
// Returns an error if I/O fails.
func WriteConfig(worktreeDir, relpath string, look Look, lymPath string) error {
	if lymPath == "" {
		lymPath = "lym"
	}

	vscodePath := filepath.Join(worktreeDir, relpath, ".vscode")
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		return err
	}
	if err := writeSettings(filepath.Join(vscodePath, "settings.json"), look); err != nil {
		return err
	}
	return writeAttachTask(filepath.Join(vscodePath, "tasks.json"), lymPath)
}

// writeSettings creates settingsPath with lym's defaults and look, or sets look's keys in the existing file, keeping its other keys.
// An existing file that does not decode as a JSONC object is logged and left untouched.
func writeSettings(settingsPath string, look Look) error {
	content, err := os.ReadFile(settingsPath)
	var settings map[string]any
	switch {
	case errors.Is(err, fs.ErrNotExist):
		settings = map[string]any{
			"files.watcherExclude": map[string]any{
				"**/_lym/**": true,
			},
			"workbench.startupEditor":                      "none",
			"workbench.secondarySideBar.defaultVisibility": "hidden",
			// The reed attach terminal is the window's main content, so it opens as an editor tab, not in the panel.
			"terminal.integrated.defaultLocation": "editor",
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(stripJSONC(content), &settings); err != nil || settings == nil {
			logger.Warn("vscode: settings.json is not a JSON object; leaving its title and colours as they are", "path", settingsPath, "error", err)
			return nil
		}
	}

	customizations, _ := settings[colorCustomizationsKey].(map[string]any)
	if customizations == nil {
		customizations = map[string]any{}
	}
	titleForeground := Foreground(look.ProjectColor)
	activityForeground := Foreground(look.WorktreeColor)
	customizations[titleBarActiveBackground] = look.ProjectColor
	customizations[titleBarInactiveBackground] = look.ProjectColor
	customizations[titleBarActiveForeground] = titleForeground
	customizations[titleBarInactiveForeground] = titleForeground + inactiveForegroundAlphaHex
	customizations[activityBarBackground] = look.WorktreeColor
	customizations[activityBarForeground] = activityForeground
	customizations[activityBarInactiveForeground] = activityForeground + inactiveForegroundAlphaHex
	settings[colorCustomizationsKey] = customizations
	settings[windowTitleKey] = look.Title

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if string(data) == string(content) {
		return nil
	}
	return os.WriteFile(settingsPath, data, 0o644)
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
