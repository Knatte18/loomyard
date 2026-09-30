// spawn.go implements both ide entry points: Spawn (`ide spawn`) and SpawnDriven (batten's driven-pair open).
// Each assigns a title-bar color, generates the worktree's .vscode/ config, and launches VS Code;
// Spawn writes the interactive chain when absent, SpawnDriven the attach-only chain.
// A task slug opens its bare folder.
// The prime instead opens a lyx-generated hub workspace (prime, _board, _portals) under <hub>/_launchers/<AnchorRel>, regenerated on every prime spawn.

package ideengine

import (
	"fmt"
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

// Spawn generates a worktree's .vscode/ config (if absent) and launches VS Code.
// A task slug launches its bare folder <hub>/<slug>/<AnchorRel> and writes no workspace file.
// When slug names the prime, Spawn writes the hub workspace file, whose settings carry the prime's .vscode/settings.json, and launches that file instead;
// every error on that path is returned wrapped with its step, never degraded to the bare folder.
// A failure to resolve the prime's name is logged and degrades to the bare-folder path.
func Spawn(l *lyxcwd.Location, slug string) error {
	worktreeDir, color, primeName, primeResolved := resolveSpawnTarget(l, slug)

	// Resolve both binary paths the generated folderOpen chain stamps in absolute.
	// Each degrades to the empty string on error so vscode.WriteConfig's own bare-name
	// fallback rule applies; Spawn substitutes no bare name itself.
	lyxPath, _ := os.Executable()
	claudePath, _ := exec.LookPath("claude")

	if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, slug, color, lyxPath, claudePath, vscode.TaskChainInteractive); err != nil {
		return err
	}

	openDir := filepath.Join(worktreeDir, l.AnchorRel)
	if primeResolved && slug == primeName {
		workspacePath, err := writePrimeWorkspace(l, primeName, openDir)
		if err != nil {
			return err
		}
		return CodeLauncher(workspacePath)
	}

	return CodeLauncher(openDir)
}

// SpawnDriven opens VS Code on a driven pair's task folder, wired to attach to the child run's own reed session.
// The caller decides the pair is driven; nothing here detects it.
// The folder gets an attach-only tasks.json (vscode.TaskChainAttachOnly), overwritten if present, and a settings.json written only when absent.
// When .vscode/tasks.json is already tracked in the pair's repo, .vscode/ belongs to the repo:
// SpawnDriven then writes neither file, leaves the shared info/exclude alone, logs one warning, and still launches.
// Otherwise it first keeps .vscode/ out of git with an anchored line in the shared info/exclude, so the child's commits never sweep it up.
// gitignore.Ensure is never called: the child's loom run commits in that worktree.
func SpawnDriven(l *lyxcwd.Location, slug string) error {
	worktreeDir, color, _, _ := resolveSpawnTarget(l, slug)

	tasksRel := path.Join(filepath.ToSlash(l.AnchorRel), ".vscode", "tasks.json")
	tracked, err := fabricengine.PathTracked(worktreeDir, tasksRel)
	if err != nil {
		return fmt.Errorf("check %s tracked: %w", tasksRel, err)
	}
	if tracked {
		logger.Warn("tracked .vscode/tasks.json; leaving .vscode/ untouched", "slug", slug, "path", tasksRel)
	} else {
		excludePath, changed, err := fabricengine.ExcludeAnchoredDir(worktreeDir, l.AnchorRel, ".vscode")
		if err != nil {
			return fmt.Errorf("exclude .vscode: %w", err)
		}
		if changed {
			logger.Info("excluded .vscode/ from git", "slug", slug, "exclude", excludePath)
		}
		lyxPath, _ := os.Executable()
		if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, slug, color, lyxPath, "", vscode.TaskChainAttachOnly); err != nil {
			return fmt.Errorf("write vscode config: %w", err)
		}
	}

	return CodeLauncher(filepath.Join(worktreeDir, l.AnchorRel))
}

// resolveSpawnTarget resolves slug's worktree path and title-bar color, and the prime's name.
// A prime-resolution failure is logged here, once, and degrades rather than failing the spawn:
// PickColor skips its prime-skip step, and the returned flag is false.
// A wrong title-bar color or a missing hub workspace is not worth aborting over.
func resolveSpawnTarget(l *lyxcwd.Location, slug string) (worktreeDir, color, primeName string, primeResolved bool) {
	worktreeDir = fabricengine.WorktreePath(l, slug)
	primeName, primeErr := fabricengine.PrimeName(l)
	if primeErr != nil {
		logger.Warn("resolve prime name; opening the bare folder", "slug", slug, "error", primeErr)
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
	hubFolders, err := fabricengine.HubWorkspaceFolders(l, primeName)
	if err != nil {
		return "", fmt.Errorf("compute workspace folders: %w", err)
	}
	folders := make([]vscode.WorkspaceFolder, len(hubFolders))
	for i, f := range hubFolders {
		folders[i] = vscode.WorkspaceFolder{Name: f.Name, Path: f.Path}
	}
	content, err := vscode.BuildWorkspace(folders, settings)
	if err != nil {
		return "", fmt.Errorf("build workspace: %w", err)
	}
	if _, err := fabricengine.WriteHubWorkspace(l, primeName, content); err != nil {
		return "", fmt.Errorf("write workspace: %w", err)
	}
	return fabricengine.HubWorkspacePath(l, primeName), nil
}
