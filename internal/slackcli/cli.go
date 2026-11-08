// cli.go builds the `lym slack` cobra command: its operator verbs and the hidden relay verb.
// It also defines the slackCLI whose seams the verbs read.

package slackcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/slackengine"
)

const (
	// relayVerb is the name of the hidden verb that a started relay process runs.
	relayVerb = "relay"
	// botScopes names the bot scopes the app needs, in the words the way-forward messages use.
	botScopes = "channels:history and channels:read for a public channel or groups:history and groups:read for a private one, users:read and chat:write"
	// noTokenMessage is the refusal of a verb that needs the machine's token when none is stored.
	noTokenMessage = "slack: no Slack token on this machine; way forward: run `lym slack login` from the operator's terminal"
	// missingScopeCode is the error code Slack answers a call the token's scopes do not cover with.
	missingScopeCode = "missing_scope"
	// tokenPrompt is what `login` prints on a terminal before it reads the token with echo off.
	tokenPrompt = "Paste the Slack app's Bot User OAuth Token (xoxb-…) and press Enter: "
)

// slackChecks is the Slack calls that `login` and `setup` make to check the token, a channel and a user.
type slackChecks interface {
	// AuthTest checks the token.
	AuthTest(ctx context.Context) error
	// ConversationInfo reads the channel.
	ConversationInfo(ctx context.Context, channel string) (slackengine.ChannelInfo, error)
	// UserInfo checks that the Slack user exists.
	UserInfo(ctx context.Context, user string) error
}

// slackCLI holds what the verbs read: the hub's geometry and the seams to the engine, creel, Slack and the hub.
// PersistentPreRunE fills every field its verb reads, and a test sets them to fakes.
type slackCLI struct {
	// geom is the hub's Slack geometry; the verbs of a hub fill it, and relay needs none.
	geom slackengine.Geometry
	// slackBaseURL is the Slack Web API's base URL.
	slackBaseURL string
	// clock is the time source for the presence change and the wait for the relay's first read.
	clock slackengine.Clock

	// resolveLocation resolves the worktree a verb runs in from its cwd.
	resolveLocation func(cwd string) (*lymcwd.Location, error)
	// wire fills the seams below for the verb about to run; location is nil for relay, which runs from no worktree.
	wire func(c *slackCLI, location *lymcwd.Location) error

	// pane describes the process the verb runs in, which the pane check reads to decide whether the verb may run.
	pane func() paneCaller
	// readHiddenLine prints prompt and reads one line with echo off when in is a terminal, reporting onTerminal;
	// it reads nothing from any other input.
	readHiddenLine func(in io.Reader, prompt string) (line string, onTerminal bool, err error)
	// newSlack builds the Slack calls that `login` and `setup` make, against baseURL with token.
	newSlack func(baseURL, token string) slackChecks
	// newMailbox builds the creel mailbox signing as the SLACK program, which carries the presence notices.
	newMailbox func() slackengine.Mailbox
	// ensure starts the relay when it is down, for the hub's prime.
	// It reports whether the relay started or was already live.
	ensure func(geom slackengine.Geometry) (slackengine.StartOutcome, error)
	// stop ends the relay, sparing the process with callerPID and its ancestors.
	stop func(geom slackengine.Geometry, callerPID int) ([]runproc.Ended, error)
	// awaitFirstRead waits until the started relay has made its first creel read.
	awaitFirstRead func(geom slackengine.Geometry, clock slackengine.Clock, bound time.Duration) error
	// beams lists the slugs of the hub's beams.
	beams func() ([]string, error)
	// paused reports whether a pause covers the prime orch.
	paused func() (bool, error)

	// spawnIntermediate is the double fork's intermediate step: it starts the relay detached and returns.
	spawnIntermediate func(hubPath, primeName string) error
	// awaitReparent waits until the relay is no longer a child of the intermediate process.
	awaitReparent func(intermediate int) error
	// runRelay runs the hub's relay until ctx ends.
	runRelay func(ctx context.Context, hubPath, primeName string) error
}

// Command returns the cobra command for `lym slack`: the group, its operator verbs and the hidden relay.
func Command() *cobra.Command {
	cli := productionCLI()
	group := cli.command()
	group.PersistentPreRunE = cli.preRun
	return group
}

