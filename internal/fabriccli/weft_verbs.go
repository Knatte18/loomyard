// weft_verbs.go wires the weft-git content-sync verbs (status, commit, push, pull,
// sync) onto the "fabric" parent command built by fabric.go. addWeftVerbs installs
// a hidden persistent --weft-path flag and a PersistentPreRunE scoped to these five
// verb names only — the topology verbs built in fabric.go resolve their own layout
// per invocation and never touch this file's closure state. The PersistentPreRunE
// splits normal mode (resolve cwd → layout → config → pathspec → Fabric handle) from
// bypass mode (--weft-path injected by the detached push child, push-only gate),
// driving fabricengine.Fabric's StatusWeft/CommitWeft/PushWeft/PullWeft.

package fabriccli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// weftVerbNames is the set of leaf command names that require this file's
// PersistentPreRunE resolution (layout, fabric config, pathspec, and a Fabric
// handle). Every other command reachable under "fabric" — the bare group and every
// topology verb from fabric.go — resolves independently and must skip this hook.
var weftVerbNames = map[string]bool{
	"status": true,
	"commit": true,
	"push":   true,
	"pull":   true,
	"sync":   true,
}

// addWeftVerbs installs the hidden --weft-path persistent flag, the weft-verb-scoped
// PersistentPreRunE, and the status/commit/push/pull/sync subcommands onto cmd — the
// "fabric" parent command built by Command() in fabric.go.
//
// Normal mode (no --weft-path) resolves cwd → layout → weftBaseDir (the weft
// worktree joined with the caller's RelPath) → fabric config loaded from
// weftBaseDir → pathspec scoped to RelPath → a Fabric handle over the resolved
// warp and weft worktree roots. Bypass mode (--weft-path set, used by the
// detached push child spawned by spawnPush) skips all of that and permits only
// the push verb, rejecting every other verb with "subcommand requires a worktree
// context" at exit 1.
func addWeftVerbs(cmd *cobra.Command) {
	// Closure vars populated by PersistentPreRunE and read by subcommand RunEs.
	var (
		l        *hubgeometry.Layout
		cfg      fabricengine.Config
		pathspec []string
		fab      *fabricengine.Fabric
		bypass   bool   // true when --weft-path is set
		weftPath string // populated from --weft-path in bypass mode
	)

	// --weft-path is a hidden persistent flag so it is available to all subcommands
	// and visible to the PersistentPreRunE without referencing the child command directly.
	cmd.PersistentFlags().String("weft-path", "", "internal: injected absolute weft worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("weft-path") //nolint:errcheck

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Guard: skip resolution for the bare "fabric" group, an unknown-subcommand
		// error path, and every topology verb — none of those read this file's
		// closure state, and none of them require a weft worktree to be present.
		if !weftVerbNames[cmd.Name()] {
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

		// Normal mode: resolve cwd → layout → fabric config → pathspec → Fabric handle.
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

		// weftBaseDir is the RelPath-aware base: the fabric config governing
		// weft-git verbs lives inside the weft worktree, scoped to the same
		// subdirectory the caller is working in.
		weftBaseDir := filepath.Join(l.WeftWorktree(), l.RelPath)

		loadedCfg, err := fabricengine.LoadConfig(weftBaseDir)
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		cfg = loadedCfg

		pathspec = fabricengine.ScopedPathspec(l.RelPath, cfg.Dirs())

		resolvedFabric, err := fabricengine.New(l.WorktreeRoot, l.WeftWorktree())
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		fab = resolvedFabric

		return nil
	}

	// status subcommand: reports content-sync state for the weft worktree.
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "show weft content-sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			statusMap, err := fab.StatusWeft(pathspec)
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
		Short: "commit weft changes",
		Long: `Stages changes in the configured pathspec and commits them to the weft worktree.

The commit message is always the fixed string "weft sync" — it is not generated
from changed files and cannot be customized with a flag.

Every fabric weft commit carries a trailing "Warp-SHA: <sha>" trailer naming the
paired warp repo's current HEAD, recorded into the correspondence index immediately
after the commit lands.

Staging is scoped to the directories listed in the fabric config (default: _lyx).

Related commands:
  lyx fabric push   — commit then push in the same process
  lyx fabric sync   — commit then async-push (detached child process)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			sha, committed, err := fab.CommitWeft(pathspec, fabricengine.DefaultCommitMessage, fabricengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"committed": committed, "sha": sha}))
			return nil
		},
	}

	// push subcommand: commits then pushes, or in bypass mode pushes directly via --weft-path.
	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "commit and push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				// Detached push child: use injected weftPath directly, skip commit.
				if err := fabricengine.PushWeftAt(weftPath, fabricengine.SyncOptions{}); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
				return nil
			}

			// Normal mode: commit first, then push.
			opts := fabricengine.EnvSyncOptions()
			if _, _, err := fab.CommitWeft(pathspec, fabricengine.DefaultCommitMessage, opts); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := fab.PushWeft(opts); err != nil {
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
		Short: "pull weft changes from remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			if err := fab.PullWeft(fabricengine.EnvSyncOptions()); err != nil {
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
		Short: "commit and async-push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			if _, _, err := fab.CommitWeft(pathspec, fabricengine.DefaultCommitMessage, fabricengine.EnvSyncOptions()); err != nil {
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
}
