// header.go implements the `header` reed verb: it renders the header pane's text via the engine's
// tokenvocab-backed pipeline.
// The default mode returns the rendered text through the normal JSON envelope;
// --blocking prints the text then blocks forever, the one envelope-exempt tail this command has —
// the header pane boots "lyx reed header --blocking" as its keepalive.
// Both modes carry clihelp.SkipStencilSeedAnnotation, declining cmd/lyx's root pre-run stencil-seed
// pass: this is deliberate rather than a --blocking-only gate, because a cobra annotation is
// per-command and neither mode reads a stencil. Declining is what keeps the keepalive's stderr — and
// therefore the header pane's scrollback — free of stencilstore warnings, and the hub free of a
// preview command's git commits.

package reedcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// blockForever parks the keepalive tail indefinitely. It sleeps in a loop rather than using select {} to avoid Go's deadlock detector.
func blockForever() {
	for {
		time.Sleep(time.Hour)
	}
}

// headerCmd builds the `header` subcommand: calls c.eng.HeaderText() and either returns it via the JSON envelope or prints and blocks forever.
func (c *reedCLI) headerCmd() *cobra.Command {
	var blocking bool

	cmd := &cobra.Command{
		Use:   "header",
		Short: "render the operator console pane's header text",
		Long: `header renders the header-pane text over this hub's configured template
(or the embedded default), the same tokenvocab pipeline
Engine.ValidateHeader checks eagerly at boot.

Default mode returns the rendered text through the normal JSON envelope —
a plain, smoke-testable command. --blocking instead prints the rendered
text to stdout and then blocks forever; this is the header pane's own
keepalive tail and the one part of this command exempt from the JSON
envelope (everything fallible still runs pre-flight, on the envelope).

The live header pane renders its text once, at pane launch: after editing
header.template in reed.yaml, this verb previews the new rendering
immediately, but the running pane keeps its old text until the header is
next rebuilt (a server restart, a dead-header heal, or "lyx reed down" +
"up") — an "up" that finds the header alive deliberately leaves it as is.

Example:
  lyx reed header
  lyx reed header --blocking`,
		Annotations: map[string]string{
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			text, err := c.eng.HeaderText()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if blocking {
				// Display the rendered text once, then hold the pane open forever.
				fmt.Fprint(out, "\x1b[2J\x1b[H"+strings.TrimRight(text, "\r\n"))
				blockForever()
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"text": text,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&blocking, "blocking", false, "print the rendered header text then block forever (the pane keepalive)")

	return cmd
}