// command builds the slack group and its verbs over c's seams.
func (c *slackCLI) command() *cobra.Command {
	slack := &cobra.Command{
		Use:   "slack",
		Short: "relay the operator's Slack channel to the hub's orchs and post their mail back",
		Long: `slack connects a hub to one Slack channel, so the operator can talk to the
hub's orchs while away. login stores the app's bot token on this machine and
setup names the hub's channel and the one operator whose messages count. While
the presence is slack, a relay forwards the operator's channel messages to the
orchs as creel mail from SLACK and posts the orchs' mail for SLACK to the
channel. away and back set the presence, start and stop run the relay, and
status shows both.`,
		RunE: clihelp.GroupRunE,
	}
	slack.AddCommand(c.loginCmd(), c.setupCmd(), c.startCmd(), c.stopCmd(), c.awayCmd(), c.backCmd(), c.statusCmd(), c.relayCmd())
	// Slack reads no stencils, and the relay is a long-lived process that must never commit seed writes into the hub.
	for _, cmd := range append([]*cobra.Command{slack}, slack.Commands()...) {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[clihelp.SkipStencilSeedAnnotation] = clihelp.AnnotationEnabled
	}
	return slack
}

// preRun fills the seams the verb about to run reads.
// A verb of a hub first resolves the worktree it runs in.
// relay resolves none, since it runs from the hub directory, which is no worktree.
// A failure is written as a refusal and aborts the verb.
// The bare group needs nothing and prints its help.
func (c *slackCLI) preRun(cmd *cobra.Command, args []string) error {
	if cmd.HasSubCommands() {
		return nil
	}
	var location *lymcwd.Location
	err := func() error {
		if cmd.Name() != relayVerb {
			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				return err
			}
			if location, err = c.resolveLocation(cwd); err != nil {
				return err
			}
		}
		return c.wire(c, location)
	}()
	if err != nil {
		clihelp.Abort(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
	}
	return nil
}

// runVerb runs body and prints the fields it returns as the success envelope, or its error as a refusal that exits 1.
func (c *slackCLI) runVerb(cmd *cobra.Command, body func() (map[string]any, error)) error {
	ctx := cmd.Context()
	if clihelp.ShouldAbort(ctx) {
		return nil
	}
	fields, err := body()
	if err != nil {
		clihelp.SetExit(ctx, output.Err(cmd.OutOrStdout(), err.Error()))
		return nil
	}
	clihelp.SetExit(ctx, output.Ok(cmd.OutOrStdout(), fields))
	return nil
}

// operatorAnnotations marks a command as one the operator runs.
func operatorAnnotations() map[string]string {
	return map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator}
}

// terminalWayForward is the way forward of a verb only the operator's terminal may run.
func terminalWayForward(verb string) string {
	return fmt.Sprintf("run `lym slack %s` from the operator's terminal", verb)
}

// orchWayForward is the way forward of a verb the operator's terminal or an orch's pane may run.
// It adds the Slack text that does the same when slackText is not empty.
func orchWayForward(verb, slackText string) string {
	way := fmt.Sprintf("run `lym slack %s` from the operator's terminal or an orch's pane", verb)
	if slackText != "" {
		way += fmt.Sprintf(", or send the Slack text `%s`", slackText)
	}
	return way
}

// loginCmd returns the `slack login` verb.
func (c *slackCLI) loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "login",
		Short:       "store the Slack app's bot token on this machine",
		Annotations: operatorAnnotations(),
		Long: `login reads the app's Bot User OAuth Token from standard input, never an
argument, checks it with Slack and only then stores it in this machine's
credential file, private to the user. Slack rejecting the token stores nothing.
On a terminal it prompts for the token and reads one line with echo off, so the
token never shows; otherwise it reads standard input to its end, so a script
can pipe the token in.

Create a Slack app, give it the bot scopes channels:history and channels:read
for a public channel or groups:history and groups:read for a private one,
users:read and chat:write, install it, and copy the token (xoxb-...) from the
app's OAuth & Permissions page. Run it from the operator's terminal; a strand
refuses.

Example:
  lym slack login`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) { return c.login(cmd) })
		},
	}
}

// login stores the token read from standard input once Slack accepts it.
func (c *slackCLI) login(cmd *cobra.Command) (map[string]any, error) {
	if err := paneRefusal(c.pane(), false, terminalWayForward("login")); err != nil {
		return nil, err
	}
	token, err := c.readToken(cmd.InOrStdin())
	if err != nil {
		return nil, err
	}
	if err := c.newSlack(c.slackBaseURL, token).AuthTest(cmd.Context()); err != nil {
		return nil, tokenRefusal(err)
	}
	if err := slackengine.WriteToken(c.geom.TokenPath, token); err != nil {
		return nil, err
	}
	return map[string]any{"token_file": c.geom.TokenPath}, nil
}

