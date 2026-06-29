// cli.go exposes the cobra command tree for the ide module.
//
// Command() returns the root "ide" command with two subcommands — spawn and menu —
// each wrapping the existing handler bodies. Layout resolution happens once in a
// PersistentPreRunE so that the no-arg "lyx ide" listing never requires a git repo.

package idecli

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the ide module.
//
// The parent "ide" command carries a PersistentPreRunE that resolves cwd and
// layout once before any subcommand runs; the resolved layout is shared via a
// closure variable. Subcommands spawn and menu close over that variable so they
// never repeat the resolution. When the parent is invoked with no subcommand,
// cobra lists available subcommands without invoking the PreRunE (no git repo needed).
func Command() *cobra.Command {
	// l is populated by PersistentPreRunE and closed over by each subcommand RunE.
	var l *paths.Layout

	cmd := &cobra.Command{
		Use:   "ide",
		Short: "VS Code worktree launcher",
		// RunE is set so that bare "lyx ide" lists subcommands and "lyx ide bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the ide group command itself is invoked (bare listing or
			// unknown-subcommand error path), skip layout resolution so that neither
			// path requires a git repository to be present.
			if cmd.Name() == "ide" {
				return nil
			}

			ctx := cmd.Context()

			// Resolve current working directory; fail fast if the lookup errors.
			cwd, err := paths.Getwd()
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			// Resolve layout from cwd; requires being inside a git repository.
			resolved, err := paths.Resolve(cwd)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to resolve layout: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			l = resolved
			return nil
		},
	}

	// spawn subcommand: assigns color, generates .vscode config, and opens VS Code.
	spawnCmd := &cobra.Command{
		Use:   "spawn <slug>",
		Short: "Spawn a worktree in VS Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Honour abort from PersistentPreRunE (layout resolution failed).
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			// cobra strips the "spawn" token; the slug is now args[0].
			if len(args) < 1 {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), "usage: lyx ide spawn <slug>"))
				return nil
			}
			slug := args[0]

			if err := ideengine.Spawn(l, slug); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("spawn failed: %v", err)))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(cmd.OutOrStdout(), map[string]any{}))
			return nil
		},
	}

	// menu subcommand: presents an interactive picker over active worktrees.
	menuCmd := &cobra.Command{
		Use:   "menu",
		Short: "Open the interactive worktree picker",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Honour abort from PersistentPreRunE (layout resolution failed).
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if err := ideengine.Menu(l, os.Stdin, cmd.OutOrStdout()); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("menu failed: %v", err)))
				return nil
			}
			return nil
		},
	}

	cmd.AddCommand(spawnCmd, menuCmd)
	return cmd
}

// RunCLI is the public seam for the ide module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
