// hookbatching.go implements the hidden hook-batching verb, which a session's after-tool-call hook runs to nudge the session back to batching its calls.

package shuttlecli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// hookBatchingCmd builds the hidden hook-batching verb, run only by a session's after-tool-call hook.
// Its standard output is the engine's hook output carrying a nudge, or nothing;
// a failure goes to standard error and the exit code is 0 either way, so the hook never disturbs the session.
func hookBatchingCmd() *cobra.Command {
	return &cobra.Command{
		Use:         shuttleengine.BatchingHookVerb + " <engine> <state-file>",
		Short:       "print the engine's batching nudge for the tool call on standard input (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-batching reads the engine's after-tool-call hook input from standard
input, judges the call and the calling chain's recent messages against the
run's batching nudge state file, and prints the engine's hook output that adds one
line naming the breach and the rule to the session's context. With no breach,
below the state file's context threshold or past its nudge cap it prints
nothing. It needs no hub and reads no config: the state file carries the
settings the run started with. A failure goes to standard error, and the exit
code is always 0.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if err := hookBatching(cmd, args[0], args[1]); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "shuttle: "+err.Error())
			}
			return nil
		},
	}
}

// hookBatching judges the hook input on the command's standard input through the named engine's batching nudge and writes its output, when there is one, to standard output.
func hookBatching(cmd *cobra.Command, engine, statePath string) error {
	nudger, ok := shuttleengines.BatchingNudger(engine)
	if !ok {
		return fmt.Errorf("the engine %q has no batching nudge; the registered engines: %s", engine, strings.Join(shuttleengines.Names(), ", "))
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("could not read the hook input: %w", err)
	}
	output, err := nudger.BatchingNudge(statePath, payload)
	if err != nil || len(output) == 0 {
		return err
	}
	_, err = cmd.OutOrStdout().Write(append(output, '\n'))
	return err
}
