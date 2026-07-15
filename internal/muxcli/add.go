// add.go implements the `add` mux verb: a thin flag-to-spec mapper over
// muxengine.AddStrand. The CLI validates only the closed --anchor
// vocabulary here; guid generation, worktree stamping, and name resolution
// all belong to the engine (muxengine.AddStrand), not this file.

package muxcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/muxengine/render"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// addCmd builds the `add` subcommand: registers a new strand from the
// --cmd/--role/--round/--name/--resume-cmd/--parent/--anchor/--focus
// flags. It rejects --anchor own-window (declared but deferred) and any
// value outside below-parent|hidden before ever calling the engine, then
// builds a muxengine.AddSpec and delegates to c.eng.AddStrand.
func (c *muxCLI) addCmd() *cobra.Command {
	var (
		cmdFlag   string
		role      string
		round     string
		name      string
		resumeCmd string
		parent    string
		anchor    string
		focus     bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "add a new strand to the mux layout",
		Long: `add registers a new strand and, unless --anchor hidden, realizes it into
a live pane and runs --cmd in it.

The strand's display name resolves from --name (if given), else the
configured strand-name template filled from --role/--round, else a short
guid. The generated guid and resolved name are printed on success so a
later --parent or "lyx mux remove" can reference this strand.

Example:
  lyx mux add --cmd "claude --session-id %SID%" --role producer --round 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// The anchor vocabulary is closed to below-parent/hidden in v1;
			// own-window is declared but deferred, so reject it here with a
			// specific message rather than letting a confusing render-layer
			// error surface later.
			switch render.Anchor(anchor) {
			case render.AnchorBelowParent, render.AnchorHidden:
			case render.AnchorOwnWindow:
				clihelp.SetExit(cmd.Context(), output.Err(out, "--anchor own-window is deferred, not supported in v1"))
				return nil
			default:
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("invalid --anchor %q; want below-parent|hidden", anchor)))
				return nil
			}

			spec := muxengine.AddSpec{
				Role:         role,
				Round:        round,
				NameOverride: name,
				Cmd:          cmdFlag,
				ResumeCmd:    resumeCmd,
				Parent:       parent,
				Display: render.Display{
					Anchor:                   render.Anchor(anchor),
					Focus:                    focus,
					ShrinkWhenWaitingOnChild: true,
				},
			}

			strand, err := c.eng.AddStrand(spec)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"guid": strand.GUID,
				"name": strand.Name,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&cmdFlag, "cmd", "", "command to launch in the strand's pane (required)")
	cmd.Flags().StringVar(&role, "role", "", "role token used to fill the strand-name template")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().StringVar(&name, "name", "", "explicit display name, overriding the strand-name template")
	cmd.Flags().StringVar(&resumeCmd, "resume-cmd", "", `command "lyx mux resume" replays instead of --cmd`)
	cmd.Flags().StringVar(&parent, "parent", "", "parent strand's guid")
	cmd.Flags().StringVar(&anchor, "anchor", string(render.AnchorBelowParent), "placement: below-parent|hidden")
	cmd.Flags().BoolVar(&focus, "focus", false, "give this strand tmux input focus")
	if err := cmd.MarkFlagRequired("cmd"); err != nil {
		// MarkFlagRequired only errors when the named flag does not exist on
		// cmd; --cmd is registered immediately above, so this can never fire
		// at runtime. Panicking surfaces a wiring mistake immediately rather
		// than silently accepting adds with no --cmd.
		panic(fmt.Sprintf("muxcli: mark --cmd required: %v", err))
	}

	return cmd
}
