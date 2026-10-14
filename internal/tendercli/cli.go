// cli.go builds the tender cobra command with its pause, resume and status verbs, the caller rules and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package tendercli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/spf13/cobra"
)

const (
	// defaultGraceSeconds is how long busy sessions and network operations get to finish before the Esc.
	defaultGraceSeconds = 10
	// maxGraceSeconds is the longest grace a pause accepts.
	maxGraceSeconds = 60
)

// caller says who runs a verb.
type caller struct {
	// orch is true for the hub orchestrator.
	orch bool
	// agent is true for any other strand.
	agent bool
	// slug is the pair an agent belongs to, empty for an agent with no pair.
	slug string
}

// callerOf reads the caller from the strand name: empty is external, the orch role in the prime is the orch, and any other name is an agent of the slug it carries.
func callerOf(strand string) caller {
	if strand == "" {
		return caller{}
	}
	name, err := agentname.Parse(strand)
	if err == nil && name.Role == agentname.RoleOrch && name.Slug == "" {
		return caller{orch: true}
	}
	if err != nil {
		return caller{agent: true}
	}
	return caller{agent: true, slug: name.Slug}
}

// refuseCaller refuses an agent's hub-wide pause, resume or revive, and any of them naming a slug other than its own pair's.
// The orch and an external caller are never refused.
func refuseCaller(c caller, slug string) error {
	if !c.agent || (slug != "" && slug == c.slug) {
		return nil
	}
	return errors.New(`pause: an agent may pause, resume or revive only its own pair; way forward: ask your parent session, which runs "lyx pause", "lyx resume" or "lyx tender revive" for you`)
}

// Command returns the cobra command for `lyx tender`: the bare group and its pause, resume, revive and status verbs.
func Command() *cobra.Command {
	tender := &cobra.Command{
		Use:   "tender",
		Short: "hub-wide control over what runs: pause, resume, revive and status",
		Long: `tender holds the hub-wide verbs over the agent and network work that runs:
pause holds it, resume lifts the hold, revive gives a carry-on line to sessions
stalled on an API error, and status shows the live entries, the holds and what
runs. "lyx pause" and "lyx resume" are shortcuts for the first two.`,
		RunE: clihelp.GroupRunE,
	}
	tender.AddCommand(pauseCmd(), resumeCmd(), reviveCmd(), statusCmd())
	return tender
}

// pauseCmd returns the `tender pause` verb.
func pauseCmd() *cobra.Command {
	var grace int
	cmd := &cobra.Command{
		Use:         "pause [<slug>]",
		Short:       "pause the hub or one pair: hold agent and network work and interrupt running turns",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `pause records a pause, waits a short grace for busy agent sessions and network
operations to finish, interrupts every covered session still in a running turn
with Esc, and holds every path that would start agent or network work until
"lyx resume".

A second Esc goes only after the provider's settle time, and only into a pane
still showing a running turn. A session whose turn ended with background work
is sent no Esc and is reported under "background".

With no slug it covers the whole hub; with a slug it covers that pair. A repeated
pause adds no entry and interrupts only the sessions in a running turn now. The
orchestrator is interrupted by its own watcher at the grace end, not by pause.

--grace sets the grace in seconds, 0 to 60.

Lift it with "lyx resume", which gives each interrupted session one line to
carry on.

Example:
  lyx tender pause
  lyx tender pause my-task --grace 5`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			if grace < 0 || grace > maxGraceSeconds {
				return fail(fmt.Errorf("pause: --grace %d is outside the range 0 to %d seconds; way forward: pass a --grace within it", grace, maxGraceSeconds))
			}
			if err := refuseCaller(callerOf(os.Getenv(agentname.StrandNameEnv)), slug); err != nil {
				return fail(err)
			}
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}
			deps, slugs, err := hubDeps(location, os.Getenv(agentname.StrandNameEnv))
			if err != nil {
				return fail(err)
			}
			if slug != "" && !slices.Contains(slugs, slug) {
				return fail(fmt.Errorf("pause: %q has no pair in this hub; way forward: name one of the pairs: %s", slug, strings.Join(slugs, ", ")))
			}
			report, err := tenderengine.Pause(deps, tenderengine.PauseRequest{Slug: slug, Grace: time.Duration(grace) * time.Second})
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"pause": report}))
			return nil
		},
	}
	cmd.Flags().IntVar(&grace, "grace", defaultGraceSeconds, "seconds busy sessions and network operations get to finish before the Esc, 0 to 60")
	return cmd
}

