// spawn.go implements `ide spawn`: it assigns a title-bar color, generates the worktree's .vscode/
// config when absent, and launches VS Code.
// A task slug opens its bare folder.
// The prime instead opens a lyx-generated hub workspace (prime, _board, _portals) under
// <hub>/_launchers/<AnchorRel>, regenerated on every prime spawn.

package ideengine

import (
	"fmt"
	"os"
	"os/exec"
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
// When slug names the prime, Spawn writes the hub workspace file, whose settings carry the prime's
// .vscode/settings.json, and launches that file instead; every error on that path is returned
// wrapped with its step, never degraded to the bare folder.
// A failure to resolve the prime's name is logged and degrades to the bare-folder path.
func Spawn(l *lyxcwd.Location, slug string) error {
	worktreeDir := fabricengine.WorktreePath(l, slug)
	// A prime-resolution failure degrades to an empty prime name (PickColor
	// then skips the prime-skip step) rather than failing the spawn — a wrong
	// title-bar color is cosmetic, not worth aborting over.
	primeName, primeErr := fabricengine.PrimeName(l)
	if primeErr != nil {
		logger.Warn("resolve prime name; opening the bare folder", "slug", slug, "error", primeErr)
	}
	color := vscode.PickColor(l, primeName)

	// Resolve both binary paths the generated folderOpen chain stamps in absolute.
	// Each degrades to the empty string on error so vscode.WriteConfig's own bare-name
	// fallback rule applies; Spawn substitutes no bare name itself.
	lyxPath, _ := os.Executable()
	claudePath, _ := exec.LookPath("claude")

	if err := vscode.WriteConfig(worktreeDir, l.AnchorRel, slug, color, lyxPath, claudePath); err != nil {
		return err
	}

	openDir := filepath.Join(worktreeDir, l.AnchorRel)
	if primeErr == nil && slug == primeName {
		workspacePath, err := writePrimeWorkspace(l, primeName, openDir)
		if err != nil {
			return err
		}
		return CodeLauncher(workspacePath)
	}

	return CodeLauncher(openDir)
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
