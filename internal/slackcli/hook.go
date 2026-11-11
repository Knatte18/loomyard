// hook.go implements the hidden `hook` verb that an orch session's prompt and turn-end hooks run.
// It sets the machine's presence back to terminal on a line the operator typed.
// It also forwards a turn's final reply while away.

package slackcli

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/slackengine"
)

const (
	// hookVerb is the hidden verb's name, which slackengine.HookCommand spells.
	hookVerb = "hook"
	// injectMatchWindow is how long before a prompt an inject creel typed into the strand's pane still counts as its source.
	injectMatchWindow = 2 * time.Minute
	// replyLimit is the most bytes of a reply the hook forwards, under creel's 8 KiB cap on a message's line.
	replyLimit = 7000
	// replyCutNote closes a reply cut to replyLimit.
	replyCutNote = "\n… (cut here; the whole reply is in the terminal)"
	// lymLinePrefix opens every line lym's own modules type into a pane without creel.
	// Examples are shuttle's API-error retry line and tender's resume line.
	lymLinePrefix = "[lym]"
)

// The actions a hook run reports, under `action`.
const (
	hookActionNone      = "none"
	hookActionLymLine   = "lym-line"
	hookActionInject    = "inject"
	hookActionBack      = "presence-terminal"
	hookActionForward   = "forwarded"
	hookActionButtonOff = "button-off"
	hookActionSubagent  = "subagent"
)

// strandClient is the creel client of the orch strand a hook runs in.
type strandClient interface {
	// Post leaves body as a message for receiver and returns its id.
	Post(receiver, body string) (int64, error)
	// Log returns the journal entries matching filter, oldest first.
	Log(filter creelengine.LogFilter) ([]creelengine.LogEntry, error)
	// Shortname returns the shortname of the client's hub.
	Shortname() string
}

// hookCmd returns the hidden `slack hook` verb.
func (c *slackCLI) hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:         hookVerb + " <engine>",
		Short:       "switch the presence on the operator's typed line and forward a turn's reply while away (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook is what an orch session's prompt and turn-end hooks run, never an operator.
It reads the named engine's hook input on standard input.

On a prompt typed into the orch's pane while the presence is slack, it sets the
presence to terminal: the operator is back at the terminal. A line lym put in
the pane switches nothing: a slash command, a prompt the provider submits by
itself, a line that opens [creel] or [lym], and a line creel typed into this
pane as an inject in the two minutes before.

On the end of a turn of the session's main thread while the presence is slack
and the orch's slack button is on, it posts the turn's final reply to the relay
as the orch, so it reaches the orch's own Slack channel. A turn that ended with
no text sends nothing, and a reply over 7000 bytes is cut.

It runs only in an orch's pane, and its output is the hook's to discard.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runVerb(cmd, func() (map[string]any, error) { return c.hook(cmd, args[0]) })
		},
	}
}

// hook parses the engine's hook input on the command's standard input and acts on a prompt or a turn end.
// A subagent's call and any other event do nothing.
func (c *slackCLI) hook(cmd *cobra.Command, engine string) (map[string]any, error) {
	renderer, ok := shuttleengines.ToolBoundaryRenderer(engine)
	if !ok {
		return nil, fmt.Errorf("slack: the engine %q parses no hook input; the registered engines: %s", engine, strings.Join(shuttleengines.Names(), ", "))
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("slack: read the hook input: %w", err)
	}
	input, err := renderer.HookInput(payload)
	if err != nil {
		return nil, err
	}
	if input.Subagent {
		return hookAction(hookActionSubagent), nil
	}
	switch input.Event {
	case shuttleengine.HookEventPrompt:
		return c.promptHook(input.Prompt)
	case shuttleengine.HookEventStop:
		return c.replyHook(input.Reply)
	}
	return hookAction(hookActionNone), nil
}

// hookAction returns the fields of a hook run that took action.
func hookAction(action string) map[string]any {
	return map[string]any{"action": action}
}