// resumeCmd returns the `tender resume` verb.
func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "resume [<slug>]",
		Short:       "lift a pause and give each interrupted session one line to carry on",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `resume lifts the live pause entries. With no slug it lifts every entry; with a
slug it lifts that pair's own entry. A slug covered only by a hub-wide pause is
refused: run "lyx resume" to lift the hub pause. Resuming nothing is a no-op.

Each session the pause interrupted gets one line to carry on, unless its strand
is not live or its guarded send is refused, as for a running turn, a draft, a
dialog on screen or an open rewind picker; those are named with the reason. A
session another live entry still covers moves to that entry with no line. The
orchestrator's line comes from its own watcher.

Example:
  lyx resume
  lyx resume my-task`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			if err := refuseCaller(callerOf(os.Getenv(agentname.StrandNameEnv)), slug); err != nil {
				return fail(err)
			}
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}
			deps, _, err := hubDeps(location, os.Getenv(agentname.StrandNameEnv))
			if err != nil {
				return fail(err)
			}
			report, err := tenderengine.Resume(deps, tenderengine.ResumeRequest{Slug: slug})
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"resume": report}))
			return nil
		},
	}
}

// reviveCmd returns the `tender revive` verb.
func reviveCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "revive [<slug>]",
		Short:       "give one carry-on line to each session stalled on an API error",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `revive types one fixed carry-on line into each session whose last turn stopped
on an API error, the case a network outage without a machine sleep leaves
behind. A session in any other state is skipped with the state read, and a
session a live pause covers is left to "lyx resume".

With no slug it covers the whole hub, the orchestrator included: the orch gets
a conditional notice its own watcher delivers while it still reads stalled on
the error. With a slug it covers that pair. Every send is refused at once when
the session no longer reads stalled, or when a draft, operator activity or an
open rewind picker holds the pane; none is retried.

Example:
  lyx tender revive
  lyx tender revive my-task`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			slug := ""
			if len(args) == 1 {
				slug = args[0]
			}
			if err := refuseCaller(callerOf(os.Getenv(agentname.StrandNameEnv)), slug); err != nil {
				return fail(err)
			}
			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}
			deps, slugs, err := hubDeps(location, os.Getenv(agentname.StrandNameEnv))
			if err != nil {
				return fail(err)
			}
			if slug != "" && !slices.Contains(slugs, slug) {
				return fail(fmt.Errorf("revive: %q has no pair in this hub; way forward: name one of the pairs: %s", slug, strings.Join(slugs, ", ")))
			}
			report, err := tenderengine.Revive(deps, tenderengine.ReviveRequest{Slug: slug})
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"revive": report}))
			return nil
		},
	}
}

// statusCmd returns the `tender status` verb, which any caller may run since it writes nothing.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "print the live pause entries, the network operations in flight, the parent locks held, every live session and the processes running a replaced binary",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints the live pause entries, the network operations in flight and the
parent locks held, then every live session in the hub with its state, cause and
whether an entry covers it, and the lyx processes running a replaced binary.
It changes nothing. Any caller may run it.

Example:
  lyx tender status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			deps, _, err := hubDeps(location, os.Getenv(agentname.StrandNameEnv))
			if err != nil {
				return fail(err)
			}
			report, err := tenderengine.Status(deps)
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"status": report}))
			return nil
		},
	}
}

// RunCLI is the public seam for the tender module CLI, delegating to clihelp.Execute for output capture.
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
