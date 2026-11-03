// cli.go builds the hibernate cobra command with its request, cancel and status verbs, and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package hibernatecli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateconfig"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// resumeOnWakeFlag is the `hibernate request` flag that chooses, for one request, whether its wake lifts the hub pause.
const resumeOnWakeFlag = "resume-on-wake"

// Command returns the cobra command for `lym hibernate`: the bare group and its request, cancel and status verbs.
func Command() *cobra.Command {
	hibernate := &cobra.Command{
		Use:   "hibernate",
		Short: "queue a hibernate of the machine that runs once every session in the hub is paused",
		Long: `hibernate asks lym itself to hibernate the machine once every session in the hub
is safely paused, so no LLM session runs the hibernate and none is cut mid-call.
request records the request, which the prime's dobby carries out, cancel
withdraws it and status shows it. Linux only.`,
		RunE: clihelp.GroupRunE,
	}
	hibernate.AddCommand(requestCmd(), cancelCmd(), statusCmd())
	return hibernate
}

// runVerb resolves the hub from the command's cwd, fills the engine's Deps for it and prints what run returns under key in the standard envelope, or a refusal through output.Err.
func runVerb(cmd *cobra.Command, key string, run func(*lymcwd.Location, hibernateengine.Deps) (any, error)) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	fail := func(err error) error {
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return nil
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return fail(err)
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return fail(err)
	}
	deps, err := hubDeps(location)
	if err != nil {
		return fail(err)
	}
	result, err := run(location, deps)
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
		resume   bool
	)
	cmd := &cobra.Command{
		Use:         "request",
		Short:       "queue a hibernate that the prime's dobby carries out",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `request records the hub's one hibernate request and returns. The prime's dobby
takes it up: it waits for the trigger, pauses the hub, confirms every session
paused, hibernates the machine, and on wake reconciles the hub with origin. It
posts creel mail to the orch when it is done or failed. The request stays on
disk, so a prime's dobby that is not running takes it up once it starts; the
envelope says whether it runs and names "lym beam start root" when it does not.

The wake lifts the request's pause only when hibernate.yaml's resume_on_wake is
on or --resume-on-wake is given; --resume-on-wake=false keeps the pause for this
request whatever the config holds. A kept pause stands until "lym tender resume"
or "lym hibernate cancel" lifts it, and a new request is refused meanwhile.

With no trigger flag the trigger holds at once. --after <slug> waits until that
batten run has finished its Land row; --after-all waits until no batten run is
in flight. The two are exclusive. Only the orch or an external caller may
request; an agent asks its parent session.

Example:
  lym hibernate request
  lym hibernate request --after my-task
  lym hibernate request --resume-on-wake`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "request", func(location *lymcwd.Location, deps hibernateengine.Deps) (any, error) {
				resumeOnWake, err := chooseResumeOnWake(cmd.Flags().Changed(resumeOnWakeFlag), resume, fabricengine.ConfigDirs(location))
				if err != nil {
					return nil, err
				}
				return hibernateengine.Submit(deps, hibernateengine.SubmitRequest{After: after, AfterAll: afterAll, ResumeOnWake: resumeOnWake})
			})
		},
	}
	cmd.Flags().StringVar(&after, "after", "", "hibernate once the batten run of this slug has finished its Land row")
	cmd.Flags().BoolVar(&afterAll, "after-all", false, "hibernate once no batten run is in flight")
	cmd.Flags().BoolVar(&resume, resumeOnWakeFlag, false, "lift the hub pause on the wake, so paused runs go on by themselves; overrides hibernate.yaml's resume_on_wake for this request")
	return cmd
}

// chooseResumeOnWake is a request's choice of whether its wake lifts the hub pause: the flag's value when the flag was given, else hibernate.yaml's resume_on_wake under dirs.
func chooseResumeOnWake(flagGiven, flag bool, dirs configengine.Dirs) (bool, error) {
	if flagGiven {
		return flag, nil
	}
	cfg, err := hibernateconfig.LoadConfig(dirs)
	if err != nil {
		return false, err
	}
	return cfg.ResumeOnWake, nil
}

// cancelCmd returns the `hibernate cancel` verb.
func cancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "cancel",
		Short:       "withdraw the pending hibernate request and lift the pause it added",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `cancel ends the pending request and lifts the pause entry the request added; the
prime's dobby stops carrying it at its next step. No request, and one that has
ended, succeed with nothing cancelled, and one that has ended still has the
pause entry it added lifted when it was left live or its wake kept it. Once the machine is
going into hibernation cancel is refused until the wake. Only the orch or an
external caller may cancel.

Example:
  lym hibernate cancel`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "cancel", func(_ *lymcwd.Location, deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Cancel(deps)
			})
		},
	}
}

// statusCmd returns the `hibernate status` verb, which any caller may run since it writes nothing.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "print the hibernate request and whether the dobby that carries it runs",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints the hub's hibernate request, its phase and last reason, and
whether the daemon that carries it, the prime's dobby, runs. It changes nothing.
Any caller may run it.

Example:
  lym hibernate status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerb(cmd, "status", func(_ *lymcwd.Location, deps hibernateengine.Deps) (any, error) {
				return hibernateengine.Status(deps)
			})
		},
	}
}

// RunCLI is the public seam for the hibernate module CLI, delegating to clihelp.Execute for output capture.
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
