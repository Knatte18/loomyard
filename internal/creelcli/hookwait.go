// hookwait.go implements the hidden `hook-wait` verb, the idle waiter a strand's hook holds open until the daemon wakes the idle session.

package creelcli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// hookWaitVerb is the hidden verb's name, which the Claude engine's waiter hook spells.
const hookWaitVerb = "hook-wait"

// hookWaitCmd builds the `hook-wait` subcommand, hidden because only a strand's idle waiter hook runs it.
// It prints nothing to standard output; on a ring it writes the engine's wake text to standard error and exits with the engine's wake code, and every other end exits 0 printing nothing.
func hookWaitCmd() *cobra.Command {
	return &cobra.Command{
		Use:         hookWaitVerb + " <engine>",
		Short:       "hold the caller strand's idle waiter until the daemon wakes it (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-wait reads the engine's hook input from standard input and holds the
caller strand's idle waiter open on the daemon, with no timeout of its own: the
hook entry's timeout bounds it. When the daemon wakes the idle strand it writes
the engine's wake text to standard error and exits with the engine's wake code,
2 for Claude, which wakes the session. A call from a subagent or fork, a waiter
a newer one superseded, an unreachable daemon and any refusal exit 0 and print
nothing, so the waiter exits with the wake code on a wake alone.`,
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

// hookWait holds the caller strand's idle waiter and returns the exit code the command sets.
// It returns the engine's wake code only after writing the wake text to standard error on a ring, and 0 in every other case.
func hookWait(cmd *cobra.Command, engine string) (exitCode int) {
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
	resp, err := client.Call(creelengine.Request{Verb: creelengine.VerbHookWait, Wait: true}, 0, nil)
	if err != nil || resp.Outcome != creelengine.OutcomeRing {
		return 0
	}
	stderr, code := renderer.RewakeOutput(creelengine.BellLine)
	if _, err := cmd.ErrOrStderr().Write(stderr); err != nil {
		return 0
	}
	return code
}
