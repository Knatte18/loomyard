// remotecontrol.go holds the orch's remote control.
// It forms the session name a launch passes.
// It also holds the two hidden verbs behind reed's RC button.
// `rc-bar` prints the button's state, and `rc-on` is what a click runs.

package orchcli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/orchroute"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// remoteControlBarVerb is the name of the hidden verb reed's status bar runs for the RC button's state.
	// It skips the orch wiring, which refuses every worktree but the prime and a beam.
	// The bar runs it from every session, a task pair's included.
	remoteControlBarVerb = reedengine.RemoteControlBarVerb
	// remoteControlOnVerb is the name of the hidden verb a click on reed's RC button runs.
	remoteControlOnVerb = reedengine.RemoteControlOnVerb
	// remoteControlProgram is the program name the RC click signs its creel command with.
	remoteControlProgram = "REMOTECONTROL"
)

// orchModelChoice returns the engine, model and effort the orch session runs on.
func (c *orchCLI) orchModelChoice() shuttleengine.ModelChoice {
	return shuttleengine.ModelChoice{Model: c.cfg.Model, Effort: c.cfg.Effort}
}

// orchHasRemoteControl reports whether the engine the orch session runs on has the provider's remote control.
func (c *orchCLI) orchHasRemoteControl() bool {
	engine, err := c.engines.Lookup(c.orchModelChoice().Engine)
	if err != nil {
		return false
	}
	_, ok := engine.(shuttleengine.RemoteController)
	return ok
}

// remoteControlSessionName returns the remote session's name of the orch strand strandName on this machine.
// It fills orch.yaml's remote_control_name.
func (c *orchCLI) remoteControlSessionName(strandName string) (string, error) {
	parsed, err := agentname.Parse(strandName)
	if err != nil {
		return "", fmt.Errorf("orch: the orch strand name %q does not parse: %w", strandName, err)
	}
	machine, err := c.machineName()
	if err != nil {
		return "", fmt.Errorf("orch: read this machine's name for the remote session's name: %w", err)
	}
	return c.cfg.RemoteControlSessionName(machine, parsed), nil
}

// launchRemoteControlName returns the remote session name a launch of the orch strand passes.
// The name is empty when orch.yaml switches remote control off or the orch's engine has none.
// The strand is listed when hasStrand holds; otherwise it takes the fixed name of this worktree's orch.
// A name that cannot be formed is logged at Warn and the session launches without remote control,
// so the switch never stops a bring-up.
func (c *orchCLI) launchRemoteControlName(listed reedengine.StrandStatus, hasStrand bool) string {
	if !c.cfg.RemoteControl || !c.orchHasRemoteControl() {
		return ""
	}
	strandName := listed.Name
	if !hasStrand {
		fixed, err := orchroute.FixedStrandName(c.location)
		if err != nil {
			logger.Warn("orch: launching the orch session without remote control, since its strand name is unknown", "err", err)
			return ""
		}
		strandName = fixed
	}
	name, err := c.remoteControlSessionName(strandName)
	if err != nil {
		logger.Warn("orch: launching the orch session without remote control", "err", err)
		return ""
	}
	return name
}

// recordRemoteControl records the remote control a launch set for the strand guid, for reed's RC button.
// An orch whose engine has no remote control records nothing, so reed draws no RC button for it.
// A failed write is logged at Warn and never fails the caller, since the record is display only.
func (c *orchCLI) recordRemoteControl(guid string, on bool) {
	if !c.orchHasRemoteControl() {
		return
	}
	if err := orchengine.WriteRemoteControl(c.paths, orchengine.RemoteControlRecord{Strand: guid, On: on, At: time.Now()}); err != nil {
		logger.Warn("orch: could not record the orch session's remote control for the RC button", "strandGUID", guid, "err", err)
	}
}

// recordUnrecordedRemoteControl records remote control off for the live strand guid when the record does not name it.
// A start that finds the strand already running did not launch it, so what lym knows of its remote control is nothing.
// Off keeps the RC button drawn, and a click turns remote control on.
func (c *orchCLI) recordUnrecordedRemoteControl(guid string) {
	if _, recorded, err := orchengine.ReadRemoteControl(c.paths, guid); err != nil || recorded {
		return
	}
	c.recordRemoteControl(guid, false)
}

