// weft_verbs.go wires the weft-git content-sync verbs (status, commit, push, pull, sync, diff) and
// the merge lifecycle verbs (merge, merge-in, merge-stage) onto the "fabric" parent command built by
// fabric.go.
// addWeftVerbs installs two hidden persistent flags — --weft-path and --warp-path — and a
// PersistentPreRunE scoped to these nine verb names only — the topology verbs built in fabric.go
// resolve their own layout per invocation and never touch this file's closure state.
// The PersistentPreRunE splits normal mode (resolve cwd → layout → config → pathspec → Fabric
// handle) from bypass mode (either hidden path flag injected by the detached push child, push-only
// gate), driving fabricengine.Fabric's Status/Commit/PushWeft/Pull/Diff/MergeIn/Merge/MergeContinue/
// MergeAbort/MergeStageResolved in normal mode and fabricengine.CoalescePushBothAt's loop-until-clean coalescing push
// directly in bypass mode. The merge verbs are registered in merge_verbs.go's addMergeVerbs, which
// reaches the resolved Fabric handle through a getter closure over this file's fab local, since
// PersistentPreRunE assigns it only at run time, after registration.

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
	"status":      true,
	"commit":      true,
	"push":        true,
	"pull":        true,
	"sync":        true,
	"diff":        true,
	"merge":       true,
	"merge-in":    true,
	"merge-stage": true,
}

