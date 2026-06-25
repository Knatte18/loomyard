// Command lyx is the CLI for the Loomyard task tracker.
//
// main.go is a thin module dispatcher: it routes the first argument to the
// matching module and delegates all further parsing and execution to that
// module. Each module owns its own flags, subcommands, and output format.
//
// Usage:
//
//	lyx <module> [module-args...]
//
// Modules:
//
//	init       scaffold _lyx/config/ with all module configs and .gitignore in the current directory
//	board      task-tracker board — see internal/board.RunCLI for subcommands
//	config     edit module configuration — see internal/configcli.RunCLI for subcommands
//	update     reconcile module configs against templates — see internal/update.RunCLI
//	ide        VS Code launcher — see internal/ide.RunCLI for subcommands
//	muxpoc     proof-of-concept psmux mux — see internal/muxpoc.RunCLI for subcommands
//	worktree   git-worktree lifecycle — see internal/worktree.RunCLI for subcommands
//	weft       weft git operations — see internal/weft.RunCLI for subcommands
//	warp       host↔weft coordination — see internal/warp.RunCLI for subcommands
//
// All output is JSON on stdout. Exit code 1 on error.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/ide"
	"github.com/Knatte18/loomyard/internal/initcli"
	"github.com/Knatte18/loomyard/internal/muxpoc"
	"github.com/Knatte18/loomyard/internal/update"
	"github.com/Knatte18/loomyard/internal/warp"
	"github.com/Knatte18/loomyard/internal/weft"
	"github.com/Knatte18/loomyard/internal/worktree"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

// run dispatches args to the matching module, writing module output to out.
// It returns the process exit code (0 on success, 1 on error). Usage and
// unknown-module messages go to stderr; module output goes to out.
func run(args []string, out io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: lyx <module> [args...]")
		return 1
	}

	module, moduleArgs := args[0], args[1:]

	switch module {
	case "init":
		return initcli.RunInit(out, moduleArgs)
	case "board":
		return board.RunCLI(out, moduleArgs)
	case "config":
		return configcli.RunCLI(out, moduleArgs)
	case "update":
		return update.RunCLI(out, moduleArgs)
	case "ide":
		return ide.RunCLI(out, moduleArgs)
	case "muxpoc":
		return muxpoc.RunCLI(out, moduleArgs)
	case "worktree":
		return worktree.RunCLI(out, moduleArgs)
	case "weft":
		return weft.RunCLI(out, moduleArgs)
	case "warp":
		return warp.RunCLI(out, moduleArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown module: %s\n", module)
		return 1
	}
}
