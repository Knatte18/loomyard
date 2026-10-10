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
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// websterInFlightWarning is the one warning merge-in reports while a Webster run is mid-flight in this worktree.
const websterInFlightWarning = "Webster is mid-run in this worktree: `lyx webster record-batch` tolerates a clean parent merge commit " +
	"after a fork's commit but refuses a fast-forward past it and a merge whose conflicts were resolved by hand"

// websterInFlightWarnings returns the merge-in warning list for the worktree at l.
// It is nil when no run is in flight;
// a RunInFlight error degrades to nil plus a logged warning, never a failure.
func websterInFlightWarnings(l *lyxcwd.Location) []string {
	inFlight, err := websterengine.RunInFlight(l.AnchorPath())
	if err != nil {
		logger.Warn("fabriccli: merge-in: webster in-flight check failed, emitting no warning", "error", err)
		return nil
	}
	if !inFlight {
		return nil
	}
	logger.Warn("fabriccli: merge-in: merged the parent while a Webster run is in flight")
	return []string{websterInFlightWarning}
}

// setMergeExit maps a merge lifecycle verb's (res, err) pair onto the shared envelope shape — error
// (errWithRecord), conflicts (errConflictsWithRecord), otherwise ok (okWithRecord with committed and
// already_up_to_date) — and sets it as cmd's exit via clihelp.SetExit.
// The three RunE bodies for merge-in, merge --continue, and merge --abort/default all funnel through
// this one dispatch, per the card 16 "one envelope mapping, shared by all modes" requirement.
// continue and abort never populate res.Conflicts, so the conflicts branch is a no-op for them, not a
// behavior change.
//
// warnings rides the ok and conflict envelopes only, under a "warnings" key present only when non-empty;
// a hard failure never carries it.
func setMergeExit(cmd *cobra.Command, out io.Writer, res fabricengine.MergeResult, err error, warnings []string) {
	if err != nil {
		// A failing verb that nevertheless knows which paths are still conflicted reports them under
		// "unresolved", never under "conflicts". The two are different claims and only one of them is
		// the documented discriminator: merge-in's own help promises that a conflict RESULT, and only a
		// conflict result, carries a "conflicts" array, so a script tells a conflict apart from a hard
		// failure by that key alone. Reusing it here would break exactly that test while adding no
		// information the separate key does not carry.
		// The only producer today is MergeContinue's unresolved-conflicts refusal, which an operator
		// reaches by staging some of the reported paths and not all of them — and which named nothing
		// at all before, leaving a weft-side path invisible from the visible worktree.
		if len(res.Conflicts) > 0 {
			clihelp.SetExit(cmd.Context(), errWithRecordFields(out, res.Mutated(), err, map[string]any{
				"unresolved": res.Conflicts,
			}))
			return
		}
		clihelp.SetExit(cmd.Context(), errWithRecord(out, res.Mutated(), err))
		return
	}
	if len(res.Conflicts) > 0 {
		clihelp.SetExit(cmd.Context(), errConflictsWithRecord(out, res.Mutated(), res.Conflicts, warnings))
		return
	}
	fields := map[string]any{
		"committed":          res.Committed,
		"already_up_to_date": res.AlreadyUpToDate,
	}
	if len(warnings) > 0 {
		fields["warnings"] = warnings
	}
	clihelp.SetExit(cmd.Context(), okWithRecord(out, res.Mutated(), fields))
}

// addMergeVerbs registers the "merge", "merge-in" and "merge-stage" subcommands on cmd.
// fabric is a getter closure, not a bound value: weft_verbs.go's PersistentPreRunE assigns the
// resolved *fabricengine.Fabric handle to a local at run time, after cobra has already built and
// registered every command, so a value parameter here would capture that local's nil zero value and
// every merge verb would nil-panic. Each RunE body calls fabric() after PersistentPreRunE has run.
//
// loc is a getter for the same reason: it returns the *lyxcwd.Location PersistentPreRunE resolved, which merge-in reads to ask whether a Webster run is in flight in this worktree.
func addMergeVerbs(cmd *cobra.Command, fabric func() *fabricengine.Fabric, loc func() *lyxcwd.Location) {
	mergeInCmd := &cobra.Command{
		Use:         "merge-in <branch>",
		Args:        cobra.ExactArgs(1),
		Short:       "merge a branch into this worktree, surfacing conflicts",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `merge-in merges <branch> into this pair's own warp and weft checkouts and
surfaces any conflicts here for resolution.

Reach for it in the task worktree, before "lyx fabric merge", to bring the
parent branch in and resolve its conflicts where the work lives.

A conflict leaves the pair mid-merge. Resolve it in three steps, none optional:

  1. edit each path the "conflicts" array listed, removing its markers
  2. lyx fabric merge-stage <those same paths>
  3. lyx fabric merge --continue

or abandon it with "lyx fabric merge --abort". A conflict and a hard failure
both exit 1 with "ok": false; only a conflict result carries the "conflicts"
array of worktree-relative paths.

While a Webster run is in flight in this worktree (its state file exists and
its outcome is absent, paused or stuck), a merge-in that moved HEAD or
conflicted adds a "warnings" array to its envelope; the merge itself proceeds.

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
			var warnings []string
			if err == nil && (!res.AlreadyUpToDate || len(res.Conflicts) > 0) {
				warnings = websterInFlightWarnings(loc())
			}
			setMergeExit(cmd, out, res, err, warnings)
			return nil
		},
	}

	mergeCmd := &cobra.Command{
		Use:         "merge [<branch>]",
		Short:       "merge a branch into this worktree, or continue/abort a merge",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `merge merges <branch> into the pair this worktree opens, squash-capable and
expected conflict-free. It brings the target up to date with its own upstream
first, and aborts itself on any conflict: resolve conflicts in the source
branch's own worktree with "lyx fabric merge-in", then retry merge here.

Reach for it to bring a branch whose conflicts are already resolved into this
worktree's pair, or to conclude or abandon a merge left in progress.

Two forms take no <branch>, only one of two mutually exclusive flags:

  merge --continue   concludes an in-progress merge once every conflict is
                     edited and marked resolved with "lyx fabric merge-stage";
                     while some remain it refuses and lists them in an
                     "unresolved" array
  merge --abort      discards an in-progress merge, restoring both sides to
                     their pre-merge state

--squash applies to the <branch> form only. -m names the conclude-commit, so
it applies to the <branch> form and to --continue, never to --abort.

Example:
  lyx fabric merge my-task --squash
  lyx fabric merge-stage _lyx/raddle/notes.md
  lyx fabric merge --continue
  lyx fabric merge --abort`,
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
				setMergeExit(cmd, out, res, err, nil)
				return nil
			case abortFlag:
				res, err := fabric().MergeAbort()
				setMergeExit(cmd, out, res, err, nil)
				return nil
			default:
				res, err := fabric().Merge(args[0], fabricengine.MergeOptions{Squash: squash, Message: message})
				setMergeExit(cmd, out, res, err, nil)
				return nil
			}
		},
	}
	mergeStageCmd := &cobra.Command{
		Use:         "merge-stage <path>...",
		Args:        cobra.MinimumNArgs(1),
		Short:       "mark conflicted paths resolved so a merge can continue",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
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
