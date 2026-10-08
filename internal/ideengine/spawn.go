// spawn.go implements the ide entry point Spawn (`ide spawn`).
// It assigns a title-bar color, generates the worktree's .vscode/ config, and launches VS Code;
// it regenerates the interactive chain on every call and keeps .vscode/ out of git through info/exclude.
// A task slug opens its bare folder.
// The prime instead opens a lyx-generated hub workspace (prime, _board, _portals) under <hub>/_launchers/<AnchorRel>, regenerated on every prime spawn.

package ideengine

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// CodeLauncher is the exported package-level injectable seam that can be overridden in tests.
// It defaults to vscode.Launch but can be stubbed to record its argument for testing.
// Exported so that cli_test.go in the idecli package can swap it.
var CodeLauncher = vscode.Launch

// KeybindingsPath is the injectable seam that names the user's VS Code keybindings.json.
// It defaults to vscode.UserKeybindingsPath and is exported so that tests in other packages can point it at a temporary file.
var KeybindingsPath = vscode.UserKeybindingsPath

// Spawn regenerates a worktree's interactive .vscode/ chain on every call and launches VS Code.
// It keeps .vscode/ out of git through info/exclude and never writes a .gitignore;
// a tracked .vscode/tasks.json is left alone with a warning, and the launch still happens.
// A task slug launches its bare folder <hub>/<slug>/<AnchorRel> and writes no workspace file.
// When slug names the prime, Spawn writes the hub workspace file, whose settings carry the prime's .vscode/settings.json, and launches that file instead;
// every error on that path is returned wrapped with its step, never degraded to the bare folder.
// A failure to resolve the prime's name is logged and degrades to the bare-folder path.
// Before launching it seeds the lyx block of terminal key bindings into the user's VS Code keybindings.json and returns the outcome beside the error;
// a skipped seed never fails the spawn.
func Spawn(l *lyxcwd.Location, slug string) (vscode.KeybindingsResult, error) {
	worktreeDir, color, primeName, primeResolved := resolveSpawnTarget(l, slug, targetUnknown)

	// Resolve both binary paths the generated folderOpen chain stamps in absolute.
	// Each degrades to the empty string on error so vscode.WriteConfig's own bare-name
	// fallback rule applies; Spawn substitutes no bare name itself.
	lyxPath, _ := os.Executable()
	claudePath, _ := exec.LookPath("claude")

	if err := writeVSCodeConfig(l, worktreeDir, slug, color, lyxPath, claudePath, vscode.TaskChainInteractive); err != nil {
		return vscode.KeybindingsResult{}, err
	}

	keybindings := seedKeybindings()

	openDir := filepath.Join(worktreeDir, l.AnchorRel)
	if primeResolved && slug == primeName {
		workspacePath, err := writePrimeWorkspace(l, primeName, openDir)
		if err != nil {
			return keybindings, err
		}
		return keybindings, CodeLauncher(workspacePath)
	}

	return keybindings, CodeLauncher(openDir)
}

// seedKeybindings seeds the user's keybindings.json through the KeybindingsPath seam; a path it cannot resolve is a skip.
func seedKeybindings() vscode.KeybindingsResult {
	keybindingsPath, err := KeybindingsPath()
	if err != nil {
		return vscode.KeybindingsResult{Outcome: vscode.KeybindingsSkipped, Reason: fmt.Sprintf("resolve the keybindings path: %v", err)}
	}
	return vscode.SeedKeybindings(keybindingsPath)
}

// writeVSCodeConfig keeps .vscode/ out of git and writes the chain's config at the worktree's anchor.
// When .vscode/tasks.json is already tracked, it logs one warning and writes neither file nor the exclude line.
// Otherwise it first adds an anchored .vscode line to the shared info/exclude, then calls vscode.WriteConfig.
func writeVSCodeConfig(l *lyxcwd.Location, worktreeDir, slug, color, lyxPath, claudePath string, chain vscode.TaskChain) error {
	tasksRel := path.Join(filepath.ToSlash(l.AnchorRel), ".vscode", "tasks.json")
	tracked, err := fabricengine.PathTracked(worktreeDir, tasksRel)
	if err != nil {
		return fmt.Errorf("check %s tracked: %w", tasksRel, err)
	}
	if tracked {
		logger.Warn("tracked .vscode/tasks.json; leaving .vscode/ untouched", "slug", slug, "path", tasksRel)
		return nil
	}
	excludePath, changed, err := fabricengine.ExcludeAnchoredDir(worktreeDir, l.AnchorRel, ".vscode")
	if err != nil {
		return fmt.Errorf("exclude .vscode: %w", err)
	}
	if changed {
		logger.Info("excluded .vscode/ from git", "slug", slug, "exclude", excludePath)
	}
	if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, slug, color, lyxPath, claudePath, chain); err != nil {
		return fmt.Errorf("write vscode config: %w", err)
	}
	return nil
}