// turnOnRemoteControl queues the remote-control command into this worktree's live orch strand through creel.
// The name is the one a launch passes, whatever orch.yaml's switch says.
// It records the strand's remote control on.
// A failed record write is logged and the click still succeeds with a warning field, since the command is already queued.
// It refuses a worktree with no live orch strand and a worktree with several orch strands.
// It refuses an orch whose engine has no remote control, and a strand whose record already reads on.
// The last sends nothing.
// What Claude Code shows for a second command in a session with remote control on is unverified.
func (c *orchCLI) turnOnRemoteControl() (map[string]any, error) {
	if !c.orchHasRemoteControl() {
		return nil, errors.New("orch: the orch runs on an engine with no remote control; way forward: run the orch on the claude engine")
	}
	strands, err := c.strands.Strands()
	if err != nil {
		return nil, err
	}
	named := orchStrands(strands)
	if len(named) > 1 {
		guids := make([]string, len(named))
		for i, s := range named {
			guids[i] = s.GUID
		}
		return nil, fmt.Errorf("orch: %d strands are named %q (%s); way forward: remove the extras with `lym reed remove` and click RC again", len(named), orchStrandName, strings.Join(guids, ", "))
	}
	if len(named) == 0 || !named[0].Live {
		return nil, errors.New("orch: this worktree runs no live orch strand; way forward: run `lym beam start`, whose launch turns remote control on")
	}
	strand := named[0]
	if on, err := orchengine.RemoteControlOn(c.paths, strand.GUID); err == nil && on {
		return nil, fmt.Errorf("orch: remote control is already on for %s, so nothing was sent; way forward: when you turned it off inside Claude Code, type `/remote-control` by hand in the orch pane", strand.Name)
	}
	name, err := c.remoteControlSessionName(strand.Name)
	if err != nil {
		return nil, err
	}
	if err := c.injectRemoteControl(strand.Name, name); err != nil {
		return nil, fmt.Errorf("orch: queue the remote-control command for %s: %w; way forward: repair what the error names and click RC again", strand.Name, err)
	}
	fields := map[string]any{"strand": strand.Name, "remote_control_name": name}
	if err := orchengine.WriteRemoteControl(c.paths, orchengine.RemoteControlRecord{Strand: strand.GUID, On: true, At: time.Now()}); err != nil {
		logger.Warn("orch: could not record the orch session's remote control for the RC button", "strandGUID", strand.GUID, "err", err)
		fields["warning"] = fmt.Sprintf("the command is queued, but recording it failed (%v); the RC button stays dim, and another click would queue the command again", err)
	}
	return fields, nil
}

// injectRemoteControlCommand queues creel's remote-control command with name into the orch strand receiver.
// It signs the command as the RC click's program.
func injectRemoteControlCommand(location *lymcwd.Location, receiver, name string) error {
	client, err := creelcli.NewProgramClient(location, remoteControlProgram)
	if err != nil {
		return err
	}
	_, err = client.InjectCommand(receiver, creelengine.Command{Name: creelengine.CommandRemoteControl, Argument: name}, 0)
	return err
}

// remoteControlBarCmd returns the hidden `orch rc-bar` verb.
func remoteControlBarCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:         remoteControlBarVerb,
		Short:       "print the remote-control state for reed's RC button",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `rc-bar prints the RC button's state on one line for reed's status bar. When
--dir names a worktree whose orch state records an orch strand, and the
worktree's remote-control record names that strand, it prints rc-on while lym
last turned the strand's remote control on, by a launch or a click on RC, and
rc-off otherwise. For any other directory, and for a strand with no record, as
an orch on an engine with no remote control has, it prints nothing, so the bar
draws no RC button there. Claude Code tells lym nothing of the state, so a
session whose remote control was turned off inside Claude Code still prints
rc-on. A record it cannot read prints as rc-off. reed's status bar runs it at
each redraw.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), remoteControlBarWord(dir))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "run directory of the session whose bar is drawn")
	return cmd
}

// remoteControlBarWord returns the RC word of the worktree at dir, empty when dir resolves to no worktree.
func remoteControlBarWord(dir string) string {
	if dir == "" {
		return ""
	}
	location, err := lymcwd.Resolve(dir)
	if err != nil {
		return ""
	}
	return remoteControlBarWordAt(orchroute.Paths(location))
}

// remoteControlBarWordAt returns the RC word of the orch whose files paths name.
// An orch state that records no strand, or none at all, gives no word, so the bar draws no RC button.
// So does a strand the remote-control record does not name, such as an orch on an engine with no remote control.
// A recorded strand that reads on gives RemoteControlLit.
// A recorded strand that reads off, or whose record cannot be read, gives RemoteControlUnlit.
func remoteControlBarWordAt(paths orchengine.Paths) string {
	st, found, err := orchengine.PeekState(paths)
	if err != nil || !found || st.Strand == "" {
		return ""
	}
	rec, recorded, err := orchengine.ReadRemoteControl(paths, st.Strand)
	switch {
	case err != nil:
		return reedengine.RemoteControlUnlit
	case !recorded:
		return ""
	case rec.On:
		return reedengine.RemoteControlLit
	}
	return reedengine.RemoteControlUnlit
}

// remoteControlOnCmd returns the hidden `orch rc-on` verb.
func (c *orchCLI) remoteControlOnCmd() *cobra.Command {
	var tmuxPath, socketPath, client string
	cmd := &cobra.Command{
		Use:         remoteControlOnVerb + " --socket <path> --tmux <path> --client <name>",
		Short:       "turn on remote control in the orch session, as a click on reed's RC button asks",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `rc-on is what a click on reed's RC button runs, from the clicked session's
worktree. It queues creel's remote-control command into the worktree's live orch
strand, under the name orch.yaml's remote_control_name forms, whether or not
remote_control is on, and records the strand's remote control on. The command
is typed once the session is idle. A worktree with no live orch strand or with
several, an orch on an engine with no remote control, and a strand whose record
already reads on, refuse and send nothing. A session whose remote control was
turned off inside Claude Code takes /remote-control typed by hand in its pane.

It then redraws the clicking client's status bar on the server at --socket,
showing the refusal there first when the click failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			fields, err := c.turnOnRemoteControl()
			message := ""
			if err != nil {
				message = "lym: " + err.Error()
			}
			reedengine.ShowToggleOutcome(tmuxPath, socketPath, client, message)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
	cmd.Flags().StringVar(&socketPath, "socket", "", "socket path of the tmux server whose bar was clicked")
	cmd.Flags().StringVar(&tmuxPath, "tmux", "", "path of the tmux binary")
	cmd.Flags().StringVar(&client, "client", "", "the clicking tmux client")
	for _, flag := range []string{"socket", "tmux", "client"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}
