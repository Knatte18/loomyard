// spawn.go implements `ide spawn`: it assigns a title-bar color, generates the
// worktree's .vscode/ config when absent, and launches VS Code.

package ide

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// codeLauncher is a package-level injectable seam that can be overridden in tests.
// It defaults to vscode.Launch but can be stubbed to record its argument for testing.
var codeLauncher = vscode.Launch

// Spawn generates a worktree's .vscode/ config (if absent) and launches VS Code.
//
// It performs the following steps:
//  1. Compute worktreeDir := l.WorktreePath(slug)
//  2. Compute color := vscode.PickColor(l)
//  3. Call vscode.WriteConfig(worktreeDir, l.RelPath, slug, color)
//  4. Open the worktree at its relpath (dir holding _lyx/ and .vscode/) via codeLauncher
//
// Returns an error if any step fails.
func Spawn(l *paths.Layout, slug string) error {
	// Compute worktreeDir from slug
	worktreeDir := l.WorktreePath(slug)

	// Compute color for this worktree
	color := vscode.PickColor(l)

	// Generate VS Code config (settings.json, tasks.json, register in .gitignore)
	if err := vscode.WriteConfig(worktreeDir, l.RelPath, slug, color); err != nil {
		return err
	}

	// Launch VS Code at the rel path (the dir holding _lyx/ and .vscode/)
	openDir := filepath.Join(worktreeDir, l.RelPath)
	if err := codeLauncher(openDir); err != nil {
		return err
	}

	return nil
}