// spawnTarget is what a spawn opens, as far as the caller can tell before the prime's name is known.
type spawnTarget int

const (
	// targetUnknown is a spawn whose slug may name the prime; fabricengine has no structural check that tells without the prime-name read.
	targetUnknown spawnTarget = iota
	// targetTask is a spawn that always opens a task pair.
	targetTask
	// targetPrime is a spawn known to open the prime.
	targetPrime
)

// primeResolveLogLevel decides how loudly a prime-name resolution failure is logged for target.
// A prime or unknown target logs at WARN, since a prime spawn must never lose its warning;
// a task target logs at debug, since only its title-bar color degrades.
// No error logs nothing.
func primeResolveLogLevel(target spawnTarget, err error) (level slog.Level, log bool) {
	if err == nil {
		return 0, false
	}
	if target == targetTask {
		return slog.LevelDebug, true
	}
	return slog.LevelWarn, true
}

// resolveSpawnTarget resolves slug's worktree path and title-bar color, and the prime's name.
// A prime-resolution failure is logged here, once, at the level primeResolveLogLevel picks for target, and degrades rather than failing the spawn:
// PickColor skips its prime-skip step, and the returned flag is false.
// A wrong title-bar color or a missing hub workspace is not worth aborting over.
func resolveSpawnTarget(l *lyxcwd.Location, slug string, target spawnTarget) (worktreeDir, color, primeName string, primeResolved bool) {
	worktreeDir = fabricengine.WorktreePath(l, slug)
	primeName, primeErr := fabricengine.PrimeName(l)
	if level, ok := primeResolveLogLevel(target, primeErr); ok {
		if level == slog.LevelDebug {
			logger.Debug("resolve prime name; opening the bare folder", "slug", slug, "error", primeErr)
		} else {
			logger.Warn("resolve prime name; opening the bare folder", "slug", slug, "error", primeErr)
		}
	}
	color = vscode.PickColor(l, primeName)
	return worktreeDir, color, primeName, primeErr == nil
}

// writePrimeWorkspace builds and writes the prime's hub workspace file and returns its path.
// primeDir is the prime's anchor directory, whose .vscode/settings.json the file's settings splice.
func writePrimeWorkspace(l *lyxcwd.Location, primeName, primeDir string) (string, error) {
	settings, err := vscode.ReadSettings(primeDir)
	if err != nil {
		return "", fmt.Errorf("read prime settings: %w", err)
	}
	if keys := vscode.RelativeSettingKeys(settings); len(keys) > 0 {
		logger.Warn("prime settings hold relative paths that now resolve against the _launchers workspace file's directory", "keys", keys)
	}
	// WriteHubWorkspace creates the _portals/<AnchorRel> folder, which HubWorkspaceFolders lists only once it exists,
	// so the first spawn on a hub writes twice; the second write is a no-op when the file is unchanged.
	for range 2 {
		if err := writeWorkspaceFile(l, primeName, settings); err != nil {
			return "", err
		}
	}
	return fabricengine.HubWorkspacePath(l, primeName), nil
}

// writeWorkspaceFile builds the hub workspace file from the folders that exist now and writes it.
func writeWorkspaceFile(l *lyxcwd.Location, primeName string, settings []byte) error {
	hubFolders, err := fabricengine.HubWorkspaceFolders(l, primeName)
	if err != nil {
		return fmt.Errorf("compute workspace folders: %w", err)
	}
	folders := make([]vscode.WorkspaceFolder, len(hubFolders))
	for i, f := range hubFolders {
		folders[i] = vscode.WorkspaceFolder{Name: f.Name, Path: f.Path}
	}
	content, err := vscode.BuildWorkspace(folders, settings)
	if err != nil {
		return fmt.Errorf("build workspace: %w", err)
	}
	if _, err := fabricengine.WriteHubWorkspace(l, primeName, content); err != nil {
		return fmt.Errorf("write workspace: %w", err)
	}
	return nil
}
