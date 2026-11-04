// Package vscode generates VS Code configuration and manages VS Code-specific launch behavior for
// worktrees.
// It is responsible for config generation (settings.json and tasks.json), color-palette selection, and launching VS Code.
//
// A window is coloured in the two areas that keep their colour in an unfocused window:
// the title bar takes the project's colour, the prime's green or its beam's colour, and the activity bar the worktree's own colour.
// A task's activity-bar colour is any palette colour but its project's, avoiding its siblings', the worktrees whose title bar holds the same project colour, while one is free.
// Text on either bar is black or white by the background's luminance.
// No status-bar, border or tab colour is written: the status bar loses its background in an unfocused window, the active border is not drawn, and tab colours follow the selected tab.
// lym owns those keys and window.title in a worktree's settings.json and rewrites them at every seed.
// Every other key is the operator's, but for the "*.md" entry of workbench.editorAssociations:
// a seed asked to render markdown points that entry at the markdown preview, so a markdown file opens rendered, unless the operator holds the setting in its array form, which is left as it is.
// Launch opens a folder or workspace file and any files beside it in the new window.
// A workspace window keeps its project's colour on the title bar and takes WorkspaceColor, which no worktree takes, on its activity bar;
// WorkspaceSettings sets that look in the settings a workspace file splices.
// It also seeds the marked block of key bindings into the user's keybindings.json (see SeedKeybindings):
// the Alt-arrow keys the integrated terminal forwards to reed's tmux bindings, and ctrl+alt+a, which runs the reed attach task.
// A user-level binding applies in every VS Code window, so the Alt-arrow keys reach reed in any hub.
// The mill values (palette, settings keys, cmd /c code) are baked in — no external Python is read.
//
// Limitation: the hub workspace file at <hub>/<RepoName>.code-workspace carries the prime's settings, so a relative path value (starting with ./ or ../) resolves against the hub directory, no longer against the prime's own folder.
// RelativeSettingKeys names such keys so the caller can warn; no relative path value is rewritten.

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

// The editor association lym owns in a worktree's settings.json: the "*.md" entry, pointed at VS Code's markdown preview.
const (
	editorAssociationsKey = "workbench.editorAssociations"
	markdownPattern       = "*.md"
	markdownPreviewEditor = "vscode.markdown.preview.editor"
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
// With renderMarkdown on, either file's "*.md" editor association is set to the markdown preview, so a markdown file opens rendered, unless the file holds the setting in its array form, which is left as it is;
// with it off, that entry is removed while it names the preview, so an editor the operator chose stays.
// tasks.json is always written, overwriting any existing file, and holds one "reed attach" task, run on demand or by its seeded key binding;
// no task runs on folder open, so opening the folder starts no session and no agent.
// Neither file touches .gitignore; the caller keeps .vscode/ out of git through info/exclude.
// lymPath is the resolved absolute lym path stamped into the task;
// an empty lymPath becomes "lym", so a resolution failure upstream still produces a runnable (PATH-dependent) task file rather than a broken one.
// Returns an error if I/O fails.
func WriteConfig(worktreeDir, relpath string, look Look, renderMarkdown bool, lymPath string) error {
	if lymPath == "" {
		lymPath = "lym"
	}

	vscodePath := filepath.Join(worktreeDir, relpath, ".vscode")
	if err := os.MkdirAll(vscodePath, 0o755); err != nil {
		return err
	}
	if err := writeSettings(filepath.Join(vscodePath, "settings.json"), look, renderMarkdown); err != nil {
		return err
	}
	return writeAttachTask(filepath.Join(vscodePath, "tasks.json"), lymPath)
}

// writeSettings creates settingsPath with lym's defaults, look and the markdown association renderMarkdown asks for, or sets those keys in the existing file, keeping its other keys.
// An existing file that does not decode as a JSONC object is logged and left untouched.
func writeSettings(settingsPath string, look Look, renderMarkdown bool) error {
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

	applyLook(settings, look)
	applyMarkdownPreview(settings, renderMarkdown)

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if string(data) == string(content) {
		return nil
	}
	return os.WriteFile(settingsPath, data, 0o644)
}

// applyLook sets the keys lym owns in settings from look: window.title and the title-bar and activity-bar keys of workbench.colorCustomizations, keeping every other colour customization.
func applyLook(settings map[string]any, look Look) {
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
}

// applyMarkdownPreview sets the "*.md" entry of settings' workbench.editorAssociations to the markdown preview when render is on, keeping every other association.
// When render is off it removes that entry only while it names the preview, and drops an association map left empty.
// A value that is neither a map nor null, such as the array form of the setting, is left as it is, since lym cannot set its entry without replacing the operator's.
func applyMarkdownPreview(settings map[string]any, render bool) {
	existing := settings[editorAssociationsKey]
	associations, isMap := existing.(map[string]any)
	if existing != nil && !isMap {
		return
	}
	if render {
		if associations == nil {
			associations = map[string]any{}
		}
		associations[markdownPattern] = markdownPreviewEditor
		settings[editorAssociationsKey] = associations
		return
	}
	if associations[markdownPattern] != markdownPreviewEditor {
		return
	}
	delete(associations, markdownPattern)
	if len(associations) == 0 {
		delete(settings, editorAssociationsKey)
	}
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
