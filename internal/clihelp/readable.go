// readable.go implements the readable summary of operator verbs.
// It holds the global --envelope flag, the rule for which commands print the summary, and the writer that renders it.

package clihelp

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/output"
)

// EnvelopeFlagUsage is the help text of the global --envelope flag.
const EnvelopeFlagUsage = "print the full JSON envelope where an operator verb on a terminal prints its readable summary"

// InstallReadableOutput declares the global --envelope flag as a persistent flag on root.
// A command that declares its own envelope flag shadows the global one.
// When that flag is set, the command still prints the envelope.
// Call InstallReadableOutput once during root command construction.
func InstallReadableOutput(root *cobra.Command) {
	root.PersistentFlags().Bool(output.EnvelopeFlag, false, EnvelopeFlagUsage)
}

// WriterIsTerminal reports whether w is an *os.File whose descriptor is a terminal.
func WriterIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// UseReadableOutput points cmd's output at an output.SummaryWriter when cmd prints the readable summary.
// It flushes that writer when the invocation ends.
// cmd prints the summary only when it carries the operator audience and its output is a terminal per isTerminal.
// It also requires that no envelope flag is set on cmd.
// Likewise a command that declares its own json flag is skipped, since it renders its own view for a terminal.
// A person at a terminal reads each envelope line as its summary.
// An agent or script, whose output is not a terminal, reads the envelope.
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
