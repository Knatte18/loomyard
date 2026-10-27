// cli.go builds the cobra command for the cleanup module and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package cleanupcli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the cleanup module.
func Command() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:         "cleanup",
		Short:       "remove the hub prime's routine leftovers: finished batten runs, done board entries, old handoffs, scratch, logs and temp",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Without --apply, cleanup is a dry run: it lists, per segment, what --apply would
remove, prints "applied": false and changes nothing. With --apply it removes it.
It runs from the hub's prime worktree and from any beam, and refuses elsewhere.

Before its segments it reconciles the hub's branches against origin and prints the
report as "reconcile", beside "mutations" and "partial". The dry run only judges:
it moves, replays and tears down nothing, no tracking ref included, and the report
names each move and teardown that --apply's segments account for. --apply moves the
branches and tears down the pairs of runs origin has ended, then runs the segments
over the reconciled state. A stop on Bolt or the prime's records branch, or a
reconcile error, refuses --apply with the report and runs no segment; a stop on a
pair is only reported, and an unreachable origin is a warning.

Segments, in order, each calling its owner's own cleanup in-process:
  batten   done runs' records and scratch, and aged orphan scratch (lym batten cleanup);
           push_spawned says its records push was started, not that origin took it
  board    done board entries (lym board prune)
  orch     every handoff but the last one the orchestrator cycled from
  scratch  old children of the prime's .scratch/ that no open board entry cites
  logs     trace files past the retention bounds, in the prime and the board
  temp     temp directories left by dead lym processes

Every segment runs even when an earlier one fails; the failing ones are named in
the error. Nothing is removed that the segment's own verb would refuse to remove.

failed_pushes lists every push origin refused that this machine still holds a
record of, each with the way forward for its class; it prints with and without --apply.

Example:
  lym cleanup
  lym cleanup --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err = battencli.ResolveBattenPrime(location)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			report, reconcileErr := reconcileHub(location, apply)
			fields := map[string]any{"applied": apply, "reconcile": report, "mutations": report.Mutations.Entries(), "partial": false}
			// An error envelope reports what the reconcile changed, so a partial record is never lost.
			fail := func(message string) error {
				fields["partial"] = report.Mutations.Len() > 0
				clihelp.SetExit(ctx, output.ErrFields(out, message, fields))
				return nil
			}
			if report.Unreachable {
				fields["warning"] = "origin was unreachable, so the hub's branches were neither judged against it nor moved"
			}
			failedPushes, err := fabricengine.FailedPushFields(location.HubPath)
			if err != nil {
				return fail(err.Error())
			}
			fields["failed_pushes"] = failedPushes
			if apply {
				if reconcileErr != nil {
					return fail(fmt.Sprintf("cleanup --apply: the origin reconcile failed, so no segment ran: %v; repair what the error names, then re-run lym cleanup --apply from the prime", reconcileErr))
				}
				if stops := sharedStopText(report); stops != "" {
					return fail("cleanup --apply: the origin reconcile stopped, so no segment ran: " + stops)
				}
			} else if reconcileErr != nil {
				fields["reconcile_error"] = reconcileErr.Error()
			}

			results, allSucceeded := runSegments(primeSegments(location), apply)
			fields["segments"] = results
			if allSucceeded {
				clihelp.SetExit(ctx, output.Ok(out, fields))
				return nil
			}
			var failed []string
			for _, result := range results {
				if result.Error != "" {
					failed = append(failed, result.Name)
				}
			}
			return fail("cleanup: segments failed: " + strings.Join(failed, ", "))
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "remove what the dry run lists (default is dry-run/report)")
	return cmd
}

// RunCLI is the public seam for the cleanup module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
