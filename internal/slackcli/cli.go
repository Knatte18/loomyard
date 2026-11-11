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
	"slices"
	"sort"
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
	// botScopes names the scopes the app needs and the document that lists them, in the words the way-forward messages use.
	botScopes = "the bot scopes channels:history, channels:join, channels:manage, channels:read, chat:write, chat:write.customize and users:read " +
		"and the app-level scope connections:write, as docs/slack-app.md lists them"
	// noTokenMessage is the refusal of a verb that needs the machine's tokens when none is stored.
	noTokenMessage = "slack: no Slack tokens on this machine; way forward: run `lym slack login` from the operator's terminal " +
		"and give the " + botTokenKind + " and the " + appTokenKind + " in one run"
	// missingScopeCode is the error code Slack answers a call the token's scopes do not cover with.
	missingScopeCode = "missing_scope"
	// tokenPrompt is what `login` prints on a terminal before it reads each token with echo off.
	tokenPrompt = "Paste one Slack token, the bot token (xoxb-…) or the app-level token (xapp-…), and press Enter: "
	// botTokenKind and appTokenKind name the two kinds of token `login` takes, in the words the messages use.
	botTokenKind = "bot token (xoxb-)"
	appTokenKind = "app-level token (xapp-)"
	// botTokenPrefix and appTokenPrefix are the prefixes by which `login` files a token under its kind.
	botTokenPrefix = "xoxb-"
	appTokenPrefix = "xapp-"
	// tokensPerLogin is how many tokens one `login` run takes: one of each kind.
	tokensPerLogin = 2
)

// slackChecks is the Slack calls that `login` and `setup` make to check the tokens and a user.
type slackChecks interface {
	// AuthTest checks the token and tells who it belongs to.
	AuthTest(ctx context.Context) (slackengine.Identity, error)
	// BotAppID reads the id of the Slack app the bot belongs to.
	BotAppID(ctx context.Context, botID string) (string, error)
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
	// newDialer builds the Socket Mode dial that `login` makes with the app-level token, against baseURL.
	newDialer func(baseURL, appToken string) slackengine.Dialer
	// liveHubs lists the Slack geometries of the machine's live hubs.
	// `login` ends their relays when it replaces a token.
	liveHubs slackengine.LiveHubs
	// worktreeOrch returns the creel name of the orch that runs in the verb's worktree and has a slack button:
	// the hub's own orch in the prime and a beam's orch in a beam, and the empty string in any other worktree.
	worktreeOrch func() (string, error)
	// machine returns the paths of the machine's own Slack files, which `bar` reads with no hub.
	machine func() (slackengine.MachinePaths, error)
	// strandName returns the name of the strand whose pane the process runs in, empty when it names none.
	strandName func() string
	// newStrandClient builds the creel client of the strand named strand, which `hook` reads the log and posts as.
	newStrandClient func(strand string) (strandClient, error)
	// showToggle redraws the clicking client's status bar, showing message on it first when message is not empty.
	showToggle func(tmuxPath, socketPath, client, message string)
	// ensure starts the relay when it is down, for the hub's prime.
	// It reports whether the relay started or was already live.
	ensure func(geom slackengine.Geometry) (slackengine.StartOutcome, error)
	// stop ends the relay, sparing the process with callerPID and its ancestors.
	stop func(geom slackengine.Geometry, callerPID int) ([]runproc.Ended, error)
	// awaitFirstRead waits until the started relay has made its first creel read.
	awaitFirstRead func(geom slackengine.Geometry, clock slackengine.Clock, bound time.Duration) error
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
		Short: "relay the operator's Slack channels to the hub's orchs and post their mail back",
		Long: `slack connects a hub to Slack, so the operator can talk to the hub's orchs
while away. Each orch with a status bar, the prime's and each beam's, has a
slack button there; an orch gets its own channel once its button is pressed,
named by machine, hub and project, such as neve-ly-p0-root. A second press
switches the orch's forwarding off and leaves the channel in place, and the
relay runs only while some orch of the hub has its button on. login stores the
app's bot and app-level tokens on this machine and setup names the one operator
whose messages count; a running relay takes up a new operator at its next wake,
with no restart. A relay forwards the operator's messages in an orch's channel
to that orch as creel mail from SLACK and posts the orchs' mail for SLACK to
their channels, under the sender's name on this machine, such as
neve::ly:orch.

The presence is the machine's: away holds for every hub and orch on it. away
and back set it, and so do the operator's own words: a message in Slack sets it
to slack, and a line typed in an orch's pane sets it back to terminal. While it
is slack, each turn's final reply of an orch whose button is on goes to its
channel. start and stop run the relay, and status shows both. reed's status
bar shows the orch's button and the presence as its slack and away toggles.`,
		RunE: clihelp.GroupRunE,
	}
	slack.AddCommand(c.loginCmd(), c.setupCmd(), c.startCmd(), c.stopCmd(), c.awayCmd(), c.backCmd(), c.statusCmd(), c.relayCmd(),
		c.hookCmd(), c.barCmd(), c.toggleCmd())
	// Slack reads no stencils, and the relay is a long-lived process that must never commit seed writes into the hub.
	for _, cmd := range append([]*cobra.Command{slack}, slack.Commands()...) {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[clihelp.SkipStencilSeedAnnotation] = clihelp.AnnotationEnabled
	}
	return slack
}

