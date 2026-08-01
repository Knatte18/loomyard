// weft_verbs.go wires the weft-git content-sync verbs (status, commit, push, pull,
// sync) onto the "fabric" parent command built by fabric.go. addWeftVerbs installs
// two hidden persistent flags — --weft-path and --warp-path — and a
// PersistentPreRunE scoped to these five verb names only — the topology verbs built
// in fabric.go resolve their own layout per invocation and never touch this file's
// closure state. The PersistentPreRunE splits normal mode (resolve cwd → layout →
// config → pathspec → Fabric handle) from bypass mode (either hidden path flag
// injected by the detached push child, push-only gate), driving fabricengine.Fabric's
// StatusWeft/CommitWeft/PushWeft/Pull in normal mode and
// fabricengine.CoalescePushBothAt's loop-until-clean coalescing push directly in
// bypass mode.

package fabriccli

import (
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
// Normal mode (neither --weft-path nor --warp-path set) resolves cwd → layout →
// weftBaseDir (the weft worktree joined with the caller's RelPath) → fabric config
// loaded from weftBaseDir → pathspec scoped to RelPath → a Fabric handle over the
// resolved warp and weft worktree roots. Bypass mode (--weft-path and/or --warp-path
// set, used by the detached push child spawned by fabricengine.SpawnDetachedPush)
// skips all of that and permits only the push verb, rejecting every other verb with
// "subcommand requires a worktree context" at exit 1.
func addWeftVerbs(cmd *cobra.Command) {
	// Closure vars populated by PersistentPreRunE and read by subcommand RunEs.
	var (
		l        *hubgeometry.Layout
		cfg      fabricengine.Config
		pathspec []string
		fab      *fabricengine.Fabric
		bypass   bool   // true when --weft-path and/or --warp-path is set
		weftPath string // populated from --weft-path in bypass mode
		warpPath string // populated from --warp-path in bypass mode
	)

	// --weft-path and --warp-path are hidden persistent flags so they are available
	// to all subcommands and visible to the PersistentPreRunE without referencing
	// the child command directly.
	cmd.PersistentFlags().String("weft-path", "", "internal: injected absolute weft worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("weft-path") //nolint:errcheck
	cmd.PersistentFlags().String("warp-path", "", "internal: injected absolute warp worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("warp-path") //nolint:errcheck

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Guard: skip resolution for the bare "fabric" group, an unknown-subcommand
		// error path, and every topology verb — none of those read this file's
		// closure state, and none of them require a weft worktree to be present.
		if !weftVerbNames[cmd.Name()] {
			return nil
		}

		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		// Read the hidden persistent flags via InheritedFlags to make explicit
		// that these flags are inherited from the parent command, not local.
		injectedWeft, _ := cmd.InheritedFlags().GetString("weft-path")
		injectedWarp, _ := cmd.InheritedFlags().GetString("warp-path")

		if injectedWeft != "" || injectedWarp != "" {
			// Bypass mode: --weft-path and/or --warp-path was injected by the
			// detached push child. Only the push subcommand is valid in this mode;
			// reject everything else to prevent accidental invocation without a
			// worktree context.
			bypass = true
			weftPath = injectedWeft
			warpPath = injectedWarp

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

		// Fabric config is a repo-wide fact on weft:main, not a per-worktree
		// file: load it from the board dir, never from the weft worktree.
		// pathspec scoping below is unchanged — only the config's home moves.
		loadedCfg, err := fabricengine.LoadConfig(hubgeometry.BoardDir(l.Hub))
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

Staging is scoped to the directories listed in the fabric config (default: _lyx _pattern).

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

	// push subcommand: commits then pushes, or in bypass mode pushes directly via --weft-path and/or --warp-path.
	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "commit and push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				// Detached push child: run the loop-until-clean coalescing push
				// over whichever of the injected warpPath/weftPath were
				// supplied, skipping commit entirely. CoalescePushBothAt holds
				// its own absorbing push lock under weftPath's .weft/ for the
				// whole loop and requires weftPath to be non-empty for that
				// lock's home — the detached push child (the only production
				// caller of this bypass) always injects both paths, so that
				// guard is unreachable here in practice.
				if err := fabricengine.CoalescePushBothAt(warpPath, weftPath, fabricengine.SyncOptions{}); err != nil {
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

	// pull subcommand: drives fabricengine.Fabric's unified Pull, fast-forwarding
	// weft, then fetching and inspecting warp, reconciling a rewritten warp
	// history when it is safe to do so.
	pullCmd := &cobra.Command{
		Use:   "pull",
		Short: "pull warp and weft, reconciling a rebased warp",
		Long: `Pulls both sides of the pair. Weft is fast-forwarded first via a plain
git pull. Warp is then fetched and inspected against its upstream tracking ref:

  - A clean fast-forward (local warp HEAD is still an ancestor of the fetched
    upstream tip) simply advances warp — no reconcile needed.
  - A detected warp history rewrite (rebase or force-push upstream, so local
    warp HEAD is no longer an ancestor of the fetched tip) is auto-reconciled
    when it is safe: weft's correspondence is re-anchored to the nearest
    surviving Warp-SHA, warp is reset to the new tip, and a new empty weft
    anchor commit records the fresh correspondence. The result reports which
    post-anchor weft commits touch _pattern/ and need review, since they were
    written against a warp baseline that no longer exists upstream.
  - Two cases abort loudly and make no change to either repo: local warp
    already carries unpushed commits of its own AND the remote diverged (the
    double-conflict case pull refuses to resolve unattended), or the warp
    rewrite is so thorough that no recorded correspondence survives (no safe
    baseline to re-anchor against).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			result, err := fab.Pull(fabricengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, pullResultMap(result)))
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

// pullResultMap converts a fabricengine.PullResult into the map[string]any
// shape pullCmd's RunE surfaces through output.Ok — mirroring the
// map[string]any style the status/commit verbs already use, so the pull
// result reaches the caller through the same one-JSON-object-per-line
// envelope. patternResidueMap flattens each PatternResidueEntry the same way.
func pullResultMap(result fabricengine.PullResult) map[string]any {
	residue := make([]map[string]any, 0, len(result.PatternResidue))
	for _, entry := range result.PatternResidue {
		residue = append(residue, map[string]any{
			"weft_sha": entry.WeftSHA,
			"paths":    entry.Paths,
		})
	}
	return map[string]any{
		"weft_pulled":       result.WeftPulled,
		"warp_fetched":      result.WarpFetched,
		"warp_advanced":     result.WarpAdvanced,
		"new_warp_head":     result.NewWarpHEAD,
		"rewrite_detected":  result.RewriteDetected,
		"reconciled":        result.Reconciled,
		"anchor_warp_sha":   result.AnchorWarpSHA,
		"anchor_weft_sha":   result.AnchorWeftSHA,
		"reanchor_weft_sha": result.ReanchorWeftSHA,
		"pattern_residue":   residue,
	}
}