// promptHook sets the presence to terminal for prompt, a line the operator typed into the orch's pane while away.
// It changes nothing for an empty prompt, which is a slash command or one the provider submitted.
// It changes nothing for a line that opens creelengine.LinePrefix, as every line creel types or hands over does.
// It changes nothing for a line that opens lymLinePrefix, nor while the presence is not slack.
// Nor does it for a prompt creel typed into the strand's pane as an inject within injectMatchWindow before now.
func (c *slackCLI) promptHook(prompt string) (map[string]any, error) {
	if prompt == "" || strings.HasPrefix(prompt, creelengine.LinePrefix) || strings.HasPrefix(prompt, lymLinePrefix) {
		return hookAction(hookActionLymLine), nil
	}
	presence, err := slackengine.ReadPresence(c.geom.PresencePath)
	if err != nil {
		return nil, warnHookFailure("reading the presence failed", "", err)
	}
	if presence != slackengine.PresenceSlack {
		return hookAction(hookActionNone), nil
	}
	client, strand, err := c.orchClient()
	if err != nil {
		return nil, warnHookFailure("finding the orch's creel client failed", "", err)
	}
	now := c.clock.Now()
	entries, err := client.Log(creelengine.LogFilter{To: strand, Since: now.Add(-injectMatchWindow)})
	if err != nil {
		// The turn hook discards the verb's output, so the log is the only place the failure shows.
		logger.Warn("slack: hook: reading creel's injects failed; the presence stays slack", "strand", strand, "error", err)
		return nil, fmt.Errorf("slack: read creel's injects into %s: %w", strand, err)
	}
	for _, entry := range entries {
		if entry.TypesInject() && strings.TrimSpace(entry.Subject) == prompt {
			return hookAction(hookActionInject), nil
		}
	}
	if _, err := slackengine.WritePresence(c.geom.PresencePath, slackengine.PresenceTerminal, now); err != nil {
		return nil, warnHookFailure("setting the presence to terminal failed; replies keep going to Slack", strand, err)
	}
	return hookAction(hookActionBack), nil
}

// replyHook posts reply, a turn's final text, to the relay as the orch while the presence is slack.
// The relay posts it to that orch's own channel.
// An empty reply, any presence but slack, and an orch whose slack button is off send nothing.
func (c *slackCLI) replyHook(reply string) (map[string]any, error) {
	if reply == "" {
		return hookAction(hookActionNone), nil
	}
	presence, err := slackengine.ReadPresence(c.geom.PresencePath)
	if err != nil {
		return nil, warnHookFailure("reading the presence failed", "", err)
	}
	if presence != slackengine.PresenceSlack {
		return hookAction(hookActionNone), nil
	}
	client, strand, err := c.orchClient()
	if err != nil {
		return nil, warnHookFailure("finding the orch's creel client failed", "", err)
	}
	orch, err := agentname.Format(client.Shortname(), agentname.TaskSlug(strand), agentname.RoleOrch)
	if err != nil {
		return nil, err
	}
	on, err := slackengine.ForwardingOn(c.geom.ForwardingPath, orch)
	if err != nil {
		return nil, warnHookFailure("reading the orch's slack button failed", strand, err)
	}
	if !on {
		return hookAction(hookActionButtonOff), nil
	}
	receiver, err := agentname.FormatProgram(client.Shortname(), slackengine.SlackProgram)
	if err != nil {
		return nil, err
	}
	if _, err := client.Post(receiver, fitReply(reply)); err != nil {
		logger.Warn("slack: hook: posting the turn's reply failed; the reply is lost to Slack", "strand", strand, "error", err)
		return nil, fmt.Errorf("slack: post the turn's reply to %s: %w", receiver, err)
	}
	return hookAction(hookActionForward), nil
}

// warnHookFailure logs a hook step's failure and returns err unchanged.
// The turn hook discards the verb's output, so the log is the only place the failure shows.
func warnHookFailure(message, strand string, err error) error {
	logger.Warn("slack: hook: "+message, "strand", strand, "error", err)
	return err
}

// orchClient returns the creel client of the orch strand the process runs in, and that strand's name.
// It refuses a process whose strand name names no orch; creel refuses a claim from outside that strand's pane.
func (c *slackCLI) orchClient() (strandClient, string, error) {
	strand := c.strandName()
	if !creelengine.IsOrchStrand(strand) {
		return nil, "", fmt.Errorf("slack: this runs in no orch's pane (strand %q); way forward: none, only an orch session's hooks run `lym slack %s`", strand, hookVerb)
	}
	client, err := c.newStrandClient(strand)
	if err != nil {
		return nil, "", err
	}
	return client, strand, nil
}

// fitReply returns reply whole when it fits replyLimit.
// Otherwise it returns the longest start of reply that fits with replyCutNote after it, cut at a rune's start.
func fitReply(reply string) string {
	if len(reply) <= replyLimit {
		return reply
	}
	cut := replyLimit - len(replyCutNote)
	for cut > 0 && !utf8.RuneStart(reply[cut]) {
		cut--
	}
	return reply[:cut] + replyCutNote
}