// worktreeFreeVerbs are the verbs that resolve no worktree:
// relay runs from the hub directory, and bar from wherever reed's status bar runs it.
var worktreeFreeVerbs = []string{relayVerb, barVerb}

// preRun fills the seams the verb about to run reads.
// A verb of a hub first resolves the worktree it runs in; a verb of worktreeFreeVerbs resolves none.
// A failure is written as a refusal and aborts the verb.
// The bare group needs nothing and prints its help.
func (c *slackCLI) preRun(cmd *cobra.Command, args []string) error {
	if cmd.HasSubCommands() {
		return nil
	}
	var location *lymcwd.Location
	err := func() error {
		if !slices.Contains(worktreeFreeVerbs, cmd.Name()) {
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
		Short:       "store the Slack app's bot and app-level tokens on this machine",
		Annotations: operatorAnnotations(),
		Long: `login reads two tokens in one run, the app's Bot User OAuth Token (xoxb-...)
and its app-level token (xapp-...), never from an argument. It files each by its
prefix, checks both with Slack and only then stores them in this machine's
credential file, private to the user. Slack rejecting a token, or tokens of two
different apps, stores nothing. On a terminal it prompts for each token and
reads one line with echo off, so a token never shows; otherwise it reads
standard input to its end and splits it at white space, so a script can pipe
both in. Replacing a stored token ends the relay of each live hub of this
machine; run lym slack start in each to bring it back.

Make the Slack app from the manifest in docs/slack-app.md: the bot scopes
channels:history, channels:join, channels:manage, channels:read, chat:write,
chat:write.customize and users:read, and Socket Mode on. Install it, copy the
bot token from the app's OAuth & Permissions page, and make an app-level token
with the scope connections:write on its Basic Information page. Run it from the
operator's terminal; a strand refuses.

Example:
  lym slack login`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) { return c.login(cmd) })
		},
	}
}

// login stores the bot and app-level tokens read from the terminal or standard input once Slack accepts both.
// It ends the relay of each live hub when it replaces a stored token.
func (c *slackCLI) login(cmd *cobra.Command) (map[string]any, error) {
	ctx := cmd.Context()
	if err := paneRefusal(c.pane(), false, terminalWayForward("login")); err != nil {
		return nil, err
	}
	tokens, err := c.readTokens(cmd.InOrStdin())
	if err != nil {
		return nil, err
	}
	credentials, err := fileTokens(tokens)
	if err != nil {
		return nil, err
	}
	api := c.newSlack(c.slackBaseURL, credentials.BotToken)
	identity, err := api.AuthTest(ctx)
	if err != nil {
		return nil, tokenRefusal(botTokenKind, err)
	}
	botAppID, err := api.BotAppID(ctx, identity.BotID)
	if err != nil {
		return nil, tokenRefusal(botTokenKind, err)
	}
	// The connection is opened to prove the app-level token and read its app, and closed at once.
	// An event Slack hands it meanwhile goes unacknowledged, so Slack retries it.
	socket, helloAppID, err := c.newDialer(c.slackBaseURL, credentials.AppToken)(ctx)
	if err != nil {
		return nil, tokenRefusal(appTokenKind, err)
	}
	_ = socket.Close()
	if botAppID == "" {
		return nil, appIDMissingRefusal(botTokenKind)
	}
	if helloAppID == "" {
		return nil, appIDMissingRefusal(appTokenKind)
	}
	if helloAppID != botAppID {
		return nil, fmt.Errorf("slack: the %s belongs to app %s but the %s belongs to app %s; nothing was stored; way forward: run `lym slack login` with an app-level token of app %s, made on that app's Basic Information page",
			appTokenKind, helloAppID, botTokenKind, botAppID, botAppID)
	}
	stored, err := slackengine.ReadCredentials(c.geom.TokenPath)
	if err != nil {
		return nil, err
	}
	replaced := replacesToken(stored.BotToken, credentials.BotToken) || replacesToken(stored.AppToken, credentials.AppToken)
	credentials.AppID = botAppID
	if err := slackengine.WriteCredentials(c.geom.TokenPath, credentials); err != nil {
		return nil, err
	}
	fields := map[string]any{"token_file": c.geom.TokenPath}
	if !replaced {
		return fields, nil
	}
	return fields, c.endLiveRelays(fields)
}

