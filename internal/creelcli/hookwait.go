// hookwait.go implements the hidden `hook-wait` verb, the idle waiter a strand's hook holds open until the daemon wakes the idle session.

package creelcli

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// hookWaitVerb is the hidden verb's name, which the Claude engine's waiter hook spells.
const hookWaitVerb = "hook-wait"

// hookWaitCmd builds the `hook-wait` subcommand, hidden because only a strand's idle waiter hook runs it.
// It prints nothing to standard output; on a ring it writes the claimed mail to standard error as the engine's wake text, acknowledges it and exits with the engine's wake code, and every other end exits 0 printing nothing.
func hookWaitCmd() *cobra.Command {
	return &cobra.Command{
		Use:         hookWaitVerb + " <engine>",
		Short:       "hold the caller strand's idle waiter until the daemon wakes it with its mail (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-wait reads the engine's hook input from standard input and holds the
caller strand's idle waiter open on the daemon, with no timeout of its own: the
hook entry's timeout bounds it. When the daemon wakes the idle strand with its
mail, hook-wait writes the claimed [creel] lines to standard error as the
engine's wake text, acknowledges them and exits with the engine's wake code, 2
for Claude, which wakes the session with the mail. The wait is never resumed
once a ring reached it. A call from a subagent or fork, a waiter a newer one
superseded, an unreachable daemon, any refusal, a ring whose lines could not be
written and a ring whose acknowledgement could not be sent exit 0 and print
nothing more, so the waiter exits with the wake code only once it wrote the
lines and sent the acknowledgement, whether or not the daemon confirmed it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), hookWait(cmd, args[0]))
			return nil
		},
	}
}

// hookWaitExit decides the exit code of hook-wait from the engine's wake code, whether the ring's lines were written and the error the call ended with.
// It is 0 when no lines were written, since nothing woke the session, and 0 when the acknowledgement could not be sent, since the daemon journaled nothing and the mail stays unread for the next claim;
// otherwise it is the wake code, whether the daemon confirmed the acknowledgement, refused it on a failed journal write or the confirmation was lost.
func hookWaitExit(wakeCode int, wroteLines bool, err error) int {
	if !wroteLines || errors.Is(err, creelengine.ErrAckUnsent) {
		return 0
	}
	return wakeCode
}

// hookWait holds the caller strand's idle waiter and returns the exit code the command sets.
// On a ring it writes the claimed mail to standard error as the engine's wake text before the client acknowledges it, so a write that fails leaves the mail unread.
func hookWait(cmd *cobra.Command, engine string) int {
	renderer, ok := shuttleengines.ToolBoundaryRenderer(engine)
	if !ok {
		return 0
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return 0
	}
	input, err := renderer.HookInput(payload)
	if err != nil || input.Subagent {
		return 0
	}
	client, err := hubClient(cmd.Context())
	if err != nil {
		return 0
	}
	var wakeCode int
	var wroteLines bool
	writeWake := func(resp creelengine.Response) error {
		lines := make([]string, len(resp.Messages))
		for i, msg := range resp.Messages {
			lines[i] = creelengine.MessageLine(msg)
		}
		stderr, code := renderer.RewakeOutput(strings.Join(lines, "\n"))
		if _, err := cmd.ErrOrStderr().Write(stderr); err != nil {
			return err
		}
		wakeCode, wroteLines = code, true
		return nil
	}
	_, err = client.Call(creelengine.Request{Verb: creelengine.VerbHookWait, Wait: true, TakesMail: true}, 0, writeWake)
	return hookWaitExit(wakeCode, wroteLines, err)
}
