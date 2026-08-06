// weft_verbs.go wires the weft-git content-sync verbs (status, commit, push, pull,
// sync, diff) onto the "fabric" parent command built by fabric.go. addWeftVerbs
// installs two hidden persistent flags — --weft-path and --warp-path — and a
// PersistentPreRunE scoped to these six verb names only — the topology verbs built
// in fabric.go resolve their own layout per invocation and never touch this file's
// closure state. The PersistentPreRunE splits normal mode (resolve cwd → layout →
// config → pathspec → Fabric handle) from bypass mode (either hidden path flag
// injected by the detached push child, push-only gate), driving fabricengine.Fabric's
// Status/Commit/PushWeft/Pull/Diff in normal mode and
// fabricengine.CoalescePushBothAt's loop-until-clean coalescing push directly in
// bypass mode.

package fabriccli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// weftVerbNames is the set of leaf commands needing PersistentPreRunE resolution.
// Topology verbs resolve independently.
var weftVerbNames = map[string]bool{
	"status": true,
	"commit": true,
	"push":   true,
	"pull":   true,
	"sync":   true,
	"diff":   true,
}

// addWeftVerbs installs the hidden --weft-path/--warp-path persistent flags,
// the weft-verb-scoped PersistentPreRunE, and the status/commit/push/pull/sync/diff
// subcommands. Normal mode resolves cwd → layout → fabric config → pathspec → Fabric
// handle. Bypass mode (injected --weft-path/--warp-path by detached push) permits
// only push, rejecting others with "subcommand requires a worktree context".
func addWeftVerbs(cmd *cobra.Command) {
	var (
		l        *lyxcwd.Location
		cfg      fabricengine.Config
		pathspec []string
		fab      *fabricengine.Fabric
		bypass   bool
		weftPath string
		warpPath string
	)

	cmd.PersistentFlags().String("weft-path", "", "internal: injected absolute weft worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("weft-path") //nolint:errcheck
	cmd.PersistentFlags().String("warp-path", "", "internal: injected absolute warp worktree path for the detached push child")
	cmd.PersistentFlags().MarkHidden("warp-path") //nolint:errcheck

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if !weftVerbNames[cmd.Name()] {
			return nil
		}

		ctx := cmd.Context()
		out := cmd.OutOrStdout()

		injectedWeft, _ := cmd.InheritedFlags().GetString("weft-path")
		injectedWarp, _ := cmd.InheritedFlags().GetString("warp-path")

		if injectedWeft != "" || injectedWarp != "" {
			bypass = true
			weftPath = injectedWeft
			warpPath = injectedWarp

			if cmd.Name() != "push" {
				output.Err(out, "subcommand requires a worktree context")
				clihelp.Abort(ctx, 1)
				return nil
			}
			return nil
		}

		cwd, err := lyxcwd.Getwd()
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}

		resolved, err := lyxcwd.Resolve(cwd)
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		l = resolved

		loadedCfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(l.HubPath))
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		cfg = loadedCfg

		pathspec = fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())

		resolvedFabric, err := fabricengine.New(l.WorktreePath(), fabricengine.WeftWorktree(l))
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		fab = resolvedFabric

		return nil
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "show unified warp+weft uncommitted-change status",
		Long: `Reports every currently-uncommitted path across both sides of the
warp<->weft pair, each labelled with which side (warp or weft) it changed on.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			entries, err := fab.Status()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"changes": changeEntriesMap(entries)}))
			return nil
		},
	}

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
			res, err := fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"committed": res.WeftCommitted, "sha": res.WeftSHA}))
			return nil
		},
	}

	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "commit and push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				if err := fabricengine.CoalescePushBothAt(warpPath, weftPath, fabricengine.SyncOptions{}); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
				return nil
			}

			opts := fabricengine.EnvSyncOptions()
			if _, err := fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, opts); err != nil {
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

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "commit and async-push weft changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			if _, err := fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.EnvSyncOptions()); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := spawnPush(fabricengine.WeftWorktree(l)); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{}))
			return nil
		},
	}

	diffCmd := &cobra.Command{
		Use:   "diff <since-warp-sha>",
		Short: "show unified warp+weft diff since a warp SHA",
		Long: `Reports what changed on both sides of the warp<->weft pair since the given
warp SHA: warp-side changes are <since-warp-sha>..HEAD in the warp repo, and
weft-side changes are computed against the nearest recorded weft
correspondence at or before that warp SHA.

If <since-warp-sha> predates any recorded correspondence, the weft side is
empty and the result's no_weft_correspondence field is true.

Example:
  lyx fabric diff abc1234`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			res, err := fab.Diff(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"entries":                changeEntriesMap(res.Entries),
				"no_weft_correspondence": res.NoWeftCorrespondence,
			}))
			return nil
		},
	}

	cmd.AddCommand(statusCmd, commitCmd, pushCmd, pullCmd, syncCmd, diffCmd)
}

// changeEntriesMap flattens a []fabricengine.ChangeEntry into the map shape
// output.Ok's JSON envelope expects.
func changeEntriesMap(entries []fabricengine.ChangeEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{"path": e.Path, "side": string(e.Side)})
	}
	return out
}

// pullResultMap converts a fabricengine.PullResult into the map shape output.Ok
// expects, flattening each PatternResidueEntry the same way.
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
