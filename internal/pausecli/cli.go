// cli.go builds the pause and resume cobra commands, the caller rules and the RunCLI/RunCLIIn seams that wire them into the standard io.Writer-based call contract.

package pausecli

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
	"github.com/Knatte18/loomyard/internal/pauseengine"
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

// refuseCaller refuses an agent's hub-wide pause or resume, and either naming a slug other than its own pair's.
// The orch and an external caller are never refused.
func refuseCaller(c caller, slug string) error {
	if !c.agent || (slug != "" && slug == c.slug) {
		return nil
	}
	return errors.New(`pause: an agent may pause or resume only its own pair; way forward: ask your parent session, which runs "lyx pause" or "lyx resume" for you`)
}

// Command returns the cobra command for `lyx pause`.
func Command() *cobra.Command {
	var grace int
	var status bool
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

--grace sets the grace in seconds, 0 to 60. --status prints the live entries,
the network operations in flight and the parent locks held, and changes nothing.

Lift it with "lyx resume", which gives each interrupted session one line to
carry on.

Example:
  lyx pause
  lyx pause my-task --grace 5
  lyx pause --status`,
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
			if !status && (grace < 0 || grace > maxGraceSeconds) {
				return fail(fmt.Errorf("pause: --grace %d is outside the range 0 to %d seconds; way forward: pass a --grace within it", grace, maxGraceSeconds))
			}
			if !status {
				if err := refuseCaller(callerOf(os.Getenv(agentname.StrandNameEnv)), slug); err != nil {
					return fail(err)
				}
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
			if status {
				report, err := pauseengine.Status(deps)
				if err != nil {
					return fail(err)
				}
				clihelp.SetExit(ctx, output.Ok(out, map[string]any{"status": report}))
				return nil
			}
			if slug != "" && !slices.Contains(slugs, slug) {
				return fail(fmt.Errorf("pause: %q has no pair in this hub; way forward: name one of the pairs: %s", slug, strings.Join(slugs, ", ")))
			}
			report, err := pauseengine.Pause(deps, pauseengine.PauseRequest{Slug: slug, Grace: time.Duration(grace) * time.Second})
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"pause": report}))
			return nil
		},
	}
	cmd.Flags().IntVar(&grace, "grace", defaultGraceSeconds, "seconds busy sessions and network operations get to finish before the Esc, 0 to 60")
	cmd.Flags().BoolVar(&status, "status", false, "print the live entries and holds and change nothing")
	return cmd
}

// ResumeCommand returns the cobra command for `lyx resume`, the second root command of the pause module.
func ResumeCommand() *cobra.Command {
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
			report, err := pauseengine.Resume(deps, pauseengine.ResumeRequest{Slug: slug})
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"resume": report}))
			return nil
		},
	}
}

// RunCLI is the public seam for the pause module CLI, delegating to clihelp.Execute for output capture.
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
