// open.go implements the `open` shuttle verb: it opens a named LLM session with no prompt in a window of its own through c.runner.Open, or lists the model registry a --model spec resolves against.

package shuttlecli

import (
	"fmt"
	"sort"

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
		listModels     bool
	)

	cmd := &cobra.Command{
		Use:         "open",
		Short:       "open a named llm session with no prompt in its own window",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `open starts one interactive LLM session in a detached window of its own and
returns at once. --role is required and names the strand; a second open of the
same role is numbered by reed.
The session takes no turn and writes no run record, so lym shuttle state never
lists it. Its window closes when the session exits. It is refused under a pause.

--model chooses the engine, model and effort as a model spec, in one of two forms:
  <alias>[<effort>]               an alias from the model registry, e.g. opus or opus[high]
  <engine>:<model-id>[<effort>]   an engine and a provider model id, e.g. claude:claude-opus-5-5[max]
The bracket is optional and overrides the alias's default effort; the engine
validates the effort. Without --model the session gets the default engine and
the provider's default model and effort.
--list-models prints the registry's aliases with their engine, model and
default effort, and opens nothing.
The output names the engine, model and effort the session got, an empty model
or effort meaning the provider default.

The session opens in its engine's bypass mode, without permission dialogs.
--permission-mode prompt opts out; an unknown mode is refused before anything
is created.

Example:
  lym shuttle open --role explore --model opus[high]`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if listModels {
				registry, err := modelspec.LoadRegistry(c.configDirs)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("--list-models: load the model registry: %v", err)))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"models": registryRows(registry)}))
				return nil
			}

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
				"engine":    result.Engine,
				"model":     spec.Model,
				"effort":    spec.Effort,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "", "strand role of the session (required unless --list-models)")
	cmd.Flags().StringVar(&model, "model", "", "model spec, <alias>[<effort>] or <engine>:<model-id>[<effort>]; empty defers to the provider default")
	cmd.Flags().StringVar(&segment, "segment", "", "loom segment the session's window is colored by")
	cmd.Flags().StringVar(&permissionMode, "permission-mode", "", "permission mode of the session: bypass or prompt; empty is the engine's bypass mode")
	cmd.Flags().BoolVar(&listModels, "list-models", false, "print the model registry's aliases with their engine, model and default effort, and open nothing")

	return cmd
}

// registryRows returns one row per alias of registry, sorted by alias, each naming the alias's engine, model and default effort.
func registryRows(registry modelspec.Registry) []map[string]string {
	aliases := make([]string, 0, len(registry))
	for alias := range registry {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	rows := make([]map[string]string, 0, len(aliases))
	for _, alias := range aliases {
		entry := registry[alias]
		rows = append(rows, map[string]string{
			"alias":  alias,
			"engine": entry.Engine,
			"model":  entry.Model,
			"effort": entry.Defaults["effort"],
		})
	}
	return rows
}
