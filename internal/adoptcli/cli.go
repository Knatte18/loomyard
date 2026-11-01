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
  4. each run this call takes over, and each unstarted released task of an adopted subject
     bar a front task, is held until its subject's adopt check is green
  5. one owner write for every beam not stopped, and for the prime with root
  6. the beam steps of the prime with root and of each adopted beam: its dobby and orch
  7. one adopt check per subject, root first, starts detached in that subject's worktree;
     a green check lifts the subject's holds and starts its runs, and a red one files a
     fix task, up to adopt_fix_tries

The envelope carries reconcile, mutations, partial, per slug what happened and its
beam_steps, prime_beam_steps, warnings, and checks with message, or check_warnings. A
slug's error never stops the others. runs holds one item per run under the named
subjects, sorted by slug, with its state and a plain line.

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
			fields := adoptFields(result)
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

// adoptFields returns the fields of the adopt envelope for result.
// The always-present fields are reconcile, mutations, partial, slugs, prime_recorded and runs, an empty array for none; every other field appears only when result holds its value.
func adoptFields(result outcome) map[string]any {
	runs := result.Runs
	if runs == nil {
		runs = []runReport{}
	}
	fields := map[string]any{
		"reconcile":      result.Reconcile,
		"mutations":      result.Mutations,
		"partial":        result.Partial,
		"slugs":          result.Slugs,
		"prime_recorded": result.PrimeRecorded,
		"runs":           runs,
	}
	if result.PrimeLost != "" {
		fields["prime_lost_to"] = result.PrimeLost
	}
	if len(result.Warnings) > 0 {
		fields["warnings"] = result.Warnings
	}
	if result.PrimeBeamSteps != nil {
		fields["prime_beam_steps"] = result.PrimeBeamSteps
	}
	if result.PrimeBeamWarning != "" {
		fields["prime_beam_warning"] = result.PrimeBeamWarning
	}
	if len(result.Checks) > 0 {
		fields["checks"] = result.Checks
	}
	if result.SuiteMessage != "" {
		fields["message"] = result.SuiteMessage
	}
	if len(result.CheckWarnings) > 0 {
		fields["check_warnings"] = result.CheckWarnings
	}
	return fields
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
