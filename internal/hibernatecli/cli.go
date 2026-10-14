// cli.go builds the hibernate cobra command with its request, cancel, status and hidden watch verbs, and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package hibernatecli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// Command returns the cobra command for `lyx hibernate`: the bare group and its request, cancel, status and watch verbs.
func Command() *cobra.Command {
	hibernate := &cobra.Command{
		Use:   "hibernate",
		Short: "queue a hibernate of the machine that runs once every session in the hub is paused",
		Long: `hibernate asks lyx itself to hibernate the machine once every session in the hub
is safely paused, so no LLM session runs the hibernate and none is cut mid-call.
request records the request and spawns the daemon that carries it out, cancel
withdraws it and status shows it. Linux only.`,
		RunE: clihelp.GroupRunE,
	}
	hibernate.AddCommand(requestCmd(), cancelCmd(), statusCmd(), watchCmd())
	return hibernate
}

// runVerb resolves the hub from the command's cwd, fills the engine's Deps for it and prints what run returns under key in the standard envelope, or a refusal through output.Err.
func runVerb(cmd *cobra.Command, key string, run func(hibernateengine.Deps) (any, error)) error {
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
	deps, err := hubDeps(location)
	if err != nil {
		return fail(err)
	}
	result, err := run(deps)
	if err != nil {
		return fail(err)
	}
	clihelp.SetExit(ctx, output.Ok(out, map[string]any{key: result}))
	return nil
}

// requestCmd returns the `hibernate request` verb.
func requestCmd() *cobra.Command {
	var (
		after    string
		afterAll bool
	)
	cmd := &cobra.Command{
		Use:         "request",
		Short:       "queue a hibernate and spawn the daemon that carries it out",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `request records the hub's one hibernate request and spawns a detached daemon,
then returns. The daemon waits for the trigger, pauses the hub, confirms every
session paused, hibernates the machine, and on wake lifts its own pause. It
queues a notice for the orch when it is done or failed.

With no flag the trigger holds at once. --after <slug> waits until that batten
run has finished its Land row; --after-all waits until no batten run is in
flight. The two are exclusive. Only the orch or an external caller may request;
an agent asks its parent session.

Example:
  lyx hibernate request
  lyx hibernate request --after my-task`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "request", func(deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Submit(deps, hibernateengine.SubmitRequest{After: after, AfterAll: afterAll})
			})
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "hibernate once the batten run of this slug has finished its Land row")
	cmd.Flags().BoolVar(&afterAll, "after-all", false, "hibernate once no batten run is in flight")
	return cmd
}

// cancelCmd returns the `hibernate cancel` verb.
func cancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "cancel",
		Short:       "withdraw the pending hibernate request and lift the pause it added",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `cancel ends the pending request, stops its daemon and lifts the pause entry the
request added. No request, and one that has ended, succeed with nothing
cancelled. Once the machine is going into hibernation cancel is refused: the
daemon lifts its own pause on wake. Only the orch or an external caller may
cancel.

Example:
  lyx hibernate cancel`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "cancel", func(deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Cancel(deps)
			})
		},
	}
}

// statusCmd returns the `hibernate status` verb, which any caller may run since it writes nothing.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "print the hibernate request and whether its daemon runs",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints the hub's hibernate request, its phase and last reason, and
whether its daemon runs. It changes nothing. Any caller may run it.

Example:
  lyx hibernate status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "status", func(deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Status(deps)
			})
		},
	}
}

// watchCmd returns the hidden `hibernate watch` verb, the daemon that `hibernate request` spawns.
func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "watch",
		Short:       "run the daemon of the pending hibernate request",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `watch carries out the pending hibernate request whose daemon is this process,
and is what "lyx hibernate request" spawns. Run by hand with no request, or
beside a live daemon, it returns at once and does nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "watch", func(deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Watch(deps)
			})
		},
	}
}

// RunCLI is the public seam for the hibernate module CLI, delegating to clihelp.Execute for output capture.
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
