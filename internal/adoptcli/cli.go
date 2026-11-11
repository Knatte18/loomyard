// cli.go builds the cobra command for the adopt module and the RunCLI and RunCLIIn entry points that run it.

package adoptcli

import (
	"context"
	"io"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the adopt module.
func Command() *cobra.Command {
	var req request
	var discardFlag bool
	cmd := &cobra.Command{
		Use:   "adopt [<subject>...]",
		Short: "take over beam projects, or root, and the runs under them from another machine",
		// ArbitraryArgs lets RunCLI pass a subject to RunE, where cobra would reject it as an unknown command.
		Args:        cobra.ArbitraryArgs,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Records this machine as the owner of each subject named, a beam or the word root, and
carries on the runs under it here. A beam is a project entry whose beam flag is set: it
moves with its pair and the runs whose parent is its worktree; a nested beam keeps its owner. root moves the prime and the runs whose parent is the prime. A task
is never named: it moves with its beam or with root. --all names root and every open
beam. It runs from the prime and from any beam.

Steps, in order: reconcile, pairs, pause, owner, deploy, beam, check, done. A hidden worker
runs the steps and records each in the adopt record; this call follows the record and prints
each step as it ends. The pause holds each run the call takes over until an operator resumes
it; a green adopt check only reports. A beam or run whose pair holds local commits origin
lacks, or is live, stops with both ways forward: --keep-local records the owner over them,
--drop-local archives each local tip as archive/<slug>/<tip> and moves the branches to
origin's (refused for a live pair); a project one of whose runs stops is held back whole.
--no-orch starts each dobby and leaves its orch down; an orch session passes it by default.
--discard removes a record no worker will finish, and takes no subject.

The package doc holds the record, the resume and every field of the envelope.

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
			prime, err := resolvePrime(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			deps, err := hubCallerDeps(prime)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			req.Slugs = args
			clihelp.SetExit(ctx, runAdopt(deps, req, discardFlag, out))
			return nil
		},
	}
	cmd.Flags().BoolVar(&req.All, "all", false, "adopt root and every open beam project")
	cmd.Flags().BoolVar(&req.KeepLocal, "keep-local", false, "record the owner over local commits origin lacks and over a live pair")
	cmd.Flags().BoolVar(&req.DropLocal, "drop-local", false, "keep each local tip origin lacks under an archive tag and move the branches to origin's")
	cmd.Flags().BoolVar(&req.NoOrch, "no-orch", false, "start each dobby and leave its orch down")
	cmd.Flags().BoolVar(&discardFlag, "discard", false, "remove an unfinished adopt record that no worker will finish")
	cmd.AddCommand(workerCmd(), checkCmd())
	return cmd
}

// workerCmd returns the hidden `worker` subcommand.
// It runs the steps of an adopt and records each in the adopt record.
func workerCmd() *cobra.Command {
	var req request
	cmd := &cobra.Command{
		Use:         "worker [<subject>...]",
		Short:       "run the steps of an adopt and record each in the adopt record",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Args:        cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return driveWorker(cmd, req, args)
		},
	}
	cmd.Flags().BoolVar(&req.All, "all", false, "adopt root and every open beam project")
	cmd.Flags().BoolVar(&req.KeepLocal, "keep-local", false, "record the owner over local commits origin lacks and over a live pair")
	cmd.Flags().BoolVar(&req.DropLocal, "drop-local", false, "keep each local tip origin lacks under an archive tag and move the branches to origin's")
	cmd.Flags().BoolVar(&req.NoOrch, "no-orch", false, "start each dobby and leave its orch down")
	return cmd
}

// resolvePrime returns the prime of the hub the command's working directory belongs to.
func resolvePrime(ctx context.Context) (*lymcwd.Location, error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return nil, err
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	return battencli.ResolveBattenPrime(location)
}

// driveWorker resolves the prime from the command's working directory and runs the worker for req naming args.
// It prints the end envelope: ok for a done adopt, an error envelope for a refusal or a stop.
func driveWorker(cmd *cobra.Command, req request, args []string) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	prime, err := resolvePrime(ctx)
	if err != nil {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}
	deps, err := hubWorkerDeps(prime, machinename.Name)
	if err != nil {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	req.Slugs = args
	result, _, err := runWorker(deps, req)
	fields := adoptFields(result)
	if err != nil {
		clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), fields))
		return nil
	}
	clihelp.SetExit(ctx, output.Ok(out, fields))
	return nil
}

// adoptFields returns the fields of the adopt envelope for result.
// The always-present fields are reconcile, mutations, partial, slugs, prime_recorded and runs;
// runs is an empty array when there are none.
// Every other field appears only when result holds its value.
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
	if result.Deploy != nil {
		fields["deploy"] = result.Deploy
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

// RunCLI runs the adopt command in-process with args and writes its output to out.
// It runs from the process working directory and returns the exit code.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the adopt command in-process with args, writes its output to out, and returns the exit code.
// An empty cwd reads the process working directory;
// any other value is seeded into the execution context, because seeding an empty one panics.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