// readToken reads the trimmed token from in: one hidden line after a prompt on a terminal, and all of in otherwise.
// An empty token is refused, so no empty token reaches Slack.
func (c *slackCLI) readToken(in io.Reader) (string, error) {
	line, onTerminal, err := c.readHiddenLine(in, tokenPrompt)
	if err != nil {
		return "", fmt.Errorf("slack: read the token from the terminal: %w; nothing was stored; way forward: run `lym slack login` again", err)
	}
	if onTerminal {
		token := strings.TrimSpace(line)
		if token == "" {
			return "", errors.New("slack: no token entered at the prompt; way forward: run `lym slack login` again and paste the app's Bot User OAuth Token (`xoxb-…`) at the prompt")
		}
		return token, nil
	}
	raw, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("slack: read the token from standard input: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("slack: no token on standard input; way forward: pipe the app's Bot User OAuth Token (`xoxb-…`) into `lym slack login`, or run it from a terminal and paste the token at the prompt")
	}
	return token, nil
}

// tokenRefusal is the refusal of a token Slack's auth check failed on.
func tokenRefusal(err error) error {
	var refused *slackengine.APIError
	if errors.As(err, &refused) {
		return fmt.Errorf("slack: Slack rejected the token with %s; way forward: copy the app's Bot User OAuth Token (`xoxb-…`) from its OAuth & Permissions page and run `lym slack login` again", refused.Code)
	}
	return fmt.Errorf("slack: the token could not be checked: %w; way forward: check the network and run `lym slack login` again", err)
}

// setupCmd returns the `slack setup` verb.
func (c *slackCLI) setupCmd() *cobra.Command {
	var channel, operator string
	cmd := &cobra.Command{
		Use:         "setup --channel <id> --operator <user-id>",
		Short:       "name the hub's Slack channel and operator",
		Annotations: operatorAnnotations(),
		Long: `setup checks the channel and the operator with Slack and only then records them
for this hub. The relay reads the channel and forwards only the messages of that
one operator. Invite the app to the channel first, with /invite in the channel.
Copy the operator's Slack user id from their profile, under More, Copy member
ID. Give each machine's hub its own channel.

Run it from the operator's terminal; a strand refuses.

Example:
  lym slack setup --channel C0123456789 --operator U0123456789`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) { return c.setup(cmd.Context(), channel, operator) })
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "Slack id of the channel the relay reads and posts in")
	cmd.Flags().StringVar(&operator, "operator", "", "Slack user id of the operator whose messages the relay forwards")
	_ = cmd.MarkFlagRequired("channel")
	_ = cmd.MarkFlagRequired("operator")
	return cmd
}

// setup records the channel and the operator once Slack confirms both.
func (c *slackCLI) setup(ctx context.Context, channel, operator string) (map[string]any, error) {
	if err := paneRefusal(c.pane(), false, terminalWayForward("setup")); err != nil {
		return nil, err
	}
	token, found, err := slackengine.ReadToken(c.geom.TokenPath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New(noTokenMessage)
	}
	api := c.newSlack(c.slackBaseURL, token)
	info, err := api.ConversationInfo(ctx, channel)
	if err != nil {
		return nil, channelRefusal(channel, err)
	}
	if !info.IsMember {
		return nil, fmt.Errorf("slack: the bot is not a member of channel %s; way forward: run `/invite @<app>` in that channel, naming this app, then run `lym slack setup` again", channel)
	}
	if err := api.UserInfo(ctx, operator); err != nil {
		return nil, userRefusal(operator, err)
	}
	if err := slackengine.WriteSetup(c.geom.SetupPath, slackengine.Setup{ChannelID: channel, OperatorID: operator}); err != nil {
		return nil, err
	}
	return map[string]any{"channel": channel, "channel_name": info.Name, "operator": operator}, nil
}

// channelRefusal is the refusal of a channel Slack's channel check failed on.
func channelRefusal(channel string, err error) error {
	var refused *slackengine.APIError
	if !errors.As(err, &refused) {
		return fmt.Errorf("slack: channel %s could not be checked: %w; way forward: check the network and run `lym slack setup` again", channel, err)
	}
	switch refused.Code {
	case "channel_not_found":
		return fmt.Errorf("slack: Slack shows no channel %s to the bot (channel_not_found), as it does for a private channel the bot was not invited to; way forward: run `/invite @<app>` in that channel, naming this app, then run `lym slack setup` again", channel)
	case missingScopeCode:
		return scopeRefusal()
	}
	return fmt.Errorf("slack: Slack refused the check of channel %s with %s; way forward: fix what that code names, then run `lym slack setup` again", channel, refused.Code)
}

