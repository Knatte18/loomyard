// cli.go exposes the cobra command tree for the weft module.
//
// Command() returns the root "weft" command with five subcommands — status, commit,
// push, pull, sync — each wrapping the existing handler bodies. Layout and config
// resolution happens once in a PersistentPreRunE so that the no-arg "lyx weft"
// listing never requires a git repo. A hidden persistent --weft-path flag enables
// the detached push child path (bypass mode) used by spawnPush.

package weft

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/paths"
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
		l        *paths.Layout
		cfg      Config
		pathspec []string
		bypass   bool   // true when --weft-path is set
		weftPath string // populated from --weft-path in bypass mode
	)

	cmd := &cobra.Command{
		Use:   "weft",
		Short: "weft git operations",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			// Read the hidden flag value.
			injectedPath, _ := cmd.Flags().GetString("weft-path")

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
			cwd, err := paths.Getwd()
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := paths.Resolve(cwd)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			l = resolved

			weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)

			loadedCfg, err := LoadConfig(weftBaseDir)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}
			cfg = loadedCfg

			pathspec = scopedPathspec(l.RelPath, cfg.Dirs())
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
			statusMap, err := Status(l.WeftWorktree(), pathspec)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			committed, err := Commit(l.WeftWorktree(), pathspec, envSyncOptions())
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
				if err := Push(weftPath, SyncOptions{}); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
				return nil
			}

			// Normal mode: commit first, then push.
			opts := envSyncOptions()
			_, err := Commit(l.WeftWorktree(), pathspec, opts)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := Push(l.WeftWorktree(), opts); err != nil {
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
			if err := Pull(l.WeftWorktree(), envSyncOptions()); err != nil {
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
			_, err := Commit(l.WeftWorktree(), pathspec, envSyncOptions())
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

// envSyncOptions reads WEFT_SKIP_* environment variables and returns a SyncOptions.
func envSyncOptions() SyncOptions {
	return SyncOptions{
		SkipGit:  os.Getenv("WEFT_SKIP_GIT") == "1",
		SkipPush: os.Getenv("WEFT_SKIP_PUSH") == "1",
	}
}

// RunCLI is the public seam for the weft module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
