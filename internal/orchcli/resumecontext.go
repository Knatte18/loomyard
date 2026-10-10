// resumecontext.go implements the hidden `resume-context` orch verb, which the provider's session-start hook runs after every compaction to deliver the resume pointer, and builds the hook command the orch run's spec carries.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/spf13/cobra"
)

// resumeContextVerb is the hidden verb's name, which the hook command spells.
const resumeContextVerb = "resume-context"

// resumeContextCommand returns the hook command that delivers the resume pointer: a change of directory to the prime's anchor chained with the bare verb, so the verb runs in the prime whatever directory the session has moved to.
// It is built through the POSIX dialect whatever the host, because the provider runs its hooks under a POSIX shell on every OS.
// `lyx` is named bare and resolves to the binary that spawned the session, through the pane's PATH prelude.
func resumeContextCommand(primeAnchor string) string {
	sh := shell.Posix()
	return sh.Chain(sh.ChangeDir(primeAnchor), "lyx orch "+resumeContextVerb)
}

// resumeContextCmd builds the `resume-context` subcommand, hidden because only the session-start hook runs it.
// Its standard output is the provider's additional-context JSON and nothing else;
// a failure prints the JSON error envelope and writes no delivery mark, so the typed pointer delivers instead.
func (c *orchCLI) resumeContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:         resumeContextVerb,
		Short:       "print the resume pointer as session-start hook context (run by the hook)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `resume-context renders the resume pointer from the persisted state and prints it as
the provider's additional-context JSON, which the session-start hook adds to the session
after a compaction. It records a delivery mark after a successful render, so the watcher
does not type the same pointer again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			text, err := orchengine.ResumeContext(c.paths, c.stencilsDir, time.Now())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			data, err := claudeengine.SessionStartContext(text)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_, err = out.Write(append(data, '\n'))
			return err
		},
	}
}
