// weft_verbs.go wires the weft-git content-sync verbs (status, commit, push, pull, sync, diff) and
// the merge lifecycle verbs (merge, merge-in, merge-stage) onto the "fabric" parent command built by
// fabric.go.
// addWeftVerbs installs two hidden persistent flags — --weft-path and --warp-path — and a
// PersistentPreRunE scoped to these nine verb names only — the topology verbs built in fabric.go
// resolve their own layout per invocation and never touch this file's closure state.
// The pre-run opens with the sandboxed-role guard of roleguard.go, which covers every subcommand.
// The PersistentPreRunE splits normal mode (resolve cwd → layout → config → pathspec → Fabric
// handle) from bypass mode (either hidden path flag injected by the detached push child, push-only
// gate), driving fabricengine.Fabric's Status/Commit/Pull/Diff/MergeIn/Merge/MergeContinue/
// MergeAbort/MergeStageResolved in normal mode and fabricengine.CoalescePushRequest's loop-until-clean coalescing push
// directly in bypass mode. The merge verbs are registered in merge_verbs.go's addMergeVerbs, which
// reaches the resolved Fabric handle through a getter closure over this file's fab local, since
// PersistentPreRunE assigns it only at run time, after registration.
// push and sync both push the code side and the records side, except in the hub's prime, where they push the records side only:
// push in-process through fabricengine.PushPairAnchored, sync through a detached child that re-enters bypass mode.

package fabriccli

