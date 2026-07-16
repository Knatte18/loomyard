// header.go implements the `header` mux verb: it renders the header pane's
// text via the engine's tokenvocab-backed pipeline. The default mode returns
// the rendered text through the normal JSON envelope; --blocking prints the
// text then blocks forever, the one envelope-exempt tail this command has —
// the header pane boots "lyx mux header --blocking" as its keepalive.

package muxcli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// headerCmd builds the `header` subcommand: calls c.eng.HeaderText() and
// either returns it via the JSON envelope (default) or prints it then blocks
// forever (--blocking, the header pane's keepalive tail). Rendering is the
// only fallible step and it always runs pre-flight, on the envelope, before
// either mode's tail — a bad template surfaces as output.Err with a non-zero
// exit in both modes, never a silent hang.
func (c *muxCLI) headerCmd() *cobra.Command {
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
header.template in mux.yaml, this verb previews the new rendering
immediately, but the running pane keeps its old text until the header is
next rebuilt (a server restart, a dead-header heal, or "lyx mux down" +
"up") — an "up" that finds the header alive deliberately leaves it as is.

Example:
  lyx mux header
  lyx mux header --blocking`,
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
				// The pane keepalive: display the rendered text once, then
				// hold the pane open forever. No JSON is written here even on
				// success — this is the one documented envelope exception,
				// scoped to exactly this tail (rendering above already ran
				// pre-flight, on the envelope).
				//
				// Display mechanics are load-bearing for the DEFAULT 1-row
				// header pane (height_rows: 1): the pane's shell has already
				// echoed this command's own (long) launch line, and a
				// trailing newline after the text would park the cursor on a
				// fresh empty row — which is the only row a 1-row pane shows,
				// scrolling the text itself out of view (observed live,
				// tmux 3.6). So: clear the pane and home the cursor (hides
				// the echoed launch line at any height), then print the text
				// with its trailing newlines trimmed via Fprint, leaving the
				// cursor on the text's last row so that row stays visible.
				fmt.Fprint(out, "\x1b[2J\x1b[H"+strings.TrimRight(text, "\r\n"))
				select {}
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
