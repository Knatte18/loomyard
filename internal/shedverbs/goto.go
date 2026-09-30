// goto.go implements the generic `goto` verb body: it moves a halted run onto a named row and leaves
// it paused, by calling shedengine.Goto, which owns the status-file write.

package shedverbs

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// gotoCmd builds the generic `goto` subcommand.
// goto never calls spec.BuildShed -- a nil one is legal here, exactly as on status and pause.
func gotoCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   texts.Goto.Use,
		Short: texts.Goto.Short,
		Long:  texts.Goto.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			target, _ := cmd.Flags().GetString("to")
			if target == "" {
				names := make([]string, len(spec.Routing.Producers))
				for i, def := range spec.Routing.Producers {
					names[i] = def.Name
				}
				clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("goto requires --to <producer>; way forward: re-run goto with --to naming one of: %s", strings.Join(names, ", "))))
				return nil
			}

			if spec.EnsureStatusLockDir {
				if err := ensureStatusLockDir(spec.DecodeErrPrefix, spec.StatusLockPath); err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}

			st, err := shedengine.Goto(shedengine.GotoRequest{
				StatusPath:     spec.StatusPath,
				LockPath:       spec.LockPath,
				StatusLockPath: spec.StatusLockPath,
				Producers:      spec.Routing.Producers,
				Target:         target,
			})
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"status_file":      spec.StatusPath,
				"run_id":           spec.RunID,
				"current_producer": st.CurrentProducer,
				"state":            string(st.State),
			}))
			return nil
		},
	}
	cmd.Flags().String("to", "", "the producer row to move the run onto")
	return cmd
}
