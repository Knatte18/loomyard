// toggle.go implements the hidden verbs behind reed's two Slack toggles:
// `bar`, which prints the Slack state the toggles draw, and `toggle`, which a click on one of them runs.

package slackcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
)

const (
	// barVerb is the name of the hidden verb reed's status bar runs for the toggles' state.
	barVerb = "bar"
	// toggleSlack and toggleAway are the toggles `toggle` takes, named as reed's ranges name them.
	toggleSlack = "slack"
	toggleAway  = "away"
	// awayToggleOff is the word of the state line for the away toggle while it is off.
	awayToggleOff = "away-off"
)

// barCmd returns the hidden `slack bar` verb.
func (c *slackCLI) barCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:         barVerb,
		Short:       "print the Slack state for reed's slack and away toggles",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `bar prints the Slack state on one line for reed's status bar. When --dir
names the worktree of an orch that has a slack button, the prime's or a beam's,
it first prints slack-on while that orch's button is on and slack-off
otherwise; for any other directory it prints no slack word, so the bar draws no
slack button there. It then prints away-on while the machine's presence is
slack and away-off otherwise. A state it cannot read prints as off. reed's
status bar runs it at each redraw.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), c.barLine(dir))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "run directory of the session whose bar is drawn")
	return cmd
}

// barLine returns the state line `bar` prints.
// It holds the slack word of the orch that runs in dir, when it has a button, then the away word.
// Each word is lit while its state is on.
// A state that cannot be read prints as off.
func (c *slackCLI) barLine(dir string) string {
	away := awayToggleOff
	if paths, err := c.machine(); err == nil {
		if presence, err := slackengine.ReadPresence(paths.PresencePath); err == nil && presence == slackengine.PresenceSlack {
			away = reedengine.AwayToggleLit
		}
	}
	slack, hasButton := c.barSlackWord(dir)
	if !hasButton {
		return away
	}
	return slack + " " + away
}

// barSlackWord returns the slack word of the state line for the worktree at dir.
// It also reports whether the orch that runs there has a slack button.
// A directory that resolves to no worktree, or a worktree that runs no orch with a button, has none.
// A button whose state cannot be read is unlit.
func (c *slackCLI) barSlackWord(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	location, err := c.resolveLocation(dir)
	if err != nil {
		return "", false
	}
	if err := c.wire(c, location); err != nil {
		return "", false
	}
	orch, err := c.worktreeOrch()
	if err != nil || orch == "" {
		return "", false
	}
	if on, err := slackengine.ForwardingOn(c.geom.ForwardingPath, orch); err == nil && on {
		return reedengine.SlackToggleLit, true
	}
	return reedengine.SlackToggleUnlit, true
}

// toggleCmd returns the hidden `slack toggle` verb.
func (c *slackCLI) toggleCmd() *cobra.Command {
	var tmuxPath, socketPath, client string
	cmd := &cobra.Command{
		Use:         "toggle <toggle> --socket <path> --tmux <path> --client <name>",
		Short:       "flip an orch's slack button or the presence, as a click on reed's toggle asks",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `toggle is what a click on one of reed's Slack toggles runs, from the clicked
session's worktree.

toggle slack flips the slack button of the orch that runs in that worktree, the
prime's or a beam's. Switched on, it needs login and setup, and it starts the
hub's relay, which makes the orch's channel. Switched off, it leaves the
channel in place and ends the hub's relay once no orch of the hub has its
button on. A worktree whose orch has no button, a pair's, refuses. It never
changes the presence.

toggle away sets the presence to terminal while it is slack, as back does, and
otherwise does what away does.

It then redraws the clicking client's status bar on the server at --socket,
showing the refusal there first when the toggle failed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) {
				fields, err := c.toggle(args[0])
				message := ""
				if err != nil {
					message = "lym: " + err.Error()
				}
				c.showToggle(tmuxPath, socketPath, client, message)
				return fields, err
			})
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

// toggle flips the named toggle: slack the button of the worktree's orch, and away the presence.
func (c *slackCLI) toggle(name string) (map[string]any, error) {
	switch name {
	case toggleSlack:
		return c.toggleForwarding()
	case toggleAway:
		presence, err := slackengine.ReadPresence(c.geom.PresencePath)
		if err != nil {
			return nil, err
		}
		if presence == slackengine.PresenceSlack {
			return c.back()
		}
		return c.away()
	}
	return nil, fmt.Errorf("slack: no toggle %q; way forward: click the slack or the away toggle on reed's status bar", name)
}

// toggleForwarding flips the slack button of the orch that runs in the verb's worktree.
// A worktree whose orch has no button, a pair's or a task's, is refused.
func (c *slackCLI) toggleForwarding() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("toggle slack", "")); err != nil {
		return nil, err
	}
	orch, err := c.worktreeOrch()
	if err != nil {
		return nil, err
	}
	if orch == "" {
		return nil, errors.New("slack: this worktree runs no orch with a slack button, and a pair's orch has none; way forward: click the slack button on the status bar of the prime's or a beam's session")
	}
	on, err := slackengine.ForwardingOn(c.geom.ForwardingPath, orch)
	if err != nil {
		return nil, err
	}
	if on {
		return c.forwardingOff(orch)
	}
	return c.forwardingOn(orch)
}

// forwardingOn switches the orch's button on once login and setup are done, and starts the hub's relay.
// The relay makes the orch's channel at its start or its next wake.
func (c *slackCLI) forwardingOn(orch string) (map[string]any, error) {
	if _, _, err := slackengine.RequireConfigured(c.geom); err != nil {
		return nil, err
	}
	if _, err := slackengine.WriteForwarding(c.geom.ForwardingPath, orch, true, c.clock.Now()); err != nil {
		return nil, err
	}
	outcome, err := c.ensure(c.geom)
	if err != nil {
		return nil, fmt.Errorf("slack: the slack button of %s is on, but the hub's relay did not start: %w; way forward: run `lym slack start`", orch, err)
	}
	return map[string]any{"orch": orch, "forwarding": "on", "relay": string(outcome)}, nil
}

// forwardingOff switches the orch's button off and leaves its channel in place.
// Once no orch of the hub has its button on, it ends the hub's relay, so the machine's connection follows the buttons.
func (c *slackCLI) forwardingOff(orch string) (map[string]any, error) {
	if _, err := slackengine.WriteForwarding(c.geom.ForwardingPath, orch, false, c.clock.Now()); err != nil {
		return nil, err
	}
	fields := map[string]any{"orch": orch, "forwarding": "off"}
	remaining, err := slackengine.ReadForwarding(c.geom.ForwardingPath)
	if err != nil {
		return nil, err
	}
	if len(remaining) > 0 {
		return fields, nil
	}
	ended, err := c.stop(c.geom, os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("slack: the slack button of %s is off, but the hub's relay was not ended: %w; way forward: run `lym slack stop`", orch, err)
	}
	fields["relay_ended"] = len(ended)
	return fields, nil
}
