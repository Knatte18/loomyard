// cli.go builds the cobra command for the adopt module and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package adoptcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the adopt module.
func Command() *cobra.Command {
	var req request
	cmd := &cobra.Command{
		Use:   "adopt [<subject>...]",
		Short: "take over beam projects, or root, and the runs under them from another machine",
		// ArbitraryArgs lets a standalone RunCLI, where this command is the cobra root, reach RunE with a subject instead of cobra's own unknown-command error beside the check subcommand.
		Args:        cobra.ArbitraryArgs,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Records this machine as the owner of each subject named, a beam or the word root, and
carries on the runs under it here. A beam is a project entry whose beam flag is set: it
moves with its pair and the runs whose parent is its worktree; a nested beam keeps its owner. root moves the prime and the runs whose parent is the prime. A task
is never named: it moves with its beam or with root. --all names root and every open
beam. It runs from the prime and from any beam.

Steps, in order:
  1. a moving origin reconcile; a stop on Bolt or the prime's records branch, an
     unreachable origin or a reconcile error refuses the verb
  2. a beam or run whose pair holds local commits origin lacks, or is live, stops with its
     branches and both ways forward; --keep-local records the owner over them, --drop-local
     archives each local tip as archive/<slug>/<tip> and moves the branches to origin's
     (refused for a live pair); a project one of whose runs stops is held back whole
  3. the pair of a beam or run this machine lacks is added
  4. one owner write for every beam not stopped, and for the prime with root
  5. each owner pause of an adopted beam and of a moved run is lifted
  6. each moved run not done that this machine now owns is resumed as a detached first
     pass, a beam's pawl run only when this machine owns its parent; a child halted at
     Finalize's owner check is resumed
  7. the beam steps of the prime with root and of each adopted beam, as "lym beam start"
     runs them: its dobby and orch; a beam whose landing holds it gets none
  8. the adopt check starts detached in the prime and reports each tier to the orch; it shows
     in lym beam status, and red only reports

The envelope carries reconcile, mutations, partial, per slug what happened and its
beam_steps, prime_beam_steps, and suite (pid, log, record) or suite_warning.
A slug's error never stops the others.

Example:
  lym adopt payments
  lym adopt root
  lym adopt --all --keep-local`,
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
			if result.PrimeBeamSteps != nil {
				fields["prime_beam_steps"] = result.PrimeBeamSteps
			}
			if result.PrimeBeamWarning != "" {
				fields["prime_beam_warning"] = result.PrimeBeamWarning
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
	cmd.Flags().BoolVar(&req.All, "all", false, "adopt root and every open beam project")
	cmd.Flags().BoolVar(&req.KeepLocal, "keep-local", false, "record the owner over local commits origin lacks and over a live pair")
	cmd.Flags().BoolVar(&req.DropLocal, "drop-local", false, "keep each local tip origin lacks under an archive tag and move the branches to origin's")
	cmd.AddCommand(checkCmd())
	return cmd
}

// RunCLI is the public seam for the adopt module CLI, delegating to clihelp.Execute for output capture.
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
