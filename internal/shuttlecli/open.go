// open.go implements the `open` shuttle verb: it opens a named LLM session with no prompt in a window of its own through c.runner.Open.

package shuttlecli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// openCmd builds the `open` subcommand, which opens a named session that takes no turn and returns at once.
func (c *shuttleCLI) openCmd() *cobra.Command {
	var (
		role           string
		model          string
		segment        string
		permissionMode string
	)

	cmd := &cobra.Command{
		Use:         "open",
		Short:       "open a named llm session with no prompt in its own window",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `open starts one interactive LLM session in a detached window of its own and
returns at once. --role is required and names the strand; a second open of the
same role is numbered by reed.
The session takes no turn and writes no run record, so lyx shuttle state never
lists it. Its window closes when the session exits. It is refused under a pause.
--permission-mode bypass opens the session without permission dialogs, under its
strand name for the role, and an unknown mode is refused before anything is created.

Example:
  lyx shuttle open --role explore --model opus --permission-mode bypass`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if role == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "--role is required"))
				return nil
			}

			spec := shuttleengine.OpenSpec{Role: role, Segment: segmentcolor.Segment(segment), PermissionMode: permissionMode}
			if model != "" {
				parsed, err := modelspec.Parse(model)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("--model: %v", err)))
					return nil
				}
				registry, err := modelspec.LoadRegistry(c.configDirs)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("--model: load the model registry: %v", err)))
					return nil
				}
				resolved, err := registry.Resolve(parsed)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("--model: %v", err)))
					return nil
				}
				spec.ModelChoice = shuttleengine.NewModelChoice(resolved)
			}

			result, err := c.runner.Open(spec)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"guid":      result.StrandGUID,
				"name":      result.StrandName,
				"sessionId": result.SessionID,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "", "strand role of the session (required)")
	cmd.Flags().StringVar(&model, "model", "", "model spec; empty defers to the provider default")
	cmd.Flags().StringVar(&segment, "segment", "", "loom segment the session's window is colored by")
	cmd.Flags().StringVar(&permissionMode, "permission-mode", "", "permission mode of the session: bypass or prompt; empty keeps the engine's interactive default")

	return cmd
}
