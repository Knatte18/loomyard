// hookedit.go implements the hidden hook-edit verb.
// A session's after-edit hook runs it to record the edited file in the run's edit records file.

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

// hookEditCmd builds the hidden hook-edit verb, run only by a session's after-edit hook.
// It prints nothing on success;
// a failure goes to standard error and the exit code is 0 either way, so the hook never disturbs the session.
func hookEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:         shuttleengine.EditHookVerb + " <engine> <edit-records-file>",
		Short:       "record the file edit on standard input in a run's edit records file (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `hook-edit reads the engine's after-edit hook input from standard input and
appends a record of each edited file and a hash of its content to the run's edit
records file. The run's wait counts the edits that return a file to content it held
earlier, to tell its parent about an agent that undoes or repeats its own edits.
It needs no hub and reads no config. A failure goes to standard error, and the
exit code is always 0.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if err := hookEdit(cmd, args[0], args[1]); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "shuttle: "+err.Error())
			}
			return nil
		},
	}
}

// hookEdit records the file edit that the hook input on the command's standard input describes.
// It goes through the named engine's edit hook and lands in the edit records file at recordsPath.
func hookEdit(cmd *cobra.Command, engine, recordsPath string) error {
	recorder, ok := shuttleengines.EditRecorder(engine)
	if !ok {
		return fmt.Errorf("the engine %q has no edit hook; the registered engines: %s", engine, strings.Join(shuttleengines.Names(), ", "))
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("could not read the hook input: %w", err)
	}
	return recorder.RecordEdit(recordsPath, payload)
}
