// spawn.go implements `ide spawn`: it assigns a title-bar color, generates the
// worktree's .vscode/ config when absent, and launches VS Code.

package ideengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// CodeLauncher is the exported package-level injectable seam that can be overridden
// in tests. It defaults to vscode.Launch but can be stubbed to record its argument
// for testing. Exported so that cli_test.go in the idecli package can swap it.
var CodeLauncher = vscode.Launch

// Spawn generates a worktree's .vscode/ config (if absent) and launches VS Code.
func Spawn(l *hubgeometry.Layout, slug string) error {
	worktreeDir := l.WorktreePath(slug)
	// A prime-resolution failure degrades to an empty prime name (PickColor
	// then skips the prime-skip step) rather than failing the spawn — a wrong
	// title-bar color is cosmetic, not worth aborting over.
	primeName, _ := fabricengine.PrimeName(l)
	color := vscode.PickColor(l, primeName)

	if err := vscode.WriteConfig(worktreeDir, l.RelPath, slug, color); err != nil {
		return err
	}

	openDir := filepath.Join(worktreeDir, l.RelPath)
	if err := CodeLauncher(openDir); err != nil {
		return err
	}

	return nil
}
