// merge_verbs.go wires the fabric merge lifecycle verbs (merge, merge-in, merge-stage) onto the
// "fabric" parent command built by fabric.go.
// merge-stage is the CLI surface for the engine's MergeStageResolved, and it is load-bearing rather
// than a convenience wrapper over `git add`: MergeContinue gates on the git index, and a conflict
// under a wired junction name (every _lyx/… conflict) cannot be staged from the visible worktree at
// all, since git refuses to stage through the junction. Without it a merge whose conflicts land on
// the weft side is uncompletable through the CLI.
// All three verbs join the weft-verb family: weft_verbs.go's weftVerbNames set and PersistentPreRunE
// resolve the pair handle they need exactly as commit/pull do, so addMergeVerbs reaches that handle
// indirectly through a getter closure rather than a value captured at registration time.
// Flag combinations that would be silently ignored rather than obeyed (--squash or -m alongside
// --abort) are rejected in a pre-flight check, before any handle is touched.

package fabriccli

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// setMergeExit maps a merge lifecycle verb's (res, err) pair onto the shared envelope shape — error
// (errWithRecord), conflicts (errConflictsWithRecord), otherwise ok (okWithRecord with committed and
// already_up_to_date) — and sets it as cmd's exit via clihelp.SetExit.
// The three RunE bodies for merge-in, merge --continue, and merge --abort/default all funnel through
// this one dispatch, per the card 16 "one envelope mapping, shared by all modes" requirement.
// continue and abort never populate res.Conflicts, so the conflicts branch is a no-op for them, not a
// behavior change.
func setMergeExit(cmd *cobra.Command, out io.Writer, res fabricengine.MergeResult, err error) {
	if err != nil {
		clihelp.SetExit(cmd.Context(), errWithRecord(out, res.Mutated(), err))
		return
	}
	if len(res.Conflicts) > 0 {
		clihelp.SetExit(cmd.Context(), errConflictsWithRecord(out, res.Mutated(), res.Conflicts))
		return
	}
	clihelp.SetExit(cmd.Context(), okWithRecord(out, res.Mutated(), map[string]any{
		"committed":          res.Committed,
		"already_up_to_date": res.AlreadyUpToDate,
	}))
}

