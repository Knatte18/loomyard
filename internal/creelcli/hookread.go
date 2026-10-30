// hookread.go implements the hidden `hook-read` verb, which a strand's tool-boundary and Stop hooks run to take the strand's unread mail into its session.

package creelcli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// hookReadVerb is the hidden verb's name, which the Claude engine's hooks spell.
const hookReadVerb = "hook-read"

// hookReadTimeout bounds the verb's call to the daemon, since the provider holds the session at the tool boundary until its hook ends.
const hookReadTimeout = 10 * time.Second

// hookReadCmd builds the `hook-read` subcommand, hidden because only a strand's hooks run it.
// Its standard output is the named engine's hook output for the event, or nothing when nothing was claimed;
// a refusal goes to standard error and leaves standard output empty, since the provider parses the hook's standard output as its JSON.
// The exit code is 0 after a tool-boundary call that raised no error and, on a Stop, exactly when mail was written.
func hookReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:         hookReadVerb + " <engine>",
		Short:       "print the caller strand's unread mail as the engine's hook output and mark it read (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-read reads the engine's hook input from standard input and, for the
after-tool-call or the turn-end event of the session's main thread, claims the
caller strand's unread mail, oldest first, and prints it as the named engine's
hook output, one [creel] #<id> from "<sender>": <body> line per message, which
the provider hands to the session as additional context and, after a tool call,
shows to the operator as the system message. The messages are marked read once
the output is written. With nothing to claim it prints nothing.
A hold limits it to the holder's mail, a pause to none.

A strand whose idle wake already handed it mail is owed an echo of those lines.
The claim prints them under "[creel] Delivered while idle:" ahead of any mail,
as the system message alone, so the operator sees them and the session does
not read them twice; they are dropped once the output is written.

A call from a subagent or fork claims and prints nothing, so the mail stays for
the main thread. After a tool call the exit code is 0. On a turn end it is 0
exactly when mail was written and 1 otherwise, an echo alone included, which the
merged turn-end command reads as "record the turn end" and keeps the printed
echo as the hook's output. An input of any other event is refused on standard
error with exit 1.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			event, wrote, err := hookRead(cmd, args[0])
			code, printErr := hookReadExit(event, wrote, err)
			clihelp.SetExit(cmd.Context(), code)
			if printErr {
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

// hookReadExit decides the exit code of hook-read and whether its error is printed, from the event the input carried, whether mail was written and the error the call ended with.
// A Stop exits 0 exactly when mail was written, whatever followed the write, since exit 0 tells the merged turn-end command mail was handed to the session, which continues the turn, and no turn end is to be recorded;
// an echo-only Stop exits 1, so the merged command records the turn end and keeps the printed echo as the hook's output;
// any other call exits 1 on an error.
// An error is always printed, so a failed acknowledgement after the write still reaches standard error.
func hookReadExit(event shuttleengine.HookEvent, wrote bool, err error) (exitCode int, printErr bool) {
	printErr = err != nil
	switch {
	case event == shuttleengine.HookEventStop && wrote:
		return 0, printErr
	case event == shuttleengine.HookEventStop, err != nil:
		return 1, printErr
	}
	return 0, false
}

// hookRead parses the hook input on the command's standard input and, for the main thread's tool-boundary or Stop event, claims the caller's mail through the daemon and writes it as the engine's hook output, before the client acknowledges it.
// It returns the event the input carried and whether mail was written, beside the error the call ended with; an echo written alone leaves it false.
// A claim that hands out an echo prints it as the system message under creelengine.EchoHeading, ahead of any mail.
func hookRead(cmd *cobra.Command, engine string) (event shuttleengine.HookEvent, wrote bool, err error) {
	renderer, ok := shuttleengines.ToolBoundaryRenderer(engine)
	if !ok {
		return "", false, fmt.Errorf("the engine %q renders no creel hook delivery; the registered engines: %s", engine, strings.Join(shuttleengines.Names(), ", "))
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", false, fmt.Errorf("could not read the hook input: %w", err)
	}
	input, err := renderer.HookInput(payload)
	if err != nil {
		return "", false, err
	}
	switch input.Event {
	case shuttleengine.HookEventToolBoundary, shuttleengine.HookEventStop:
	default:
		return "", false, fmt.Errorf("hook-read takes the engine's after-tool-call and turn-end hook inputs, and this input is neither")
	}
	if input.Subagent {
		return input.Event, false, nil
	}
	client, err := hubClient(cmd.Context())
	if err != nil {
		return input.Event, false, err
	}
	out := cmd.OutOrStdout()
	writeOutput := func(resp creelengine.Response) error {
		lines := make([]string, len(resp.Messages))
		for i, msg := range resp.Messages {
			lines[i] = creelengine.MessageLine(msg)
		}
		mail := strings.Join(lines, "\n")
		echo := ""
		if len(resp.Echo) > 0 {
			echo = creelengine.EchoHeading + "\n" + strings.Join(resp.Echo, "\n")
		}
		data, err := renderer.HookOutput(input.Event, mail, echo)
		if err != nil {
			return err
		}
		if _, err := out.Write(append(data, '\n')); err != nil {
			var stays []string
			if mail != "" {
				stays = append(stays, "its mail stays unread")
			}
			if echo != "" {
				stays = append(stays, "its echo stays owed")
			}
			return fmt.Errorf("could not write the hook output, so %s: %w", strings.Join(stays, " and "), err)
		}
		wrote = mail != ""
		return nil
	}
	request := creelengine.Request{Verb: creelengine.VerbClaim, Stop: input.Event == shuttleengine.HookEventStop, StopHookActive: input.StopHookActive}
	_, err = client.Call(request, hookReadTimeout, writeOutput)
	return input.Event, wrote, err
}