// addWeftVerbs installs the hidden --weft-path/--warp-path persistent flags,
// the weft-verb-scoped PersistentPreRunE, and the status/commit/push/pull/sync/diff
// subcommands. Normal mode resolves cwd → layout → fabric config → pathspec → Fabric
// handle. Bypass mode (injected --weft-path/--warp-path by detached push) permits
// only push, rejecting others with "subcommand requires a worktree context".
func addWeftVerbs(cmd *cobra.Command) {
	var (
		l        *lyxcwd.Location
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

		_, resolved, err := resolveWarpLocation(ctx)
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		l = resolved

		// Build from fabricengine's own routing set (PathspecNames), never a raw, unfiltered
		// Config.Dirs(): with _lyx no longer in template.yaml's default, a repo's pathspec
		// names only whatever optional directory an operator has added, so a raw-Dirs()
		// sync would silently stop syncing _lyx entirely. The routing set can never name
		// .lyx either, since it excludes structuralNeverCommittedDirs by construction.
		routingNames, err := fabricengine.PathspecNames(fabricengine.BoardDir(l.HubPath))
		if err != nil {
			output.Err(out, err.Error())
			clihelp.Abort(ctx, 1)
			return nil
		}
		pathspec = fabricengine.ScopedPathspec(l.AnchorRel, routingNames)

		resolvedFabric, err := fabricengine.Open(l)
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
		Short: "commit weft changes",
		Long: `Stages changes in the configured pathspec and commits them to the weft worktree.

The commit message is always the fixed string "weft sync" — it is not generated
from changed files and cannot be customized with a flag.

Every fabric weft commit carries a trailing "Warp-SHA: <sha>" trailer naming the
paired warp repo's current HEAD, recorded into the correspondence index immediately
after the commit lands.

Staging is scoped to the durable structural directory (_lyx -- code-injected, never listed
in the fabric config) plus whatever the fabric config's optional pathspec adds (default:
none). The machine-local scratch directory (.lyx) is NEVER staged or committed -- a path
under it in a commit request is refused, not silently dropped.

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
				clihelp.SetExit(cmd.Context(), errWithRecord(out, res.Mutated(), err))
				return nil
			}
			clihelp.SetExit(cmd.Context(), okWithRecord(out, res.Mutated(), map[string]any{"committed": res.WeftCommitted, "sha": res.WeftSHA}))
			return nil
		},
	}

	pushCmd := &cobra.Command{
		Use:   "push",
		Args:  cobra.NoArgs,
		Short: "commit and push weft changes",
		Long: `Commit weft changes exactly as "lyx fabric commit" does, then push the weft
branch's unpushed commits in the same process.

Related commands:
  lyx fabric commit — commit only
  lyx fabric sync   — commit then async-push (detached child process)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				// The bypass branch has no *lyxcwd.Location in scope (it takes the injected
				// --warp-path/--weft-path values and returns before resolveWarpLocation runs), so its
				// recorder is built with an empty hub root: it appends no path-targeted entries of
				// its own, only Extends CoalescePushBothAt's already-converted record, and Extend
				// performs no conversion — the hub root is never consulted on this path.
				rec := fabricengine.NewMutations("")
				res, err := fabricengine.CoalescePushBothAt(warpPath, weftPath, fabricengine.SyncOptions{})
				rec.Extend(res.Mutated())
				if err != nil {
					clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
					return nil
				}
				clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), map[string]any{}))
				return nil
			}

			// The ordinary branch's envelope concatenates the commit call's record with the push
			// call's record, in execution order: a commit that lands followed by a push that fails is
			// "mutated, then errored", and only the combined record makes that visible.
			rec := fabricengine.NewMutations(l.HubPath)
			opts := fabricengine.EnvSyncOptions()
			commitRes, err := fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, opts)
			rec.Extend(commitRes.Mutated())
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			pushRes, err := fab.PushWeft(opts)
			rec.Extend(pushRes.Mutated())
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), map[string]any{}))
			return nil
		},
	}

	pullCmd := &cobra.Command{
		Use:   "pull",
		Args:  cobra.NoArgs,
		Short: "pull warp and weft, reconciling a rebased warp",
		Long: `Pulls both sides of the pair. Weft is fast-forwarded first via a plain
git pull — skipped as a no-op on a freshly bootstrapped hub whose weft branch
has no upstream yet. Warp is then fetched and inspected against its upstream
tracking ref:

  - A clean fast-forward (local warp HEAD is still an ancestor of the fetched
    upstream tip) simply advances warp — no reconcile needed.
  - A detected warp history rewrite (rebase or force-push upstream, so local
    warp HEAD is no longer an ancestor of the fetched tip) is auto-reconciled
    when it is safe: weft's correspondence is re-anchored to the nearest
    surviving Warp-SHA, warp is reset to the new tip, and a new empty weft
    anchor commit records the fresh correspondence. The result reports which
    post-anchor weft commits touch _lyx/PATTERN.md or _lyx/pattern/ and need
    review, since they were written against a warp baseline that no longer
    exists upstream.
  - Two cases abort loudly and make no change to either repo: local warp
    already carries unpushed commits of its own AND the remote diverged (the
    double-conflict case pull refuses to resolve unattended), or the warp
    rewrite is so thorough that no recorded correspondence survives (no safe
    baseline to re-anchor against).
  - A warp worktree with uncommitted tracked changes is refused before warp
    is moved at all — commit or stash, then re-run. Advancing warp goes
    through a hard reset that would silently discard those changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			result, err := fab.Pull(fabricengine.EnvSyncOptions())
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, result.Mutated(), err))
				return nil
			}
			clihelp.SetExit(cmd.Context(), okWithRecord(out, result.Mutated(), pullResultMap(result)))
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Args:  cobra.NoArgs,
		Short: "commit and async-push weft changes",
		Long: `Commit weft changes exactly as "lyx fabric commit" does, then hand the push
to a detached child process and return immediately — the push happens in the
background, coalesced under fabric's push lock.

Related commands:
  lyx fabric commit — commit only
  lyx fabric push   — commit then push in the same process`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			rec := fabricengine.NewMutations(l.HubPath)
			commitRes, err := fab.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.EnvSyncOptions())
			rec.Extend(commitRes.Mutated())
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			weftWorktree := fabricengine.WeftWorktree(l)
			if err := spawnPush(weftWorktree); err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			// The push happens in a detached child process after this one returns, so its outcome is
			// unobservable here: record exactly one KindPushSpawned entry, never branch_pushed, which
			// would assert an outcome this process did not observe.
			rec.Append(fabricengine.KindPushSpawned, weftWorktree, "detached")
			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), map[string]any{}))
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

	addMergeVerbs(cmd, func() *fabricengine.Fabric { return fab })
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
// weft_pulled can now be false inside a SUCCESS envelope — the weft arm is non-fatal, so a false
// value here means the warp side pulled while the weft did not; the operator's remedy is to
// reconcile the weft by hand (`git -C <weft> reset --hard origin/<branch>`).
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
