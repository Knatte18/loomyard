// cli.go builds the cobra command for the cleanup module and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package cleanupcli

import (
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
It runs from the hub's prime worktree only and refuses elsewhere.

Segments, in order, each calling its owner's own cleanup in-process:
  batten   done runs' records and scratch, and aged orphan scratch (lyx batten cleanup)
  board    done board entries (lyx board prune)
  orch     every handoff but the last one the orchestrator cycled from (lyx orch cleanup)
  scratch  old children of the prime's .scratch/ that no open board entry cites
  logs     trace files past the retention bounds, in the prime and the board
  temp     temp directories left by dead lyx processes

Every segment runs even when an earlier one fails; the failing ones are named in
the error. Nothing is removed that the segment's own verb would refuse to remove.

Example:
  lyx cleanup
  lyx cleanup --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if err := battencli.RefuseUnlessPrime(location); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			results, allSucceeded := runSegments(primeSegments(location), apply)
			fields := map[string]any{"applied": apply, "segments": results}
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
			clihelp.SetExit(ctx, output.ErrFields(out, "cleanup: segments failed: "+strings.Join(failed, ", "), fields))
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "remove what the dry run lists (default is dry-run/report)")
	return cmd
}

// RunCLI is the public seam for the cleanup module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
