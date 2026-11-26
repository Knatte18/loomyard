// remotecontrol.go holds remote control for the orch and for every agent reed's RC button serves.
// It forms the session name a launch of the orch passes,
// and it marks the launched pane for the button once the session records its remote control connecting.
// It also holds the hidden verb `rc-on`, which a click on the RC button runs.

package orchcli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchroute"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// remoteControlOnVerb is the name of the hidden verb a click on reed's RC button runs.
	remoteControlOnVerb = reedengine.RemoteControlOnVerb
	// remoteControlProgram is the program name the RC click signs its creel command with.
	remoteControlProgram = "REMOTECONTROL"
)

// orchEngine is the engine the orch session runs on, whatever shuttle.yaml's default engine is:
// the orch role is worded for it.
const orchEngine = "claude"

// orchModelChoice returns the engine, model and effort the orch session runs on.
func (c *orchCLI) orchModelChoice() shuttleengine.ModelChoice {
	return shuttleengine.ModelChoice{Engine: orchEngine, Model: c.cfg.Model, Effort: c.cfg.Effort}
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

// remoteControlSessionName returns the remote session's name of the strand strandName on this machine.
// It fills orch.yaml's remote_control_name.
func (c *orchCLI) remoteControlSessionName(strandName string) (string, error) {
	parsed, err := agentname.Parse(strandName)
	if err != nil {
		return "", fmt.Errorf("orch: the strand name %q does not parse: %w", strandName, err)
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

// launchRemoteControlCheckpoint returns how far the orch session sessionID's record reaches before a launch resumes it.
// A launch without remote control, or one with no session to resume, starts from zero.
// read is false when the record cannot be read; the launch then leaves its pane unmarked, logged at Warn.
func (c *orchCLI) launchRemoteControlCheckpoint(sessionID, remoteControlName string) (checkpoint int64, read bool) {
	if remoteControlName == "" || sessionID == "" {
		return 0, true
	}
	checkpoint, err := c.remoteControlCheckpoint(sessionID)
	if err != nil {
		logger.Warn("orch: could not read where the orch session's record ends, so the RC button stays dim after the launch", "sessionID", sessionID, "err", err)
		return 0, false
	}
	return checkpoint, true
}

// markLaunchedRemoteControl marks the pane of the strand started, which a launch asked to start with remote control on.
// It marks only once the session records its remote control connecting, so reed's RC button shows the orch lit only then.
// The record counts past checkpoint when started resumed the session resumed, and whole when it started another session.
// A connection the provider refuses, such as by an organization's policy, leaves the pane unmarked and the button dim.
// A failed read or mark is logged at Warn and never fails the caller, since the mark is display only.
func (c *orchCLI) markLaunchedRemoteControl(started startedSession, resumed string, checkpoint int64) {
	if c.markRemoteControl == nil {
		return
	}
	if started.SessionID != resumed {
		checkpoint = 0
	}
	connected, err := c.awaitRemoteControl(started.SessionID, checkpoint)
	if err != nil {
		logger.Warn("orch: could not read whether the orch session's remote control connected, so the RC button stays dim", "strandGUID", started.GUID, "err", err)
		return
	}
	if !connected {
		logger.Info("orch: the orch session recorded no remote-control connection, so the RC button stays dim", "strandGUID", started.GUID, "sessionID", started.SessionID)
		return
	}
	if err := c.markRemoteControl(started.GUID); err != nil {
		logger.Warn("orch: could not mark the orch pane's remote control for the RC button", "strandGUID", started.GUID, "err", err)
	}
}

// turnOnRemoteControl queues the remote-control command into the agent pane runs, through creel.
// The name is the one remote_control_name forms for the agent, whatever orch.yaml's switch says.
// It marks nothing: creel marks the pane once the session records its remote control connecting.
// It refuses a pane that runs no agent and a pane whose mark already reads on; both send nothing.
// An agent whose engine has no remote control gets nothing typed.
func (c *orchCLI) turnOnRemoteControl(pane reedengine.AgentPane) (map[string]any, error) {
	if pane.StrandName == "" {
		return nil, errors.New("orch: the active pane runs no agent; way forward: select an agent's pane and click RC again")
	}
	if pane.RemoteControlOn {
		return nil, fmt.Errorf("orch: remote control is already on for %s, so nothing was sent; way forward: when you turned it off inside Claude Code, type `/remote-control` by hand in the agent's pane", pane.StrandName)
	}
	name, err := c.remoteControlSessionName(pane.StrandName)
	if err != nil {
		return nil, err
	}
	if err := c.injectRemoteControl(pane.StrandName, name); err != nil {
		return nil, fmt.Errorf("orch: queue the remote-control command for %s: %w; way forward: repair what the error names and click RC again", pane.StrandName, err)
	}
	return map[string]any{"strand": pane.StrandName, "remote_control_name": name}, nil
}

// injectRemoteControlCommand queues creel's remote-control command with name into the strand receiver.
// It signs the command as the RC click's program.
func injectRemoteControlCommand(location *lymcwd.Location, receiver, name string) error {
	client, err := creelcli.NewProgramClient(location, remoteControlProgram)
	if err != nil {
		return err
	}
	_, err = client.InjectCommand(receiver, creelengine.Command{Name: creelengine.CommandRemoteControl, Argument: name}, 0)
	return err
}

// remoteControlOnCmd returns the hidden `orch rc-on` verb.
func (c *orchCLI) remoteControlOnCmd() *cobra.Command {
	var tmuxPath, socketPath, client string
	cmd := &cobra.Command{
		Use:         remoteControlOnVerb + " --socket <path> --tmux <path> --client <name>",
		Short:       "turn on remote control for the agent of the clicked window, as a click on reed's RC button asks",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `rc-on is what a click on reed's RC button runs, from the clicked session's run
directory: the session's own worktree for the prime and a beam, the parent's for
a task. It acts on the agent of the clicking client's active pane. It queues
creel's remote-control command into that agent, under the name
` + orchKeyName("remote_control_name") + ` forms, whether or not ` + orchKeyName("remote_control") + ` is on. The command is
typed once the session is idle, and the pane is marked so the RC button shows
lit only once the session records its remote control connecting. A connection
the provider refuses, such as by an organization's policy, leaves the button
dim. A pane that runs no agent and a pane already marked refuse and send
nothing. A session whose remote control was turned off inside Claude Code takes
/remote-control typed by hand in its pane.

It then redraws the clicking client's status bar on the server at --socket,
showing the refusal there first when the click failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			var fields map[string]any
			pane, err := reedengine.ClientAgentPane(tmuxPath, socketPath, client)
			if err == nil {
				fields, err = c.turnOnRemoteControl(pane)
			}
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
