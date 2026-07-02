// initcli.go implements the lyx init command.
//
// It is a thin cobra wrapper: cwd resolution, flag dispatch, and JSON output
// formatting only. Both directions' core logic — scaffolding on plain
// `lyx init` and reversal on `lyx init --undo` — live in internal/initengine.

// Package initcli provides the cobra command and public seam for the lyx init command.
// It wires the --undo flag to internal/initengine.Undo and the default path to
// internal/initengine.Init.
package initcli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/initengine"
	"github.com/Knatte18/loomyard/internal/output"
)

// Command returns the cobra command for lyx init.
//
// The returned command is a leaf with Use "init". It scaffolds _lyx/config/ in
// the current directory, wires warp junctions, and maintains the managed
// .gitignore block. The public RunInit seam delegates here via clihelp.Execute,
// so all in-process callers continue to work unchanged. A local initCmd
// variable holds the composite literal (mirroring configcli.Command()'s
// configCmd pattern) so the --undo flag can be registered on it and read back
// in the RunE closure.
func Command() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "scaffold _lyx/config/ in the current directory (or reverse it with --undo)",
		Long: `init activates the lyx topology for the current worktree.

It wires cwd-keyed warp junctions, creates _lyx/ and _lyx/config/ directories,
maintains the managed .gitignore block for .lyx/, and reconciles all module
config files against their templates (idempotent: existing user edits are
preserved). A weft pairing must already exist (run 'lyx warp add' or
'lyx warp clone' first).

Pass --undo to reverse a previous init: this removes the host _lyx junction,
clears the weft-side _lyx content (committing and pushing the deletion),
and reverts the managed .gitignore block and the .git/info/exclude entry
that init added. --undo is safe to run on a directory that was never
initialized (a clean no-op) and is mainly useful for test/sandbox cleanup.

  lyx init --undo`,
	}
	initCmd.Flags().Bool("undo", false, "reverse a previous init: remove the _lyx junction, weft-side content, and the .gitignore/.git-exclude entries it added")
	initCmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
		undo, _ := initCmd.Flags().GetBool("undo")
		if undo {
			return runUndo(out, args)
		}
		return runInit(out, args)
	})
	return initCmd
}

// RunInit is the public seam for the lyx init command.
//
// It delegates to clihelp.Execute(Command(), out, args) so that all existing
// in-process callers and tests compile and pass unchanged. The cobra command
// carries both stdout and stderr into out for single-buffer test capture.
func RunInit(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runInit is the package-private handler for plain `lyx init`.
//
// It resolves cwd and delegates the actual scaffolding to initengine.Init,
// then formats the result as the JSON output envelope.
func runInit(out io.Writer, args []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
	}

	result, err := initengine.Init(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	modules := make([]map[string]any, len(result.Modules))
	for i, m := range result.Modules {
		modules[i] = map[string]any{
			"module":  m.Module,
			"status":  m.Status,
			"applied": m.Applied,
		}
	}

	return output.Ok(out, map[string]any{
		"lyx_dir":   result.LyxDir,
		"gitignore": result.Gitignore,
		"modules":   modules,
	})
}

// runUndo is the package-private handler for `lyx init --undo`.
//
// It resolves cwd and delegates the actual reversal to initengine.Undo, then
// formats the result as the JSON output envelope.
func runUndo(out io.Writer, args []string) int {
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
	}

	result, err := initengine.Undo(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	return output.Ok(out, map[string]any{
		"lyx_junction": result.LyxJunction,
		"weft_content": result.WeftContent,
		"git_exclude":  result.GitExclude,
		"gitignore":    result.Gitignore,
	})
}
