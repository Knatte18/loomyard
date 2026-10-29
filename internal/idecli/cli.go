// cli.go exposes the cobra command tree for the ide module.
//
// Command() returns the root "ide" command with four subcommands — spawn, menu, seed and open.
// spawn and menu wrap the existing handler bodies;
// seed writes an existing worktree's .vscode/ config, window title and colours, the hub workspace file for the prime, and the user's lym key bindings, and launches nothing;
// with no slug it seeds the worktree it runs in, the prime included.
// open starts VS Code on a pair's folder and takes a slug.
// Layout resolution happens once in a PersistentPreRunE so that the no-arg "lym ide" listing never
// requires a git repo.

package idecli

import (
	"fmt"
	"io"
	"maps"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the ide module.
func Command() *cobra.Command {
	var l *lymcwd.Location

	cmd := &cobra.Command{
		Use:   "ide",
		Short: "open a worktree in VS Code, or pick one from a menu",
		RunE:  clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "ide" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := lymcwd.Resolve(cwd)
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
		Use:         "spawn <slug>",
		Short:       "spawn a worktree in VS Code",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if len(args) < 1 {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), "usage: lym ide spawn <slug>"))
				return nil
			}
			slug := args[0]

			keybindings, err := ideengine.Spawn(l, slug)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("spawn failed: %v", err)))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(cmd.OutOrStdout(), map[string]any{"keybindings": keybindings}))
			return nil
		},
	}

	menuCmd := &cobra.Command{
		Use:         "menu",
		Short:       "open the interactive worktree picker",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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

	seedCmd := slugVerbCommand(&l, "seed", "write the VS Code config, window title and colours of the prime or a task worktree, the current worktree with no slug, and the user key bindings", true,
		func(l *lymcwd.Location, slug string) (map[string]any, error) {
			keybindings, err := ideengine.Seed(l, slug)
			return map[string]any{"keybindings": keybindings}, err
		})
	openCmd := slugVerbCommand(&l, "open", "open a task worktree's folder in VS Code", false,
		func(l *lymcwd.Location, slug string) (map[string]any, error) {
			return nil, ideengine.OpenTask(l, slug)
		})

	cmd.AddCommand(spawnCmd, menuCmd, seedCmd, openCmd)
	return cmd
}

// slugVerbCommand builds the operator-audience verb name <slug> that runs action on an existing worktree of the hub and prints the slug and the fields action returns.
// With slugOptional, the slug may be left out and names the worktree the verb runs in, the prime included.
// Before calling action it refuses a slug failing the slug grammar and one whose worktree directory under the hub does not exist,
// so the verb acts only on a directory directly under the hub.
// The location is read through l when the verb runs, after the root's pre-run has resolved it.
func slugVerbCommand(l **lymcwd.Location, name, short string, slugOptional bool, action func(*lymcwd.Location, string) (map[string]any, error)) *cobra.Command {
	use := name + " <slug>"
	if slugOptional {
		use = name + " [<slug>]"
	}
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			var slug string
			switch {
			case len(args) > 0:
				slug = args[0]
			case slugOptional:
				slug = (*l).WorktreeName
			default:
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), "usage: lym ide "+use))
				return nil
			}

			if err := agentname.ValidateSlug(slug); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("%s failed: %v", name, err)))
				return nil
			}
			worktreeDir := fabricengine.WorktreePath(*l, slug)
			if info, err := os.Stat(worktreeDir); err != nil || !info.IsDir() {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("%s failed: no pair worktree %s for slug %q", name, worktreeDir, slug)))
				return nil
			}

			fields, err := action(*l, slug)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("%s failed: %v", name, err)))
				return nil
			}
			result := map[string]any{"slug": slug}
			maps.Copy(result, fields)
			clihelp.SetExit(cmd.Context(), output.Ok(cmd.OutOrStdout(), result))
			return nil
		},
	}
}

// RunCLI is the public seam for the ide module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lymcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