// userRefusal is the refusal of an operator Slack's user check failed on.
func userRefusal(operator string, err error) error {
	var refused *slackengine.APIError
	if !errors.As(err, &refused) {
		return fmt.Errorf("slack: user %s could not be checked: %w; way forward: check the network and run `lym slack setup` again", operator, err)
	}
	switch refused.Code {
	case "user_not_found":
		return fmt.Errorf("slack: Slack knows no user %s (user_not_found); way forward: copy the operator's member id from their Slack profile, under More, Copy member ID, and run `lym slack setup` again", operator)
	case missingScopeCode:
		return scopeRefusal()
	}
	return fmt.Errorf("slack: Slack refused the check of user %s with %s; way forward: fix what that code names, then run `lym slack setup` again", operator, refused.Code)
}

// scopeRefusal is the refusal of a check the token's scopes do not cover.
func scopeRefusal() error {
	return fmt.Errorf("slack: the token lacks a bot scope (%s); way forward: give the app %s on its OAuth & Permissions page, reinstall the app, then run `lym slack setup` again", missingScopeCode, botScopes)
}

// startCmd returns the `slack start` verb.
func (c *slackCLI) startCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "start",
		Short:       "start the hub's Slack relay when it is down",
		Annotations: operatorAnnotations(),
		Long: `start brings up the hub's relay, which runs out of every pane, and reports
whether it started or was already live. It needs login and setup first. The
operator's terminal and an orch's pane may run it; any other strand refuses.

Example:
  lym slack start`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.start)
		},
	}
}

// start starts the relay once login and setup are done.
func (c *slackCLI) start() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("start", "")); err != nil {
		return nil, err
	}
	if _, _, err := slackengine.RequireConfigured(c.geom); err != nil {
		return nil, err
	}
	outcome, err := c.ensure(c.geom)
	if err != nil {
		return nil, err
	}
	return map[string]any{"relay": string(outcome)}, nil
}

// stopCmd returns the `slack stop` verb.
func (c *slackCLI) stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Short:       "set the presence to terminal and end the hub's Slack relay",
		Annotations: operatorAnnotations(),
		Long: `stop sets the presence to terminal, telling the orchs, and then ends the relay,
so no tick of the prime's dobby starts it again. If ending the relay fails,
run it again: the presence is then unchanged and no notice is sent twice. The
operator's terminal and an orch's pane may run it; any other strand refuses.

Example:
  lym slack stop`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.stopRelay)
		},
	}
}

// stopRelay sets the presence terminal and ends the relay.
func (c *slackCLI) stopRelay() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("stop", "")); err != nil {
		return nil, err
	}
	fields, err := c.setPresence(slackengine.PresenceTerminal)
	if err != nil {
		return nil, err
	}
	ended, err := c.stop(c.geom, os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("slack: the presence is terminal but the relay was not stopped: %w; way forward: run `lym slack stop` again", err)
	}
	fields["relay_ended"] = len(ended)
	return fields, nil
}

// awayCmd returns the `slack away` verb.
func (c *slackCLI) awayCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "away",
		Short:       "set the presence to slack once the relay is up",
		Annotations: operatorAnnotations(),
		Long: `away starts the relay when it is down, waits for its first creel read, and
only then sets the presence to slack, telling the orchs. A relay that cannot
read its mail refuses it, naming why. The operator's terminal and an orch's pane
may run it, and so may the Slack text away; any other strand refuses.

Example:
  lym slack away`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.away)
		},
	}
}

// away starts the relay, waits for it to read creel and sets the presence slack.
func (c *slackCLI) away() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("away", "away")); err != nil {
		return nil, err
	}
	if _, _, err := slackengine.RequireConfigured(c.geom); err != nil {
		return nil, err
	}
	outcome, err := c.ensure(c.geom)
	if err != nil {
		return nil, err
	}
	if err := c.awaitFirstRead(c.geom, c.clock, slackengine.FirstReadBound); err != nil {
		return nil, err
	}
	fields, err := c.setPresence(slackengine.PresenceSlack)
	if err != nil {
		return nil, err
	}
	fields["relay"] = string(outcome)
	return fields, nil
}

// backCmd returns the `slack back` verb.
func (c *slackCLI) backCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "back",
		Short:       "set the presence to terminal",
		Annotations: operatorAnnotations(),
		Long: `back sets the presence to terminal, telling the orchs, and leaves the relay
running. The operator's terminal and an orch's pane may run it, and so may the
Slack text back; any other strand refuses.

Example:
  lym slack back`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.back)
		},
	}
}

