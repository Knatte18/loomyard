// run.go implements the generic `run` verb body: it runs a *shedengine.Shed's whole phase machine
// in one call, from wherever the status file's current_producer currently sits, and reports the
// result as a JSON envelope.

package shedverbs

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// runCmd builds the generic `run` subcommand from texts (build-time help text) and spec (the
// arming module's own resolved values, filled by its PersistentPreRunE before this RunE runs).
func runCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	return &cobra.Command{
		Use:   texts.Run.Use,
		Short: texts.Run.Short,
		Long:  texts.Run.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if spec.Hooks.PreRun != nil {
				if err := spec.Hooks.PreRun(ctx); err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}

			if spec.BuildShed == nil {
				clihelp.SetExit(ctx, output.Err(out, "shedverbs: run: no BuildShed constructor configured"))
				return nil
			}
			shed, err := spec.BuildShed()
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			result, runErr := shed.Run(ctx)

			// PostRun runs unconditionally, before either envelope is written, including on the
			// hard-error arm below -- see Hooks.PostRun's own field doc for why that placement is
			// load-bearing.
			var extras map[string]any
			if spec.Hooks.PostRun != nil {
				extras = spec.Hooks.PostRun(ctx, result, runErr)
			}

			if runErr != nil {
				msg := runErr.Error()
				if errors.Is(runErr, shedengine.ErrShedBusy) && spec.RunBusyMessage != "" {
					msg = spec.RunBusyMessage
				}
				clihelp.SetExit(ctx, output.Err(out, msg))
				return nil
			}

			envelope := map[string]any{
				"outcome":         string(result.Outcome),
				"halted_producer": result.HaltedProducer,
				"reason":          result.Reason,
				"history_length":  len(result.History),
			}
			for k, v := range extras {
				envelope[k] = v
			}
			clihelp.SetExit(ctx, output.Ok(out, envelope))
			return nil
		},
	}
}
