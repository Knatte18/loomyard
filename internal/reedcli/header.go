// header.go implements the `header` reed verb: it renders the header pane's text via the engine's
// tokenvocab-backed pipeline.
// The default mode returns the rendered text through the normal JSON envelope;
// --blocking prints the text then blocks forever, the one envelope-exempt tail this command has —
// the header pane boots "lyx reed header --blocking" as its keepalive.

package reedcli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// blockForever parks the keepalive tail indefinitely. It sleeps in a loop rather than using select {} to avoid Go's deadlock detector.
func blockForever() {
	for {
		time.Sleep(time.Hour)
	}
}

// headerWatch is the resize self-heal loop the blocking tail enters after painting
// the header text. A package var so header_test.go can substitute a fake that
// returns, and assert the tail still reaches headerPark.
var headerWatch = func(ctx context.Context, eng *reedengine.Engine) error { return eng.Watch(ctx) }

// headerPark is the keepalive park the blocking tail ends on, unconditionally.
var headerPark = blockForever

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
The blocking pane additionally runs reed's resize self-heal watch loop,
which re-applies the planned layout after the terminal window is
resized, and is turned off with "watchdog: off" in reed.yaml followed
by "lyx reed down" + "up".

The live header pane renders its text once, at pane launch: after editing
header.template in reed.yaml, this verb previews the new rendering
immediately, but the running pane keeps its old text until the header is
next rebuilt (a server restart, a dead-header heal, or "lyx reed down" +
"up") — an "up" that finds the header alive deliberately leaves it as is.

Example:
  lyx reed header
  lyx reed header --blocking`,
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

				// The header pane's stdout/stderr is its visible screen: internal/logger's stderr half
				// defaults to slog.LevelWarn, and the watch loop reaches already-shipped Warn call sites
				// (liveBoxLocked on a failed or malformed window-size query, pinGeometryOptionsLocked on a
				// failed pin), so without this rebind the first degraded tmux round trip paints a slog
				// line over the operator console. Only the stderr half is discarded -- logger.SetOutput
				// rebinds that half alone, and the durable handler is enabled unconditionally at Info and
				// above, so nothing is lost for diagnosis, it just stops being drawn.
				logger.SetOutput(io.Discard)

				// A non-nil return is logged only: never output.Err, never fmt.Fprint, never anything
				// written to stdout or stderr, because the pane's stdio is its screen.
				if err := headerWatch(cmd.Context(), c.eng); err != nil {
					logger.Warn("reed: header pane watch loop returned", "err", err)
				}

				// Deliberate redundancy, not dead code: Watch never returns while the pane must live, and
				// this guarantees that no future edit to Watch can make RunE fall through and kill the
				// keepalive pane -- the one failure this design must never permit.
				headerPark()

				// headerPark() itself must never return (it blocks forever), but if it ever does --
				// e.g. under test via a substitutable stub -- this return prevents falling through to
				// the unconditional output.Ok write below, which would leak a JSON envelope onto the
				// pane's own screen in violation of the "pane's stdio is its screen" invariant above.
				return nil
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