// addMergeVerbs registers the "merge", "merge-in" and "merge-stage" subcommands on cmd.
// fabric is a getter closure, not a bound value: weft_verbs.go's PersistentPreRunE assigns the
// resolved *fabricengine.Fabric handle to a local at run time, after cobra has already built and
// registered every command, so a value parameter here would capture that local's nil zero value and
// every merge verb would nil-panic. Each RunE body calls fabric() after PersistentPreRunE has run.
func addMergeVerbs(cmd *cobra.Command, fabric func() *fabricengine.Fabric) {
	mergeInCmd := &cobra.Command{
		Use:   "merge-in <branch>",
		Args:  cobra.ExactArgs(1),
		Short: "merge a branch into this worktree, surfacing conflicts",
		Long: `merge-in (the engine's MergeIn) is the workflow step that runs in the task
worktree, before "lyx fabric merge": it merges <branch> into this pair's own
warp and weft checkouts and surfaces any conflicts here for resolution.

Conflicts are a result, not a failure to run again differently: a merge-in
that conflicts leaves the pair mid-merge, concluded once every conflict is
resolved, or abandoned with "lyx fabric merge --abort" (the engine's
MergeAbort).

Resolving takes three steps, and the middle one is not optional:

  1. edit each path the "conflicts" array listed, removing its markers
  2. lyx fabric merge-stage <those same paths>
  3. lyx fabric merge --continue   (the engine's MergeContinue)

Step 2 exists because --continue gates on the git INDEX, not on file content,
so editing a file is not by itself enough to let the merge conclude. For a path
under a fabric-managed directory (anything reached through a wired junction,
such as _lyx/) "git add" cannot do it at all — git refuses to stage through the
junction — so merge-stage is the only route.

A conflict result and a hard failure both exit 1 with "ok": false, so a
script must not tell them apart by exit status. The discriminator is the
envelope: a conflict result, and only a conflict result, carries a
"conflicts" array of worktree-relative paths. This lifecycle is shared with "lyx fabric merge" — both
verbs continue and abort the same way — but the two verbs are not symmetric:
merge-in resolves conflicts in this worktree, merge does not.

Example:
  lyx fabric merge-in my-task
  lyx fabric merge-stage _lyx/raddle/notes.md src/app.txt
  lyx fabric merge --continue`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			res, err := fabric().MergeIn(args[0])
			setMergeExit(cmd, out, res, err)
			return nil
		},
	}

	var mergeCmd *cobra.Command
	mergeCmd = &cobra.Command{
		Use:   "merge (<branch> | --continue | --abort) [--squash] [-m <message>]",
		Short: "merge a branch into this worktree, or continue/abort a merge",
		Long: `merge (the engine's Merge) merges <branch> into the target pair this
worktree opens a handle on — squash-capable, expected conflict-free. It
synchronizes the target to its own upstream first, and self-aborts to
"the engine's ErrMergeInRequired" on any conflict: conflict resolution
belongs in the source branch's own worktree, so run "lyx fabric merge-in"
there first, then retry "lyx fabric merge" here.

--continue (the engine's MergeContinue) concludes an in-progress merge once
every conflict has been resolved in the worktree AND marked resolved with
"lyx fabric merge-stage" — it gates on the git index, not on file content, so
editing the files alone leaves it refusing. --abort (the engine's
MergeAbort) discards an in-progress merge, restoring both sides to their
pre-merge state. --continue and --abort are mutually exclusive, and neither
takes a positional branch argument; --squash applies only to the default
merge mode and is rejected alongside either of them. -m gives the
conclude-commit its message, so it applies to the default mode and to
--continue, and is rejected alongside --abort, which has no commit to name.

Example:
  lyx fabric merge-in my-task
  lyx fabric merge my-task --squash
  lyx fabric merge-stage _lyx/raddle/notes.md
  lyx fabric merge --continue`,
		Args: func(cmd *cobra.Command, args []string) error {
			continueFlag, _ := cmd.Flags().GetBool("continue")
			abortFlag, _ := cmd.Flags().GetBool("abort")
			if continueFlag || abortFlag {
				if len(args) != 0 {
					return fmt.Errorf("usage: lyx fabric merge (--continue | --abort) takes no positional arguments")
				}
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("usage: lyx fabric merge <branch> [--squash] [-m <message>]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			squash, _ := cmd.Flags().GetBool("squash")
			continueFlag, _ := cmd.Flags().GetBool("continue")
			abortFlag, _ := cmd.Flags().GetBool("abort")
			message, _ := cmd.Flags().GetString("message")

			if squash && (continueFlag || abortFlag) {
				// Nothing was mutated at this pre-flight validation point, so a bare output.Err
				// carries no record — the pre-flight carve-out.
				clihelp.SetExit(cmd.Context(), output.Err(out, "usage: --squash cannot be combined with --continue or --abort"))
				return nil
			}
			// -m is the message override for the conclude-commit, so it is meaningful for --continue
			// and meaningless for --abort, which discards it. Rejecting it is the same pre-flight
			// carve-out as --squash above: silently ignoring a flag the caller passed is worse than
			// refusing it, since the caller cannot tell the difference from a message that landed.
			if abortFlag && message != "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "usage: -m cannot be combined with --abort"))
				return nil
			}

			switch {
			case continueFlag:
				res, err := fabric().MergeContinue(message)
				setMergeExit(cmd, out, res, err)
				return nil
			case abortFlag:
				res, err := fabric().MergeAbort()
				setMergeExit(cmd, out, res, err)
				return nil
			default:
				res, err := fabric().Merge(args[0], fabricengine.MergeOptions{Squash: squash, Message: message})
				setMergeExit(cmd, out, res, err)
				return nil
			}
		},
	}
	mergeStageCmd := &cobra.Command{
		Use:   "merge-stage <path>...",
		Args:  cobra.MinimumNArgs(1),
		Short: "mark conflicted paths resolved so a merge can continue",
		Long: `merge-stage (the engine's MergeStageResolved) marks conflicted paths as
resolved, taking the same worktree-relative paths the "conflicts" array of a
merge-in or merge envelope reported.

It is a required step, not a convenience. "lyx fabric merge --continue" gates on
the git INDEX, not on file content, so editing a conflicted file to remove its
markers does not by itself let the merge conclude — the path has to be marked
resolved. For a path in the ordinary source tree "git add" does that. For a path
under one of the fabric-managed directories (anything reached through a wired
junction, such as _lyx/) it cannot: git refuses to stage through the junction
with "pathspec is beyond a symbolic link", and this verb is the only way to
resolve such a conflict.

Pass the paths exactly as the conflicts array reported them. A path that is not
conflicted is an error rather than a silent skip, and no path is staged unless
every path passed is conflicted, so a typo fails the whole call instead of
leaving the merge half-resolved. Deleting the file is a legitimate resolution of
a delete/modify conflict and stages as a removal.

Example:
  lyx fabric merge-in my-task
  # edit each path the "conflicts" array listed, resolving the markers
  lyx fabric merge-stage _lyx/raddle/notes.md src/app.txt
  lyx fabric merge --continue`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			res, err := fabric().MergeStageResolved(args)
			if err != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, res.Mutated(), err))
				return nil
			}
			// The staged echo deduplicates (order-preserving) rather than repeating args verbatim:
			// the engine tolerates a path passed twice, but an envelope claiming two stagings for one
			// path would be reporting something that did not happen twice.
			clihelp.SetExit(cmd.Context(), okWithRecord(out, res.Mutated(), map[string]any{
				"staged": uniquePreservingOrder(args),
			}))
			return nil
		},
	}

	mergeCmd.Flags().Bool("squash", false, "squash the merge into a single commit on each side")
	mergeCmd.Flags().Bool("continue", false, "conclude an in-progress merge once conflicts are resolved")
	mergeCmd.Flags().Bool("abort", false, "discard an in-progress merge, restoring both sides")
	mergeCmd.Flags().StringP("message", "m", "", "commit message for the merge commit")
	mergeCmd.MarkFlagsMutuallyExclusive("continue", "abort")

	cmd.AddCommand(mergeInCmd, mergeCmd, mergeStageCmd)
}

// uniquePreservingOrder returns values with every duplicate after a value's first occurrence
// dropped, keeping the first-occurrence order — the shape merge-stage's staged echo reports.
func uniquePreservingOrder(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		unique = append(unique, v)
	}
	return unique
}