// appIDMissingRefusal is the refusal of a login whose token of the given kind came back from Slack without an app id.
// Without it the two tokens cannot be proven to belong to one app.
func appIDMissingRefusal(kind string) error {
	return fmt.Errorf("slack: Slack gave no app id for the %s, so the two tokens cannot be shown to belong to one app; nothing was stored; way forward: run `lym slack login` again with tokens of the app made from docs/slack-app.md",
		kind)
}

// replacesToken reports whether a login that brings token replaces a stored token that differs from it.
func replacesToken(stored, token string) bool {
	return stored != "" && stored != token
}

// endLiveRelays ends the relay of each hub the machine record lists.
// When every relay ended, it names those hubs under relays_ended in fields.
// The tokens are stored by now, so every refusal says so and names how to bring the relays back.
// A hub whose relay will not end does not stop the others from being ended;
// the one refusal names every hub not ended with its error, and every hub ended.
// The bound: a hub absent from the machine record keeps its older relay until its own `lym slack stop`.
func (c *slackCLI) endLiveRelays(fields map[string]any) error {
	hubs, err := c.liveHubs()
	if err != nil {
		return fmt.Errorf("slack: both tokens are stored, but the machine's live hubs could not be read: %w; way forward: run `lym slack stop` then `lym slack start` in each hub of this machine, since a second login with the same tokens ends no relay", err)
	}
	ended := make([]string, 0, len(hubs))
	var failures []string
	for _, hub := range hubs {
		if _, err := c.stop(hub, os.Getpid()); err != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", hub.HubPath, err))
			continue
		}
		ended = append(ended, hub.HubPath)
	}
	if len(failures) > 0 {
		message := fmt.Sprintf("slack: both tokens are stored, but the relay was not ended in: %s; way forward: run `lym slack stop` then `lym slack start` in each of those hubs",
			strings.Join(failures, ", "))
		if len(ended) > 0 {
			message += fmt.Sprintf("; the relays ended in %s come back with `lym slack start` in each", strings.Join(ended, ", "))
		}
		return errors.New(message)
	}
	if len(ended) > 0 {
		fields["relays_ended"] = ended
		fields["way_forward"] = "run `lym slack start` in each hub under relays_ended"
	}
	return nil
}

// readTokens reads the trimmed tokens from in:
// one hidden line per token after a prompt on a terminal, and all of in split at white space otherwise.
// An empty token is refused, so no empty token reaches Slack.
func (c *slackCLI) readTokens(in io.Reader) ([]string, error) {
	line, onTerminal, err := c.readHiddenLine(in, tokenPrompt)
	if err == nil && !onTerminal {
		raw, err := io.ReadAll(in)
		if err != nil {
			return nil, fmt.Errorf("slack: read the tokens from standard input: %w", err)
		}
		tokens := strings.Fields(string(raw))
		if len(tokens) == 0 {
			return nil, fmt.Errorf("slack: no token on standard input; way forward: pipe the %s and the %s into `lym slack login`, or run it from a terminal and paste each token at the prompt", botTokenKind, appTokenKind)
		}
		return tokens, nil
	}
	tokens := make([]string, 0, tokensPerLogin)
	for {
		if err != nil {
			return nil, fmt.Errorf("slack: read a token from the terminal: %w; nothing was stored; way forward: run `lym slack login` again", err)
		}
		token := strings.TrimSpace(line)
		if token == "" {
			return nil, fmt.Errorf("slack: no token entered at the prompt; way forward: run `lym slack login` again and paste the %s and the %s at the prompts", botTokenKind, appTokenKind)
		}
		tokens = append(tokens, token)
		if len(tokens) == tokensPerLogin {
			return tokens, nil
		}
		line, _, err = c.readHiddenLine(in, tokenPrompt)
	}
}

