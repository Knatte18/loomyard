// statusline.go implements the `statusline` reed verb: it renders this hub's tmux status-line text
// via the engine's tokenvocab-backed pipeline and returns it through the normal JSON envelope.
// It carries clihelp.SkipStencilSeedAnnotation, declining cmd/lyx's root pre-run stencil-seed
// pass: this is deliberate even though this command is a plain preview rather than a keepalive —
// neither this command nor the gate reads a stencil, and declining keeps the hub free of a
// preview command's git commits.

package reedcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// statuslineCmd builds the `statusline` subcommand: calls c.eng.StatusLineText() and returns it
// through the JSON envelope.
func (c *reedCLI) statuslineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "statusline",
		Short: "render this hub's tmux status-line text",
		Long: `statusline previews the rendered status-line text over this hub's configured
template (or the embedded default), the same tokenvocab pipeline
Engine.ValidateStatusLine checks eagerly at boot.

The rendered text now lives in a tmux option that pinGeometryOptionsLocked
rewrites on every boot and every attach, so this preview is never stale
against a running session the way the old header pane's once-at-launch
render was: editing status_line.template in reed.yaml and re-running this
verb always shows what the next boot or attach will paint.

Example:
  lyx reed statusline`,
		Annotations: map[string]string{
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			text, err := c.eng.StatusLineText()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"text": text,
			}))
			return nil
		},
	}

	return cmd
}
