// readable.go implements the readable summary of operator verbs: the global --envelope flag, the choice of whether a command prints the summary, and the writer that renders it.

package clihelp

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/output"
)

// EnvelopeFlagUsage is the help text of the global --envelope flag InstallReadableOutput declares.
const EnvelopeFlagUsage = "print the full JSON envelope where an operator verb on a terminal prints its readable summary"

// InstallReadableOutput declares the global --envelope flag as a persistent flag on root.
// A command that declares its own envelope flag shadows it, and its own flag, when set, keeps the envelope too.
// Call InstallReadableOutput once during root command construction.
func InstallReadableOutput(root *cobra.Command) {
	root.PersistentFlags().Bool(output.EnvelopeFlag, false, EnvelopeFlagUsage)
}

// WriterIsTerminal reports whether w is an *os.File whose descriptor is a terminal.
func WriterIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// UseReadableOutput points cmd's output at an output.SummaryWriter when cmd prints the readable summary, and flushes that writer when the invocation ends.
// cmd prints it when it carries the operator audience, its output writer is a terminal by isTerminal, no envelope flag is set on it,
// and it declares no json flag of its own, since such a command renders its own view for a terminal.
// A person at a terminal so reads each envelope line as its summary, while an agent or script, whose output is never a terminal, reads the envelope.
func UseReadableOutput(cmd *cobra.Command, isTerminal func(io.Writer) bool) {
	if cmd.Annotations[AudienceAnnotation] != AudienceOperator {
		return
	}
	if flag := cmd.Flags().Lookup(output.EnvelopeFlag); flag != nil && flag.Changed {
		return
	}
	if flag := cmd.Flags().Lookup("json"); flag != nil {
		if _, global := flag.Value.(*jsonHelpValue); !global {
			return
		}
	}
	out := cmd.OutOrStdout()
	if !isTerminal(out) {
		return
	}
	summary := output.NewSummaryWriter(out)
	cmd.SetOut(summary)
	onFinish(cmd.Context(), func() { _ = summary.Flush() })
}
