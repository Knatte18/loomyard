// hookread.go implements the hidden `hook-read` verb, which a strand's tool-boundary hook runs to take the strand's unread mail into its session.

package creelcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// hookReadVerb is the hidden verb's name, which the Claude engine's PostToolUse hook spells.
const hookReadVerb = "hook-read"

// hookReadTimeout bounds the verb's call to the daemon, since the provider holds the session at the tool boundary until its hook ends.
const hookReadTimeout = 10 * time.Second

// hookReadCmd builds the `hook-read` subcommand, hidden because only a strand's tool-boundary hook runs it.
// Its standard output is the named engine's tool-boundary context JSON, or nothing when no mail was claimed;
// a refusal goes to standard error with exit 1 and leaves standard output empty, since the provider parses the hook's standard output as its JSON.
func hookReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:         hookReadVerb + " <engine>",
		Short:       "print the caller strand's unread mail as the engine's tool-boundary hook context and mark it read (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-read claims the caller strand's unread mail, oldest first, and prints it as
the named engine's tool-boundary hook output, one "[creel] #<id> from <sender>: <body>"
line per message, which the provider adds to the session's context after the tool call.
The messages are marked read once the output is written. With nothing to claim it
prints nothing. A hold limits it to the holder's mail, a pause to none.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if err := hookRead(cmd, args[0]); err != nil {
				clihelp.SetExit(cmd.Context(), 1)
				msg := err.Error()
				if !strings.HasPrefix(msg, "creel: ") {
					msg = "creel: " + msg
				}
				fmt.Fprintln(cmd.ErrOrStderr(), msg)
			}
			return nil
		},
	}
}

// hookRead claims the caller's mail through the daemon and writes it as engine's tool-boundary context, before the client acknowledges it.
func hookRead(cmd *cobra.Command, engine string) error {
	renderer, ok := shuttleengines.ToolBoundaryRenderer(engine)
	if !ok {
		return fmt.Errorf("the engine %q renders no tool-boundary context; the registered engines: %s", engine, strings.Join(shuttleengines.Names(), ", "))
	}
	client, err := hubClient(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	writeContext := func(resp creelengine.Response) error {
		lines := make([]string, len(resp.Messages))
		for i, msg := range resp.Messages {
			lines[i] = creelengine.MessageLine(msg)
		}
		data, err := renderer.ToolBoundaryContext(strings.Join(lines, "\n"))
		if err != nil {
			return err
		}
		if _, err := out.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("could not write the mail, so it stays unread: %w", err)
		}
		return nil
	}
	_, err = client.Call(creelengine.Request{Verb: creelengine.VerbClaim}, hookReadTimeout, writeContext)
	return err
}
