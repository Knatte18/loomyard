// toggle.go implements the hidden verbs behind reed's two Slack toggles:
// `bar`, which prints the machine's Slack state the toggles draw, and `toggle`, which a click on one of them runs.

package slackcli

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
	// slackToggleOff and awayToggleOff are the words of the state line for a toggle that is off.
	slackToggleOff = "slack-off"
	awayToggleOff  = "away-off"
)

// barCmd returns the hidden `slack bar` verb.
func (c *slackCLI) barCmd() *cobra.Command {
	return &cobra.Command{
		Use:         barVerb,
		Short:       "print the machine's Slack state for reed's slack and away toggles",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `bar prints the machine's Slack state on one line for reed's status bar:
slack-on while a relay holds the machine's Slack connection and slack-off
otherwise, then away-on while the presence is slack and away-off otherwise. A
state it cannot read prints as off. It reads no hub, and reed's status bar runs
it at each redraw.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), c.barLine())
			return nil
		},
	}
}

// barLine returns the state line `bar` prints: the slack word, then the away word, each lit while its state is on.
// A state that cannot be read prints as off.
func (c *slackCLI) barLine() string {
	slack, away := slackToggleOff, awayToggleOff
	paths, err := c.machine()
	if err != nil {
		return slack + " " + away
	}
	if connected, _ := slackengine.Connected(paths); connected {
		slack = reedengine.SlackToggleLit
	}
	if presence, err := slackengine.ReadPresence(paths.PresencePath); err == nil && presence == slackengine.PresenceSlack {
		away = reedengine.AwayToggleLit
	}
	return slack + " " + away
}

// toggleCmd returns the hidden `slack toggle` verb.
func (c *slackCLI) toggleCmd() *cobra.Command {
	var tmuxPath, socketPath, client string
	cmd := &cobra.Command{
		Use:         "toggle <toggle> --socket <path> --tmux <path> --client <name>",
		Short:       "flip the Slack connection or the presence, as a click on reed's toggle asks",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `toggle is what a click on one of reed's Slack toggles runs, from the clicked
session's worktree.

toggle slack, while a relay holds the machine's Slack connection, switches the
connection off and ends the relay of every live hub of the machine, so no dobby
tick starts one again. Otherwise it switches the connection on and starts this
hub's relay, as start does. It never changes the presence.

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

// toggle flips the named toggle: slack switches the machine's connection, and away the presence.
func (c *slackCLI) toggle(name string) (map[string]any, error) {
	switch name {
	case toggleSlack:
		connected, err := slackengine.Connected(c.geom.MachinePaths())
		if err != nil {
			return nil, err
		}
		if connected {
			return c.switchOff()
		}
		return c.start()
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

// switchOff switches the machine's Slack connection off and ends the relay of every live hub of the machine.
// This hub's relay ends first, and the presence is left as it is.
// The switch stands before any relay ends, so no start made meanwhile brings one back.
// A hub whose relay will not end does not stop the others from being ended; the refusal names each with its error.
func (c *slackCLI) switchOff() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("toggle slack", "")); err != nil {
		return nil, err
	}
	if err := slackengine.WriteSwitchedOff(c.geom.SwitchPath, true, c.clock.Now()); err != nil {
		return nil, err
	}
	hubs, err := c.liveHubs()
	if err != nil {
		return nil, fmt.Errorf("slack: the connection is switched off, but the machine's live hubs could not be read: %w; way forward: run `lym slack stop` in each hub of this machine", err)
	}
	geometries := []slackengine.Geometry{c.geom}
	for _, hub := range hubs {
		if hub.HubPath != c.geom.HubPath {
			geometries = append(geometries, hub)
		}
	}
	ended := 0
	var failures []string
	for _, hub := range geometries {
		relays, err := c.stop(hub, os.Getpid())
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", hub.HubPath, err))
			continue
		}
		ended += len(relays)
	}
	if len(failures) > 0 {
		return nil, errors.New("slack: the connection is switched off, but the relay was not ended in: " + strings.Join(failures, ", ") + "; way forward: run `lym slack stop` in each of those hubs")
	}
	return map[string]any{"connection": "off", "relays_ended": ended}, nil
}