// fileTokens files each token under its kind by its prefix.
// It refuses a token of any other prefix,
// and a run that does not give exactly one token of each kind.
// The refusal names neither token.
func fileTokens(tokens []string) (slackengine.Credentials, error) {
	var credentials slackengine.Credentials
	var bots, apps int
	for _, token := range tokens {
		switch {
		case strings.HasPrefix(token, botTokenPrefix):
			credentials.BotToken = token
			bots++
		case strings.HasPrefix(token, appTokenPrefix):
			credentials.AppToken = token
			apps++
		default:
			return slackengine.Credentials{}, tokenKindsRefusal()
		}
	}
	if bots != 1 || apps != 1 {
		return slackengine.Credentials{}, tokenKindsRefusal()
	}
	return credentials, nil
}

// tokenKindsRefusal is the refusal of a login run that did not give one token of each kind.
func tokenKindsRefusal() error {
	return fmt.Errorf("slack: `lym slack login` takes exactly one %s and one %s in one run, and no token of another kind; nothing was stored; way forward: run `lym slack login` again with both, the bot token from the app's OAuth & Permissions page and the app-level token from its Basic Information page",
		botTokenKind, appTokenKind)
}

// tokenRefusal is the refusal of a token of the given kind that Slack's check failed on.
func tokenRefusal(kind string, err error) error {
	var refused *slackengine.APIError
	if !errors.As(err, &refused) {
		return fmt.Errorf("slack: the %s could not be checked: %w; way forward: check the network and run `lym slack login` again", kind, err)
	}
	if refused.Code == missingScopeCode {
		return scopeRefusal("login")
	}
	source := "copy the app's Bot User OAuth Token (`xoxb-…`) from its OAuth & Permissions page"
	if kind == appTokenKind {
		source = "make an app-level token with the scope connections:write on the app's Basic Information page"
	}
	return fmt.Errorf("slack: Slack rejected the %s with %s; way forward: %s and run `lym slack login` again", kind, refused.Code, source)
}

// setupCmd returns the `slack setup` verb.
func (c *slackCLI) setupCmd() *cobra.Command {
	var operator string
	cmd := &cobra.Command{
		Use:         "setup --operator <user-id>",
		Short:       "name the hub's Slack operator",
		Annotations: operatorAnnotations(),
		Long: `setup checks the operator with Slack and only then records them for this hub.
lym makes one channel per orch, so setup names only the operator, and the relay
forwards only the messages of that one operator. A running relay takes up a new
operator at its next wake, with no restart. Copy the operator's Slack user id
from their profile, under More, Copy member ID.

Run it from the operator's terminal; a strand refuses.

Example:
  lym slack setup --operator U0123456789`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) { return c.setup(cmd.Context(), operator) })
		},
	}
	cmd.Flags().StringVar(&operator, "operator", "", "Slack user id of the operator whose messages the relay forwards")
	_ = cmd.MarkFlagRequired("operator")
	return cmd
}

// setup records the operator once Slack confirms the user exists.
func (c *slackCLI) setup(ctx context.Context, operator string) (map[string]any, error) {
	if err := paneRefusal(c.pane(), false, terminalWayForward("setup")); err != nil {
		return nil, err
	}
	credentials, err := slackengine.ReadCredentials(c.geom.TokenPath)
	if err != nil {
		return nil, err
	}
	if credentials.BotToken == "" {
		return nil, errors.New(noTokenMessage)
	}
	if err := c.newSlack(c.slackBaseURL, credentials.BotToken).UserInfo(ctx, operator); err != nil {
		return nil, userRefusal(operator, err)
	}
	if err := slackengine.SetOperator(c.geom.SetupPath, operator); err != nil {
		return nil, err
	}
	return map[string]any{"operator": operator}, nil
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
		return scopeRefusal("setup")
	}
	return fmt.Errorf("slack: Slack refused the check of user %s with %s; way forward: fix what that code names, then run `lym slack setup` again", operator, refused.Code)
}

// scopeRefusal is the refusal of a check the token's scopes do not cover, with the verb to run again.
func scopeRefusal(verb string) error {
	return fmt.Errorf("slack: the token lacks a scope (%s); way forward: give the app %s on its OAuth & Permissions page, reinstall the app, then run `lym slack %s` again", missingScopeCode, botScopes, verb)
}

// startCmd returns the `slack start` verb.
func (c *slackCLI) startCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "start",
		Short:       "start the hub's Slack relay when it is down",
		Annotations: operatorAnnotations(),
		Long: `start brings up the hub's relay, which runs out of every pane, and reports
whether it started or was already live. It needs login and setup first, and it
starts nothing while no orch of the hub has its slack button on, reporting
forwarding-off. The operator's terminal and an orch's pane may run it; any
other strand refuses.

Example:
  lym slack start`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.start)
		},
	}
}

