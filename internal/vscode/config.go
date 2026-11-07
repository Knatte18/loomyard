// config.go writes a worktree's VS Code configuration under .vscode.
// settings.json sets the window's title and colours, and the markdown editor association when asked;
// tasks.json holds the reed attach task.
// Every other settings key is the operator's and is kept.
// Limitation: the hub workspace file at <hub>/<RepoName>.code-workspace carries the prime's settings,
// so a relative path value (starting with ./ or ../) resolves against the hub directory, not the prime's own folder.
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

// The editor association lym owns in a worktree's settings.json: the "*.md" entry.
// It points at VS Code's markdown preview.
const (
	editorAssociationsKey = "workbench.editorAssociations"
	markdownPattern       = "*.md"
	markdownPreviewEditor = "vscode.markdown.preview.editor"
)

// Look is how a worktree's VS Code window shows itself.
// It holds the window title.
// The project colour takes the title bar, and the worktree's own colour takes the activity bar.
// Each colour is a #rrggbb string.
type Look struct {
	Title         string
	ProjectColor  string
	WorktreeColor string
}

// WriteConfig writes a worktree's VS Code configuration under <worktreeDir>/<relpath>/.vscode.
// A missing settings.json is created with look and lym's defaults.
// In an existing one only the keys lym owns are set from look:
// window.title and the title-bar and activity-bar keys of workbench.colorCustomizations.
// Every other key is kept, though comments are not;
// a file that does not decode as JSONC is left untouched with a warning.
// With renderMarkdown on, settings.json's "*.md" editor association is set to the markdown preview.
// A setting held in its array form is left as it is.
// With renderMarkdown off, that entry is removed while it names the preview, so an editor the operator chose stays.
// tasks.json is always written, overwriting any existing file.
// It holds one "reed attach" task, run on demand or by its seeded key binding;
// no task runs on folder open, so opening the folder starts no session and no agent.
// Neither file touches .gitignore; the caller keeps .vscode/ out of git through info/exclude.
// lymPath is the resolved absolute lym path stamped into the task.
// An empty lymPath becomes "lym",
// so a resolution failure upstream still produces a runnable, PATH-dependent task file rather than a broken one.
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

// writeSettings creates settingsPath with lym's defaults, look and the markdown association renderMarkdown asks for.
// An existing file has those keys set in place, and its other keys are kept.
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

// applyLook sets the keys lym owns in settings from look:
// window.title and the title-bar and activity-bar keys of workbench.colorCustomizations.
// Every other colour customization is kept.
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

// applyMarkdownPreview points the "*.md" association at the markdown preview when render is on.
// Every other association is kept.
// When render is off, it removes that entry only while it names the preview,
// and it drops an association map left empty.
// A value that is neither a map nor null, such as the array form of the setting, is left as it is.
// lym cannot set its entry without replacing the operator's.
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
// The task carries no runOptions.
// VS Code runs it only from "Tasks: Run Task" or its seeded key binding, never on folder open.
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