import (
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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
		l        *lymcwd.Location
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
	cmd.PersistentFlags().String(fabricengine.PauseSlugFlag, "", "internal: the slug the detached push child's network calls are held under, empty for the prime")
	cmd.PersistentFlags().MarkHidden(fabricengine.PauseSlugFlag) //nolint:errcheck
	cmd.PersistentFlags().String(fabricengine.TeardownSlugFlag, "", "internal: the pair whose teardown the detached push child yields to")
	cmd.PersistentFlags().MarkHidden(fabricengine.TeardownSlugFlag) //nolint:errcheck
	cmd.PersistentFlags().Bool(fabricengine.ParentLockedFlag, false, "internal: the detached push child's pair held its parent lock when the child was spawned")
	cmd.PersistentFlags().MarkHidden(fabricengine.ParentLockedFlag) //nolint:errcheck

	fabricCmd := cmd
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// The role guard runs first, ahead of the topology verbs' early return.
		// It runs only under a parent: configcli's in-process sync runs Command() as its own root and passes.
		if fabricCmd.HasParent() && cmd != fabricCmd {
			if refusal := sandboxedRoleRefusal(cmd, os.Getenv(agentname.StrandNameEnv)); refusal != "" {
				output.Err(cmd.OutOrStdout(), refusal)
				clihelp.Abort(cmd.Context(), 1)
				return nil
			}
		}

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

		// Build from fabricengine's own routing set (PathspecNames), never a raw, unfiltered Config.Dirs(): with _lym no longer in template.yaml's default, a repo's pathspec names only whatever optional directory an operator has added, so a raw-Dirs() sync would silently stop syncing _lym entirely.
		// The routing set can never name .lym either, since it excludes structuralNeverCommittedDirs by construction.
		routingNames, err := fabricengine.PathspecNames(fabricengine.ConfigDirs(l))
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
		Use:         "status",
		Args:        cobra.NoArgs,
		Short:       "show unified warp+weft uncommitted-change status",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Reports every currently-uncommitted path across both sides of the
warp<->weft pair, each labelled with which side (warp or weft) it changed on.

status also reports merge_in_progress: whether THIS pair has a fabric merge
parked awaiting "lym fabric merge --continue" or "lym fabric merge --abort".
It does NOT report whether some other pair in the hub is mid-merge on this
pair's branch, so "lym fabric remove" refusing on that hub-wide condition can
coexist with merge_in_progress being false here.

status also reports failed_pushes: every push origin refused that this machine
still holds a record of, hub-wide, each with its repository side, branch, class,
last error, first and last seen, count and the way forward for its class.
An empty list means no push is outstanding.`,
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
			inProgress, err := fab.MergeInProgress()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			failedPushes, err := fabricengine.FailedPushFields(l.HubPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"changes":           changeEntriesMap(entries),
				"merge_in_progress": inProgress,
				"failed_pushes":     failedPushes,
			}))
			return nil
		},
	}

	commitCmd := &cobra.Command{
		Use:         "commit",
		Args:        cobra.NoArgs,
		Short:       "commit weft changes",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Stages changes in the configured pathspec and commits them to the weft worktree.

The commit message is always the fixed string "weft sync" — it is not generated
from changed files and cannot be customized with a flag.

Every fabric weft commit carries a trailing "Warp-SHA: <sha>" trailer naming the
paired warp repo's current HEAD, recorded into the correspondence index immediately
after the commit lands.

Staging is scoped to the durable structural directory (_lym -- code-injected, never listed
in the fabric config) plus whatever the fabric config's optional pathspec adds (default:
none). The machine-local scratch directory (.lym) is NEVER staged or committed -- a path
under it in a commit request is refused, not silently dropped.

Related commands:
  lym fabric push   — commit then push in the same process
  lym fabric sync   — commit then async-push (detached child process)`,
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
		Use:         "push",
		Args:        cobra.NoArgs,
		Short:       "commit weft changes and push both sides (records only in the prime)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Commit weft changes exactly as "lym fabric commit" does, then push the unpushed
commits of both the warp branch and the weft branch in the same process.
The push is plain and rebase-free, never a force, and waits for a push already
running under fabric's push lock.
A side the remote rejects is reported as an error naming that side, after the
other side has been pushed.
In the hub's prime only the records side is pushed, because the prime's code
branch is the operator's parent branch: when it has unpushed commits the
envelope carries code_push_skipped, naming the branch, and the operator pushes it.

Related commands:
  lym fabric commit — commit only
  lym fabric sync   — commit then async-push (detached child process)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if bypass {
				// The bypass branch has no *lymcwd.Location in scope (it takes the injected
				// --warp-path/--weft-path values and returns before resolveWarpLocation runs), so its
				// recorder is built with an empty hub root: it appends no path-targeted entries of
				// its own, only Extends CoalescePushRequest's already-converted record, and Extend
				// performs no conversion — the hub root is never consulted on this path.
				rec := fabricengine.NewMutations("")
				pauseSlug, _ := cmd.Flags().GetString(fabricengine.PauseSlugFlag)
				teardownSlug, _ := cmd.Flags().GetString(fabricengine.TeardownSlugFlag)
				parentLocked, _ := cmd.Flags().GetBool(fabricengine.ParentLockedFlag)
				// The child starts in the hub's board worktree, so the hub is the parent of the worktree root the ungated resolution finds there.
				cwd, err := lymcwd.CwdFrom(cmd.Context())
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				hubLocation, err := lymcwd.ResolveWorktree(cwd)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
				logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubLocation.HubPath))
				res, err := fabricengine.DetachedPush(hubLocation.HubPath, fabricengine.PushSpawn{
					WarpPath:     warpPath,
					WeftPath:     weftPath,
					PauseSlug:    pauseSlug,
					TeardownSlug: teardownSlug,
					ParentLocked: parentLocked,
				})
				rec.Extend(res.Mutated())
				if err != nil {
					// The detached child's stdout goes nowhere, so the trace log is the only place its failure surfaces.
					logger.Warn("fabriccli: detached push failed; the commits stay local for the pair's next push", "warp_path", warpPath, "weft_path", weftPath, "error", err)
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
			pushRes, err := fabricengine.PushPairAnchored(l, opts, fabricengine.LockWaitUnbounded)
			rec.Extend(pushRes.Mutated())
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			fields := map[string]any{}
			if pushRes.CodePushSkipped != "" {
				fields["code_push_skipped"] = pushRes.CodePushSkipped
			}
			if len(pushRes.LocalSkipped) > 0 {
				fields["local_skipped"] = pushRes.LocalSkipped
			}
			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), fields))
			return nil
		},
	}

	pullCmd := &cobra.Command{
		Use:         "pull",
		Args:        cobra.NoArgs,
		Short:       "pull warp and weft, reconciling a rebased warp",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Pulls both sides of the pair. Weft is brought level with origin first:
fast-forwarded when it can be, and otherwise its local commits are replayed
onto origin's, the commits of a run origin has ended being kept under a local
tag instead. A weft with no upstream yet, as on a freshly bootstrapped hub, is
skipped as a no-op. A replay that conflicts, or an uncommitted change that
blocks the move, still lets warp run, and the pull then fails naming the weft
branch, the paths and the way forward, with the warp result kept. Warp is then
fetched and inspected against its upstream tracking ref:

  - A clean fast-forward (local warp HEAD is still an ancestor of the fetched
    upstream tip) simply advances warp — no reconcile needed.
  - A detected warp history rewrite (rebase or force-push upstream, so local
    warp HEAD is no longer an ancestor of the fetched tip) is auto-reconciled
    when it is safe: weft's correspondence is re-anchored to the nearest
    surviving Warp-SHA, warp is reset to the new tip, and a new empty weft
    anchor commit records the fresh correspondence. The result reports the
    re-anchor baseline (anchor_warp_sha, anchor_weft_sha) and the new anchor
    commit (reanchor_weft_sha).
  - Three cases abort loudly and make no change to warp: local warp already
    carries a commit no push of this machine reached AND the remote diverged
    (the double-conflict case pull refuses to resolve unattended), the warp
    rewrite is so thorough that no recorded correspondence survives (no safe
    baseline to re-anchor against), or origin rewound a task branch past
    commits this checkout holds (the refusal names the two ways to settle it).
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
		Use:         "sync",
		Args:        cobra.NoArgs,
		Short:       "commit weft changes and async-push both sides (records only in the prime)",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Commit weft changes exactly as "lym fabric commit" does, then hand the push
of both the warp branch and the weft branch to a detached child process and
return immediately — the push happens in the background, coalesced under
fabric's push lock.
In the hub's prime only the weft branch is handed over, because the prime's
code branch is the operator's parent branch to push.
A branch marked local-only is left out, and the envelope's local_skipped names it.

Related commands:
  lym fabric commit — commit only
  lym fabric push   — commit then push in the same process`,
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
			spawn, err := spawnPush(l)
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), err))
				return nil
			}
			// The push happens in a detached child process after this one returns,
			// so its outcome is unobservable here:
			// record one KindPushSpawned entry per side handed to the child, never branch_pushed, which would assert an outcome this process did not observe.
			if spawn.Request.WarpPath != "" {
				rec.Append(fabricengine.KindPushSpawned, spawn.Request.WarpPath, "detached")
			}
			if spawn.Request.WeftPath != "" {
				rec.Append(fabricengine.KindPushSpawned, spawn.Request.WeftPath, "detached")
			}
			fields := map[string]any{}
			if len(spawn.LocalSkipped) > 0 {
				fields["local_skipped"] = spawn.LocalSkipped
			}
			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), fields))
			return nil
		},
	}

	diffCmd := &cobra.Command{
		Use:         "diff <since-warp-sha>",
		Short:       "show unified warp+weft diff since a warp SHA",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Reports what changed on both sides of the warp<->weft pair since the given
warp SHA: warp-side changes are <since-warp-sha>..HEAD in the warp repo, and
weft-side changes are computed against the nearest recorded weft
correspondence at or before that warp SHA.

If <since-warp-sha> predates any recorded correspondence, the weft side is
empty and the result's no_weft_correspondence field is true.

Example:
  lym fabric diff abc1234`,
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

	addMergeVerbs(cmd, func() *fabricengine.Fabric { return fab }, func() *lymcwd.Location { return l })
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
// expects.
// weft_pulled is false inside a SUCCESS envelope only when the weft's origin could not be reached, so the warp side pulled while the weft did not.
// Re-running the pull once origin is reachable settles it.
// A weft that a conflict or an uncommitted change keeps from being brought level is an error envelope naming the way forward, never a false field.
func pullResultMap(result fabricengine.PullResult) map[string]any {
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
	}
}
