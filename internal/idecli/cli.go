// cli.go exposes the cobra command tree for the ide module.
//
// Command() returns the root "ide" command with two subcommands — spawn and menu — each wrapping the existing handler bodies.
// Layout resolution happens once in a PersistentPreRunE so that the no-arg "lyx ide" listing never requires a git repo.

package idecli

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the ide module.
func Command() *cobra.Command {
	var l *lyxcwd.Location

	cmd := &cobra.Command{
		Use:   "ide",
		Short: "VS Code worktree launcher",
		RunE:  clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "ide" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lyxcwd.Getwd()
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := lyxcwd.Resolve(cwd)
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			l = resolved
			return nil
		},
	}

	spawnCmd := &cobra.Command{
		Use:   "spawn <slug>",
		Short: "Spawn a worktree in VS Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

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

	menuCmd := &cobra.Command{
		Use:   "menu",
		Short: "Open the interactive worktree picker",
		RunE: func(cmd *cobra.Command, args []string) error {
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
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
