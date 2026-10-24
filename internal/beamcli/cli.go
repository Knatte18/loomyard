// cli.go builds the beam cobra command with its create, promote and land verbs and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.

package beamcli

import (
	"context"
	"io"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for the beam module.
func Command() *cobra.Command {
	beam := &cobra.Command{
		Use:   "beam",
		Short: "make, promote and land beams, the long-lived pairs that group board work",
		Long: `A beam is a long-lived pair that groups board work: its tasks fork from the
beam's own branch and land back into it, and it can run an orch of its own.
Every verb runs from the hub's prime worktree and from any beam, and acts on the
prime's location.`,
		RunE: clihelp.GroupRunE,
	}
	beam.AddCommand(createCmd(), promoteCmd(), landCmd())
	return beam
}

// createCmd returns the `beam create` verb.
func createCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "create <slug>",
		Short:       "make a project entry a beam: record its owner and make its pair",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turns the board's project entry <slug> into a beam. The entry's title, brief and
labels are board writing, so the verb makes no entry: make the project first with
"lyx board upsert". A task becomes a beam with "lyx beam promote".

Steps, in order, each skipped when its result is already there, so a re-run
finishes a stopped create:
  1. record this machine as the project's owner with the beam flag
  2. make the pair from the prime, or from the parent beam of a nested beam,
     and mark it as a beam; an existing pair is marked
  3. spawn the pair's push
  4. regenerate the workspace files

The envelope names the beam's worktree, where "lyx orch start" gives it an orch;
the verb starts none.

Example:
  lyx beam create payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(_ context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubCreateDeps(prime)
				if err != nil {
					return nil, err
				}
				return create(deps, args[0])
			})
		},
	}
}

// landCmd returns the `beam land` verb.
func landCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "land <slug>",
		Short:       "land a beam into its parent with a short batten run",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Ends the beam <slug> with a landing run over batten and the beam recipe: the run
writes the beam's description, merges the beam into its parent worktree's branch,
and removes the beam's pair, whose board entry is then done. The window of the run
opens in the beam's parent worktree's session.

It runs from the prime and from any beam but the beam itself, whose pair the landing
removes, and refuses while work remains under the beam:
  - an open entry whose parent is the beam, through grouping projects
  - a local pair forked from the beam
  - the beam's orch strand or its watcher live ("lyx orch stop" in the beam's worktree)
Each refusal names its way forward. Land or cancel the members first.

Example:
  lyx beam land payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(_ context.Context, acting string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubLandDeps(prime)
				if err != nil {
					return nil, err
				}
				return land(deps, acting, args[0])
			})
		},
	}
}

// promoteCmd returns the `beam promote` verb.
func promoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "promote <slug>",
		Short:       "turn a spawned task into a beam, keeping its pair and its commits",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `Turns the started task <slug> into a beam, keeping its pair and the commits on its
branch. A task that was never started has no pair to keep: make a project entry
with "lyx board upsert" and run "lyx beam create" on it. On a project entry the
verb runs the steps of "lyx beam create".

Steps, in order, each skipped when its result is already there, so a re-run
finishes a stopped promote:
  1. end the task's run: its processes and strands, with the pair kept
  2. mark the pair as a beam and spawn its push
  3. remove the run's records and clear its claim
  4. reset the pair's records to a fresh fork of its parent's, and spawn the push
  5. record this machine as the owner and turn the entry into a beam project
  6. regenerate the workspace files

The run ends for good: its code commits stay on the pair's branch, and its
discussion, plan and run records stay reachable in the records history.
The verb refuses a task another machine owns or this machine has no pair for
("lyx adopt"), a task an open task depends on ("lyx board set-deps"), a task whose
run already reached its parent ("lyx batten run"), and a root pair.

Example:
  lyx beam promote payments`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, func(ctx context.Context, _ string, prime *lyxcwd.Location) (any, error) {
				deps, err := hubPromoteDeps(prime)
				if err != nil {
					return nil, err
				}
				return promote(ctx, deps, args[0])
			})
		},
	}
}

// runVerb resolves the prime from the command's cwd, runs the verb over it and prints the verb's result in the standard envelope.
// The verb is told the name of the worktree it was run from beside the prime's location.
// A verb's error is printed with the result it returned beside it.
func runVerb(cmd *cobra.Command, run func(ctx context.Context, acting string, prime *lyxcwd.Location) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	fail := func(err error) error {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return fail(err)
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return fail(err)
	}
	prime, err := battencli.ResolveBattenPrime(location)
	if err != nil {
		return fail(err)
	}
	result, err := run(ctx, location.WorktreeName, prime)
	fields := map[string]any{"result": result}
	if err != nil {
		clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), fields))
		return nil
	}
	clihelp.SetExit(ctx, output.Ok(out, fields))
	return nil
}

// RunCLI is the public seam for the beam module CLI, delegating to clihelp.Execute for output capture.
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
