// goto.go implements the generic `goto` verb body: it moves a halted run onto a named row and leaves it paused, by calling shedengine.Goto, which owns the status-file write.

package shedverbs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// RunGoto is the body of `goto` once the target is known, the entry an in-process driver calls and the cobra verb runs.
// It runs the PreGoto hook, makes the status-lock directory when the spec asks, and moves the run onto target through shedengine.Goto over the spec's paths.
// It returns the moved status, or the first error, which wraps shedengine.ErrShedBusy when another holder has the run.
func RunGoto(ctx context.Context, spec *Spec, target string) (shedengine.Status, error) {
	if spec.Hooks.PreGoto != nil {
		if err := spec.Hooks.PreGoto(ctx, target); err != nil {
			return shedengine.Status{}, err
		}
	}

	if spec.EnsureStatusLockDir {
		if err := ensureStatusLockDir(spec.DecodeErrPrefix, spec.StatusLockPath); err != nil {
			return shedengine.Status{}, err
		}
	}

	return shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     spec.StatusPath,
		LockPath:       spec.LockPath,
		StatusLockPath: spec.StatusLockPath,
		Producers:      spec.Routing.Producers,
		Target:         target,

		RunID:                   spec.RunID,
		MissingStatusWayForward: spec.MissingStatusWayForward,
		PauseWayForward:         spec.PauseWayForward,
	})
}

// gotoCmd builds the generic `goto` subcommand.
// goto never calls spec.BuildShed -- a nil one is legal here, exactly as on status and pause.
func gotoCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	cmd := &cobra.Command{
		Use:         texts.Goto.Use,
		Short:       texts.Goto.Short,
		Long:        texts.Goto.Long,
		Annotations: map[string]string{clihelp.AudienceAnnotation: texts.Goto.Audience},
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

			st, err := RunGoto(ctx, spec, target)
			if err != nil {
				if errors.Is(err, shedengine.ErrShedBusy) {
					clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": shedengine.ShedBusyKind}))
					return nil
				}
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
