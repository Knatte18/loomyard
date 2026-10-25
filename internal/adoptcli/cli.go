// cli.go builds the cobra command for the adopt module and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package adoptcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the adopt module.
func Command() *cobra.Command {
	var req request
	cmd := &cobra.Command{
		Use:         "adopt [<slug>...]",
		Short:       "take over tasks, beams or the prime from another machine",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Records this machine as the owner of the named tasks and beams, and of the prime with
--prime, and carries on their runs here. A beam is a project entry whose beam flag is set;
adopting it moves the beam only, its members keeping their own owners. --all names the
prime, every task that is not done and has an owner or a run status, and every open beam
with an owner. It runs from the hub's prime worktree and from any beam.

Steps, in order:
  1. a moving origin reconcile of the hub; a stop on Bolt or the prime's records branch,
     an unreachable origin or a reconcile error refuses the verb and runs nothing
  2. an entry whose pair holds local commits origin lacks, or whose pair is live, stops
     with its branches and both ways forward; --keep-local records the owner over them,
     --drop-local keeps each local tip under an archive/<slug>/<tip> tag and moves the
     branches to origin's (refused for a live pair)
  3. a refusal, before any step writes, of an entry whose parent another machine owns,
     and of a beam or the prime while a member another machine owns is left out; each
     names the call that adopts them together
  4. the pair of a task this machine lacks is added from the prime
  5. one owner write for every entry not stopped, pushed to origin
  6. each owner pause of an adopted task is lifted
  7. each adopted task whose batten run is not done is resumed as a detached first pass;
     a child halted at Finalize's owner check is resumed in its worktree
  8. the dobby of the prime and of every beam this machine owns is started when it is not live
  9. once an owner is recorded, the finalize gate starts detached in the prime; red only reports

The envelope carries reconcile, mutations and partial, and per slug what happened,
dobby_warning when a dobby did not start, and suite or suite_warning. A slug's error never
stops the other slugs.

Example:
  lyx adopt fix-parser
  lyx adopt --prime
  lyx adopt --all --keep-local`,
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
			prime, err := battencli.ResolveBattenPrime(location)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			deps, err := hubDeps(prime)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			req.Slugs = args
			result, err := adopt(deps, req)
			fields := map[string]any{
				"reconcile":      result.Reconcile,
				"mutations":      result.Mutations,
				"partial":        result.Partial,
				"slugs":          result.Slugs,
				"prime_recorded": result.PrimeRecorded,
			}
			if result.PrimeLost != "" {
				fields["prime_lost_to"] = result.PrimeLost
			}
			if result.DobbyWarning != "" {
				fields["dobby_warning"] = result.DobbyWarning
			}
			if result.Suite != nil {
				fields["suite"] = result.Suite
			}
			if result.SuiteWarning != "" {
				fields["suite_warning"] = result.SuiteWarning
			}
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), fields))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, fields))
			return nil
		},
	}
	cmd.Flags().BoolVar(&req.Prime, "prime", false, "record this machine as the prime's owner")
	cmd.Flags().BoolVar(&req.All, "all", false, "adopt the prime, every task that is not done and has an owner or a run status, and every open owned beam")
	cmd.Flags().BoolVar(&req.KeepLocal, "keep-local", false, "record the owner over local commits origin lacks and over a live pair")
	cmd.Flags().BoolVar(&req.DropLocal, "drop-local", false, "keep each local tip origin lacks under an archive tag and move the branches to origin's")
	return cmd
}

// RunCLI is the public seam for the adopt module CLI, delegating to clihelp.Execute for output capture.
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