// start starts the relay once login and setup are done and some orch of the hub has its slack button on.
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
		Long: `stop sets the machine's presence to terminal and then ends the hub's relay,
so no tick of the prime's dobby starts it again. If ending the relay fails, run
it again: the presence is then unchanged. The operator's terminal and an orch's
pane may run it; any other strand refuses.

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
		Long: `away starts the hub's relay when it is down and some orch of the hub has its
slack button on, waits for its first creel read, and only then sets the
machine's presence to slack. A relay that cannot read its mail refuses it,
naming why. With no button on, it sets the presence without a relay. The
operator's terminal and an orch's pane may run it, and so may the Slack text
away; any other strand refuses.

Example:
  lym slack away`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.away)
		},
	}
}

// away starts the relay, waits for it to read creel and sets the presence slack.
// With no orch of the hub whose button is on there is no relay to wait for, and the presence is set at once.
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
	if outcome == slackengine.OutcomeStarted || outcome == slackengine.OutcomeLive {
		if err := c.awaitFirstRead(c.geom, c.clock, slackengine.FirstReadBound); err != nil {
			return nil, err
		}
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
		Long: `back sets the machine's presence to terminal and leaves the relay running. The
operator's terminal and an orch's pane may run it, and so may the Slack text
back and any line the operator types in an orch's pane; any other strand
refuses.

Example:
  lym slack back`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.back)
		},
	}
}

// back sets the presence terminal.
func (c *slackCLI) back() (map[string]any, error) {
	if err := paneRefusal(c.pane(), true, orchWayForward("back", "back")); err != nil {
		return nil, err
	}
	return c.setPresence(slackengine.PresenceTerminal)
}

// setPresence writes presence as the machine's and reports whether it changed.
// It mails no orch: the turn-end hook reads the presence itself.
func (c *slackCLI) setPresence(presence slackengine.Presence) (map[string]any, error) {
	changed, err := slackengine.WritePresence(c.geom.PresencePath, presence, c.clock.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"presence": string(presence), "changed": changed}, nil
}

// statusCmd returns the `slack status` verb.
func (c *slackCLI) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "show the presence, the relay and the hub's Slack setup",
		Annotations: operatorAnnotations(),
		Long: `status prints the machine's presence, whether the hub's relay is live, whether
a relay holds the machine's Slack connection, the hub's orchs whose slack
button is on, whether a pause covers the prime orch, the operator the setup
names, the hub's channels, one per orch with its name, id and whether it is
archived, and when the relay was last healthy. Any process may run it.

Example:
  lym slack status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, c.status)
		},
	}
}

// channelRows lists the entries of the channel map as status fields, sorted by orch.
func channelRows(channelMap slackengine.ChannelMap) []map[string]any {
	orchs := make([]string, 0, len(channelMap.Channels))
	for orch := range channelMap.Channels {
		orchs = append(orchs, orch)
	}
	sort.Strings(orchs)
	rows := make([]map[string]any, 0, len(orchs))
	for _, orch := range orchs {
		entry := channelMap.Channels[orch]
		rows = append(rows, map[string]any{"orch": orch, "name": entry.Name, "id": entry.ID, "archived": entry.Archived})
	}
	return rows
}

// status reads the presence, the relay's health, the connection, the buttons, the pause, the operator and the channels.
func (c *slackCLI) status() (map[string]any, error) {
	presence, err := slackengine.ReadPresence(c.geom.PresencePath)
	if err != nil {
		return nil, err
	}
	live, err := slackengine.RelayLive(c.geom)
	if err != nil {
		return nil, err
	}
	connected, err := slackengine.Connected(c.geom.MachinePaths())
	if err != nil {
		return nil, err
	}
	forwarding, err := slackengine.ReadForwarding(c.geom.ForwardingPath)
	if err != nil {
		return nil, err
	}
	paused, err := c.paused()
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		"presence": string(presence), "relay_live": live, "connected": connected, "forwarding": forwarding, "paused": paused,
	}
	setup, found, err := slackengine.ReadSetup(c.geom.SetupPath)
	if err != nil {
		return nil, err
	}
	if found {
		fields["operator"] = setup.OperatorID
	}
	channelMap, err := slackengine.ReadChannelMap(c.geom.ChannelMapPath)
	if err != nil {
		return nil, err
	}
	if len(channelMap.Channels) > 0 {
		fields["channels"] = channelRows(channelMap)
	}
	relayState, found, err := slackengine.ReadRelayState(c.geom)
	if err != nil {
		return nil, err
	}
	if found && !relayState.LastSlackOK.IsZero() {
		fields["last_healthy"] = relayState.LastSlackOK.UTC().Format(time.RFC3339)
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
