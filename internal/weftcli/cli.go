// cli.go exposes the cobra command tree for the weft module.
//
// Command() returns the root "weft" command with five subcommands — status, commit,
// push, pull, sync — each wrapping the existing handler bodies. Layout and config
// resolution happens once in a PersistentPreRunE so that the no-arg "lyx weft"
// listing never requires a git repo. A hidden persistent --weft-path flag enables
// the detached push child path (bypass mode) used by spawnPush.

package weftcli

import (
	"io"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/weftengine"
	"github.com/spf13/cobra"
)

// Command returns the cobra command tree for the weft module.
//
// The parent "weft" command carries a hidden persistent --weft-path flag and a
// PersistentPreRunE that either enters bypass mode (--weft-path set) or resolves
// cwd, layout, config, and pathspec into closure variables. Subcommands close over
// those variables so no resolution is duplicated. When the parent is invoked with
// no subcommand, cobra lists available subcommands without invoking the PreRunE
// (no git repo needed).
func Command() *cobra.Command {
	// Closure vars populated by PersistentPreRunE and read by subcommand RunEs.
	var (
		l        *hubgeometry.Layout
		cfg      weftengine.Config
		pathspec []string
		bypass   bool   // true when --weft-path is set
		weftPath string // populated from --weft-path in bypass mode
	)

	cmd := &cobra.Command{
		Use:   "weft",
		Short: "weft git operations",
		// RunE is set so that bare "lyx weft" lists subcommands and "lyx weft bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the weft group command itself is invoked (bare listing or
			// unknown-subcommand error path), skip layout/config resolution so that
			// neither path requires a git repository to be present.
			if cmd.Name() == "weft" {
				return nil
			}

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			// Read the hidden persistent flag via InheritedFlags to make explicit
			// that this flag is inherited from the parent command, not local.
			injectedPath, _ := cmd.InheritedFlags().GetString("weft-path")

			if injectedPath != "" {
				// Bypass mode: --weft-path was injected by the detached push child.
				// Only the push subcommand is valid in this mode; reject everything else
				// to prevent accidental invocation without a worktree context.
				bypass = true
				weftPath = injectedPath

				// In PersistentPreRunE, cmd is the leaf subcommand being executed,
				// so cmd.Name() returns the subcommand name (e.g. "push", "status").
				if cmd.Name() != "push" {
					output.Err(out, "subcommand requires a worktree context")
					clihelp.Abort(ctx, 1)
					return nil
				}
				return nil
			}

			// Normal mode: resolve cwd → layout → config → pathspec.
			cwd, err := hubgeometry.Getwd()
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := hubgeometry.Resolve(cwd)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			l = resolved

			weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)

			loadedCfg, err := weftengine.LoadConfig(weftBaseDir)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			cfg = loadedCfg

			pathspec = weftengine.ScopedPathspec(l.RelPath, cfg.Dirs())
			return nil
		},
	}

	// --weft-path is a hidden persistent flag so it is available to all subcommands
	// and visible to the PersistentPreRunE without referencing the child command directly.
	cmd.PersistentFlags().String("weft-path", "", "internal: injected absolute weft worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("weft-path") //nolint:errcheck

	// status subcommand: reports content-sync state for the weft worktree.
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show weft sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			statusMap, err := weftengine.Status(l.WeftWorktree(), pathspec)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, statusMap))
			return nil
		},
	}

	// commit subcommand: stages and commits pathspec-scoped changes.
	commitCmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit weft changes",
		Long: `Stages changes in the configured pathspec and commits them to the weft worktree.

The commit message is always the fixed string "weft sync" — it is not generated
from changed files and cannot be customized with a flag.

Staging is scoped to the directories listed in the weft config (default: _lyx).

Related commands:
  lyx weft push   — commit then push in the same process
  lyx weft sync   — commit then async-push (detached child process)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			committed, err := weftengine.Commit(l.WeftWorktree(), pathspec, weftengine.DefaultCommitMessage, weftengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"committed": committed}))
			return nil
		},
	}

	// push subcommand: commits then pushes, or in bypass mode pushes directly via --weft-path.
	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "Commit and push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				// Detached push child: use injected weftPath directly, skip commit.
				if err := weftengine.Push(weftPath, weftengine.SyncOptions{}); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
				return nil
			}

			// Normal mode: commit first, then push.
			opts := weftengine.EnvSyncOptions()
			_, err := weftengine.Commit(l.WeftWorktree(), pathspec, weftengine.DefaultCommitMessage, opts)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := weftengine.Push(l.WeftWorktree(), opts); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
			return nil
		},
	}

	// pull subcommand: fast-forwards from the remote.
	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull weft changes from remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			if err := weftengine.Pull(l.WeftWorktree(), weftengine.EnvSyncOptions()); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
			return nil
		},
	}

	// sync subcommand: commits pathspec changes then spawns a detached push child.
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit and async-push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			_, err := weftengine.Commit(l.WeftWorktree(), pathspec, weftengine.DefaultCommitMessage, weftengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := spawnPush(l.WeftWorktree()); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
			return nil
		},
	}

	cmd.AddCommand(statusCmd, commitCmd, pushCmd, pullCmd, syncCmd)
	return cmd
}

// RunCLI is the public seam for the weft module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
