// Package ide provides a one-shot launcher with spawn and interactive menu
// for managing worktrees. Spawn assigns a color and launches the worktree.
// Menu presents an interactive picker over active worktrees.
//
// The spawn command delegates config generation (settings.json, tasks.json,
// .gitignore registration), color picking, and VS Code launch to internal/vscode.
// The menu command resolves titles from the board facade.
//
// Spawn and menu are Windows-only (POSIX no-ops/errors with a clear message);
// cross-platform support is in the delegated vscode package.
package ide

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
)

// RunCLI is the main entry point for the ide module CLI.
// It parses the command-line arguments and dispatches to the appropriate subcommand.
//
// Subcommands:
//   - spawn <slug>   Spawn a worktree in VS Code
//   - menu           Open the interactive worktree picker
//
// Returns the exit code (0 on success, 1 on error).
func RunCLI(out io.Writer, args []string) int {
	// Resolve current working directory
	cwd, err := paths.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
	}

	// Resolve layout
	l, err := paths.Resolve(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to resolve layout: %v", err))
	}

	// Parse subcommand
	if len(args) < 1 {
		return output.Err(out, "usage: lyx ide <spawn|menu> [args...]")
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "spawn":
		if len(subArgs) < 1 {
			return output.Err(out, "usage: lyx ide spawn <slug>")
		}
		slug := subArgs[0]
		if err := Spawn(l, slug); err != nil {
			return output.Err(out, fmt.Sprintf("spawn failed: %v", err))
		}
		return output.Ok(out, map[string]any{})

	case "menu":
		if err := Menu(l, os.Stdin, out); err != nil {
			return output.Err(out, fmt.Sprintf("menu failed: %v", err))
		}
		return 0

	default:
		return output.Err(out, fmt.Sprintf("unknown subcommand: %s", subcommand))
	}
}