func (c *slackCLI) back() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("back", "back")); err != nil {
		return nil, err
	}
	return c.setPresence(slackengine.PresenceTerminal)
}

// setPresence writes presence and tells the orchs when it changed.
// A write that stood but whose notice failed is reported under notice_error, not as a failure.
func (c *slackCLI) setPresence(presence slackengine.Presence) (map[string]any, error) {
	changed, err := slackengine.SetPresence(c.geom, c.newMailbox(), c.beams, presence, c.clock.Now())
	fields := map[string]any{"presence": string(presence), "changed": changed}
	switch {
	case err != nil && !changed:
		return nil, err
	case err != nil:
		fields["notice_error"] = err.Error()
	}
	return fields, nil
}

// statusCmd returns the `slack status` verb.
func (c *slackCLI) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "show the presence, the relay and the hub's Slack setup",
		Annotations: operatorAnnotations(),
		Long: `status prints the presence, whether the relay is live, whether a pause covers
the prime orch, the channel and the operator the setup names, and when a Slack
call of the relay last succeeded. Any process may run it.

Example:
  lym slack status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.status)
		},
	}
}

// status reads the presence, the relay's health, the pause and the setup.
func (c *slackCLI) status() (map[string]any, error) {
	presence, err := slackengine.ReadPresence(c.geom.PresencePath)
	if err != nil {
		return nil, err
	}
	live, err := slackengine.RelayLive(c.geom)
	if err != nil {
		return nil, err
	}
	paused, err := c.paused()
	if err != nil {
		return nil, err
	}
	fields := map[string]any{"presence": string(presence), "relay_live": live, "paused": paused}
	setup, found, err := slackengine.ReadSetup(c.geom.SetupPath)
	if err != nil {
		return nil, err
	}
	if found {
		fields["channel"], fields["operator"] = setup.ChannelID, setup.OperatorID
	}
	relayState, found, err := slackengine.ReadRelayState(c.geom)
	if err != nil {
		return nil, err
	}
	if found && !relayState.LastSlackOK.IsZero() {
		fields["last_slack_ok"] = relayState.LastSlackOK.UTC().Format(time.RFC3339)
	}
	return fields, nil
}

// relayCmd returns the hidden `slack relay` verb, which the relay's start runs.
func (c *slackCLI) relayCmd() *cobra.Command {
	var (
		hubPath string
		prime   string
		fork    bool
		after   int
	)
	cmd := &cobra.Command{
		Use:         relayVerb + " --hub-path <path> --prime <name>",
		Short:       "run the Slack relay of one hub",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `relay is what the start of the relay runs, never an operator. With --fork it is
the double fork's intermediate: it starts the relay detached and exits at once.
Otherwise it is the relay: with --after it first waits until the process
that started it has exited, then it serves the hub at --hub-path until a signal
ends it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) {
				return c.relay(cmd, hubPath, prime, fork, after, cmd.Flags().Changed("after"))
			})
		},
	}
	cmd.Flags().StringVar(&hubPath, "hub-path", "", "absolute path of the hub to serve")
	cmd.Flags().StringVar(&prime, "prime", "", "name of the hub's prime worktree")
	cmd.Flags().BoolVar(&fork, "fork", false, "start the relay detached and exit, as the double fork's intermediate")
	cmd.Flags().IntVar(&after, "after", 0, "wait until the process with this pid, the relay's start, is no longer its parent")
	_ = cmd.MarkFlagRequired("hub-path")
	_ = cmd.MarkFlagRequired("prime")
	return cmd
}

// relay runs the double fork's intermediate when fork is set, and otherwise the relay itself.
// A relay told afterGiven first waits for its start process to exit, and exits with the wait's error when it does not.
func (c *slackCLI) relay(cmd *cobra.Command, hubPath, prime string, fork bool, after int, afterGiven bool) (map[string]any, error) {
	if !filepath.IsAbs(hubPath) {
		return nil, fmt.Errorf("slack: --hub-path %q is not an absolute path; way forward: pass the hub's absolute path", hubPath)
	}
	if fork {
		if err := c.spawnIntermediate(hubPath, prime); err != nil {
			return nil, err
		}
		return map[string]any{"relay": "spawned"}, nil
	}
	if afterGiven {
		if err := c.awaitReparent(after); err != nil {
			return nil, err
		}
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := c.runRelay(ctx, hubPath, prime); err != nil {
		return nil, err
	}
	return map[string]any{"relay": "stopped"}, nil
}

// RunCLI is the public seam for the slack module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the slack module CLI with cwd seeded into its execution context.
// An empty cwd reads the process cwd instead, since lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
