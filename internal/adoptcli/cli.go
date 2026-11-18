// cli.go builds the cobra command for the adopt module and renders the adopt's envelope.

package adoptcli

import (
	"context"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the adopt module.
func Command() *cobra.Command {
	var req request
	var flags callFlags
	cmd := &cobra.Command{
		Use:   "adopt [<subject>...]",
		Short: "take over beam projects, or root, and the runs under them from another machine",
		// ArbitraryArgs lets the command pass a subject to RunE, where cobra would reject it as an unknown command.
		Args:        cobra.ArbitraryArgs,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Records this machine as the owner of each subject named, a beam or the word root, and
carries on the runs under it here. A beam is a project entry whose beam flag is set: it
moves with its pair and the runs whose parent is its worktree; a nested beam keeps its owner. root moves the prime and the runs whose parent is the prime. A task
is never named: it moves with its beam or with root. --all names root and every open
beam. It runs from the prime and from any beam.

It runs every step in this process and prints each as it ends: one pushed Bolt commit writes
the hold, the owners and this machine's registry entry; each beam's pair is added from origin and
each run another machine left is taken over from origin with a pushed takeover entry; the catch-up
over the hub, the deploy when root is a subject and the workspace follow; the hub's adopt gate then
runs in each subject's worktree with its output on this terminal. Each subject is brought up after
the check, and --no-orch leaves each orch down; an orch session passes it by default. A green
check clears the hold. A failing one files a fix task and keeps the hold, with its try counts,
under it, and the call exits non-zero. Nothing is detached and no record is written: a re-run of
the same subjects goes on from the tracked state. While another machine holds the hub, a plain
adopt refuses, naming it and its time. --take-hold moves that hold here and is the operator's alone.

--here adopts the subject of the worktree it runs in, root in the prime and the beam in a
beam: it pulls Bolt and ends owned_here when no other machine owns the subject; in any other
worktree it does nothing. --archives <subject>... lists the archive-owner/ tags the old owner
pushed whose commit the slug's branch on origin lacks, and runs no step.

The package doc holds the steps, the hold and every field of the envelope.

Example:
  lym adopt payments
  lym adopt root
  lym adopt --all
  lym adopt --here
  lym adopt --archives root`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			location, prime, err := resolveCall(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			req.Slugs = args
			if flags.Archives {
				deps, err := hubArchiveDeps(prime)
				if err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				clihelp.SetExit(ctx, runArchives(deps, req, flags, out))
				return nil
			}
			deps, err := hubCallerDeps(prime)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			deps.Worktree = location.WorktreeName
			clihelp.SetExit(ctx, runAdopt(deps, req, flags, out))
			return nil
		},
	}
	cmd.Flags().BoolVar(&req.All, "all", false, "adopt root and every open beam project")
	cmd.Flags().BoolVar(&req.TakeHold, "take-hold", false, "move another machine's hold over the same subjects here; the operator's alone")
	cmd.Flags().BoolVar(&req.NoOrch, "no-orch", false, "start each dobby and leave its orch down")
	cmd.Flags().BoolVar(&flags.Here, "here", false, "adopt the subject of the worktree this runs in when another machine owns it")
	cmd.Flags().BoolVar(&flags.Archives, "archives", false, "list the owner archive tags of the named subjects whose commits origin's branches lack")
	cmd.AddCommand(checkCmd())
	return cmd
}

// resolveCall returns the location of the command's working directory and the prime of the hub it belongs to.
func resolveCall(ctx context.Context) (location, prime *lymcwd.Location, err error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return nil, nil, err
	}
	location, err = lymcwd.Resolve(cwd)
	if err != nil {
		return nil, nil, err
	}
	prime, err = battencli.ResolveBattenPrime(location)
	return location, prime, err
}

// adoptFields returns the fields of the adopt envelope for result.
// The always-present fields are slugs and prime_recorded.
// Every other field appears only when result holds its value.
// Those are the hold while one stands, the fix task and the check state of root.
// The rest are the check's lines, the deploy, the warnings, the prime's beam steps and warning, and the archives call.
func adoptFields(result outcome) map[string]any {
	slugs := result.Slugs
	if slugs == nil {
		slugs = []slugOutcome{}
	}
	fields := map[string]any{
		"slugs":          slugs,
		"prime_recorded": result.PrimeRecorded,
	}
	if result.Hold != nil {
		fields["hold"] = result.Hold
	}
	if result.FixTask != "" {
		fields["fix_task"] = result.FixTask
	}
	if result.PrimeCheck != "" {
		fields["prime_check"] = result.PrimeCheck
	}
	if len(result.CheckLines) > 0 {
		fields["check_lines"] = result.CheckLines
	}
	if result.Deploy != nil {
		fields["deploy"] = result.Deploy
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
	if command := archivesCommand(result); command != "" {
		fields["archives"] = command
	}
	return fields
}
