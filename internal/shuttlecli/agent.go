// agent.go implements the `agent` shuttle verb, which starts the operator's agent session in this worktree.
// The session opens on a prompt that names its hub, worktree and board entry.
// It runs under every setting a run session gets.
// The file also holds the role-name check `agent` and `open` share.

package shuttlecli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/operatoragent"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// defaultAgentName is the strand role `agent` takes when --name is not given.
const defaultAgentName = "agent"

// agentCmd builds the `agent` subcommand.
// The subcommand starts one interactive agent session for the operator and returns once it is up.
func (c *shuttleCLI) agentCmd() *cobra.Command {
	var (
		name  string
		model string
	)

	cmd := &cobra.Command{
		Use:         "agent",
		Short:       "start an agent session for the operator that knows its hub, worktree and board entry",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `agent starts one interactive agent session in a focused window of its own
and returns once the session has taken its opening prompt.
The prompt tells the agent what it is, the hub and worktree it sits in, and the
board entry the worktree serves or that it is the prime.
The session starts as a run session does: it gets the same settings, shell
guards, standing passages and skills, the agents.yaml list of the agent role
followed by the hub's language skills.
Its window closes when the session exits. It is refused under a pause.

--name names the strand, ` + agentname.RoleRule + `;
its words are joined with dashes and lowered, so "My Agent" names my-agent,
and a second session of a held name is numbered by reed.
--model takes the model spec lym shuttle open takes.

Example:
  lym shuttle agent --name review-helper --model 'opus[high]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			role, err := roleFromFlag("--name", name)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			var choice shuttleengine.ModelChoice
			if model != "" {
				if choice, err = c.resolveModelSpec(model); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("--model: %v", err)))
					return nil
				}
			}
			// The error already names the failing key and its own way forward.
			language, err := languageconfig.Load(c.configDirs)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			geometry := c.reedGeometry
			engine := c.runner.EngineName(choice)
			spec, err := operatoragent.SessionSpec(operatoragent.Inputs{
				StencilsDir:    fabriclayout.StencilsDir(geometry.HubPath),
				Name:           role,
				HubShortname:   geometry.NameShortname,
				HubPath:        geometry.HubPath,
				WorktreeRoot:   geometry.WorktreeRoot,
				Slug:           geometry.NameSlug,
				ParentName:     geometry.ParentName,
				LanguageSkills: language.Skills,
			}, choice, engine)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// A start waits out a pause, and a button click would show nothing while it did.
			if err := c.runner.RefuseCovered("re-run lym shuttle agent"); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			result, err := c.runner.StartSession(spec)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"guid":      result.StrandGUID,
				"name":      result.StrandName,
				"sessionId": result.SessionID,
				"engine":    result.Engine,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", defaultAgentName, "strand role of the session: "+agentname.RoleRule+"; spaces become dashes")
	cmd.Flags().StringVar(&model, "model", "", "model spec, <alias>[<effort>] or <engine>:<model-id>[<effort>]; empty defers to the provider default")

	return cmd
}

// roleFromFlag returns the strand role a person typed as flag's value, its words joined with dashes and lowered.
// It refuses a value that is still no role, naming the flag, the value and the rule a role follows.
func roleFromFlag(flag, value string) (string, error) {
	role := agentname.RoleFromText(value)
	if err := agentname.ValidateRole(role); err != nil {
		return "", fmt.Errorf("%s %q is no valid agent name: use %s, as in my-agent", flag, value, agentname.RoleRule)
	}
	return role, nil
}
